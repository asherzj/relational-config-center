-- Contract stage. Every assertion is evaluated before the single destructive
-- ALTER. Running this file directly therefore fails closed just like the Go
-- migration command, although `go run ./cmd/policy-migrate` provides richer
-- per-table diagnostics and is the recommended operator path.

DELIMITER $$
DROP PROCEDURE IF EXISTS `rcc_contract_legacy_table_policies`$$
CREATE PROCEDURE `rcc_contract_legacy_table_policies`()
BEGIN
  DECLARE affected_tables text;
  DECLARE diagnostic text;

  SELECT GROUP_CONCAT(`table_name` ORDER BY `table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies`
  WHERE `query_policy_code` IS NULL OR TRIM(`query_policy_code`) = ''
     OR `mutation_policy_code` IS NULL OR TRIM(`mutation_policy_code`) = '';
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: missing Policy Codes for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  SELECT GROUP_CONCAT(tp.`table_name` ORDER BY tp.`table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies` tp
  LEFT JOIN `rcc_query_policies` qp ON qp.`code` = tp.`query_policy_code`
  WHERE qp.`code` IS NULL OR qp.`status` NOT IN ('ACTIVE', 'DEPRECATED')
     OR qp.`type_code` <> 'page_query'
     OR qp.`code` NOT REGEXP '^[a-z][a-z0-9_]*_v[1-9][0-9]*$'
     OR qp.`code` REGEXP '^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_'
     OR qp.`default_order_field` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'
     OR qp.`default_order_direction` NOT IN ('ASC', 'DESC')
     OR qp.`default_page_size` <= 0 OR qp.`max_page_size` <= 0
     OR qp.`default_page_size` > qp.`max_page_size` OR qp.`max_page_size` > 200;
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: missing or non-executable Query Policy for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  SELECT GROUP_CONCAT(tp.`table_name` ORDER BY tp.`table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies` tp
  LEFT JOIN `rcc_mutation_policies` mp ON mp.`code` = tp.`mutation_policy_code`
  WHERE mp.`code` IS NULL OR mp.`status` NOT IN ('ACTIVE', 'DEPRECATED')
     OR mp.`type_code` <> 'single_table_mutation'
     OR mp.`code` NOT REGEXP '^[a-z][a-z0-9_]*_v[1-9][0-9]*$'
     OR mp.`code` REGEXP '^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_'
     OR mp.`allow_add` NOT IN (0, 1) OR mp.`allow_modify` NOT IN (0, 1) OR mp.`allow_delete` NOT IN (0, 1)
     OR (mp.`create_operator_field` IS NOT NULL AND (mp.`create_operator_field` = 'id' OR mp.`create_operator_field` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'))
     OR (mp.`create_time_field` IS NOT NULL AND (mp.`create_time_field` = 'id' OR mp.`create_time_field` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'))
     OR (mp.`modify_operator_field` IS NOT NULL AND (mp.`modify_operator_field` = 'id' OR mp.`modify_operator_field` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'))
     OR (mp.`modify_time_field` IS NOT NULL AND (mp.`modify_time_field` = 'id' OR mp.`modify_time_field` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'))
     OR (mp.`allow_add` = 0 AND (mp.`create_operator_field` IS NOT NULL OR mp.`create_time_field` IS NOT NULL))
     OR (mp.`allow_add` = 0 AND mp.`allow_modify` = 0 AND (mp.`modify_operator_field` IS NOT NULL OR mp.`modify_time_field` IS NOT NULL))
     OR (mp.`create_operator_field` IS NOT NULL AND mp.`create_operator_field` IN (mp.`create_time_field`, mp.`modify_operator_field`, mp.`modify_time_field`))
     OR (mp.`create_time_field` IS NOT NULL AND mp.`create_time_field` IN (mp.`modify_operator_field`, mp.`modify_time_field`))
     OR (mp.`modify_operator_field` IS NOT NULL AND mp.`modify_operator_field` = mp.`modify_time_field`);
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: missing or non-executable Mutation Policy for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  SELECT GROUP_CONCAT(tp.`table_name` ORDER BY tp.`table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies` tp
  WHERE tp.`query_policy` <> 'mysql_page_query_v1'
     OR JSON_TYPE(tp.`query_policy_config`) <> 'OBJECT'
     OR JSON_LENGTH(JSON_REMOVE(tp.`query_policy_config`, '$.default_order', '$.default_page_size', '$.max_page_size')) <> 0
     OR (JSON_CONTAINS_PATH(tp.`query_policy_config`, 'one', '$.default_order') = 1 AND (
          JSON_TYPE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_order')) <> 'OBJECT'
          OR JSON_LENGTH(JSON_REMOVE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_order'), '$.field', '$.direction')) <> 0
          OR JSON_CONTAINS_PATH(tp.`query_policy_config`, 'all', '$.default_order.field', '$.default_order.direction') = 0
          OR JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_order.field')) NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'
          OR JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_order.direction')) NOT IN ('ASC', 'DESC')
        ))
     OR (JSON_CONTAINS_PATH(tp.`query_policy_config`, 'one', '$.default_page_size') = 1 AND (
          JSON_TYPE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_page_size')) <> 'INTEGER'
          OR CAST(JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_page_size')) AS SIGNED) <= 0
        ))
     OR (JSON_CONTAINS_PATH(tp.`query_policy_config`, 'one', '$.max_page_size') = 1 AND (
          JSON_TYPE(JSON_EXTRACT(tp.`query_policy_config`, '$.max_page_size')) <> 'INTEGER'
          OR CAST(JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.max_page_size')) AS SIGNED) <= 0
          OR CAST(JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.max_page_size')) AS SIGNED) > 200
        ))
     OR COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.default_page_size')) AS SIGNED), 20)
        > COALESCE(CAST(JSON_UNQUOTE(JSON_EXTRACT(tp.`query_policy_config`, '$.max_page_size')) AS SIGNED), 200);
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: unsupported legacy Query configuration for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  SELECT GROUP_CONCAT(DISTINCT tp.`table_name` ORDER BY tp.`table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies` tp
  LEFT JOIN JSON_TABLE(
    COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT()),
    '$.*' COLUMNS (`rule` json PATH '$')
  ) add_rules ON TRUE
  LEFT JOIN JSON_TABLE(
    COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()),
    '$.*' COLUMNS (`rule` json PATH '$')
  ) modify_rules ON TRUE
  WHERE tp.`mutation_policy` <> 'mysql_single_table_mutation_v1'
     OR JSON_TYPE(tp.`mutation_policy_config`) <> 'OBJECT'
     OR JSON_LENGTH(JSON_REMOVE(tp.`mutation_policy_config`, '$.auto_fill')) <> 0
     OR (JSON_CONTAINS_PATH(tp.`mutation_policy_config`, 'one', '$.auto_fill') = 1 AND (
          JSON_TYPE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill')) <> 'OBJECT'
          OR JSON_LENGTH(JSON_REMOVE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill'), '$.add', '$.modify')) <> 0
          OR (JSON_CONTAINS_PATH(tp.`mutation_policy_config`, 'one', '$.auto_fill.add') = 1 AND JSON_TYPE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add')) <> 'OBJECT')
          OR (JSON_CONTAINS_PATH(tp.`mutation_policy_config`, 'one', '$.auto_fill.modify') = 1 AND JSON_TYPE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify')) <> 'OBJECT')
        ))
     OR (add_rules.`rule` IS NOT NULL AND (
          JSON_TYPE(add_rules.`rule`) <> 'OBJECT'
          OR JSON_LENGTH(JSON_REMOVE(add_rules.`rule`, '$.source')) <> 0
          OR JSON_CONTAINS_PATH(add_rules.`rule`, 'one', '$.source') = 0
          OR JSON_UNQUOTE(JSON_EXTRACT(add_rules.`rule`, '$.source')) NOT IN ('operator', 'now')
        ))
     OR (modify_rules.`rule` IS NOT NULL AND (
          JSON_TYPE(modify_rules.`rule`) <> 'OBJECT'
          OR JSON_LENGTH(JSON_REMOVE(modify_rules.`rule`, '$.source')) <> 0
          OR JSON_CONTAINS_PATH(modify_rules.`rule`, 'one', '$.source') = 0
          OR JSON_UNQUOTE(JSON_EXTRACT(modify_rules.`rule`, '$.source')) NOT IN ('operator', 'now')
        ));
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: unsupported legacy Mutation configuration for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  -- The Go preflight additionally reports exact fixed-slot multiplicity and
  -- mirrored MODIFY diagnostics. This SQL guard rejects those shapes using
  -- aggregate counts before DDL as well.
  SELECT GROUP_CONCAT(DISTINCT tp.`table_name` ORDER BY tp.`table_name` SEPARATOR ', ')
    INTO affected_tables
  FROM `rcc_table_policies` tp
  WHERE (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) ak
         WHERE ak.`field_name` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$') > 0
     OR (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) mk
         WHERE mk.`field_name` NOT REGEXP '^[A-Za-z_][A-Za-z0-9_]*$') > 0
     OR (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) mk
         WHERE NOT JSON_CONTAINS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT()),
                                 JSON_EXTRACT(tp.`mutation_policy_config`, CONCAT('$.auto_fill.modify."', mk.`field_name`, '"')),
                                 CONCAT('$."', mk.`field_name`, '"'))) > 0
     OR (tp.`allow_add` = 0 AND (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) ak
         WHERE JSON_CONTAINS_PATH(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()), 'one', CONCAT('$."', ak.`field_name`, '"')) = 0) > 0)
     OR (tp.`allow_add` = 0 AND tp.`allow_modify` = 0 AND JSON_LENGTH(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT())) > 0)
     OR (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) ak
         WHERE JSON_UNQUOTE(JSON_EXTRACT(tp.`mutation_policy_config`, CONCAT('$.auto_fill.add."', ak.`field_name`, '".source'))) = 'operator'
           AND JSON_CONTAINS_PATH(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()), 'one', CONCAT('$."', ak.`field_name`, '"')) = 0) > 1
     OR (SELECT COUNT(*) FROM JSON_TABLE(JSON_KEYS(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.add'), JSON_OBJECT())), '$[*]' COLUMNS (`field_name` varchar(64) PATH '$')) ak
         WHERE JSON_UNQUOTE(JSON_EXTRACT(tp.`mutation_policy_config`, CONCAT('$.auto_fill.add."', ak.`field_name`, '".source'))) = 'now'
           AND JSON_CONTAINS_PATH(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()), 'one', CONCAT('$."', ak.`field_name`, '"')) = 0) > 1
     OR (SELECT COUNT(*) FROM JSON_TABLE(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()), '$.*' COLUMNS (`rule` json PATH '$')) mr WHERE JSON_UNQUOTE(JSON_EXTRACT(mr.`rule`, '$.source')) = 'operator') > 1
     OR (SELECT COUNT(*) FROM JSON_TABLE(COALESCE(JSON_EXTRACT(tp.`mutation_policy_config`, '$.auto_fill.modify'), JSON_OBJECT()), '$.*' COLUMNS (`rule` json PATH '$')) mr WHERE JSON_UNQUOTE(JSON_EXTRACT(mr.`rule`, '$.source')) = 'now') > 1;
  IF affected_tables IS NOT NULL THEN
    SET diagnostic = CONCAT('contraction blocked: legacy Auto Fill cannot map to fixed slots for table(s): ', affected_tables);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = diagnostic;
  END IF;

  ALTER TABLE `rcc_table_policies`
    MODIFY COLUMN `query_policy_code` varchar(100) NOT NULL,
    MODIFY COLUMN `mutation_policy_code` varchar(100) NOT NULL,
    DROP COLUMN `query_policy`,
    DROP COLUMN `query_policy_config`,
    DROP COLUMN `mutation_policy`,
    DROP COLUMN `mutation_policy_config`,
    DROP COLUMN `allow_add`,
    DROP COLUMN `allow_modify`,
    DROP COLUMN `allow_delete`,
    RENAME COLUMN `created_at` TO `gmt_created`,
    RENAME COLUMN `updated_at` TO `gmt_modified`,
    ADD CONSTRAINT `chk_table_policy_enabled` CHECK (`enabled` IN (0, 1));
END$$
DELIMITER ;

CALL `rcc_contract_legacy_table_policies`();
DROP PROCEDURE `rcc_contract_legacy_table_policies`;
