package mysql

import (
	"context"
	"errors"
	"fmt"
)

var ErrAccountRoleSchemaIncomplete = errors.New("account role schema is incomplete; apply migration 008")

func (a *Adapter) accountRoleSchemaReady(ctx context.Context) error {
	required := []accountColumnContract{
		{accountTable, "roles", "tinyint unsigned", ""}, {accountTable, "role_version", "bigint unsigned", ""},
		{roleHistoryTable, "id", "bigint unsigned", ""}, {roleHistoryTable, "actor_kind", "varchar(16)", "ascii_bin"},
		{roleHistoryTable, "actor_id", "char(36)", "ascii_bin"}, {roleHistoryTable, "account_id", "char(36)", "ascii_bin"},
		{roleHistoryTable, "before_roles", "tinyint unsigned", ""}, {roleHistoryTable, "after_roles", "tinyint unsigned", ""},
		{roleHistoryTable, "version", "bigint unsigned", ""}, {roleHistoryTable, "request_key", "varchar(64)", "ascii_bin"},
		{roleHistoryTable, "request_digest", "char(64)", "ascii_bin"}, {roleHistoryTable, "result", "json", ""},
		{roleHistoryTable, "created_at", "datetime(6)", ""},
	}
	var columns []struct{ TableName, ColumnName, ColumnType, CollationName, IsNullable, Engine, Extra string }
	err := a.gorm.WithContext(ctx).Raw(`SELECT c.TABLE_NAME AS table_name,c.COLUMN_NAME AS column_name,c.COLUMN_TYPE AS column_type,COALESCE(c.COLLATION_NAME,'') AS collation_name,c.IS_NULLABLE AS is_nullable,t.ENGINE AS engine,c.EXTRA AS extra FROM information_schema.COLUMNS c JOIN information_schema.TABLES t ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME WHERE c.TABLE_SCHEMA=? AND c.TABLE_NAME IN (?,?)`, a.database, accountTable, roleHistoryTable).Scan(&columns).Error
	if err != nil {
		return err
	}
	found := map[accountColumnContract]bool{}
	autoID := false
	for _, c := range columns {
		if c.IsNullable == "NO" && c.Engine == "InnoDB" {
			found[accountColumnContract{c.TableName, c.ColumnName, c.ColumnType, c.CollationName}] = true
		}
		if c.TableName == roleHistoryTable && c.ColumnName == "id" && c.Extra == "auto_increment" {
			autoID = true
		}
	}
	for _, c := range required {
		if !found[c] {
			return ErrAccountRoleSchemaIncomplete
		}
	}
	if !autoID {
		return ErrAccountRoleSchemaIncomplete
	}
	var indexes []struct {
		Columns            string
		NonUnique, Partial int
	}
	err = a.gorm.WithContext(ctx).Raw(`SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns, NON_UNIQUE AS non_unique,COALESCE(MAX(SUB_PART),0) AS partial FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? GROUP BY INDEX_NAME,NON_UNIQUE`, a.database, roleHistoryTable).Scan(&indexes).Error
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, index := range indexes {
		key := fmt.Sprintf("%d:%s", index.NonUnique, index.Columns)
		if index.Partial != 0 {
			return ErrAccountRoleSchemaIncomplete
		}
		if index.NonUnique == 0 && key != "0:id" && key != "0:actor_id,request_key" {
			return ErrAccountRoleSchemaIncomplete
		}
		seen[key] = true
	}
	for _, key := range []string{"0:id", "0:actor_id,request_key", "1:account_id,id"} {
		if !seen[key] {
			return ErrAccountRoleSchemaIncomplete
		}
	}
	var invalid int64
	if err := a.gorm.WithContext(ctx).Table(accountTable).Where("roles < 1 OR roles > 31 OR role_version < 1").Count(&invalid).Error; err != nil {
		return err
	}
	if invalid != 0 {
		return ErrAccountRoleSchemaIncomplete
	}
	return nil
}
