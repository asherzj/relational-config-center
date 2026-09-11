-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE IF NOT EXISTS rcc_approval_roles (
 id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 name VARCHAR(200) NOT NULL,
 description VARCHAR(2000) NOT NULL DEFAULT '',
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 version BIGINT UNSIGNED NOT NULL DEFAULT 1,
 creator CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 modifier CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL,
 CONSTRAINT chk_approval_role_enabled CHECK (enabled IN (0,1)),
 CONSTRAINT chk_approval_role_version CHECK (version > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS rcc_approval_role_members (
 role_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 account_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 PRIMARY KEY (role_id,account_id),
 CONSTRAINT fk_approval_member_role FOREIGN KEY (role_id) REFERENCES rcc_approval_roles(id) ON DELETE CASCADE,
 CONSTRAINT fk_approval_member_account FOREIGN KEY (account_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS rcc_approval_role_requests (
 actor_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 result JSON NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (actor_id,request_key),
 CONSTRAINT fk_approval_request_actor FOREIGN KEY (actor_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Permanent reference facts survive unassignment; names at reference time remain
-- attributable independently of later renames. Never cascade them away.
CREATE TABLE IF NOT EXISTS rcc_approval_role_references (
 role_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reference_kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 reference_key VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
 role_name VARCHAR(200) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (role_id,reference_kind,reference_key),
 CONSTRAINT fk_approval_reference_role FOREIGN KEY (role_id) REFERENCES rcc_approval_roles(id),
 CONSTRAINT chk_approval_reference_kind CHECK (reference_kind IN ('table','snapshot'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
