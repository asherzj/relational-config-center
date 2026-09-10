-- +goose NO TRANSACTION
-- +goose Up
CREATE TABLE IF NOT EXISTS rcc_approval_notifications (
 account_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 order_id VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 sequence BIGINT UNSIGNED NOT NULL DEFAULT 0,
 pending BOOLEAN NOT NULL DEFAULT FALSE,
 pending_sequence BIGINT UNSIGNED NOT NULL DEFAULT 0,
 result_sequence BIGINT UNSIGNED NOT NULL DEFAULT 0,
 read_sequence BIGINT UNSIGNED NOT NULL DEFAULT 0,
 PRIMARY KEY (account_id,order_id),
 KEY ix_approval_notification_order (order_id),
 CONSTRAINT fk_approval_notification_account FOREIGN KEY (account_id) REFERENCES rcc_accounts(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
