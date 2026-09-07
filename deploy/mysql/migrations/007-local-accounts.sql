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
