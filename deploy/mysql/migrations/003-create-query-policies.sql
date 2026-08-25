-- Expand-stage migration: add reusable Query Policy definitions without
-- changing the existing Table Policy schema or runtime behavior.

CREATE TABLE `rcc_query_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(100) NOT NULL,
  `name` varchar(100) NOT NULL,
  `description` varchar(500) NOT NULL DEFAULT '',
  `type_code` varchar(64) NOT NULL,
  `default_order_field` varchar(64) NOT NULL,
  `default_order_direction` varchar(8) NOT NULL,
  `default_page_size` int NOT NULL,
  `max_page_size` int NOT NULL,
  `status` varchar(16) NOT NULL DEFAULT 'DRAFT',
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `gmt_created` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `gmt_modified` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_query_policy_code` (`code`),
  KEY `idx_query_policy_status_type` (`status`, `type_code`),
  CONSTRAINT `chk_query_policy_code` CHECK (`code` REGEXP '^[a-z][a-z0-9_]*_v[1-9][0-9]*$'),
  CONSTRAINT `chk_query_policy_status` CHECK (`status` IN ('DRAFT', 'ACTIVE', 'DEPRECATED')),
  CONSTRAINT `chk_query_policy_order_field` CHECK (`default_order_field` REGEXP '^[A-Za-z_][A-Za-z0-9_]*$'),
  CONSTRAINT `chk_query_policy_order_direction` CHECK (`default_order_direction` IN ('ASC', 'DESC')),
  CONSTRAINT `chk_query_policy_positive_page_sizes` CHECK (`default_page_size` > 0 AND `max_page_size` > 0),
  CONSTRAINT `chk_query_policy_page_size_order` CHECK (`default_page_size` <= `max_page_size`),
  CONSTRAINT `chk_query_policy_max_page_size` CHECK (`max_page_size` <= 200)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Query Policy Catalog';
