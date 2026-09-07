package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

const recordVersionsTable = "rcc_record_versions"

// Identity is derived from the stored primary key with MySQL's own collation
// weights. Resolving through WHERE id also respects numeric coercion. Tombstones
// therefore survive spelling changes when an equivalent id is reinserted.
func recordIdentity(ctx context.Context, db *gorm.DB, table string, id any, lock bool) (string, []byte, error) {

	meta, err := recordIdentityMetadata(ctx, db, table)
	if err != nil {
		return "", nil, err
	}
	expression := recordWeightExpression(meta, "`id`")
	query := "SELECT WEIGHT_STRING(" + expression + ") FROM " + db.Statement.Quote(table) + " WHERE `id` = ?"
	if lock {
		query += " FOR UPDATE"
	}
	var identity []byte
	if err := db.WithContext(ctx).Raw(query, id).Row().Scan(&identity); err != nil {
		return "", nil, err
	}
	if identity == nil {
		return "", nil, application.ErrIncompatibleTable
	}
	digest := sha256.Sum256(identity)
	return meta.TableName, digest[:], nil
}

func recordVersion(ctx context.Context, db *gorm.DB, table string, key []byte) (uint64, error) {
	var version uint64
	// Empty key is reserved for maintenance. Normal data operations never lock it.
	err := db.WithContext(ctx).Raw(`SELECT COALESCE(MAX(lock_version),0) FROM rcc_record_versions WHERE table_name=? AND (record_key=? OR record_key=X'')`, table, key).Row().Scan(&version)
	return version, err
}

func queryRecordVersions(ctx context.Context, db *gorm.DB, table string, rows []domain.Row) ([]string, error) {
	versions := make([]string, 0, len(rows))
	for _, row := range rows {
		id := row["id"]
		if id == nil {
			return nil, fmt.Errorf("missing record identity")
		}
		name, key, err := recordIdentity(ctx, db, table, string(*id), false)
		if err != nil {
			return nil, err
		}
		version, err := recordVersion(ctx, db, name, key)
		if err != nil {
			return nil, err
		}
		versions = append(versions, strconv.FormatUint(version, 10))
	}
	return versions, nil
}

func compareAndAdvanceRecordVersion(ctx context.Context, db *gorm.DB, table string, id any, expected string) error {
	if err := application.ValidateRecordVersion(expected); err != nil {
		return err
	}
	return advanceRecordVersion(ctx, db, table, id, &expected)
}

func advanceRecordVersion(ctx context.Context, db *gorm.DB, table string, id any, expected *string) error {
	name, key, err := recordIdentity(ctx, db, table, id, true)
	if err == sql.ErrNoRows {
		return application.ErrMutationRowNotFound
	}
	if err != nil {
		return classifyMutationError(err, ctx.Err())
	}
	var floor uint64
	if err := db.WithContext(ctx).Raw(`SELECT COALESCE(MAX(lock_version),0) FROM rcc_record_versions WHERE table_name=? AND record_key=X''`, name).Row().Scan(&floor); err != nil {
		return classifyMutationError(err, ctx.Err())
	}
	if err := db.WithContext(ctx).Exec(`INSERT INTO rcc_record_versions(table_name,record_key,lock_version) VALUES(?,?,?) ON DUPLICATE KEY UPDATE record_key=record_key`, name, key, floor).Error; err != nil {
		return classifyMutationError(err, ctx.Err())
	}
	statement := `UPDATE rcc_record_versions SET lock_version=GREATEST(lock_version,?)+1 WHERE table_name=? AND record_key=? AND GREATEST(lock_version,?)<18446744073709551615`
	args := []any{floor, name, key, floor}
	if expected != nil {
		statement += ` AND GREATEST(lock_version,?)=?`
		args = append(args, floor, *expected)
	}
	result := db.WithContext(ctx).Exec(statement, args...)
	if result.Error != nil {
		return classifyMutationError(result.Error, ctx.Err())
	}
	if result.RowsAffected != 1 {
		return application.ErrRecordVersionConflict
	}
	return nil
}

type identityMetadata struct {
	TableName, Engine, PadAttribute, CollationName, CharsetName, DataType, ColumnType string
	NumericPrecision, Scale, TemporalPrecision, Capacity                              int
}

