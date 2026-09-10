-- Historical unmanaged upgrade only. No release documents or rows are rewritten.

-- The main order no longer has a single-table default. Its immutable legacy
-- document remains untouched; current table membership belongs to details.
SET @rcc_header_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_release_orders' AND COLUMN_NAME='table_name'),'ALTER TABLE rcc_release_orders DROP INDEX release_table, DROP COLUMN table_name','DO 0');
PREPARE rcc_header_stmt FROM @rcc_header_ddl;
EXECUTE rcc_header_stmt;
DEALLOCATE PREPARE rcc_header_stmt;
SET @rcc_header_ddl = NULL;
