-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE IF NOT EXISTS rcc_release_templates (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code VARCHAR(100) NOT NULL,
  name VARCHAR(100) NOT NULL,
  description VARCHAR(500) NOT NULL DEFAULT '',
  release_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  node_list JSON NOT NULL,
  monitor_list JSON NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  version BIGINT UNSIGNED NOT NULL DEFAULT 1,
  creator VARCHAR(64) NOT NULL,
  modifier VARCHAR(64) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id),
  UNIQUE KEY uk_release_template_code (code),
  KEY idx_release_template_type_enabled (release_type, enabled),
  CONSTRAINT chk_release_template_type CHECK (release_type IN ('STANDARD', 'EMERGENCY')),
  CONSTRAINT chk_release_template_enabled CHECK (enabled IN (0, 1)),
  CONSTRAINT chk_emergency_template_enabled CHECK (release_type <> 'EMERGENCY' OR enabled = 1),
  CONSTRAINT chk_release_template_version CHECK (version > 0),
  CONSTRAINT chk_release_template_nodes CHECK (JSON_TYPE(node_list) = 'ARRAY'),
  CONSTRAINT chk_release_template_monitors CHECK (JSON_TYPE(monitor_list) = 'ARRAY' AND JSON_LENGTH(monitor_list) = 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Release Template Catalog';

INSERT IGNORE INTO rcc_release_templates(code,name,description,release_type,node_list,monitor_list,enabled,version,creator,modifier)
VALUES
  ('default_standard_v1','默认常规发布','按表审批后由发布人员执行并人工完结','STANDARD',JSON_ARRAY(
    JSON_OBJECT('code','approval','type','APPROVAL','name','按表审批','required_role','TABLE_APPROVER'),
    JSON_OBJECT('code','publication','type','PUBLICATION','name','发布','required_role','PUBLISHER'),
    JSON_OBJECT('code','completion','type','COMPLETION','name','完结','required_role','PUBLISHER')
  ),JSON_ARRAY(),1,1,'system:migration','system:migration'),
  ('default_emergency_v1','默认应急发布','免审批，由发布人员执行并人工完结','EMERGENCY',JSON_ARRAY(
    JSON_OBJECT('code','publication','type','PUBLICATION','name','应急发布','required_role','PUBLISHER'),
    JSON_OBJECT('code','completion','type','COMPLETION','name','完结','required_role','PUBLISHER')
  ),JSON_ARRAY(),1,1,'system:migration','system:migration');
