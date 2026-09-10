-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE IF NOT EXISTS `rcc_table_field_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `table_name` varchar(64) NOT NULL COMMENT '真实业务表名',
  `field_name` varchar(64) NOT NULL COMMENT '真实数据库字段名',
  `display_name` varchar(200) NOT NULL COMMENT '字段显示名称',
  `description` varchar(500) NOT NULL DEFAULT '' COMMENT '字段说明',
  `is_visible` tinyint(1) NOT NULL DEFAULT 1,
  `display_order` int unsigned NOT NULL DEFAULT 0,
  `is_queryable` tinyint(1) NOT NULL DEFAULT 1,
  `query_operators` json DEFAULT NULL COMMENT '显式运算符；NULL不推导',
  `ui_type` varchar(32) NOT NULL DEFAULT 'text',
  `ui_options` json DEFAULT NULL,
  `editable_on_add` tinyint(1) NOT NULL DEFAULT 1,
  `editable_on_modify` tinyint(1) NOT NULL DEFAULT 1,
  `is_required` tinyint(1) NOT NULL DEFAULT 0,
  `default_value` json DEFAULT NULL COMMENT 'SQL NULL为未配置；JSON null为显式空值',
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_table_field` (`table_name`, `field_name`),
  KEY `idx_field_display` (`table_name`, `enabled`, `display_order`),
  CONSTRAINT `chk_field_policy_flags` CHECK (`is_visible` IN (0,1) AND `is_queryable` IN (0,1) AND `editable_on_add` IN (0,1) AND `editable_on_modify` IN (0,1) AND `is_required` IN (0,1) AND `enabled` IN (0,1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='表字段规则';
