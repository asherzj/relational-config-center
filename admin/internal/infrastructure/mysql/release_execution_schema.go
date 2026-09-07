package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// LockAndReadTableExecutionSchema acquires the business table's metadata lock in the
// owning transaction, including an ADD without a known id. Its versioned fixed
// projections omit comments, cardinalities and the current auto-increment cursor.
func (s *releaseOrderSession) LockAndReadTableExecutionSchema(ctx context.Context, table string) (domain.TableExecutionSchema, error) {
	if err := s.available(); err != nil {
		return domain.TableExecutionSchema{}, err
	}
	meta, err := recordIdentityMetadata(ctx, s.database, table)
	if err != nil {
		if errors.Is(err, application.ErrIncompatibleTable) {
			return domain.TableExecutionSchema{}, err
		}
		return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
	}
	rows, err := s.database.WithContext(ctx).Raw("SELECT `id` FROM " + s.database.Statement.Quote(meta.TableName) + " LIMIT 0 FOR UPDATE").Rows()
	if err != nil {
		return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
	}
	rows.Close()
	// Recheck compatibility after obtaining the lock, closing the preceding DDL window.
	meta, err = recordIdentityMetadata(ctx, s.database, meta.TableName)
	if err != nil {
		if errors.Is(err, application.ErrIncompatibleTable) {
			return domain.TableExecutionSchema{}, err
		}
		return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
	}
	// information_schema hides triggers without TRIGGER privilege. Require an
	// explicit effective direct grant rather than mistaking invisibility for none.
	// Legacy privilege tables use case-insensitive string columns, so apply the
	// server's filesystem-name case rules explicitly before accepting a grant.
	var grants int
	err = s.database.WithContext(ctx).Raw(`SELECT COUNT(*) FROM (
 SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.USER_PRIVILEGES WHERE NOT @@global.partial_revokes
 UNION ALL SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.SCHEMA_PRIVILEGES
 WHERE IF(@@global.partial_revokes,
   IF(@@lower_case_table_names=0,BINARY TABLE_SCHEMA=BINARY DATABASE(),BINARY LOWER(TABLE_SCHEMA)=BINARY LOWER(DATABASE())),
   IF(@@lower_case_table_names=0,BINARY DATABASE() LIKE BINARY TABLE_SCHEMA ESCAPE X'5C',BINARY LOWER(DATABASE()) LIKE BINARY LOWER(TABLE_SCHEMA) ESCAPE X'5C'))
 UNION ALL SELECT GRANTEE,PRIVILEGE_TYPE FROM information_schema.TABLE_PRIVILEGES
 WHERE IF(@@lower_case_table_names=0,BINARY TABLE_SCHEMA=BINARY DATABASE(),BINARY LOWER(TABLE_SCHEMA)=BINARY LOWER(DATABASE()))
 AND IF(@@lower_case_table_names=0,BINARY TABLE_NAME=BINARY ?,BINARY LOWER(TABLE_NAME)=BINARY LOWER(?))
 ) p WHERE PRIVILEGE_TYPE='TRIGGER' AND BINARY GRANTEE=BINARY CONCAT(
 CHAR(39),LEFT(CURRENT_USER(),CHAR_LENGTH(CURRENT_USER())-CHAR_LENGTH(SUBSTRING_INDEX(CURRENT_USER(),'@',-1))-1),
 CHAR(39),'@',CHAR(39),SUBSTRING_INDEX(CURRENT_USER(),'@',-1),CHAR(39))`, meta.TableName, meta.TableName).Row().Scan(&grants)
	if err != nil {
		return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
	}
	if grants == 0 {
		return domain.TableExecutionSchema{}, application.ErrReleaseMetadataPermission
	}
	result := domain.TableExecutionSchema{Format: "mysql-8.4-execution-v1", TableName: meta.TableName, Sections: []domain.ExecutionMetadata{}}
	for _, section := range []struct {
		name, query string
		args        []any
	}{
		{"session", `SELECT VERSION(),@@session.sql_mode,@@session.time_zone,@@session.character_set_connection,@@session.collation_connection,@@session.foreign_key_checks,@@session.unique_checks,@@session.lc_time_names,@@lower_case_table_names,@@session.auto_increment_increment,@@session.auto_increment_offset,@@session.explicit_defaults_for_timestamp,@@session.div_precision_increment`, nil},
		{"table", `SELECT TABLE_TYPE,ENGINE,TABLE_COLLATION,ROW_FORMAT,CREATE_OPTIONS FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?`, []any{meta.TableName}},
		{"columns", `SELECT COLUMN_NAME,ORDINAL_POSITION,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,CHARACTER_SET_NAME,COLLATION_NAME,EXTRA,GENERATION_EXPRESSION,SRS_ID FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION`, []any{meta.TableName}},
		{"indexes", `SELECT INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME,COLLATION,SUB_PART,NULLABLE,INDEX_TYPE,IS_VISIBLE,EXPRESSION FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY INDEX_NAME,SEQ_IN_INDEX`, []any{meta.TableName}},
		{"checks", `SELECT tc.CONSTRAINT_NAME,tc.ENFORCED,cc.CHECK_CLAUSE FROM information_schema.TABLE_CONSTRAINTS tc JOIN information_schema.CHECK_CONSTRAINTS cc ON cc.CONSTRAINT_SCHEMA=tc.CONSTRAINT_SCHEMA AND cc.CONSTRAINT_NAME=tc.CONSTRAINT_NAME WHERE tc.TABLE_SCHEMA=DATABASE() AND tc.TABLE_NAME=? ORDER BY tc.CONSTRAINT_NAME`, []any{meta.TableName}},
		{"foreign_keys", `SELECT k.CONSTRAINT_SCHEMA,k.TABLE_NAME,k.CONSTRAINT_NAME,k.ORDINAL_POSITION,k.COLUMN_NAME,k.REFERENCED_TABLE_SCHEMA,k.REFERENCED_TABLE_NAME,k.REFERENCED_COLUMN_NAME,r.MATCH_OPTION,r.UPDATE_RULE,r.DELETE_RULE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.REFERENTIAL_CONSTRAINTS r ON r.CONSTRAINT_SCHEMA=k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME=k.CONSTRAINT_NAME WHERE (k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=?) OR (k.REFERENCED_TABLE_SCHEMA=DATABASE() AND k.REFERENCED_TABLE_NAME=?) ORDER BY k.CONSTRAINT_SCHEMA,k.TABLE_NAME,k.CONSTRAINT_NAME,k.ORDINAL_POSITION`, []any{meta.TableName, meta.TableName}},
		{"triggers", `SELECT TRIGGER_NAME,EVENT_MANIPULATION,ACTION_ORDER,ACTION_CONDITION,ACTION_STATEMENT,ACTION_ORIENTATION,ACTION_TIMING,ACTION_REFERENCE_OLD_ROW,ACTION_REFERENCE_NEW_ROW,SQL_MODE,DEFINER,CHARACTER_SET_CLIENT,COLLATION_CONNECTION,DATABASE_COLLATION FROM information_schema.TRIGGERS WHERE EVENT_OBJECT_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE=? ORDER BY EVENT_MANIPULATION,ACTION_TIMING,ACTION_ORDER`, []any{meta.TableName}},
		{"partitions", `SELECT PARTITION_NAME,SUBPARTITION_NAME,PARTITION_ORDINAL_POSITION,SUBPARTITION_ORDINAL_POSITION,PARTITION_METHOD,SUBPARTITION_METHOD,PARTITION_EXPRESSION,SUBPARTITION_EXPRESSION,PARTITION_DESCRIPTION,TABLESPACE_NAME FROM information_schema.PARTITIONS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY PARTITION_ORDINAL_POSITION,SUBPARTITION_ORDINAL_POSITION`, []any{meta.TableName}},
	} {
		data, err := s.database.WithContext(ctx).Raw(section.query, section.args...).Rows()
		if err != nil {
			return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
		}
		values, err := executionMetadataRows(data)
		data.Close()
		if err != nil {
			return domain.TableExecutionSchema{}, application.ErrReleaseUnavailable
		}
		result.Sections = append(result.Sections, domain.ExecutionMetadata{Name: section.name, Rows: values})
	}
	return result, nil
}
func executionMetadataRows(rows *sql.Rows) ([][]*string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := [][]*string{}
	for rows.Next() {
		values := make([]*string, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		result = append(result, values)
	}
	return result, rows.Err()
}
