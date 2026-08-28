//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func TestPolicyCatalogMigrationsPromoteLegacySchemaWithoutDualWrite(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"testdata/001-legacy-policy-schema.sql",
		"../../../deploy/mysql/migrations/001-promote-mutation-capabilities.sql",
		"../../../deploy/mysql/migrations/002-rename-audit-timestamps.sql",
		"../../../deploy/mysql/migrations/003-create-query-policies.sql",
		"../../../deploy/mysql/migrations/004-create-mutation-policies.sql",
		"../../../deploy/mysql/migrations/005-expand-table-policy-code-references.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start expanded Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	if err := app.mysql.MigrateLegacyTablePolicies(ctx, "migration-test"); err != nil {
		t.Fatalf("preflight and backfill legacy Catalog: %v", err)
	}
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open migrated Catalog: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var allowAdd, allowModify, allowDelete bool
	var mutationConfig []byte
	if err := database.QueryRowContext(ctx, `
		SELECT allow_add, allow_modify, allow_delete, mutation_policy_config
		FROM rcc_table_policies
		WHERE table_name = 'legacy_policy'
	`).Scan(&allowAdd, &allowModify, &allowDelete, &mutationConfig); err != nil {
		t.Fatalf("read migrated Policy: %v", err)
	}
	if !allowAdd || allowModify || !allowDelete {
		t.Fatalf("legacy Mutation capabilities were not preserved: add=%t modify=%t delete=%t", allowAdd, allowModify, allowDelete)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(mutationConfig, &config); err != nil {
		t.Fatalf("decode migrated Mutation config: %v", err)
	}
	for _, removed := range []string{"allow_add", "allow_modify", "allow_delete"} {
		if _, found := config[removed]; found {
			t.Fatalf("migration left duplicate capability %s in Mutation JSON: %s", removed, mutationConfig)
		}
	}
	if _, found := config["auto_fill"]; !found {
		t.Fatalf("migration removed strategy-specific Auto Fill config: %s", mutationConfig)
	}

	var timestampColumnCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'rcc_table_policies'
		  AND column_name IN ('created_at', 'updated_at')
	`).Scan(&timestampColumnCount); err != nil {
		t.Fatalf("inspect migrated audit timestamp columns: %v", err)
	}
	if timestampColumnCount != 2 {
		t.Fatalf("expected created_at and updated_at after migration, found %d columns", timestampColumnCount)
	}

	var queryPolicyTableCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'rcc_query_policies'
	`).Scan(&queryPolicyTableCount); err != nil {
		t.Fatalf("inspect expanded Query Policy Catalog: %v", err)
	}
	if queryPolicyTableCount != 1 {
		t.Fatalf("expanded Catalog is missing rcc_query_policies")
	}

	var queryPolicyAuditColumnCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'rcc_query_policies'
		  AND column_name IN ('gmt_created', 'gmt_modified')
	`).Scan(&queryPolicyAuditColumnCount); err != nil {
		t.Fatalf("inspect Query Policy audit columns: %v", err)
	}
	if queryPolicyAuditColumnCount != 2 {
		t.Fatalf("expected gmt_created and gmt_modified on Query Policies, found %d columns", queryPolicyAuditColumnCount)
	}

	var queryPolicyScalarConstraintCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'rcc_query_policies'
		  AND constraint_type = 'CHECK'
		  AND constraint_name IN (
		    'chk_query_policy_order_direction',
		    'chk_query_policy_positive_page_sizes',
		    'chk_query_policy_page_size_order',
		    'chk_query_policy_max_page_size'
		  )
	`).Scan(&queryPolicyScalarConstraintCount); err != nil {
		t.Fatalf("inspect Query Policy scalar constraints: %v", err)
	}
	if queryPolicyScalarConstraintCount != 4 {
		t.Fatalf("expected four Query Policy scalar CHECK constraints, found %d", queryPolicyScalarConstraintCount)
	}

	var mutationPolicyTableCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'rcc_mutation_policies'
	`).Scan(&mutationPolicyTableCount); err != nil {
		t.Fatalf("inspect expanded Mutation Policy Catalog: %v", err)
	}
	if mutationPolicyTableCount != 1 {
		t.Fatalf("expanded Catalog is missing rcc_mutation_policies")
	}

	var mutationPolicyRelationalColumnCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'rcc_mutation_policies'
		  AND column_name IN (
		    'allow_add', 'allow_modify', 'allow_delete',
		    'create_operator_field', 'create_time_field',
		    'modify_operator_field', 'modify_time_field',
		    'gmt_created', 'gmt_modified'
		  )
	`).Scan(&mutationPolicyRelationalColumnCount); err != nil {
		t.Fatalf("inspect Mutation Policy relational columns: %v", err)
	}
	if mutationPolicyRelationalColumnCount != 9 {
		t.Fatalf("expected nine Mutation Policy relational/audit columns, found %d", mutationPolicyRelationalColumnCount)
	}

	var forbiddenMutationColumnCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE()
		  AND table_name = 'rcc_mutation_policies'
		  AND column_name IN ('supports_add', 'supports_modify', 'supports_delete', 'config', 'auto_fill')
	`).Scan(&forbiddenMutationColumnCount); err != nil {
		t.Fatalf("inspect forbidden Mutation Policy columns: %v", err)
	}
	if forbiddenMutationColumnCount != 0 {
		t.Fatalf("Mutation Policy Catalog contains forbidden supports/JSON columns")
	}

	var mutationCapabilityConstraintCount int
	if err := database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE()
		  AND table_name = 'rcc_mutation_policies'
		  AND constraint_type = 'CHECK'
		  AND constraint_name = 'chk_mutation_policy_capabilities'
	`).Scan(&mutationCapabilityConstraintCount); err != nil {
		t.Fatalf("inspect Mutation Policy capability constraint: %v", err)
	}
	if mutationCapabilityConstraintCount != 1 {
		t.Fatalf("expected Mutation Policy capability CHECK constraint")
	}

	var queryCode, mutationCode string
	if err := database.QueryRowContext(ctx, `SELECT query_policy_code, mutation_policy_code FROM rcc_table_policies WHERE table_name = 'legacy_policy'`).Scan(&queryCode, &mutationCode); err != nil {
		t.Fatalf("read backfilled Policy Codes: %v", err)
	}
	if !strings.HasPrefix(queryCode, "legacy_page_query_") || !strings.HasSuffix(queryCode, "_v1") ||
		!strings.HasPrefix(mutationCode, "legacy_mutation_") || !strings.HasSuffix(mutationCode, "_v1") {
		t.Fatalf("backfill did not create deterministic technology-neutral Codes: query=%q mutation=%q", queryCode, mutationCode)
	}
	var activeDefinitions int
	if err := database.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM rcc_query_policies WHERE code = ? AND status = 'ACTIVE') +
		       (SELECT COUNT(*) FROM rcc_mutation_policies WHERE code = ? AND status = 'ACTIVE')`, queryCode, mutationCode).Scan(&activeDefinitions); err != nil {
		t.Fatalf("read backfilled definitions: %v", err)
	}
	if activeDefinitions != 2 {
		t.Fatalf("expected both deterministic definitions Active, got %d", activeDefinitions)
	}
	if err := app.mysql.ContractLegacyTablePolicies(ctx); err != nil {
		t.Fatalf("contract validated legacy Catalog: %v", err)
	}

	var finalColumns string
	if err := database.QueryRowContext(ctx, `
		SELECT GROUP_CONCAT(column_name ORDER BY ordinal_position SEPARATOR ',')
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'rcc_table_policies'
	`).Scan(&finalColumns); err != nil {
		t.Fatalf("inspect final Table Policy columns: %v", err)
	}
	if finalColumns != "id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier,gmt_created,gmt_modified" {
		t.Fatalf("unexpected final Table Policy shape: %s", finalColumns)
	}

	freshCtx, freshDriverConfig := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	freshDatabase, err := sql.Open("mysql", freshDriverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fresh final Catalog: %v", err)
	}
	t.Cleanup(func() { _ = freshDatabase.Close() })
	upgradedSignature := policyCatalogSchemaSignature(t, ctx, database)
	freshSignature := policyCatalogSchemaSignature(t, freshCtx, freshDatabase)
	if upgradedSignature != freshSignature {
		t.Fatalf("fresh and upgraded Policy Catalog schemas differ\nupgraded:\n%s\nfresh:\n%s", upgradedSignature, freshSignature)
	}
	var foreignKeys int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE() AND table_name = 'rcc_table_policies'`).Scan(&foreignKeys); err != nil {
		t.Fatalf("inspect Table Policy foreign keys: %v", err)
	}
	if foreignKeys != 0 {
		t.Fatalf("Table Policy expand migration introduced forbidden foreign keys")
	}
}

