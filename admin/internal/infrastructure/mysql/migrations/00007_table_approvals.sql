-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE IF NOT EXISTS rcc_table_approval_assignments (
 table_name VARBINARY(256) NOT NULL PRIMARY KEY,
 version BIGINT UNSIGNED NOT NULL,
 role_ids JSON NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS rcc_table_approval_requests (
 actor_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 request_key VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 digest BINARY(32) NOT NULL,
 result JSON NOT NULL,
 PRIMARY KEY (actor_id,request_key),
 CONSTRAINT fk_table_approval_request_actor FOREIGN KEY (actor_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
