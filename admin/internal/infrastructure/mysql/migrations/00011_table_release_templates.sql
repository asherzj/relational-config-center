-- +goose NO TRANSACTION
-- +goose Up
-- Each existing-table change is one atomic ALTER. Recovery accepts the old or
-- target table definition, never a partially specified constraint set.
SET @rcc_table_template_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_table_policies' AND column_name='version'), 'DO 0', 'ALTER TABLE rcc_table_policies ADD COLUMN version BIGINT UNSIGNED NOT NULL DEFAULT 1, ADD CONSTRAINT chk_table_policy_version CHECK (version > 0)');
PREPARE rcc_table_template_statement FROM @rcc_table_template_ddl;
EXECUTE rcc_table_template_statement;
DEALLOCATE PREPARE rcc_table_template_statement;
SET @rcc_table_template_ddl = IF(EXISTS(SELECT 1 FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='rcc_release_templates' AND index_name='uk_release_template_id_type'), 'DO 0', 'ALTER TABLE rcc_release_templates ADD UNIQUE KEY uk_release_template_id_type(id,release_type)');
PREPARE rcc_table_template_statement FROM @rcc_table_template_ddl;
EXECUTE rcc_table_template_statement;
DEALLOCATE PREPARE rcc_table_template_statement;
SET @rcc_table_template_ddl = NULL;

CREATE TABLE IF NOT EXISTS rcc_table_release_templates (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  table_policy_id BIGINT UNSIGNED NOT NULL,
  release_type VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  template_id BIGINT UNSIGNED NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  version BIGINT UNSIGNED NOT NULL DEFAULT 1,
  creator VARCHAR(64) NOT NULL,
  modifier VARCHAR(64) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (id),
  UNIQUE KEY uk_table_release_type (table_policy_id, release_type),
  KEY idx_table_release_template_type (template_id, release_type),
  CONSTRAINT fk_table_release_policy FOREIGN KEY (table_policy_id) REFERENCES rcc_table_policies(id) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT fk_table_release_template_type FOREIGN KEY (template_id, release_type) REFERENCES rcc_release_templates(id, release_type) ON DELETE RESTRICT ON UPDATE RESTRICT,
  CONSTRAINT chk_table_release_type CHECK (release_type IN ('STANDARD', 'EMERGENCY')),
  CONSTRAINT chk_table_release_enabled CHECK (enabled IN (0, 1)),
  CONSTRAINT chk_table_release_emergency_enabled CHECK (release_type <> 'EMERGENCY' OR enabled = 1),
  CONSTRAINT chk_table_release_version CHECK (version > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='Table Release Template Associations';

-- Fill missing associations only. A deliberately disabled or deleted standard
-- default is not re-created or re-enabled by upgrading this configuration.
INSERT INTO rcc_table_release_templates(table_policy_id,release_type,template_id,enabled,version,creator,modifier)
SELECT p.id,t.release_type,t.id,1,1,'system:migration','system:migration'
FROM rcc_table_policies p
JOIN rcc_release_templates t ON (t.code='default_standard_v1' AND t.release_type='STANDARD' OR t.code='default_emergency_v1' AND t.release_type='EMERGENCY') AND t.enabled=1
LEFT JOIN rcc_table_release_templates existing ON existing.table_policy_id=p.id AND existing.release_type=t.release_type
WHERE existing.id IS NULL;
