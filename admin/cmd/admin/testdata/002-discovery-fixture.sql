CREATE TABLE `managed_alpha` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `value` varchar(64) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Alpha configuration';

CREATE TABLE `missing_primary_key` (
  `value` varchar(64) NOT NULL
) ENGINE=InnoDB COMMENT='No primary key';

CREATE TABLE `wrong_primary_key` (
  `code` varchar(64) NOT NULL,
  PRIMARY KEY (`code`)
) ENGINE=InnoDB COMMENT='Wrong primary key';

CREATE TABLE `uppercase_id` (
  `ID` bigint unsigned NOT NULL,
  PRIMARY KEY (`ID`)
) ENGINE=InnoDB COMMENT='Uppercase primary key';

CREATE TABLE `composite_key` (
  `id` bigint unsigned NOT NULL,
  `scope` varchar(64) NOT NULL,
  PRIMARY KEY (`id`, `scope`)
) ENGINE=InnoDB COMMENT='Composite key';

CREATE VIEW `managed_view` AS SELECT `id`, `value` FROM `managed_alpha`;

CREATE TABLE `RCC_shadow_control` (
  `id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB COMMENT='Protected control table';

INSERT INTO `rcc_table_policies` (
  `table_name`,
  `query_policy`,
  `query_policy_config`,
  `mutation_policy`,
  `mutation_policy_config`,
  `enabled`,
  `creator`,
  `modifier`
) VALUES (
  'managed_alpha',
  'mysql_page_query_v1',
  JSON_OBJECT(),
  'mysql_single_table_mutation_v1',
  JSON_OBJECT(),
  1,
  'integration-test',
  'integration-test'
);
