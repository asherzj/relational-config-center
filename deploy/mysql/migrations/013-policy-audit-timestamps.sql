-- Run with Admin stopped. Rename only Policy Catalog audit columns; existing
-- values, defaults and ON UPDATE behavior are preserved. Safe to rerun after
-- completion or interruption between tables. See README.md for legacy rollout.
SET NAMES utf8mb4;

DELIMITER $$
DROP PROCEDURE IF EXISTS `rcc_rename_policy_audit_timestamps`$$
CREATE PROCEDURE `rcc_rename_policy_audit_timestamps`()
BEGIN
  DECLARE catalog_index int DEFAULT 0;
  DECLARE catalog_name varchar(64);
  DECLARE table_count int;
  DECLARE created_count int;
  DECLARE updated_count int;
  DECLARE old_created_count int;
  DECLARE old_updated_count int;
  DECLARE diagnostic varchar(128);

  -- Validate every table before any DDL. Do not guess which value to keep when
  -- old and new columns coexist, or invent a timestamp when both are missing.
  WHILE catalog_index < 3 DO
    SET catalog_name = ELT(catalog_index + 1,
      'rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies');
    SELECT COUNT(*) INTO table_count FROM information_schema.tables
      WHERE table_schema = DATABASE() AND table_name = catalog_name AND table_type = 'BASE TABLE';
    SELECT COUNT(CASE WHEN column_name IN ('gmt_created', 'created_at') THEN 1 END),
           COUNT(CASE WHEN column_name IN ('gmt_modified', 'updated_at') THEN 1 END)
      INTO created_count, updated_count
      FROM information_schema.columns
      WHERE table_schema = DATABASE() AND table_name = catalog_name;
    IF table_count <> 1 OR created_count <> 1 OR updated_count <> 1 THEN
      SET diagnostic = CONCAT('013 blocked: ', catalog_name, ' needs exactly one column per old/new audit pair');
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
    END IF;
    SET catalog_index = catalog_index + 1;
  END WHILE;

  SET catalog_index = 0;
  WHILE catalog_index < 3 DO
    SET catalog_name = ELT(catalog_index + 1,
      'rcc_query_policies', 'rcc_mutation_policies', 'rcc_table_policies');
    SELECT COUNT(CASE WHEN column_name = 'gmt_created' THEN 1 END),
           COUNT(CASE WHEN column_name = 'gmt_modified' THEN 1 END)
      INTO old_created_count, old_updated_count
      FROM information_schema.columns
      WHERE table_schema = DATABASE() AND table_name = catalog_name;
    IF old_created_count + old_updated_count > 0 THEN
      SET @rcc_policy_audit_ddl = CONCAT('ALTER TABLE `', catalog_name, '` ',
        IF(old_created_count = 1, 'RENAME COLUMN `gmt_created` TO `created_at`', ''),
        IF(old_created_count = 1 AND old_updated_count = 1, ', ', ''),
        IF(old_updated_count = 1, 'RENAME COLUMN `gmt_modified` TO `updated_at`', ''));
      PREPARE rcc_policy_audit_statement FROM @rcc_policy_audit_ddl;
      EXECUTE rcc_policy_audit_statement;
      DEALLOCATE PREPARE rcc_policy_audit_statement;
      SET @rcc_policy_audit_ddl = NULL;
    END IF;
    SET catalog_index = catalog_index + 1;
  END WHILE;
END$$
DELIMITER ;

CALL `rcc_rename_policy_audit_timestamps`();
DROP PROCEDURE `rcc_rename_policy_audit_timestamps`;
