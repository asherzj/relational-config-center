-- Expand-stage migration: add reusable Mutation Policy definitions without
-- changing the existing Table Policy schema or runtime behavior.

CREATE TABLE `rcc_mutation_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(100) NOT NULL,
  `name` varchar(100) NOT NULL,
  `description` varchar(500) NOT NULL DEFAULT '',
  `type_code` varchar(64) NOT NULL,
  `allow_add` tinyint(1) NOT NULL DEFAULT 0,
  `allow_modify` tinyint(1) NOT NULL DEFAULT 0,
  `allow_delete` tinyint(1) NOT NULL DEFAULT 0,
  `create_operator_field` varchar(64) NULL,
  `create_time_field` varchar(64) NULL,
  `modify_operator_field` varchar(64) NULL,
  `modify_time_field` varchar(64) NULL,
  `status` varchar(16) NOT NULL DEFAULT 'DRAFT',
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_mutation_policy_code` (`code`),
  KEY `idx_mutation_policy_status_type` (`status`, `type_code`),
  CONSTRAINT `chk_mutation_policy_code` CHECK (`code` REGEXP '^[a-z][a-z0-9_]*_v[1-9][0-9]*$'),
  CONSTRAINT `chk_mutation_policy_status` CHECK (`status` IN ('DRAFT', 'ACTIVE', 'DEPRECATED')),
  CONSTRAINT `chk_mutation_policy_capabilities` CHECK (`allow_add` IN (0, 1) AND `allow_modify` IN (0, 1) AND `allow_delete` IN (0, 1)),
  CONSTRAINT `chk_mutation_policy_safe_fields` CHECK (
    (`create_operator_field` IS NULL OR (`create_operator_field` <> 'id' AND `create_operator_field` REGEXP '^[A-Za-z_][A-Za-z0-9_]*$')) AND
    (`create_time_field` IS NULL OR (`create_time_field` <> 'id' AND `create_time_field` REGEXP '^[A-Za-z_][A-Za-z0-9_]*$')) AND
    (`modify_operator_field` IS NULL OR (`modify_operator_field` <> 'id' AND `modify_operator_field` REGEXP '^[A-Za-z_][A-Za-z0-9_]*$')) AND
    (`modify_time_field` IS NULL OR (`modify_time_field` <> 'id' AND `modify_time_field` REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'))
  ),
  CONSTRAINT `chk_mutation_policy_distinct_fields` CHECK (
    (`create_operator_field` IS NULL OR `create_time_field` IS NULL OR `create_operator_field` <> `create_time_field`) AND
    (`create_operator_field` IS NULL OR `modify_operator_field` IS NULL OR `create_operator_field` <> `modify_operator_field`) AND
    (`create_operator_field` IS NULL OR `modify_time_field` IS NULL OR `create_operator_field` <> `modify_time_field`) AND
    (`create_time_field` IS NULL OR `modify_operator_field` IS NULL OR `create_time_field` <> `modify_operator_field`) AND
    (`create_time_field` IS NULL OR `modify_time_field` IS NULL OR `create_time_field` <> `modify_time_field`) AND
    (`modify_operator_field` IS NULL OR `modify_time_field` IS NULL OR `modify_operator_field` <> `modify_time_field`)
  ),
  CONSTRAINT `chk_mutation_policy_slot_capabilities` CHECK (
    (`allow_add` = 1 OR (`create_operator_field` IS NULL AND `create_time_field` IS NULL)) AND
    (`allow_add` = 1 OR `allow_modify` = 1 OR (`modify_operator_field` IS NULL AND `modify_time_field` IS NULL))
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Mutation Policy Catalog';
