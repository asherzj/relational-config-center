package mysql

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"gorm.io/gorm"
)

const recordVersionsTable = "rcc_record_versions"

// Identity is derived from the stored primary key with MySQL's own collation
// weights. Resolving through WHERE id also respects numeric coercion. Tombstones
// therefore survive spelling changes when an equivalent id is reinserted.
func recordIdentity(ctx context.Context, db *gorm.DB, table string, id any, lock bool) (string, []byte, error) {
	var meta struct{ TableName, Engine, PadAttribute, CollationName string }
	result := db.WithContext(ctx).Raw(`SELECT t.TABLE_NAME AS table_name,t.ENGINE AS engine,COALESCE(co.PAD_ATTRIBUTE,'NO PAD') AS pad_attribute, COALESCE(c.COLLATION_NAME,'') AS collation_name
 FROM information_schema.TABLES t JOIN information_schema.COLUMNS c ON c.TABLE_SCHEMA=t.TABLE_SCHEMA AND c.TABLE_NAME=t.TABLE_NAME AND c.COLUMN_NAME='id'
 LEFT JOIN information_schema.COLLATIONS co ON co.COLLATION_NAME=c.COLLATION_NAME
 WHERE t.TABLE_SCHEMA=DATABASE() AND t.TABLE_NAME=?`, table).Scan(&meta)
	if result.Error != nil {
		return "", nil, result.Error
	}
	if result.RowsAffected != 1 || meta.Engine != "InnoDB" {
		return "", nil, application.ErrIncompatibleTable
	}
	expression := "`id`"
	if meta.CollationName == "" {
		expression = "CAST(`id` AS CHAR CHARACTER SET ascii)"
	}
	if meta.PadAttribute == "PAD SPACE" {
		expression = "RTRIM(`id`)"
	}
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
