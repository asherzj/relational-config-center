-- Stop Admin before applying. Existing release data is neither migrated nor deleted.
SET @rcc_draft_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_table_policies' AND COLUMN_NAME='concurrency_key'),'DO 0','ALTER TABLE rcc_table_policies ADD COLUMN concurrency_key json NOT NULL DEFAULT (JSON_ARRAY())');
PREPARE rcc_draft_stmt FROM @rcc_draft_ddl;
EXECUTE rcc_draft_stmt;
DEALLOCATE PREPARE rcc_draft_stmt;
SET @rcc_draft_ddl = NULL;

CREATE TABLE IF NOT EXISTS rcc_release_table_references (
 table_name varbinary(256) NOT NULL,
 order_id varbinary(32) NOT NULL,
 PRIMARY KEY(table_name,order_id), KEY release_reference_order(order_id)
) ENGINE=InnoDB;
