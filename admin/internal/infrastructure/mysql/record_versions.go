package mysql

import (
	"context"
	"crypto/sha256"
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
func recordIdentity(ctx context.Context, db *gorm.DB, table string, id any) (string, []byte, error) {

	meta, err := recordIdentityMetadata(ctx, db, table)
	if err != nil {
		return "", nil, err
	}
	expression := recordWeightExpression(meta, "`id`")
	query := "SELECT WEIGHT_STRING(" + expression + ") FROM " + db.Statement.Quote(table) + " WHERE `id` = ?"
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
		name, key, err := recordIdentity(ctx, db, table, string(*id))
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

type identityMetadata struct {
	TableName, Engine, PadAttribute, CollationName, CharsetName, DataType, ColumnType string
	NumericPrecision, Scale, TemporalPrecision, Capacity                              int
}

func recordIdentityMetadata(ctx context.Context, db *gorm.DB, table string) (identityMetadata, error) {
	return columnIdentityMetadata(ctx, db, table, "id")
}

func columnIdentityMetadata(ctx context.Context, db *gorm.DB, table, column string) (identityMetadata, error) {
	var meta identityMetadata
	result := db.WithContext(ctx).Raw(`SELECT IF(@@lower_case_table_names=0,t.TABLE_NAME,LOWER(t.TABLE_NAME)) AS table_name,t.ENGINE AS engine,COALESCE(co.PAD_ATTRIBUTE,'NO PAD') AS pad_attribute,COALESCE(c.COLLATION_NAME,'') AS collation_name,COALESCE(c.CHARACTER_SET_NAME,'') AS charset_name,c.DATA_TYPE AS data_type,c.COLUMN_TYPE AS column_type,COALESCE(c.NUMERIC_PRECISION,0) AS numeric_precision,COALESCE(c.NUMERIC_SCALE,0) AS scale,COALESCE(c.DATETIME_PRECISION,0) AS temporal_precision,COALESCE(c.CHARACTER_MAXIMUM_LENGTH,0) AS capacity
 FROM information_schema.TABLES t JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=t.TABLE_SCHEMA AND c.TABLE_NAME=t.TABLE_NAME AND c.COLUMN_NAME=?
 LEFT JOIN information_schema.COLLATIONS co ON co.COLLATION_NAME=c.COLLATION_NAME
 WHERE t.TABLE_SCHEMA=DATABASE() AND t.TABLE_NAME=?`, column, table).Scan(&meta)
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

// Compare requested values using the stored key type in every batch join.
// A missing ENUM still cannot supply a trusted insertion identity.
func recordLookupExpression(meta identityMetadata) (string, error) {
	var expression string
	switch meta.DataType {
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum":
		if !identitySQLName.MatchString(meta.CharsetName) || !identitySQLName.MatchString(meta.CollationName) {
			return "", application.ErrIncompatibleTable
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
		return "", application.ErrReleaseSnapshotUnsupported
	}
	return expression, nil
}

// One requested row source is shared by baseline and canonical-row reads. The
// request ordinal survives SQL reordering; all candidate values remain bound.
type recordLookup struct {
	source, candidate string
	arguments         []any
}

func buildRecordLookup(meta identityMetadata, ids []any) (recordLookup, error) {
	expression, err := recordLookupExpression(meta)
	if err != nil {
		return recordLookup{}, err
	}
	sources := []string{}
	args := []any{}
	for i, id := range ids {
		if id == nil {
			continue
		}
		if meta.DataType == "char" || meta.DataType == "varchar" {
			value, ok := id.(string)
			if !ok || len([]rune(value)) > meta.Capacity {
				return recordLookup{}, &application.ReleaseItemError{Index: i, Cause: application.ErrInvalidMutation}
			}
		}
		sources = append(sources, "SELECT ? AS ordinal, ? AS candidate")
		args = append(args, i, id)
	}
	return recordLookup{source: "(" + strings.Join(sources, " UNION ALL ") + ") requested", candidate: strings.ReplaceAll(expression, "?", "requested.candidate"), arguments: args}, nil
}
