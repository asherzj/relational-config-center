SET NAMES utf8mb4;

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
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
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
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
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

CREATE TABLE `rcc_table_policies` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `table_name` varchar(64) NOT NULL,
  `query_policy_code` varchar(100) NOT NULL,
  `mutation_policy_code` varchar(100) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `creator` varchar(64) NOT NULL,
  `modifier` varchar(64) NOT NULL,
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `concurrency_key` json NOT NULL DEFAULT (JSON_ARRAY()),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_table_name` (`table_name`),
  KEY `idx_table_policy_query_code` (`query_policy_code`),
  KEY `idx_table_policy_mutation_code` (`mutation_policy_code`),
  CONSTRAINT `chk_table_policy_enabled` CHECK (`enabled` IN (0, 1))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Table Policy Catalog';
-- Local Account final control tables. Apply once during the maintenance window.
-- All rcc_ tables are excluded from generic discovery, policies and data APIs.
CREATE TABLE rcc_accounts (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 username VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 email VARCHAR(254) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 display_name VARCHAR(64) NOT NULL,
 password_hash VARCHAR(255) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 password_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
 session_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
 roles TINYINT UNSIGNED NOT NULL DEFAULT 1,
 role_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_rcc_accounts_username(username), UNIQUE KEY uq_rcc_accounts_email(email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
CREATE TABLE rcc_login_sessions (
 token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 account_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 csrf_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 password_version BIGINT UNSIGNED NOT NULL,
 session_version BIGINT UNSIGNED NOT NULL,
 created_at DATETIME(6) NOT NULL,
 last_active_at DATETIME(6) NOT NULL,
 expires_at DATETIME(6) NOT NULL,
 KEY ix_rcc_sessions_account(account_id), KEY ix_rcc_sessions_expiry(expires_at),
 CONSTRAINT fk_rcc_sessions_account FOREIGN KEY(account_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB;
CREATE TABLE rcc_preauth_credentials (
 token_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 csrf_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 expires_at DATETIME(6) NOT NULL,
 KEY ix_rcc_preauth_expiry(expires_at)
) ENGINE=InnoDB;
CREATE TABLE rcc_auth_rate_limits (
 bucket_key VARCHAR(80) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 attempts INT UNSIGNED NOT NULL,
 in_flight INT UNSIGNED NOT NULL DEFAULT 0,
 expires_at DATETIME(6) NOT NULL,
 KEY ix_rcc_rate_expiry(expires_at)
) ENGINE=InnoDB;
-- Serializes bounded control-table admissions across processes, not password work.
CREATE TABLE rcc_auth_control_lock (id INT PRIMARY KEY) ENGINE=InnoDB;
INSERT INTO rcc_auth_control_lock(id) VALUES (1);
-- Append-only role decisions also retain successful request results for retries.
CREATE TABLE IF NOT EXISTS rcc_account_role_history (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 actor_kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 actor_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 account_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 before_roles TINYINT UNSIGNED NOT NULL,
 after_roles TINYINT UNSIGNED NOT NULL,
 version BIGINT UNSIGNED NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,
 UNIQUE KEY uq_rcc_role_request(actor_id,request_key),
 KEY ix_rcc_role_history(account_id,id),
 CONSTRAINT fk_rcc_role_history_account FOREIGN KEY(account_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB;

-- The empty key is the table maintenance generation floor; 32-byte keys are record identities.
CREATE TABLE IF NOT EXISTS rcc_record_versions (
 table_name VARBINARY(256) NOT NULL,
 record_key VARBINARY(32) NOT NULL,
 lock_version BIGINT UNSIGNED NOT NULL,
 PRIMARY KEY (table_name, record_key)
) ENGINE=InnoDB;
-- Stop old writers before upgrade. This migration is restartable and does not touch business rows.
CREATE TABLE IF NOT EXISTS rcc_release_orders (
 id varbinary(32) NOT NULL PRIMARY KEY,
 table_name varbinary(256) NOT NULL,
 applicant_id varbinary(36) NOT NULL,
 state varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version bigint unsigned NOT NULL,
 document json NOT NULL,
 KEY release_table(table_name,id),
 KEY release_applicant(applicant_id,id),
 KEY release_state(state,id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_release_requests (
 actor_id varbinary(36) NOT NULL,
 operation varbinary(96) NOT NULL,
 request_key varbinary(64) NOT NULL,
 digest binary(32) NOT NULL,
 result json NULL,
 PRIMARY KEY(actor_id,operation,request_key)
) ENGINE=InnoDB;

-- Apply after 010. Reservations have no TTL and are released only by workflow.
CREATE TABLE IF NOT EXISTS rcc_release_targets (
 table_name varbinary(256) NOT NULL,
 record_key binary(32) NOT NULL,
 order_id varbinary(32) NOT NULL,
 PRIMARY KEY (table_name,record_key),
 KEY release_target_order(order_id)
) ENGINE=InnoDB;
-- Apply after 011. These immutable records describe this Admin single data source.
CREATE TABLE IF NOT EXISTS rcc_table_publications (
 table_name varbinary(256) NOT NULL,
 table_version bigint unsigned NOT NULL,
 command_cursor bigint unsigned NOT NULL,
 PRIMARY KEY(table_name)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_publication_commands (
 table_name varbinary(256) NOT NULL,
 sequence bigint unsigned NOT NULL,
 order_id varbinary(32) NOT NULL,
 execution_id varbinary(64) NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(table_name,sequence),
 KEY publication_order(order_id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS rcc_refresh_notifications (
 order_id varbinary(32) NOT NULL,
 execution_id varbinary(64) NOT NULL,
 table_name varbinary(256) NOT NULL,
 table_version bigint unsigned NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(execution_id,table_name)
) ENGINE=InnoDB;
-- Each detail retains its application and both actual results independently.
CREATE TABLE IF NOT EXISTS rcc_release_details (
 order_id varbinary(32) NOT NULL,
 position int unsigned NOT NULL,
 table_name varbinary(256) NOT NULL,
 application json NOT NULL,
 publication json NULL,
 rollback json NULL,
 PRIMARY KEY(order_id,position),
 KEY release_detail_table(table_name,order_id)
) ENGINE=InnoDB;
-- Successful executions only; the detail table owns actual per-item rows.
CREATE TABLE IF NOT EXISTS rcc_release_executions (
 order_id varbinary(32) NOT NULL,
 kind varchar(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 execution_id varbinary(64) NOT NULL,
 document json NOT NULL,
 PRIMARY KEY(order_id,kind),
 CONSTRAINT release_execution_kind CHECK(kind IN ('PUBLICATION','ROLLBACK'))
) ENGINE=InnoDB;

CREATE TABLE rcc_release_table_references (
 table_name varbinary(256) NOT NULL,
 order_id varbinary(32) NOT NULL,
 PRIMARY KEY(table_name,order_id), KEY release_reference_order(order_id)
) ENGINE=InnoDB;
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
