package mysql

import (
	"context"
	"errors"
)

var ErrReleaseSchemaIncomplete = errors.New("release order schema is incomplete; apply migrations 010, 011, 012, 015 and 016")

func (a *Adapter) releaseSchemaReady(ctx context.Context) error {
	rows, err := a.gorm.WithContext(ctx).Raw(`SELECT concurrency_key FROM rcc_table_policies LIMIT 0`).Rows()
	if err != nil {
		return ErrReleaseSchemaIncomplete
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for table, required := range map[string]map[string]string{
		"rcc_release_table_references": {"table_name": "varbinary(256)", "order_id": "varbinary(32)"},
		"rcc_release_details":          {"order_id": "varbinary(32)", "position": "int unsigned", "table_name": "varbinary(256)", "application": "json", "publication": "json", "rollback": "json"},
		"rcc_release_executions":       {"order_id": "varbinary(32)", "kind": "varchar(16)", "execution_id": "varbinary(64)", "document": "json"},
		"rcc_table_publications":       {"table_name": "varbinary(256)", "table_version": "bigint unsigned", "command_cursor": "bigint unsigned"},
		"rcc_publication_commands":     {"table_name": "varbinary(256)", "sequence": "bigint unsigned", "order_id": "varbinary(32)", "execution_id": "varbinary(64)", "document": "json"},
		"rcc_refresh_notifications":    {"execution_id": "varbinary(64)", "order_id": "varbinary(32)", "table_name": "varbinary(256)", "table_version": "bigint unsigned", "document": "json"},
		"rcc_release_targets":          {"table_name": "varbinary(256)", "record_key": "binary(32)", "order_id": "varbinary(32)"},
		"rcc_release_orders":           {"id": "varbinary(32)", "table_name": "varbinary(256)", "applicant_id": "varbinary(36)", "state": "varchar(32)", "version": "bigint unsigned", "document": "json"},
		"rcc_release_requests":         {"actor_id": "varbinary(36)", "operation": "varbinary(96)", "request_key": "varbinary(64)", "digest": "binary(32)", "result": "json"},
	} {
		var columns []struct{ Name, Type, Nullable, Engine, Collation string }
		err := a.gorm.WithContext(ctx).Raw(`SELECT c.COLUMN_NAME AS name,c.COLUMN_TYPE AS type,c.IS_NULLABLE AS nullable,t.ENGINE AS engine,COALESCE(c.COLLATION_NAME,'') AS collation FROM information_schema.COLUMNS c JOIN information_schema.TABLES t ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME WHERE c.TABLE_SCHEMA=? AND c.TABLE_NAME=?`, a.database, table).Scan(&columns).Error
		if err != nil {
			return err
		}
		if len(columns) != len(required) {
			return ErrReleaseSchemaIncomplete
		}
		for _, c := range columns {
			nullable := "NO"
			if c.Name == "result" || c.Name == "publication" || c.Name == "rollback" {
				nullable = "YES"
			}
			if required[c.Name] != c.Type || c.Nullable != nullable || c.Engine != "InnoDB" || (c.Name == "state" || c.Name == "kind") && c.Collation != "ascii_bin" {
				return ErrReleaseSchemaIncomplete
			}
		}
		var indexes []struct {
			Name, Columns      string
			NonUnique, Partial int
		}
		err = a.gorm.WithContext(ctx).Raw(`SELECT INDEX_NAME AS name,GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns,NON_UNIQUE AS non_unique,COALESCE(MAX(SUB_PART),0) AS partial FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? GROUP BY INDEX_NAME,NON_UNIQUE`, a.database, table).Scan(&indexes).Error
		if err != nil {
			return err
		}
		primary := "id"
		if table == "rcc_table_publications" {
			primary = "table_name"
		}
		if table == "rcc_publication_commands" {
			primary = "table_name,sequence"
		}
		if table == "rcc_refresh_notifications" {
			primary = "execution_id,table_name"
		}
		if table == "rcc_release_table_references" {
			primary = "table_name,order_id"
		}
		if table == "rcc_release_targets" {
			primary = "table_name,record_key"
		}
		if table == "rcc_release_requests" {
			primary = "actor_id,operation,request_key"
		}
		if table == "rcc_release_details" {
			primary = "order_id,position"
		}
		if table == "rcc_release_executions" {
			primary = "order_id,kind"
		}
		found := false
		for _, index := range indexes {
			if index.Partial != 0 {
				return ErrReleaseSchemaIncomplete
			}
			if index.NonUnique == 0 {
				if index.Name != "PRIMARY" || index.Columns != primary {
					return ErrReleaseSchemaIncomplete
				}
				found = true
			}
		}
		if !found {
			return ErrReleaseSchemaIncomplete
		}
	}
	return nil
}
