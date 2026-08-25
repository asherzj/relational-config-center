CREATE TABLE `legacy_supported_table` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB;

CREATE TABLE `legacy_unsupported_table` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `status` varchar(32) NOT NULL,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB;

INSERT INTO `rcc_table_policies` (
  `table_name`, `query_policy`, `query_policy_config`, `mutation_policy`,
  `mutation_policy_config`, `allow_add`, `allow_modify`, `allow_delete`,
  `enabled`, `creator`, `modifier`
) VALUES
  ('legacy_supported_table', 'mysql_page_query_v1', JSON_OBJECT(),
   'mysql_single_table_mutation_v1', JSON_OBJECT(), 0, 0, 0, 0, 'migration-test', 'migration-test'),
  ('legacy_unsupported_table', 'mysql_page_query_v1', JSON_OBJECT(),
   'mysql_single_table_mutation_v1',
   JSON_OBJECT('auto_fill', JSON_OBJECT('add', JSON_OBJECT('status', JSON_OBJECT('source', 'literal', 'value', 'READY')))),
   1, 0, 0, 0, 'migration-test', 'migration-test');