func policyCatalogSchemaSignature(t *testing.T, ctx context.Context, database *sql.DB) string {
	t.Helper()
	queries := []string{
		`SELECT CONCAT_WS('|', table_name, LPAD(ordinal_position, 3, '0'), column_name, column_type, is_nullable,
		                  COALESCE(column_default, '<NULL>'), extra, COALESCE(collation_name, '<NULL>'))
		 FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name IN ('rcc_query_policies','rcc_mutation_policies','rcc_table_policies')
		 ORDER BY table_name, ordinal_position`,
		`SELECT CONCAT_WS('|', table_name, index_name, non_unique, LPAD(seq_in_index, 3, '0'), column_name, COALESCE(collation, '<NULL>'))
		 FROM information_schema.statistics
		 WHERE table_schema = DATABASE() AND table_name IN ('rcc_query_policies','rcc_mutation_policies','rcc_table_policies')
		 ORDER BY table_name, index_name, seq_in_index`,
		`SELECT CONCAT_WS('|', tc.table_name, tc.constraint_name, cc.check_clause)
		 FROM information_schema.table_constraints tc
		 JOIN information_schema.check_constraints cc
		   ON cc.constraint_schema = tc.constraint_schema AND cc.constraint_name = tc.constraint_name
		 WHERE tc.constraint_schema = DATABASE()
		   AND tc.table_name IN ('rcc_query_policies','rcc_mutation_policies','rcc_table_policies')
		   AND tc.constraint_type = 'CHECK'
		 ORDER BY tc.table_name, tc.constraint_name`,
	}
	parts := make([]string, 0)
	for _, query := range queries {
		rows, err := database.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("read Policy Catalog schema metadata: %v", err)
		}
		for rows.Next() {
			var row string
			if err := rows.Scan(&row); err != nil {
				_ = rows.Close()
				t.Fatalf("scan Policy Catalog schema metadata: %v", err)
			}
			parts = append(parts, row)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate Policy Catalog schema metadata: %v", err)
		}
		_ = rows.Close()
	}
	return strings.Join(parts, "\n")
}

