CREATE TABLE `rcc_table_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `table_name` varchar(64) NOT NULL,
  `query_policy` varchar(100) NOT NULL,
  `query_policy_config` json NOT NULL,
  `mutation_policy` varchar(100) NOT NULL,
  `mutation_policy_config` json NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_table_name` (`table_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Table Policy Catalog';
