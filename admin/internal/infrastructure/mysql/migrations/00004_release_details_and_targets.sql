-- +goose NO TRANSACTION
-- +goose Up
-- Additive stage: each pre-existing table changes in one atomic ALTER.
-- Notification identity/key changes belong to the next version so recovery
-- always sees a whole previous or target table definition.
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
-- Successful executions only; the detail table owns actual per-item rows.
CREATE TABLE IF NOT EXISTS rcc_release_executions (
 order_id varbinary(32) NOT NULL,
 kind varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 execution_id varbinary(64) NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(order_id,kind),
 CONSTRAINT release_execution_kind CHECK(kind IN ('PUBLICATION','ROLLBACK'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

SET @rcc_execution_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_publication_commands' AND COLUMN_NAME='execution_id'),'DO 0','ALTER TABLE rcc_publication_commands ADD COLUMN execution_id varbinary(64) NOT NULL AFTER order_id');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;

SET @rcc_execution_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_refresh_notifications' AND COLUMN_NAME='execution_id'),'DO 0','ALTER TABLE rcc_refresh_notifications ADD COLUMN execution_id varbinary(64) NOT NULL AFTER order_id');
PREPARE rcc_execution_stmt FROM @rcc_execution_ddl;
EXECUTE rcc_execution_stmt;
DEALLOCATE PREPARE rcc_execution_stmt;

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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