func TestQueryPolicyHTTPLifecyclePersistsAndFailsClosed(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")

	types := policyIntegrationRequest(app, http.MethodGet, "/api/v1/query-policy-types", "")
	if types.Code != http.StatusOK || !strings.Contains(types.Body.String(), `"code":"page_query"`) {
		t.Fatalf("page_query Type is not exposed: HTTP %d %s", types.Code, types.Body.String())
	}

	invalidDraft := `{"code":"standard_page_query_v1","name":"Standard page query","description":"Draft may be completed before activation","type_code":"unknown_type","default_order_field":"id","default_order_direction":"DESC","default_page_size":20,"max_page_size":200}`
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", invalidDraft)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"status":"DRAFT"`) {
		t.Fatalf("create Draft: HTTP %d %s", created.Code, created.Body.String())
	}
	duplicate := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", invalidDraft)
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "query_policy_exists")
	unknownType := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/standard_page_query_v1/activate", "")
	assertIntegrationErrorCode(t, unknownType, http.StatusUnprocessableEntity, "unknown_policy_type")

	unsafeOrder := `{"code":"unsafe_order_v1","name":"Unsafe order","description":"","type_code":"page_query","default_order_field":"id;drop","default_order_direction":"DESC","default_page_size":20,"max_page_size":200}`
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", unsafeOrder), http.StatusUnprocessableEntity, "invalid_query_policy_rules")
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodGet, "/api/v1/query-policies/unsafe_order_v1", ""), http.StatusNotFound, "query_policy_not_found")

	invalidPersistentScalars := []struct {
		code      string
		field     string
		direction string
		pageSize  int
		maxSize   int
	}{
		{code: "invalid_direction_v1", field: "id", direction: "desc", pageSize: 20, maxSize: 200},
		{code: "invalid_default_size_v1", field: "id", direction: "DESC", pageSize: 0, maxSize: 200},
		{code: "inverted_sizes_v1", field: "id", direction: "DESC", pageSize: 30, maxSize: 20},
		{code: "unsafe_max_size_v1", field: "id", direction: "DESC", pageSize: 20, maxSize: 201},
	}
	for _, test := range invalidPersistentScalars {
		body := fmt.Sprintf(`{"code":%q,"name":"Invalid rules","description":"","type_code":"page_query","default_order_field":%q,"default_order_direction":%q,"default_page_size":%d,"max_page_size":%d}`, test.code, test.field, test.direction, test.pageSize, test.maxSize)
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", body), http.StatusUnprocessableEntity, "invalid_query_policy_rules")
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodGet, "/api/v1/query-policies/"+test.code, ""), http.StatusNotFound, "query_policy_not_found")
	}

	validDraft := `{"code":"standard_page_query_v1","name":"Standard page query","description":"Reusable defaults","type_code":"page_query","default_order_field":"id","default_order_direction":"DESC","default_page_size":20,"max_page_size":200}`
	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/query-policies/standard_page_query_v1", validDraft)
	if replaced.Code != http.StatusOK || !strings.Contains(replaced.Body.String(), `"type_code":"page_query"`) {
		t.Fatalf("replace Draft: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
	activated := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/standard_page_query_v1/activate", "")
	if activated.Code != http.StatusOK || !strings.Contains(activated.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("activate: HTTP %d %s", activated.Code, activated.Body.String())
	}

	immutable := strings.Replace(validDraft, `"default_page_size":20`, `"default_page_size":10`, 1)
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPut, "/api/v1/query-policies/standard_page_query_v1", immutable), http.StatusConflict, "invalid_policy_transition")
	metadata := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/query-policies/standard_page_query_v1/metadata", `{"name":"Standard pagination","description":"Display-only update"}`)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"name":"Standard pagination"`) || !strings.Contains(metadata.Body.String(), `"default_page_size":20`) {
		t.Fatalf("metadata update changed execution fields: HTTP %d %s", metadata.Code, metadata.Body.String())
	}

	deprecated := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/standard_page_query_v1/deprecate", "")
	if deprecated.Code != http.StatusOK || !strings.Contains(deprecated.Body.String(), `"status":"DEPRECATED"`) {
		t.Fatalf("deprecate: HTTP %d %s", deprecated.Code, deprecated.Body.String())
	}
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/standard_page_query_v1/activate", ""), http.StatusConflict, "invalid_policy_transition")
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodDelete, "/api/v1/query-policies/standard_page_query_v1", ""), http.StatusConflict, "invalid_policy_transition")

	draftToDelete := strings.Replace(validDraft, "standard_page_query_v1", "temporary_page_query_v2", 1)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", draftToDelete); response.Code != http.StatusCreated {
		t.Fatalf("create disposable Draft: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/query-policies/temporary_page_query_v2", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete Draft: HTTP %d %s", response.Code, response.Body.String())
	}
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodGet, "/api/v1/query-policies/temporary_page_query_v2", ""), http.StatusNotFound, "query_policy_not_found")

	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", strings.Replace(validDraft, "standard_page_query_v1", "mysql_page_query_v2", 1)), http.StatusBadRequest, "invalid_policy_code")
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", strings.Replace(validDraft, "standard_page_query_v1", "Unversioned", 1)), http.StatusBadRequest, "invalid_policy_code")

	protectedTable := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", tablePolicyCodePayload("rcc_query_policies", "unused_query_v1", "unused_mutation_v1"))
	assertIntegrationErrorCode(t, protectedTable, http.StatusForbidden, "protected_table")
	protectedQuery := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/rcc_query_policies/query", `{}`)
	assertIntegrationErrorCode(t, protectedQuery, http.StatusForbidden, "protected_table")

	listed := policyIntegrationRequest(app, http.MethodGet, "/api/v1/query-policies", "")
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), `"id":`) ||
		!strings.Contains(listed.Body.String(), `"creator":"integration-test"`) ||
		!strings.Contains(listed.Body.String(), `"gmt_created":`) ||
		!strings.Contains(listed.Body.String(), `"gmt_modified":`) ||
		strings.Contains(listed.Body.String(), `"created_at":`) ||
		strings.Contains(listed.Body.String(), `"updated_at":`) {
		t.Fatalf("unexpected Query Policy list: HTTP %d %s", listed.Code, listed.Body.String())
	}
}

