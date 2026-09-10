-- Stop Admin before applying. No old release-order data is migrated or deleted.
-- Give old technical records distinct identities before changing notification
-- uniqueness. Their documents stay unchanged; no legacy business order is read.
-- Each detail retains its application and both actual results independently.
CREATE TABLE IF NOT EXISTS rcc_release_details (
 order_id varbinary(32) NOT NULL,
 position int unsigned NOT NULL,
 table_name varbinary(256) NOT NULL,
 application json NOT NULL,
 publication json NULL,
 rollback json NULL,
 PRIMARY KEY(order_id,position),
 KEY release_detail_table(table_name,order_id)
) ENGINE=InnoDB;
-- Successful executions only; the detail table owns actual per-item rows.
CREATE TABLE IF NOT EXISTS rcc_release_executions (
 order_id varbinary(32) NOT NULL,
 kind varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 execution_id varbinary(64) NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(order_id,kind),
 CONSTRAINT release_execution_kind CHECK(kind IN ('PUBLICATION','ROLLBACK'))
) ENGINE=InnoDB;

SET @rcc_execution_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_publication_commands' AND COLUMN_NAME='execution_id'),'DO 0','ALTER TABLE rcc_publication_commands ADD COLUMN execution_id varbinary(64) NOT NULL AFTER order_id');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;

SET @rcc_execution_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_refresh_notifications' AND COLUMN_NAME='execution_id'),'DO 0','ALTER TABLE rcc_refresh_notifications ADD COLUMN execution_id varbinary(64) NOT NULL AFTER order_id');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;

-- An old order_id is at most 32 bytes; the legacy identity fits varbinary(64).
-- Resume safely after either ADD COLUMN or these updates; retain new identities.
UPDATE rcc_publication_commands SET execution_id=CONCAT('legacy:',order_id) WHERE execution_id=X'';
UPDATE rcc_refresh_notifications SET execution_id=CONCAT('legacy:',order_id) WHERE execution_id=X'';

SET @rcc_execution_ddl = IF((SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_refresh_notifications' AND INDEX_NAME='PRIMARY')='execution_id,table_name','DO 0','ALTER TABLE rcc_refresh_notifications DROP PRIMARY KEY, ADD PRIMARY KEY(execution_id,table_name)');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;
SET @rcc_execution_ddl = NULL;
