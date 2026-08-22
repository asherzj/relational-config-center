CREATE TABLE IF NOT EXISTS configs (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    namespace VARCHAR(128) NOT NULL,
    config_key VARCHAR(255) NOT NULL,
    config_value JSON NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'draft',
    version BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_configs_namespace_key (namespace, config_key),
    KEY idx_configs_status_updated_at (status, updated_at)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
