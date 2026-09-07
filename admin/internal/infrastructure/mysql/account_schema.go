package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrAuthenticationSchemaIncomplete distinguishes an actionable migration failure
// from a database outage without exposing database diagnostics to process logs.
var ErrAuthenticationSchemaIncomplete = errors.New("authentication schema is incomplete; apply migration 007")

type accountColumnContract struct{ table, name, columnType, collation string }

// Authentication depends on these persisted types and constraints for identity,
// expiry and admission serialization. Readiness never repairs a live schema.
func (adapter *Adapter) accountSchemaReady(ctx context.Context) error {
	columns := []accountColumnContract{
		{"rcc_accounts", "id", "char(36)", "ascii_bin"},
		{"rcc_accounts", "username", "varchar(32)", "ascii_bin"},
		{"rcc_accounts", "email", "varchar(254)", "ascii_bin"},
		{"rcc_accounts", "display_name", "varchar(64)", "utf8mb4_0900_ai_ci"},
		{"rcc_accounts", "password_hash", "varchar(255)", "ascii_bin"},
		{"rcc_accounts", "enabled", "tinyint(1)", ""},
		{"rcc_accounts", "password_version", "bigint unsigned", ""},
		{"rcc_accounts", "session_version", "bigint unsigned", ""},
		{"rcc_accounts", "created_at", "datetime(6)", ""},
		{"rcc_login_sessions", "token_hash", "char(64)", "ascii_bin"},
		{"rcc_login_sessions", "account_id", "char(36)", "ascii_bin"},
		{"rcc_login_sessions", "csrf_hash", "char(64)", "ascii_bin"},
		{"rcc_login_sessions", "password_version", "bigint unsigned", ""},
		{"rcc_login_sessions", "session_version", "bigint unsigned", ""},
		{"rcc_login_sessions", "created_at", "datetime(6)", ""},
		{"rcc_login_sessions", "last_active_at", "datetime(6)", ""},
		{"rcc_login_sessions", "expires_at", "datetime(6)", ""},
		{"rcc_preauth_credentials", "token_hash", "char(64)", "ascii_bin"},
		{"rcc_preauth_credentials", "csrf_hash", "char(64)", "ascii_bin"},
		{"rcc_preauth_credentials", "expires_at", "datetime(6)", ""},
		{"rcc_auth_rate_limits", "bucket_key", "varchar(80)", "ascii_bin"},
		{"rcc_auth_rate_limits", "attempts", "int unsigned", ""},
		{"rcc_auth_rate_limits", "in_flight", "int unsigned", ""},
		{"rcc_auth_rate_limits", "expires_at", "datetime(6)", ""},
		{"rcc_auth_control_lock", "id", "int", ""},
	}

	var storedColumns []struct {
		TableName, ColumnName, ColumnType, CollationName, IsNullable, Engine string
	}
	err := adapter.gorm.WithContext(ctx).Raw(`SELECT c.TABLE_NAME AS table_name,c.COLUMN_NAME AS column_name,c.COLUMN_TYPE AS column_type,COALESCE(c.COLLATION_NAME,'') AS collation_name,c.IS_NULLABLE AS is_nullable,COALESCE(t.ENGINE,'') AS engine FROM information_schema.COLUMNS c JOIN information_schema.TABLES t ON t.TABLE_SCHEMA=c.TABLE_SCHEMA AND t.TABLE_NAME=c.TABLE_NAME WHERE c.TABLE_SCHEMA=? AND c.TABLE_NAME IN ('rcc_accounts','rcc_login_sessions','rcc_preauth_credentials','rcc_auth_rate_limits','rcc_auth_control_lock')`, adapter.database).Scan(&storedColumns).Error
	if err != nil {
		return fmt.Errorf("inspect authentication columns: %w", err)
	}
	found := map[accountColumnContract]bool{}
	for _, column := range storedColumns {
		if column.IsNullable == "NO" && column.Engine == "InnoDB" {
			found[accountColumnContract{column.TableName, column.ColumnName, column.ColumnType, column.CollationName}] = true
		}
	}
	for _, c := range columns {
		if !found[c] {
			return ErrAuthenticationSchemaIncomplete
		}
	}
	// Compare index meaning, not DBA-chosen names. A composite unique index cannot
	// replace a single-column identity uniqueness constraint.

	var storedIndexes []struct {
		TableName, IndexedColumns, IsVisible string
		NonUnique, Partial                   int
	}
	err = adapter.gorm.WithContext(ctx).Raw(`SELECT TABLE_NAME AS table_name,NON_UNIQUE AS non_unique,IS_VISIBLE AS is_visible,GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) AS indexed_columns,COALESCE(MAX(SUB_PART),0) AS partial FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('rcc_accounts','rcc_login_sessions','rcc_preauth_credentials','rcc_auth_rate_limits','rcc_auth_control_lock') GROUP BY TABLE_NAME,INDEX_NAME,NON_UNIQUE,IS_VISIBLE`, adapter.database).Scan(&storedIndexes).Error
	if err != nil {
		return fmt.Errorf("inspect authentication indexes: %w", err)
	}
	indexes := map[string]bool{}
	for _, index := range storedIndexes {
		if index.Partial != 0 && index.NonUnique == 0 {
			return ErrAuthenticationSchemaIncomplete
		}
		if index.Partial == 0 && (index.IsVisible == "YES" || index.NonUnique == 0) {
			indexes[fmt.Sprintf("%s:%d:%s", index.TableName, index.NonUnique, index.IndexedColumns)] = true
		}
	}
	required := map[string]bool{}
	for _, index := range []string{"rcc_accounts:0:id", "rcc_accounts:0:username", "rcc_accounts:0:email", "rcc_login_sessions:0:token_hash", "rcc_login_sessions:1:account_id", "rcc_login_sessions:1:expires_at", "rcc_preauth_credentials:0:token_hash", "rcc_preauth_credentials:1:expires_at", "rcc_auth_rate_limits:0:bucket_key", "rcc_auth_rate_limits:1:expires_at", "rcc_auth_control_lock:0:id"} {
		required[index] = true
		if !indexes[index] {
			return ErrAuthenticationSchemaIncomplete
		}
	}
	// Additional uniqueness can forbid valid registrations or concurrent sessions.
	for index := range indexes {
		if strings.Contains(index, ":0:") && !required[index] {
			return ErrAuthenticationSchemaIncomplete
		}
	}
	var foreignKeys int
	err = adapter.gorm.WithContext(ctx).Raw(`SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=? AND TABLE_NAME='rcc_login_sessions' AND COLUMN_NAME='account_id' AND REFERENCED_TABLE_SCHEMA=? AND REFERENCED_TABLE_NAME='rcc_accounts' AND REFERENCED_COLUMN_NAME='id'`, adapter.database, adapter.database).Scan(&foreignKeys).Error
	if err != nil {
		return err
	}
	if foreignKeys != 1 {
		return ErrAuthenticationSchemaIncomplete
	}
	var lockRows int
	if err := adapter.gorm.WithContext(ctx).Raw("SELECT COUNT(*) FROM rcc_auth_control_lock WHERE id=1").Scan(&lockRows).Error; err != nil {
		return err
	}
	if lockRows != 1 {
		return ErrAuthenticationSchemaIncomplete
	}
	return nil
}
