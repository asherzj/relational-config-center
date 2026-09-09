package mysql

import (
	"context"
	"errors"
)

var ErrFieldPolicySchemaIncomplete = errors.New("table field policy schema is incomplete; apply migration 014")

func (a *Adapter) fieldPolicySchemaReady(ctx context.Context) error {
	var columns []struct{ Name, Type, Nullable, Engine string }
	err := a.gorm.WithContext(ctx).Raw(`SELECT c.COLUMN_NAME AS name,c.COLUMN_TYPE AS type,c.IS_NULLABLE AS nullable,t.ENGINE AS engine FROM information_schema.COLUMNS c JOIN information_schema.TABLES t ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME WHERE c.TABLE_SCHEMA=? AND c.TABLE_NAME='rcc_table_field_policies'`, a.database).Scan(&columns).Error
	if err != nil {
		return err
	}
	required := map[string]string{"id": "bigint unsigned", "table_name": "varchar(64)", "field_name": "varchar(64)", "display_name": "varchar(200)", "description": "varchar(500)", "display_order": "int unsigned", "is_visible": "tinyint(1)", "is_queryable": "tinyint(1)", "query_operators": "json", "ui_type": "varchar(32)", "ui_options": "json", "editable_on_add": "tinyint(1)", "editable_on_modify": "tinyint(1)", "is_required": "tinyint(1)", "default_value": "json", "enabled": "tinyint(1)", "creator": "varchar(64)", "modifier": "varchar(64)", "created_at": "datetime", "updated_at": "datetime"}
	if len(columns) != len(required) {
		return ErrFieldPolicySchemaIncomplete
	}
	for _, c := range columns {
		nullable := "NO"
		if c.Name == "query_operators" || c.Name == "ui_options" || c.Name == "default_value" {
			nullable = "YES"
		}
		if required[c.Name] != c.Type || c.Nullable != nullable || c.Engine != "InnoDB" {
			return ErrFieldPolicySchemaIncomplete
		}
	}
	var indexes []struct {
		Name, Columns      string
		NonUnique, Partial int
	}
	err = a.gorm.WithContext(ctx).Raw(`SELECT INDEX_NAME AS name,GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns,NON_UNIQUE AS non_unique,COALESCE(MAX(SUB_PART),0) AS partial FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME='rcc_table_field_policies' GROUP BY INDEX_NAME,NON_UNIQUE`, a.database).Scan(&indexes).Error
	if err != nil {
		return err
	}
	found := false
	for _, i := range indexes {
		if i.Columns == "table_name,field_name" && i.NonUnique == 0 && i.Partial == 0 {
			found = true
		}
	}
	if !found {
		return ErrFieldPolicySchemaIncomplete
	}
	return nil
}
