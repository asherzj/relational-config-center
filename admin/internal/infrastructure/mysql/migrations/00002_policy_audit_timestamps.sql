-- +goose NO TRANSACTION
-- +goose Up
-- Preserve existing Policy audit values. Each table rename is atomic; recovery
-- accepts only the verified previous or target table definition before retrying.
SET @rcc_policy_audit_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_query_policies' AND column_name='gmt_created'), 'ALTER TABLE rcc_query_policies RENAME COLUMN gmt_created TO created_at, RENAME COLUMN gmt_modified TO updated_at', 'DO 0');
PREPARE rcc_policy_audit_statement FROM @rcc_policy_audit_ddl;
EXECUTE rcc_policy_audit_statement;
DEALLOCATE PREPARE rcc_policy_audit_statement;
SET @rcc_policy_audit_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_mutation_policies' AND column_name='gmt_created'), 'ALTER TABLE rcc_mutation_policies RENAME COLUMN gmt_created TO created_at, RENAME COLUMN gmt_modified TO updated_at', 'DO 0');
PREPARE rcc_policy_audit_statement FROM @rcc_policy_audit_ddl;
EXECUTE rcc_policy_audit_statement;
DEALLOCATE PREPARE rcc_policy_audit_statement;
SET @rcc_policy_audit_ddl = IF(EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_table_policies' AND column_name='gmt_created'), 'ALTER TABLE rcc_table_policies RENAME COLUMN gmt_created TO created_at, RENAME COLUMN gmt_modified TO updated_at', 'DO 0');
PREPARE rcc_policy_audit_statement FROM @rcc_policy_audit_ddl;
EXECUTE rcc_policy_audit_statement;
DEALLOCATE PREPARE rcc_policy_audit_statement;
SET @rcc_policy_audit_ddl = NULL;
