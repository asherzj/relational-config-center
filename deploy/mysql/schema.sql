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

CREATE TABLE IF NOT EXISTS table_policies (
    resource VARCHAR(64) NOT NULL,
    policy JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (resource)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Seed policy for the built-in configs managed table. What is in the catalog
-- is entirely what the database says: Admin loads these rows at startup and
-- has no compile-time policy registration.
INSERT INTO table_policies (resource, policy) VALUES ('configs', '{
  "resource": "configs",
  "table": "configs",
  "primary_key": "id",
  "fields": {
    "created_at": {
      "column": "created_at",
      "type": "time",
      "readable": true,
      "sortable": true,
      "filter_operators": ["eq", "ne", "in", "gt", "gte", "lt", "lte"]
    },
    "id": {
      "column": "id",
      "type": "unsigned_integer",
      "readable": true,
      "sortable": true,
      "auto_increment": true,
      "filter_operators": ["eq", "ne", "in", "gt", "gte", "lt", "lte"]
    },
    "key": {
      "column": "config_key",
      "type": "string",
      "readable": true,
      "creatable": true,
      "updatable": true,
      "sortable": true,
      "required_on_create": true,
      "min_length": 1,
      "max_length": 255,
      "filter_operators": ["eq", "ne", "in", "contains"]
    },
    "namespace": {
      "column": "namespace",
      "type": "string",
      "readable": true,
      "creatable": true,
      "updatable": true,
      "sortable": true,
      "required_on_create": true,
      "min_length": 1,
      "max_length": 128,
      "filter_operators": ["eq", "ne", "in", "contains"]
    },
    "status": {
      "column": "status",
      "type": "string",
      "readable": true,
      "creatable": true,
      "updatable": true,
      "sortable": true,
      "max_length": 32,
      "allowed_values": ["draft", "published", "archived"],
      "filter_operators": ["eq", "ne", "in"]
    },
    "updated_at": {
      "column": "updated_at",
      "type": "time",
      "readable": true,
      "sortable": true,
      "filter_operators": ["eq", "ne", "in", "gt", "gte", "lt", "lte"]
    },
    "value": {
      "column": "config_value",
      "type": "json",
      "readable": true,
      "creatable": true,
      "updatable": true,
      "required_on_create": true
    },
    "version": {
      "column": "version",
      "type": "unsigned_integer",
      "readable": true,
      "sortable": true,
      "filter_operators": ["eq", "ne", "in", "gt", "gte", "lt", "lte"]
    }
  },
  "allow_create": true,
  "allow_update": true,
  "allow_delete": true,
  "default_sort": [{"field": "id", "direction": "desc"}],
  "default_page_size": 20,
  "max_page_size": 100,
  "max_filter_depth": 4,
  "max_filter_nodes": 32,
  "max_in_values": 100
}');

-- A physical table that ships without a policy: it becomes a Managed Table
-- only after a policy for it is saved through the Policy Catalog API.
CREATE TABLE IF NOT EXISTS feature_flags (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    name VARCHAR(128) NOT NULL,
    enabled TINYINT(1) NOT NULL DEFAULT 0,
    rollout_percent INT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (id),
    UNIQUE KEY uq_feature_flags_name (name)
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