func recordIdentityMetadata(ctx context.Context, db *gorm.DB, table string) (identityMetadata, error) {
	var meta identityMetadata
	result := db.WithContext(ctx).Raw(`SELECT t.TABLE_NAME AS table_name,t.ENGINE AS engine,COALESCE(co.PAD_ATTRIBUTE,'NO PAD') AS pad_attribute,COALESCE(c.COLLATION_NAME,'') AS collation_name,COALESCE(c.CHARACTER_SET_NAME,'') AS charset_name,c.DATA_TYPE AS data_type,c.COLUMN_TYPE AS column_type,COALESCE(c.NUMERIC_PRECISION,0) AS numeric_precision,COALESCE(c.NUMERIC_SCALE,0) AS scale,COALESCE(c.DATETIME_PRECISION,0) AS temporal_precision,COALESCE(c.CHARACTER_MAXIMUM_LENGTH,0) AS capacity
 FROM information_schema.TABLES t JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=t.TABLE_SCHEMA AND c.TABLE_NAME=t.TABLE_NAME AND c.COLUMN_NAME='id'
 LEFT JOIN information_schema.COLLATIONS co ON co.COLLATION_NAME=c.COLLATION_NAME
 WHERE t.TABLE_SCHEMA=DATABASE() AND t.TABLE_NAME=?`, table).Scan(&meta)
	if result.Error != nil {
		return meta, result.Error
	}
	if result.RowsAffected != 1 || meta.Engine != "InnoDB" {
		return meta, application.ErrIncompatibleTable
	}
	return meta, nil
}
func recordWeightExpression(meta identityMetadata, expression string) string {
	if meta.DataType == "float" || meta.DataType == "double" {
		// MySQL compares signed zero as one primary-key identity. Other FLOAT
		// values must be promoted before text conversion to avoid six-digit
		// collisions. Missing-row identities use this same expression.
		return "COALESCE(CAST(CAST(NULLIF(" + expression + ",0) AS DOUBLE) AS CHAR CHARACTER SET ascii),CAST('0' AS CHAR CHARACTER SET ascii))"
	}
	if meta.CollationName == "" {
		return "CAST(" + expression + " AS CHAR CHARACTER SET ascii)"
	}
	if meta.PadAttribute == "PAD SPACE" {
		return "RTRIM(" + expression + ")"
	}
	return expression
}

var identitySQLName = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Missing ids use the very same stored-type and collation weight calculation as
// existing rows. No temporary business row or control version is created.
func missingRecordIdentity(ctx context.Context, db *gorm.DB, table string, id any) (string, []byte, error) {
	meta, err := recordIdentityMetadata(ctx, db, table)
	if err != nil {
		return "", nil, err
	}
	var expression string
	switch meta.DataType {
	case "char", "varchar":
		if !identitySQLName.MatchString(meta.CharsetName) || !identitySQLName.MatchString(meta.CollationName) {
			return "", nil, application.ErrIncompatibleTable
		}
		text, ok := id.(string)
		if !ok || len([]rune(text)) > meta.Capacity {
			return "", nil, application.ErrInvalidMutation
		}
		expression = "CAST(? AS CHAR CHARACTER SET " + meta.CharsetName + ") COLLATE " + meta.CollationName
		if meta.DataType == "char" {
			expression = "RTRIM(" + expression + ")"
		}
	case "tinyint", "smallint", "mediumint", "int", "bigint":
		target := "SIGNED"
		if strings.Contains(meta.ColumnType, "unsigned") {
			target = "UNSIGNED"
		}
		expression = "CAST(? AS " + target + ")"
	case "decimal":
		expression = fmt.Sprintf("CAST(? AS DECIMAL(%d,%d))", meta.NumericPrecision, meta.Scale)
	case "date":
		expression = "CAST(? AS DATE)"
	case "datetime", "timestamp":
		expression = fmt.Sprintf("CAST(? AS DATETIME(%d))", meta.TemporalPrecision)
	case "time":
		expression = fmt.Sprintf("CAST(? AS TIME(%d))", meta.TemporalPrecision)
	case "float":
		expression = "CAST(? AS FLOAT)"
	case "double":
		expression = "CAST(? AS DOUBLE)"
	default:
		return "", nil, application.ErrReleaseSnapshotUnsupported
	}
	var identity []byte
	if err := db.WithContext(ctx).Raw("SELECT WEIGHT_STRING("+recordWeightExpression(meta, expression)+")", id).Row().Scan(&identity); err != nil {
		return "", nil, err
	}
	if identity == nil {
		return "", nil, application.ErrInvalidMutation
	}
	digest := sha256.Sum256(identity)
	return meta.TableName, digest[:], nil
}
