-- Expand stage only. Columns stay nullable so this DDL may be deployed before
-- the legacy Catalog preflight/backfill command and before every Admin process
-- has switched its management contract. There are deliberately no foreign
-- keys; lifecycle/reference integrity is enforced by Admin application logic.

ALTER TABLE `rcc_table_policies`
  ADD COLUMN `query_policy_code` varchar(100) NULL AFTER `table_name`,
  ADD COLUMN `mutation_policy_code` varchar(100) NULL AFTER `query_policy_code`,
  ADD KEY `idx_table_policy_query_code` (`query_policy_code`),
  ADD KEY `idx_table_policy_mutation_code` (`mutation_policy_code`);

-- Next, run `go run ./cmd/policy-migrate` from admin/. It locks and preflights
-- the complete legacy set before writing Policy definitions and Code
-- references, then runs the separately gated contract stage. DBA migration
-- runners may instead backfill and execute 006 explicitly; 006 has its own
-- fail-closed assertions before its single destructive ALTER.
