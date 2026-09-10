-- +goose NO TRANSACTION
-- +goose Up
-- Idempotent DML keeps stable technical identities before the atomic key change.
-- An old order_id is at most 32 bytes; the legacy identity fits varbinary(64).
-- Resume safely after either ADD COLUMN or these updates; retain new identities.
UPDATE rcc_publication_commands SET execution_id=CONCAT('legacy:',order_id) WHERE execution_id=X'';
UPDATE rcc_refresh_notifications SET execution_id=CONCAT('legacy:',order_id) WHERE execution_id=X'';

SET @rcc_execution_ddl = IF((SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_refresh_notifications' AND INDEX_NAME='PRIMARY')='execution_id,table_name','DO 0','ALTER TABLE rcc_refresh_notifications DROP PRIMARY KEY, ADD PRIMARY KEY(execution_id,table_name)');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;
SET @rcc_execution_ddl = NULL;

-- The main order no longer has a single-table default. Its immutable legacy
-- document remains untouched; current table membership belongs to details.
SET @rcc_header_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_release_orders' AND COLUMN_NAME='table_name'),'ALTER TABLE rcc_release_orders DROP INDEX release_table, DROP COLUMN table_name','DO 0');
PREPARE rcc_header_stmt FROM @rcc_header_ddl;
EXECUTE rcc_header_stmt;
DEALLOCATE PREPARE rcc_header_stmt;
SET @rcc_header_ddl = NULL;