func TestTablePolicyCodeAssignmentsValidateActiveDefinitionsAndReplaceAtomically(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/003-policy-fixture.sql",
	)

	createAndActivateQueryDefinition(t, app, "assignable_page_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "assignable_mutation_v1", nil)

	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", `{"table_name":"policy_alpha","query_policy_code":"assignable_page_query_v1","mutation_policy_code":"assignable_mutation_v1"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create Code assignment: HTTP %d %s", created.Code, created.Body.String())
	}
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(created.Body.Bytes(), &contract); err != nil {
		t.Fatalf("decode Table Policy contract: %v", err)
	}
	for _, required := range []string{"table_name", "query_policy_code", "mutation_policy_code", "enabled", "creator", "modifier", "gmt_created", "gmt_modified"} {
		if _, found := contract[required]; !found {
			t.Fatalf("Table Policy response misses %s: %s", required, created.Body.String())
		}
	}
	for _, forbidden := range []string{"query_policy", "query_policy_config", "mutation_policy", "mutation_policy_config", "allow_add", "allow_modify", "allow_delete", "created_at", "updated_at"} {
		if _, found := contract[forbidden]; found {
			t.Fatalf("Table Policy response exposes %s: %s", forbidden, created.Body.String())
		}
	}
	if string(contract["enabled"]) != "false" {
		t.Fatalf("new Table Policy must start disabled: %s", created.Body.String())
	}
	legacyJSON := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", legacyTablePolicyPayload("policy_beta", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`))
	assertIntegrationErrorCode(t, legacyJSON, http.StatusBadRequest, "invalid_request")

	enabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/policy_alpha/enable", "")
	if enabled.Code != http.StatusOK || !strings.Contains(enabled.Body.String(), `"enabled":true`) {
		t.Fatalf("enable assignment: HTTP %d %s", enabled.Code, enabled.Body.String())
	}

	createAndActivateQueryDefinition(t, app, "next_page_query_v2", "id")
	createAndActivateQueryDefinition(t, app, "missing_order_query_v1", "missing_column")
	invalid := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/policy_alpha", `{"table_name":"policy_alpha","query_policy_code":"missing_order_query_v1","mutation_policy_code":"assignable_mutation_v1"}`)
	assertIntegrationErrorCode(t, invalid, http.StatusUnprocessableEntity, "incompatible_policy_definition")
	unchanged := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/policy_alpha", "")
	if unchanged.Code != http.StatusOK || !strings.Contains(unchanged.Body.String(), `"query_policy_code":"assignable_page_query_v1"`) || !strings.Contains(unchanged.Body.String(), `"enabled":true`) {
		t.Fatalf("invalid replacement changed assignment: HTTP %d %s", unchanged.Code, unchanged.Body.String())
	}

	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/policy_alpha", `{"table_name":"policy_alpha","query_policy_code":"next_page_query_v2","mutation_policy_code":"assignable_mutation_v1"}`)
	if replaced.Code != http.StatusOK || !strings.Contains(replaced.Body.String(), `"query_policy_code":"next_page_query_v2"`) || !strings.Contains(replaced.Body.String(), `"enabled":true`) {
		t.Fatalf("valid replacement was not atomic/preserving enabled: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
	visible := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/policy_alpha", "")
	if !strings.Contains(visible.Body.String(), `"query_policy_code":"next_page_query_v2"`) {
		t.Fatalf("replacement was not immediately visible: HTTP %d %s", visible.Code, visible.Body.String())
	}

	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/next_page_query_v2/deprecate", ""); response.Code != http.StatusOK {
		t.Fatalf("deprecate assigned Query Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	readable := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/policy_alpha", "")
	if readable.Code != http.StatusOK {
		t.Fatalf("existing Deprecated reference must remain readable: HTTP %d %s", readable.Code, readable.Body.String())
	}
	rejectedDeprecated := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", `{"table_name":"policy_beta","query_policy_code":"next_page_query_v2","mutation_policy_code":"assignable_mutation_v1"}`)
	assertIntegrationErrorCode(t, rejectedDeprecated, http.StatusUnprocessableEntity, "query_policy_not_assignable")

	createAndActivateMutationDefinition(t, app, "missing_audit_mutation_v1", stringPointer("missing_column"))
	incompatibleMutation := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/policy_alpha", `{"table_name":"policy_alpha","query_policy_code":"assignable_page_query_v1","mutation_policy_code":"missing_audit_mutation_v1"}`)
	assertIntegrationErrorCode(t, incompatibleMutation, http.StatusUnprocessableEntity, "incompatible_policy_definition")
}

func TestLegacyPolicyPreflightRejectsUnsupportedAutoFillWithoutPartialRewrite(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"testdata/001-legacy-policy-schema.sql",
		"../../../deploy/mysql/migrations/001-promote-mutation-capabilities.sql",
		"../../../deploy/mysql/migrations/002-rename-audit-timestamps.sql",
		"../../../deploy/mysql/migrations/003-create-query-policies.sql",
		"../../../deploy/mysql/migrations/004-create-mutation-policies.sql",
		"../../../deploy/mysql/migrations/005-expand-table-policy-code-references.sql",
		"testdata/007-unsupported-legacy-policy-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start expanded Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	err = app.mysql.MigrateLegacyTablePolicies(ctx, "migration-test")
	if err == nil || !strings.Contains(err.Error(), "legacy_unsupported_table") || !strings.Contains(strings.ToLower(err.Error()), "literal") {
		t.Fatalf("expected actionable unsupported Auto Fill diagnostic, got %v", err)
	}
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open Catalog after rejected preflight: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	var rewritten, definitions int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM rcc_table_policies WHERE query_policy_code IS NOT NULL OR mutation_policy_code IS NOT NULL`).Scan(&rewritten); err != nil {
		t.Fatalf("count partially rewritten Table Policies: %v", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rcc_query_policies) + (SELECT COUNT(*) FROM rcc_mutation_policies)`).Scan(&definitions); err != nil {
		t.Fatalf("count partially created definitions: %v", err)
	}
	if rewritten != 0 || definitions != 0 {
		t.Fatalf("failed preflight partially rewrote Catalog: assignments=%d definitions=%d", rewritten, definitions)
	}
	contractErr := app.mysql.ContractLegacyTablePolicies(ctx)
	if contractErr == nil || !strings.Contains(contractErr.Error(), "legacy_unsupported_table") {
		t.Fatalf("contraction must repeat the unsupported-config gate, got %v", contractErr)
	}
	var legacyColumns int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = 'rcc_table_policies'
		  AND column_name IN ('query_policy','query_policy_config','mutation_policy','mutation_policy_config','allow_add','allow_modify','allow_delete')`).Scan(&legacyColumns); err != nil {
		t.Fatalf("inspect Catalog after blocked contraction: %v", err)
	}
	if legacyColumns != 7 {
		t.Fatalf("blocked contraction performed destructive DDL: legacy columns=%d", legacyColumns)
	}
}

func createAndActivateQueryDefinition(t *testing.T, app *adminApplication, code, orderField string) {
	t.Helper()
	body := fmt.Sprintf(`{"code":%q,"name":"Assignable query","description":"integration","type_code":"page_query","default_order_field":%q,"default_order_direction":"DESC","default_page_size":20,"max_page_size":200}`, code, orderField)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies", body); response.Code != http.StatusCreated {
		t.Fatalf("create Query Policy %s: HTTP %d %s", code, response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/query-policies/"+code+"/activate", ""); response.Code != http.StatusOK {
		t.Fatalf("activate Query Policy %s: HTTP %d %s", code, response.Code, response.Body.String())
	}
}

func createAndActivateMutationDefinition(t *testing.T, app *adminApplication, code string, createOperator *string) {
	t.Helper()
	operatorJSON := "null"
	if createOperator != nil {
		encoded, _ := json.Marshal(*createOperator)
		operatorJSON = string(encoded)
	}
	body := fmt.Sprintf(`{"code":%q,"name":"Assignable mutation","description":"integration","type_code":"single_table_mutation","allow_add":true,"allow_modify":true,"allow_delete":false,"create_operator_field":%s,"create_time_field":null,"modify_operator_field":null,"modify_time_field":null}`, code, operatorJSON)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", body); response.Code != http.StatusCreated {
		t.Fatalf("create Mutation Policy %s: HTTP %d %s", code, response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies/"+code+"/activate", ""); response.Code != http.StatusOK {
		t.Fatalf("activate Mutation Policy %s: HTTP %d %s", code, response.Code, response.Body.String())
	}
}

func stringPointer(value string) *string { return &value }

func TestMutationPolicyHTTPLifecyclePersistsRelationalRulesAndFailsClosed(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")

	types := policyIntegrationRequest(app, http.MethodGet, "/api/v1/mutation-policy-types", "")
	if types.Code != http.StatusOK || !strings.Contains(types.Body.String(), `"code":"single_table_mutation"`) ||
		!strings.Contains(types.Body.String(), `"operations":["ADD","MODIFY","DELETE"]`) {
		t.Fatalf("single_table_mutation Type contract is not exposed: HTTP %d %s", types.Code, types.Body.String())
	}

	unknownType := mutationPolicyPayload("standard_mutation_v1", "unknown_type", true, true, false, `"creator"`, `"created_at"`, `"modifier"`, `"updated_at"`)
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", unknownType)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"status":"DRAFT"`) {
		t.Fatalf("create incomplete Draft: HTTP %d %s", created.Code, created.Body.String())
	}
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies/standard_mutation_v1/activate", ""), http.StatusUnprocessableEntity, "unknown_policy_type")
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", unknownType), http.StatusConflict, "mutation_policy_exists")

	invalidRules := []struct {
		code string
		body string
	}{
		{code: "unsafe_auto_fill_v1", body: mutationPolicyPayload("unsafe_auto_fill_v1", "single_table_mutation", true, true, false, `"creator;drop"`, `"created_at"`, `"modifier"`, `"updated_at"`)},
		{code: "duplicate_auto_fill_v1", body: mutationPolicyPayload("duplicate_auto_fill_v1", "single_table_mutation", true, true, false, `"creator"`, `"created_at"`, `"creator"`, `"updated_at"`)},
		{code: "create_without_add_v1", body: mutationPolicyPayload("create_without_add_v1", "single_table_mutation", false, true, false, `"creator"`, `null`, `"modifier"`, `"updated_at"`)},
		{code: "modify_without_operation_v1", body: mutationPolicyPayload("modify_without_operation_v1", "single_table_mutation", false, false, false, `null`, `null`, `"modifier"`, `"updated_at"`)},
	}
	for _, test := range invalidRules {
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", test.body), http.StatusUnprocessableEntity, "invalid_mutation_policy_rules")
		assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodGet, "/api/v1/mutation-policies/"+test.code, ""), http.StatusNotFound, "mutation_policy_not_found")
	}

	validDraft := mutationPolicyPayload("standard_mutation_v1", "single_table_mutation", true, true, false, `"creator"`, `"created_at"`, `"modifier"`, `"updated_at"`)
	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/mutation-policies/standard_mutation_v1", validDraft)
	if replaced.Code != http.StatusOK || !strings.Contains(replaced.Body.String(), `"allow_add":true`) ||
		!strings.Contains(replaced.Body.String(), `"create_operator_field":"creator"`) || strings.Contains(replaced.Body.String(), "config") {
		t.Fatalf("replace Draft did not persist typed relational rules: HTTP %d %s", replaced.Code, replaced.Body.String())
	}

	activated := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies/standard_mutation_v1/activate", "")
	if activated.Code != http.StatusOK || !strings.Contains(activated.Body.String(), `"status":"ACTIVE"`) {
		t.Fatalf("activate: HTTP %d %s", activated.Code, activated.Body.String())
	}
	immutable := strings.Replace(validDraft, `"allow_delete":false`, `"allow_delete":true`, 1)
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPut, "/api/v1/mutation-policies/standard_mutation_v1", immutable), http.StatusConflict, "invalid_policy_transition")

	metadata := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/mutation-policies/standard_mutation_v1/metadata", `{"name":"Standard mutation","description":"Display-only update"}`)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), `"name":"Standard mutation"`) || !strings.Contains(metadata.Body.String(), `"allow_delete":false`) {
		t.Fatalf("metadata update changed execution fields: HTTP %d %s", metadata.Code, metadata.Body.String())
	}

	got := policyIntegrationRequest(app, http.MethodGet, "/api/v1/mutation-policies/standard_mutation_v1", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"modify_time_field":"updated_at"`) ||
		!strings.Contains(got.Body.String(), `"gmt_created":`) || !strings.Contains(got.Body.String(), `"gmt_modified":`) ||
		strings.Contains(got.Body.String(), `"id":`) || strings.Contains(got.Body.String(), `"supports_`) ||
		strings.Contains(got.Body.String(), `"created_at":`) || strings.Contains(got.Body.String(), `"updated_at":`) {
		t.Fatalf("unexpected persisted Mutation Policy response: HTTP %d %s", got.Code, got.Body.String())
	}

	deprecated := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies/standard_mutation_v1/deprecate", "")
	if deprecated.Code != http.StatusOK || !strings.Contains(deprecated.Body.String(), `"status":"DEPRECATED"`) {
		t.Fatalf("deprecate: HTTP %d %s", deprecated.Code, deprecated.Body.String())
	}
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies/standard_mutation_v1/activate", ""), http.StatusConflict, "invalid_policy_transition")
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodDelete, "/api/v1/mutation-policies/standard_mutation_v1", ""), http.StatusConflict, "invalid_policy_transition")

	draftToDelete := mutationPolicyPayload("temporary_mutation_v2", "single_table_mutation", false, false, false, `null`, `null`, `null`, `null`)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", draftToDelete); response.Code != http.StatusCreated {
		t.Fatalf("create disposable Draft: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/mutation-policies/temporary_mutation_v2", ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete Draft: HTTP %d %s", response.Code, response.Body.String())
	}
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodGet, "/api/v1/mutation-policies/temporary_mutation_v2", ""), http.StatusNotFound, "mutation_policy_not_found")

	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", strings.Replace(validDraft, "standard_mutation_v1", "mysql_mutation_v2", 1)), http.StatusBadRequest, "invalid_policy_code")
	withJSON := strings.TrimSuffix(validDraft, "}") + `,"config":{}}`
	assertIntegrationErrorCode(t, policyIntegrationRequest(app, http.MethodPost, "/api/v1/mutation-policies", withJSON), http.StatusBadRequest, "invalid_request")

	protectedTable := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", tablePolicyCodePayload("rcc_mutation_policies", "unused_query_v1", "unused_mutation_v1"))
	assertIntegrationErrorCode(t, protectedTable, http.StatusForbidden, "protected_table")
	protectedMutation := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/rcc_mutation_policies/rows", `{"content":{}}`)
	assertIntegrationErrorCode(t, protectedMutation, http.StatusForbidden, "protected_table")
}

