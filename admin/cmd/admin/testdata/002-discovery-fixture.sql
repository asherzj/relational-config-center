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

INSERT INTO `rcc_query_policies` (
  `code`, `name`, `type_code`, `default_order_field`,
  `default_order_direction`, `default_page_size`, `max_page_size`,
  `status`, `creator`, `modifier`
) VALUES (
  'discovery_query_v1', 'Discovery query', 'page_query', 'id',
  'DESC', 20, 200, 'ACTIVE', 'integration-test', 'integration-test'
);

INSERT INTO `rcc_mutation_policies` (
  `code`, `name`, `type_code`, `allow_add`, `allow_modify`, `allow_delete`,
  `status`, `creator`, `modifier`
) VALUES (
  'discovery_mutation_v1', 'Discovery mutation', 'single_table_mutation',
  0, 0, 0, 'ACTIVE', 'integration-test', 'integration-test'
);

INSERT INTO `rcc_table_policies` (
  `table_name`, `query_policy_code`, `mutation_policy_code`, `enabled`,
  `creator`, `modifier`
) VALUES (
  'managed_alpha', 'discovery_query_v1', 'discovery_mutation_v1', 1,
  'integration-test', 'integration-test'
);
