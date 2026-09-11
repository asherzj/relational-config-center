-- +goose NO TRANSACTION
-- +goose Up
-- Run in the documented stopped-writer maintenance window. Bit 4 is reserved
-- permanently for historical interpretation; all other grant numbers stay fixed.
-- A single InnoDB statement is atomic, and its predicate makes recovery repeatable.
UPDATE rcc_accounts
SET roles = CASE WHEN roles = 4 THEN 1 ELSE roles & 27 END,
    role_version = role_version + 1
WHERE roles BETWEEN 1 AND 31 AND roles & 4 <> 0;
