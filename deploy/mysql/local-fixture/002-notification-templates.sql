-- Local-development fixture only. Docker Compose reapplies this script on every
-- startup so fresh and compatible existing development volumes provide one
-- immediately usable Managed Table. Conflicts fail closed instead of being
-- overwritten. Production initialization must not load this script.

CREATE TABLE IF NOT EXISTS `notification_templates` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `template_key` varchar(100) NOT NULL,
  `channel` enum('EMAIL', 'SMS', 'PUSH') NOT NULL,
  `subject` varchar(200) NULL,
  `body` text NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `priority` int NOT NULL DEFAULT 100,
  `retry_interval_seconds` decimal(8,2) NOT NULL DEFAULT 30.00,
  `active_from` date NULL,
  `delivery_window_start` time NULL,
  `metadata` json NULL,
  `creator` varchar(64) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_template_key` (`template_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci
  COMMENT='Local notification template configuration';

-- Existing local definitions are never rewritten. A same-Code or same-table
-- lifecycle/configuration conflict fails the one-shot service so an operator
-- can resolve it through Admin instead of bypassing Policy invariants in SQL.
CREATE TEMPORARY TABLE `local_fixture_assertion` (
  `valid` tinyint NOT NULL CHECK (`valid` = 1)
);

INSERT INTO `local_fixture_assertion` (`valid`)
SELECT IF(
  EXISTS (
    SELECT 1
    FROM `information_schema`.`TABLES`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
      AND `TABLE_TYPE` = 'BASE TABLE'
  )
  AND (
    SELECT COUNT(*)
    FROM `information_schema`.`COLUMNS`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
  ) = 15
  AND (
    SELECT COUNT(*)
    FROM `information_schema`.`COLUMNS`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
      AND `GENERATION_EXPRESSION` = ''
      AND (
        (`COLUMN_NAME` = 'id' AND `COLUMN_TYPE` = 'bigint unsigned' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = 'auto_increment')
        OR (`COLUMN_NAME` = 'template_key' AND `COLUMN_TYPE` = 'varchar(100)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'channel' AND `COLUMN_TYPE` = 'enum(''EMAIL'',''SMS'',''PUSH'')' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'subject' AND `COLUMN_TYPE` = 'varchar(200)' AND `IS_NULLABLE` = 'YES' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'body' AND `COLUMN_TYPE` = 'text' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'enabled' AND `COLUMN_TYPE` = 'tinyint(1)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'priority' AND `COLUMN_TYPE` = 'int' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'retry_interval_seconds' AND `COLUMN_TYPE` = 'decimal(8,2)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'active_from' AND `COLUMN_TYPE` = 'date' AND `IS_NULLABLE` = 'YES' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'delivery_window_start' AND `COLUMN_TYPE` = 'time' AND `IS_NULLABLE` = 'YES' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'metadata' AND `COLUMN_TYPE` = 'json' AND `IS_NULLABLE` = 'YES' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'creator' AND `COLUMN_TYPE` = 'varchar(64)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'created_at' AND `COLUMN_TYPE` = 'datetime(6)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'modifier' AND `COLUMN_TYPE` = 'varchar(64)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
        OR (`COLUMN_NAME` = 'updated_at' AND `COLUMN_TYPE` = 'datetime(6)' AND `IS_NULLABLE` = 'NO' AND `EXTRA` = '')
      )
  ) = 15
  AND (
    SELECT COUNT(*)
    FROM `information_schema`.`STATISTICS`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
      AND `INDEX_NAME` = 'PRIMARY'
      AND `COLUMN_NAME` = 'id'
      AND `SEQ_IN_INDEX` = 1
  ) = 1
  AND (
    SELECT COUNT(*)
    FROM `information_schema`.`STATISTICS`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
      AND `INDEX_NAME` = 'PRIMARY'
  ) = 1
  AND EXISTS (
    SELECT 1
    FROM `information_schema`.`STATISTICS`
    WHERE `TABLE_SCHEMA` = DATABASE()
      AND `TABLE_NAME` = 'notification_templates'
      AND `NON_UNIQUE` = 0
    GROUP BY `INDEX_NAME`
    HAVING COUNT(*) = 1 AND MAX(`COLUMN_NAME` = 'template_key') = 1
  ),
  1,
  0
);

INSERT INTO `local_fixture_assertion` (`valid`)
SELECT IF(
  NOT EXISTS (
    SELECT 1 FROM `rcc_query_policies`
    WHERE `code` = 'notification_page_query_v1'
  ) OR EXISTS (
    SELECT 1 FROM `rcc_query_policies`
    WHERE `code` = 'notification_page_query_v1'
      AND `type_code` = 'page_query'
      AND `default_order_field` = 'id'
      AND `default_order_direction` = 'DESC'
      AND `default_page_size` = 20
      AND `max_page_size` = 200
      AND `status` = 'ACTIVE'
  ),
  1,
  0
);

INSERT INTO `local_fixture_assertion` (`valid`)
SELECT IF(
  NOT EXISTS (
    SELECT 1 FROM `rcc_mutation_policies`
    WHERE `code` = 'notification_full_mutation_v1'
  ) OR EXISTS (
    SELECT 1 FROM `rcc_mutation_policies`
    WHERE `code` = 'notification_full_mutation_v1'
      AND `type_code` = 'single_table_mutation'
      AND `allow_add` = 1
      AND `allow_modify` = 1
      AND `allow_delete` = 1
      AND `create_operator_field` <=> 'creator'
      AND `create_time_field` <=> 'created_at'
      AND `modify_operator_field` <=> 'modifier'
      AND `modify_time_field` <=> 'updated_at'
      AND `status` = 'ACTIVE'
  ),
  1,
  0
);

INSERT INTO `local_fixture_assertion` (`valid`)
SELECT IF(
  NOT EXISTS (
    SELECT 1 FROM `rcc_table_policies`
    WHERE `table_name` = 'notification_templates'
  ) OR EXISTS (
    SELECT 1 FROM `rcc_table_policies`
    WHERE `table_name` = 'notification_templates'
      AND `query_policy_code` = 'notification_page_query_v1'
      AND `mutation_policy_code` = 'notification_full_mutation_v1'
      AND `enabled` = 1
  ),
  1,
  0
);

DROP TEMPORARY TABLE `local_fixture_assertion`;

START TRANSACTION;

INSERT IGNORE INTO `notification_templates` (
  `template_key`, `channel`, `subject`, `body`, `enabled`, `priority`,
  `retry_interval_seconds`, `active_from`, `delivery_window_start`, `metadata`,
  `creator`, `created_at`, `modifier`, `updated_at`
) VALUES
  (
    'account-welcome', 'EMAIL', 'Welcome to RCC', 'Your account is ready.', 1, 10,
    15.00, '2026-01-01', '09:00:00', JSON_OBJECT('locale', 'en-US', 'category', 'onboarding'),
    'local-fixture', UTC_TIMESTAMP(6), 'local-fixture', UTC_TIMESTAMP(6)
  ),
  (
    'payment-received', 'PUSH', NULL, 'Payment received successfully.', 1, 20,
    30.00, NULL, NULL, JSON_OBJECT('locale', 'en-US', 'category', 'billing'),
    'local-fixture', UTC_TIMESTAMP(6), 'local-fixture', UTC_TIMESTAMP(6)
  ),
  (
    'maintenance-window', 'SMS', NULL, 'Scheduled maintenance starts soon.', 0, 30,
    60.50, '2026-08-01', '22:30:00', NULL,
    'local-fixture', UTC_TIMESTAMP(6), 'local-fixture', UTC_TIMESTAMP(6)
  );

INSERT IGNORE INTO `rcc_query_policies` (
  `code`, `name`, `description`, `type_code`, `default_order_field`,
  `default_order_direction`, `default_page_size`, `max_page_size`, `status`,
  `creator`, `modifier`
) VALUES (
  'notification_page_query_v1',
  'Notification template paging',
  'Local fixture query policy for notification_templates',
  'page_query',
  'id',
  'DESC',
  20,
  200,
  'ACTIVE',
  'local-fixture',
  'local-fixture'
);

INSERT IGNORE INTO `rcc_mutation_policies` (
  `code`, `name`, `description`, `type_code`,
  `allow_add`, `allow_modify`, `allow_delete`,
  `create_operator_field`, `create_time_field`,
  `modify_operator_field`, `modify_time_field`,
  `status`, `creator`, `modifier`
) VALUES (
  'notification_full_mutation_v1',
  'Notification template full mutation',
  'Local fixture mutation policy with standard audit Auto Fill',
  'single_table_mutation',
  1,
  1,
  1,
  'creator',
  'created_at',
  'modifier',
  'updated_at',
  'ACTIVE',
  'local-fixture',
  'local-fixture'
);

INSERT IGNORE INTO `rcc_table_policies` (
  `table_name`, `query_policy_code`, `mutation_policy_code`, `enabled`,
  `creator`, `modifier`
) VALUES (
  'notification_templates',
  'notification_page_query_v1',
  'notification_full_mutation_v1',
  1,
  'local-fixture',
  'local-fixture'
);


-- Explicit fixture associations; reruns preserve administrator choices.
INSERT INTO rcc_table_release_templates(table_policy_id,release_type,template_id,enabled,version,creator,modifier)
SELECT p.id,t.release_type,t.id,1,1,'local-fixture','local-fixture'
FROM rcc_table_policies p JOIN rcc_release_templates t
  ON (t.code='default_standard_v1' AND t.release_type='STANDARD')
  OR (t.code='default_emergency_v1' AND t.release_type='EMERGENCY')
WHERE p.table_name='notification_templates' AND t.enabled=1 AND NOT EXISTS (
  SELECT 1 FROM rcc_table_release_templates a WHERE a.table_policy_id=p.id AND a.release_type=t.release_type
);

COMMIT;
