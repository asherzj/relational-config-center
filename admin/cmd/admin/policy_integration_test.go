//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMutationCapabilityMigrationPromotesLegacyJSONWithoutDualWrite(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"testdata/001-legacy-policy-schema.sql",
		"../../../deploy/mysql/migrations/001-promote-mutation-capabilities.sql",
	)
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
}

func TestTablePolicyCreationPersistsDisabledObjectConfigsForHTTPInspection(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/003-policy-fixture.sql",
	)

	const requestBody = `{
		"table_name":"policy_alpha",
		"query_policy":"mysql_page_query_v1",
		"query_policy_config":{},
		"mutation_policy":"mysql_single_table_mutation_v1",
		"mutation_policy_config":{},
		"allow_add":false,
		"allow_modify":false,
		"allow_delete":false
	}`
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", requestBody)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201, got %d: %s", created.Code, created.Body.String())
	}
	const expected = `{"table_name":"policy_alpha","query_policy":"mysql_page_query_v1","query_policy_config":{},"mutation_policy":"mysql_single_table_mutation_v1","mutation_policy_config":{},"allow_add":false,"allow_modify":false,"allow_delete":false,"enabled":false}`
	if strings.TrimSpace(created.Body.String()) != expected {
		t.Fatalf("unexpected create response: %s", created.Body.String())
	}

	got := policyIntegrationRequest(app, http.MethodGet, "/api/v1/table-policies/policy_alpha", "")
	if got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != expected {
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
	if _, exposed := listResponse.Policies[0]["id"]; exposed {
		t.Fatalf("list exposed internal Catalog ID: %s", listed.Body.String())
	}
}

func TestTablePolicyCreationRejectsPrincipalFailuresWithoutPartialPersistence(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/003-policy-fixture.sql",
	)

	tests := []struct {
		name       string
		tableName  string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "missing", tableName: "missing_table", body: integrationPolicyPayload("missing_table", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "view", tableName: "policy_view", body: integrationPolicyPayload("policy_view", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "protected", tableName: "rcc_policy_shadow", body: integrationPolicyPayload("rcc_policy_shadow", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusForbidden, wantCode: "protected_table"},
		{name: "incompatible", tableName: "policy_incompatible", body: integrationPolicyPayload("policy_incompatible", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusUnprocessableEntity, wantCode: "incompatible_table"},
		{name: "unknown strategy", tableName: "policy_alpha", body: integrationPolicyPayload("policy_alpha", "unknown_query", `{}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusUnprocessableEntity, wantCode: "unknown_policy_strategy"},
		{name: "non-object config", tableName: "policy_alpha", body: integrationPolicyPayload("policy_alpha", "mysql_page_query_v1", `[]`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
		{name: "unknown config field", tableName: "policy_alpha", body: integrationPolicyPayload("policy_alpha", "mysql_page_query_v1", `{"sql":"SELECT 1"}`, "mysql_single_table_mutation_v1", `{}`), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
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

	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", integrationPolicyPayload("policy_beta", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{}`))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy for duplicate check: HTTP %d %s", created.Code, created.Body.String())
	}
	duplicate := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", integrationPolicyPayload("policy_beta", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{"allow_add":true}`))
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "table_policy_exists")
}

func integrationPolicyPayload(tableName, queryPolicy, queryConfig, mutationPolicy, mutationConfig string) string {
	// Mutation scenarios keep their capability switches next to Auto Fill in a
	// compact test definition; the helper emits the real wire contract with
	// allow_* at Table Policy level and removes them from strategy JSON.
	var config map[string]json.RawMessage
	allowAdd, allowModify, allowDelete := false, false, false
	if json.Unmarshal([]byte(mutationConfig), &config) == nil {
		if raw, found := config["allow_add"]; found {
			_ = json.Unmarshal(raw, &allowAdd)
			delete(config, "allow_add")
		}
		if raw, found := config["allow_modify"]; found {
			_ = json.Unmarshal(raw, &allowModify)
			delete(config, "allow_modify")
		}
		if raw, found := config["allow_delete"]; found {
			_ = json.Unmarshal(raw, &allowDelete)
			delete(config, "allow_delete")
		}
		if normalized, err := json.Marshal(config); err == nil {
			mutationConfig = string(normalized)
		}
	}
	return `{"table_name":"` + tableName + `","query_policy":"` + queryPolicy + `","query_policy_config":` + queryConfig + `,"mutation_policy":"` + mutationPolicy + `","mutation_policy_config":` + mutationConfig + `,"allow_add":` + booleanJSON(allowAdd) + `,"allow_modify":` + booleanJSON(allowModify) + `,"allow_delete":` + booleanJSON(allowDelete) + `}`
}

func booleanJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
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
