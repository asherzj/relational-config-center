package mysql

import (
	"context"
	"errors"
)

var ErrRecordVersionSchemaIncomplete = errors.New("record version schema is incomplete; apply migration 009")

func (a *Adapter) recordVersionSchemaReady(ctx context.Context) error {
	var columns []struct{ Name, Type, Nullable, Engine string }
	err := a.gorm.WithContext(ctx).Raw(`SELECT c.COLUMN_NAME AS name,c.COLUMN_TYPE AS type,c.IS_NULLABLE AS nullable,t.ENGINE AS engine FROM information_schema.COLUMNS c JOIN information_schema.TABLES t ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME WHERE c.TABLE_SCHEMA=? AND c.TABLE_NAME='rcc_record_versions'`, a.database).Scan(&columns).Error
	if err != nil {
		return err
	}
	required := map[string]string{"table_name": "varbinary(256)", "record_key": "varbinary(32)", "lock_version": "bigint unsigned"}
	if len(columns) != len(required) {
		return ErrRecordVersionSchemaIncomplete
	}
	for _, c := range columns {
		if required[c.Name] != c.Type || c.Nullable != "NO" || c.Engine != "InnoDB" {
			return ErrRecordVersionSchemaIncomplete
		}
	}
	var indexes []struct {
		Columns            string
		NonUnique, Partial int
	}
	err = a.gorm.WithContext(ctx).Raw(`SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS columns,NON_UNIQUE AS non_unique,COALESCE(MAX(SUB_PART),0) AS partial FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME='rcc_record_versions' GROUP BY INDEX_NAME,NON_UNIQUE`, a.database).Scan(&indexes).Error
	if err != nil {
		return err
	}
	if len(indexes) != 1 || indexes[0].Columns != "table_name,record_key" || indexes[0].NonUnique != 0 || indexes[0].Partial != 0 {
		return ErrRecordVersionSchemaIncomplete
	}
	return nil
}