func mutationPolicyPayload(code, typeCode string, allowAdd, allowModify, allowDelete bool, createOperator, createTime, modifyOperator, modifyTime string) string {
	return fmt.Sprintf(`{"code":%q,"name":"Reusable mutation","description":"Typed relational rules","type_code":%q,"allow_add":%t,"allow_modify":%t,"allow_delete":%t,"create_operator_field":%s,"create_time_field":%s,"modify_operator_field":%s,"modify_time_field":%s}`,
		code, typeCode, allowAdd, allowModify, allowDelete, createOperator, createTime, modifyOperator, modifyTime)
}

func TestTablePolicyCreationPersistsDisabledCodeReferencesForHTTPInspection(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/003-policy-fixture.sql",
	)
	createAndActivateQueryDefinition(t, app, "inspect_page_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "inspect_mutation_v1", nil)

	requestBody := tablePolicyCodePayload("policy_alpha", "inspect_page_query_v1", "inspect_mutation_v1")
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", requestBody)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201, got %d: %s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"query_policy_code":"inspect_page_query_v1"`) ||
		!strings.Contains(created.Body.String(), `"mutation_policy_code":"inspect_mutation_v1"`) ||
		!strings.Contains(created.Body.String(), `"enabled":false`) {
		t.Fatalf("unexpected create response: %s", created.Body.String())
	}

	got := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/policy_alpha", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"query_policy_code":"inspect_page_query_v1"`) {
		t.Fatalf("unexpected get response: HTTP %d %s", got.Code, got.Body.String())
	}

	listed := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", listed.Code, listed.Body.String())
	}
	var listResponse struct {
		Policies []map[string]json.RawMessage `json:"policies"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResponse.Policies) != 1 {
		t.Fatalf("expected exactly one Policy, got %s", listed.Body.String())
	}
	for _, forbidden := range []string{"id", "query_policy", "query_policy_config", "mutation_policy", "mutation_policy_config", "allow_add", "allow_modify", "allow_delete", "created_at", "updated_at"} {
		if _, exposed := listResponse.Policies[0][forbidden]; exposed {
			t.Fatalf("list exposed internal/compatibility field %s: %s", forbidden, listed.Body.String())
		}
	}
}

func TestTablePolicyCreationRejectsPrincipalFailuresWithoutPartialPersistence(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/003-policy-fixture.sql",
	)
	createAndActivateQueryDefinition(t, app, "failure_page_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "failure_mutation_v1", nil)
	valid := func(tableName string) string {
		return tablePolicyCodePayload(tableName, "failure_page_query_v1", "failure_mutation_v1")
	}

	tests := []struct {
		name       string
		tableName  string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "missing", tableName: "missing_table", body: valid("missing_table"), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "view", tableName: "policy_view", body: valid("policy_view"), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "protected", tableName: "rcc_policy_shadow", body: valid("rcc_policy_shadow"), wantStatus: http.StatusForbidden, wantCode: "protected_table"},
		{name: "incompatible", tableName: "policy_incompatible", body: valid("policy_incompatible"), wantStatus: http.StatusUnprocessableEntity, wantCode: "incompatible_table"},
		{name: "unknown query definition", tableName: "policy_alpha", body: tablePolicyCodePayload("policy_alpha", "unknown_query_v1", "failure_mutation_v1"), wantStatus: http.StatusUnprocessableEntity, wantCode: "query_policy_not_assignable"},
		{name: "unknown mutation definition", tableName: "policy_alpha", body: tablePolicyCodePayload("policy_alpha", "failure_page_query_v1", "unknown_mutation_v1"), wantStatus: http.StatusUnprocessableEntity, wantCode: "mutation_policy_not_assignable"},
		{name: "legacy JSON field", tableName: "policy_alpha", body: legacyTablePolicyPayload("policy_alpha", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", test.body)
			assertIntegrationErrorCode(t, response, test.wantStatus, test.wantCode)
			got := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/"+test.tableName, "")
			if got.Code != http.StatusNotFound && got.Code != http.StatusForbidden {
				t.Fatalf("failed creation partially persisted a Policy: HTTP %d %s", got.Code, got.Body.String())
			}
		})
	}

	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", valid("policy_beta"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy for duplicate check: HTTP %d %s", created.Code, created.Body.String())
	}
	duplicate := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", valid("policy_beta"))
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "table_policy_exists")
}

func tablePolicyCodePayload(tableName, queryCode, mutationCode string) string {
	return fmt.Sprintf(`{"table_name":%q,"query_policy_code":%q,"mutation_policy_code":%q}`, tableName, queryCode, mutationCode)
}

func legacyTablePolicyPayload(tableName, queryPolicy, queryConfig, mutationPolicy, mutationConfig string) string {
	return fmt.Sprintf(`{"table_name":%q,"query_policy":%q,"query_policy_config":%s,"mutation_policy":%q,"mutation_policy_config":%s,"allow_add":false,"allow_modify":false,"allow_delete":false}`,
		tableName, queryPolicy, queryConfig, mutationPolicy, mutationConfig)
}

type queryPolicyFixture struct {
	DefaultOrderField     string
	DefaultOrderDirection string
	DefaultPageSize       int
	MaxPageSize           int
}

type mutationPolicyFixture struct {
	AllowAdd            bool
	AllowModify         bool
	AllowDelete         bool
	CreateOperatorField *string
	CreateTimeField     *string
	ModifyOperatorField *string
	ModifyTimeField     *string
}

func createPolicyAssignment(t *testing.T, app *adminApplication, tableName string, query queryPolicyFixture, mutation mutationPolicyFixture) {
	t.Helper()
	queryCode, mutationCode := createPolicyDefinitions(t, app, tableName, query, mutation, 1)
	if err := app.mysql.Create(t.Context(), domain.TablePolicy{TableName: tableName, QueryPolicyCode: queryCode, MutationPolicyCode: mutationCode}, "integration-test"); err != nil {
		t.Fatalf("seed relational Table Policy assignment: %v", err)
	}
}

func replacePolicyAssignment(t *testing.T, app *adminApplication, tableName string, query queryPolicyFixture, mutation mutationPolicyFixture) {
	t.Helper()
	queryCode, mutationCode := createPolicyDefinitions(t, app, tableName, query, mutation, 2)
	replaced := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/"+tableName, tablePolicyCodePayload(tableName, queryCode, mutationCode))
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace relational Policy assignment: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
}

func createPolicyDefinitions(t *testing.T, app *adminApplication, tableName string, query queryPolicyFixture, mutation mutationPolicyFixture, version int) (string, string) {
	t.Helper()
	queryCode := fmt.Sprintf("fixture_%s_query_v%d", tableName, version)
	mutationCode := fmt.Sprintf("fixture_%s_mutation_v%d", tableName, version)

	queryPolicy := relationalQueryPolicyFixture(queryCode, query)
	if _, err := app.mysql.CreateQueryPolicy(t.Context(), queryPolicy, "integration-test"); err != nil {
		t.Fatalf("seed relational Query Policy %s: %v", queryCode, err)
	}
	if _, err := app.mysql.SetQueryPolicyStatus(t.Context(), queryCode, domain.PolicyStatusDraft, domain.PolicyStatusActive, "integration-test"); err != nil {
		t.Fatalf("activate relational Query Policy %s: %v", queryCode, err)
	}

	mutationPolicy := domain.MutationPolicy{
		Code: mutationCode, Name: "Integration Mutation Policy", Description: "Relational fixture",
		TypeCode: "single_table_mutation",
		AllowAdd: mutation.AllowAdd, AllowModify: mutation.AllowModify, AllowDelete: mutation.AllowDelete,
		CreateOperatorField: mutation.CreateOperatorField, CreateTimeField: mutation.CreateTimeField,
		ModifyOperatorField: mutation.ModifyOperatorField, ModifyTimeField: mutation.ModifyTimeField,
	}
	if _, err := app.mysql.CreateMutationPolicy(t.Context(), mutationPolicy, "integration-test"); err != nil {
		t.Fatalf("seed relational Mutation Policy %s: %v", mutationCode, err)
	}
	if _, err := app.mysql.SetMutationPolicyStatus(t.Context(), mutationCode, domain.PolicyStatusDraft, domain.PolicyStatusActive, "integration-test"); err != nil {
		t.Fatalf("activate relational Mutation Policy %s: %v", mutationCode, err)
	}
	return queryCode, mutationCode
}

func relationalQueryPolicyFixture(code string, fixture queryPolicyFixture) domain.QueryPolicy {
	if fixture.DefaultOrderField == "" {
		fixture.DefaultOrderField = "id"
	}
	if fixture.DefaultOrderDirection == "" {
		fixture.DefaultOrderDirection = "DESC"
	}
	if fixture.DefaultPageSize == 0 {
		fixture.DefaultPageSize = 20
	}
	if fixture.MaxPageSize == 0 {
		fixture.MaxPageSize = 200
	}
	return domain.QueryPolicy{
		Code: code, Name: "Integration Query Policy", Description: "Relational fixture", TypeCode: "page_query",
		DefaultOrderField: fixture.DefaultOrderField, DefaultOrderDirection: fixture.DefaultOrderDirection,
		DefaultPageSize: fixture.DefaultPageSize, MaxPageSize: fixture.MaxPageSize,
	}
}

func setPolicyAssignmentEnabled(t *testing.T, app *adminApplication, tableName string, enabled bool) {
	t.Helper()
	if _, err := app.mysql.SetEnabled(t.Context(), tableName, enabled, "integration-test"); err != nil {
		t.Fatalf("set relational Policy assignment enabled=%t: %v", enabled, err)
	}
}

func enablePolicyAssignment(t *testing.T, app *adminApplication, tableName string, query queryPolicyFixture, mutation mutationPolicyFixture) {
	t.Helper()
	createPolicyAssignment(t, app, tableName, query, mutation)
	setPolicyAssignmentEnabled(t, app, tableName, true)
}

func assertIntegrationErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("expected HTTP %d, got %d: %s", status, response.Code, response.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("expected error code %q, got %q: %s", code, body.Error.Code, response.Body.String())
	}
}

func policyIntegrationRequest(app *adminApplication, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	app.Handler().ServeHTTP(recorder, request)
	return recorder
}
