-- Stop all old Admin writers before applying this restartable migration.
-- Existing and future accounts default to VIEWER. Reruns preserve explicit grants.
SET @rcc_role_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_accounts' AND COLUMN_NAME='roles'), 'DO 0', 'ALTER TABLE rcc_accounts ADD COLUMN roles TINYINT UNSIGNED NOT NULL DEFAULT 1 AFTER session_version');
PREPARE rcc_role_stmt FROM @rcc_role_ddl;
EXECUTE rcc_role_stmt;
DEALLOCATE PREPARE rcc_role_stmt;
SET @rcc_role_ddl = IF(EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='rcc_accounts' AND COLUMN_NAME='role_version'), 'DO 0', 'ALTER TABLE rcc_accounts ADD COLUMN role_version BIGINT UNSIGNED NOT NULL DEFAULT 1 AFTER roles');
PREPARE rcc_role_stmt FROM @rcc_role_ddl;
EXECUTE rcc_role_stmt;
DEALLOCATE PREPARE rcc_role_stmt;
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
