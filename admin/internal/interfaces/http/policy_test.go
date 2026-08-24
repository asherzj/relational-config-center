package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

func TestOperatorCanCreateAndInspectDisabledTablePolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)

	const payload = `{
		"table_name":"managed_alpha",
		"query_policy":"mysql_page_query_v1",
		"query_policy_config":{
			"default_order":{"field":"id","direction":"DESC"},
			"default_page_size":20,
			"max_page_size":200
		},
		"mutation_policy":"mysql_single_table_mutation_v1",
		"allow_add":true,
		"allow_modify":true,
		"allow_delete":false,
		"mutation_policy_config":{
			"auto_fill":{
				"add":{"creator":{"source":"operator"}},
				"modify":{"modifier":{"source":"operator"}}
			}
		}
	}`
	created := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload)
	if created.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201, got %d: %s", created.Code, created.Body.String())
	}

	var createdPolicy map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &createdPolicy); err != nil {
		t.Fatalf("decode created Policy: %v", err)
	}
	assertPolicyResponse(t, createdPolicy)

	listed := performRequest(handler, http.MethodGet, "/api/v1/table-policies", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", listed.Code, listed.Body.String())
	}
	var listResponse struct {
		Policies []map[string]any `json:"policies"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &listResponse); err != nil {
		t.Fatalf("decode Policy list: %v", err)
	}
	if len(listResponse.Policies) != 1 {
		t.Fatalf("expected one Policy, got %#v", listResponse.Policies)
	}
	assertPolicyResponse(t, listResponse.Policies[0])

	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", got.Code, got.Body.String())
	}
	var gotPolicy map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &gotPolicy); err != nil {
		t.Fatalf("decode retrieved Policy: %v", err)
	}
	assertPolicyResponse(t, gotPolicy)
}

func TestCreateTablePolicyRequiresTheCompleteDefinition(t *testing.T) {
	handler := newPolicyHTTPHandler(t)

	response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", `{}`)
	assertSafeErrorEnvelope(t, response, http.StatusBadRequest, "invalid_policy_definition")
}

func TestCreateTablePolicyRejectsInvalidTargetsAndDefinitionsWithoutPersistence(t *testing.T) {
	tests := []struct {
		name       string
		tableName  string
		payload    string
		wantStatus int
		wantCode   string
	}{
		{name: "missing table", tableName: "does_not_exist", payload: validPolicyPayload("does_not_exist"), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "non-base table", tableName: "managed_view", payload: validPolicyPayload("managed_view"), wantStatus: http.StatusNotFound, wantCode: "database_table_not_found"},
		{name: "protected table", tableName: "RCC_control", payload: validPolicyPayload("RCC_control"), wantStatus: http.StatusForbidden, wantCode: "protected_table"},
		{name: "incompatible table", tableName: "incompatible", payload: validPolicyPayload("incompatible"), wantStatus: http.StatusUnprocessableEntity, wantCode: "incompatible_table"},
		{name: "unknown Query Policy", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), "mysql_page_query_v1", "unknown_query", 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "unknown_policy_strategy"},
		{name: "unknown Mutation Policy", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), "mysql_single_table_mutation_v1", "unknown_mutation", 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "unknown_policy_strategy"},
		{name: "non-object Query config", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), `"query_policy_config":{}`, `"query_policy_config":[]`, 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
		{name: "unknown typed Query field", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), `"query_policy_config":{}`, `"query_policy_config":{"sql":"SELECT 1"}`, 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
		{name: "unknown typed Mutation field", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), `"mutation_policy_config":{}`, `"mutation_policy_config":{"field_infos":[]}`, 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
		{name: "duplicated first-level Mutation capability", tableName: "managed_alpha", payload: strings.Replace(validPolicyPayload("managed_alpha"), `"mutation_policy_config":{}`, `"mutation_policy_config":{"allow_add":true}`, 1), wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_policy_config"},
		{name: "forbidden connection field", tableName: "managed_alpha", payload: strings.TrimSuffix(validPolicyPayload("managed_alpha"), "}") + `,"dsn":"mysql://elsewhere"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newPolicyHTTPHandler(t)
			response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", test.payload)
			assertHTTPErrorCode(t, response, test.wantStatus, test.wantCode)

			if test.tableName != "managed_alpha" || test.wantCode != "invalid_request" {
				got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/"+test.tableName, "")
				if got.Code != http.StatusNotFound && got.Code != http.StatusForbidden {
					t.Fatalf("failed creation partially persisted a Policy: HTTP %d %s", got.Code, got.Body.String())
				}
			}
		})
	}
}

func TestDuplicateTablePolicyIsAConflictAndDoesNotReplaceTheExistingPolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	first := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha"))
	if first.Code != http.StatusCreated {
		t.Fatalf("create initial Policy: HTTP %d %s", first.Code, first.Body.String())
	}

	replacement := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	duplicate := performRequest(handler, http.MethodPost, "/api/v1/table-policies", replacement)
	assertHTTPErrorCode(t, duplicate, http.StatusConflict, "table_policy_exists")

	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK || strings.Contains(got.Body.String(), `"allow_add":true`) {
		t.Fatalf("duplicate creation replaced the existing Policy: HTTP %d %s", got.Code, got.Body.String())
	}
}

func TestOperatorCanEnableCreatedTablePolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	created := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", created.Code, created.Body.String())
	}

	enabled := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}
	if !strings.Contains(enabled.Body.String(), `"enabled":true`) {
		t.Fatalf("enable response did not contain current enabled Policy: %s", enabled.Body.String())
	}

	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"enabled":true`) {
		t.Fatalf("enabled Policy was not immediately visible: HTTP %d %s", got.Code, got.Body.String())
	}
}

func TestOperatorCanAtomicallyReplacePolicyWithoutChangingEnabledState(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	created := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	enabled := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}

	replacement := `{"table_name":"managed_alpha","query_policy":"mysql_page_query_v1","query_policy_config":{"default_order":{"field":"value","direction":"ASC"},"default_page_size":7,"max_page_size":40},"mutation_policy":"mysql_single_table_mutation_v1","mutation_policy_config":{},"allow_add":true,"allow_modify":false,"allow_delete":false}`
	replaced := performRequest(handler, http.MethodPut, "/api/v1/table-policies/managed_alpha", replacement)
	if replaced.Code != http.StatusOK {
		t.Fatalf("replace Policy: HTTP %d %s", replaced.Code, replaced.Body.String())
	}
	if !strings.Contains(replaced.Body.String(), `"default_page_size":7`) || !strings.Contains(replaced.Body.String(), `"enabled":true`) {
		t.Fatalf("replacement did not become current while preserving state: %s", replaced.Body.String())
	}

	invalid := strings.Replace(replacement, `"default_page_size":7`, `"unsupported":true`, 1)
	rejected := performRequest(handler, http.MethodPut, "/api/v1/table-policies/managed_alpha", invalid)
	assertHTTPErrorCode(t, rejected, http.StatusUnprocessableEntity, "invalid_policy_config")

	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"default_page_size":7`) || !strings.Contains(got.Body.String(), `"enabled":true`) {
		t.Fatalf("invalid replacement changed the current Policy: HTTP %d %s", got.Code, got.Body.String())
	}
}

func TestOperatorCanDisableEnabledTablePolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	created := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha"))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	disabled := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/disable", "")
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), `"enabled":false`) {
		t.Fatalf("disable Policy: HTTP %d %s", disabled.Code, disabled.Body.String())
	}
}

func TestPolicyReplacementCannotRenameTheManagedTable(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha")); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	rejected := performRequest(handler, http.MethodPut, "/api/v1/table-policies/managed_alpha", validPolicyPayload("other_table"))
	assertHTTPErrorCode(t, rejected, http.StatusBadRequest, "invalid_policy_definition")
	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != validPolicyResponse("managed_alpha") {
		t.Fatalf("rename attempt changed the current Policy: HTTP %d %s", got.Code, got.Body.String())
	}
}

func TestEnableRevalidatesLiveFieldsReferencedByTypedPolicyConfig(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := `{"table_name":"managed_alpha","query_policy":"mysql_page_query_v1","query_policy_config":{"default_order":{"field":"missing","direction":"ASC"}},"mutation_policy":"mysql_single_table_mutation_v1","mutation_policy_config":{}}`
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create disabled Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	rejected := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", "")
	assertHTTPErrorCode(t, rejected, http.StatusUnprocessableEntity, "invalid_policy_config")
	got := performRequest(handler, http.MethodGet, "/api/v1/table-policies/managed_alpha", "")
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"enabled":false`) {
		t.Fatalf("failed enable changed Policy state: HTTP %d %s", got.Code, got.Body.String())
	}
}

func TestManagedTableQueryFailsClosedBeforeExecution(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	missing := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", `{}`)
	assertHTTPErrorCode(t, missing, http.StatusNotFound, "table_policy_not_found")

	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha")); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	disabled := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", `{}`)
	assertHTTPErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")

	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	overflowingPage := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", `{"page_number":9223372036854775807,"page_size":200}`)
	assertHTTPErrorCode(t, overflowingPage, http.StatusBadRequest, "invalid_pagination")
}

func TestOperatorCanAddRowThroughEnabledMutationPolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	added := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{"value":"created"}}`)
	if added.Code != http.StatusCreated || strings.TrimSpace(added.Body.String()) != `{"id":"42"}` {
		t.Fatalf("add row: expected HTTP 201 with string id, got HTTP %d %s", added.Code, added.Body.String())
	}
}

func TestOperatorCanPatchOneRowThroughEnabledMutationPolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(
		validPolicyPayload("managed_alpha"),
		`"allow_modify":false`,
		`"allow_modify":true`,
		1,
	)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	modified := performRequest(handler, http.MethodPatch, "/api/v1/tables/managed_alpha/rows/42", `{"content":{"value":"changed"}}`)
	if modified.Code != http.StatusOK || strings.TrimSpace(modified.Body.String()) != `{"affected":1}` {
		t.Fatalf("patch row: expected HTTP 200 with affected count, got HTTP %d %s", modified.Code, modified.Body.String())
	}
}

func TestOperatorCanHardDeleteOneRowThroughEnabledMutationPolicy(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(
		validPolicyPayload("managed_alpha"),
		`"allow_delete":false`,
		`"allow_delete":true`,
		1,
	)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	deleted := performRequest(handler, http.MethodDelete, "/api/v1/tables/managed_alpha/rows/42", "")
	if deleted.Code != http.StatusOK || strings.TrimSpace(deleted.Body.String()) != `{"affected":1}` {
		t.Fatalf("delete row: expected HTTP 200 with affected count, got HTTP %d %s", deleted.Code, deleted.Body.String())
	}
}

func TestHardDeleteDefaultsToDeniedBeforeMutationExecution(t *testing.T) {
	handler := newPolicyHTTPHandlerWithMutationExecutor(t, panicMutationExecutor{})
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha")); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	denied := performRequest(handler, http.MethodDelete, "/api/v1/tables/managed_alpha/rows/42", "")
	assertHTTPErrorCode(t, denied, http.StatusForbidden, "mutation_not_allowed")
}

func TestHardDeleteRejectsTypedInvalidIDAndMapsMissingRows(t *testing.T) {
	for _, test := range []struct {
		name     string
		executor application.MutationExecutor
		id       string
		status   int
		code     string
	}{
		{name: "invalid typed id", executor: panicMutationExecutor{}, id: "not-an-integer", status: http.StatusBadRequest, code: "invalid_mutation_content"},
		{name: "missing row", executor: missingRowMutationExecutor{}, id: "42", status: http.StatusNotFound, code: "mutation_row_not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newPolicyHTTPHandlerWithMutationExecutor(t, test.executor)
			payload := strings.Replace(
				validPolicyPayload("managed_alpha"),
				`"allow_delete":false`,
				`"allow_delete":true`,
				1,
			)
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
				t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
				t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
			}

			response := performRequest(handler, http.MethodDelete, "/api/v1/tables/managed_alpha/rows/"+test.id, "")
			assertHTTPErrorCode(t, response, test.status, test.code)
		})
	}
}

func TestPatchRequiresStrictJSONStringContentShape(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(
		validPolicyPayload("managed_alpha"),
		`"allow_modify":false`,
		`"allow_modify":true`,
		1,
	)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	for _, body := range []string{
		`{}`,
		`{"content":null}`,
		`{"content":{"value":7}}`,
		`{"content":{"value":"changed"},"unexpected":true}`,
	} {
		response := performRequest(handler, http.MethodPatch, "/api/v1/tables/managed_alpha/rows/42", body)
		assertHTTPErrorCode(t, response, http.StatusBadRequest, "invalid_request")
	}
}

func TestPatchRejectsPrimaryKeyChangesAndMapsMissingRows(t *testing.T) {
	for _, test := range []struct {
		name     string
		executor application.MutationExecutor
		body     string
		status   int
		code     string
	}{
		{name: "primary key in content", executor: memoryMutationExecutor{}, body: `{"content":{"id":"43"}}`, status: http.StatusBadRequest, code: "invalid_mutation_content"},
		{name: "invalid path id", executor: memoryMutationExecutor{}, body: `{"content":{"value":"changed"}}`, status: http.StatusBadRequest, code: "invalid_mutation_content"},
		{name: "missing row", executor: missingRowMutationExecutor{}, body: `{"content":{"value":"changed"}}`, status: http.StatusNotFound, code: "mutation_row_not_found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := newPolicyHTTPHandlerWithMutationExecutor(t, test.executor)
			payload := strings.Replace(
				validPolicyPayload("managed_alpha"),
				`"allow_modify":false`,
				`"allow_modify":true`,
				1,
			)
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
				t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
				t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
			}
			pathID := "42"
			if test.name == "invalid path id" {
				pathID = "not-an-integer"
			}
			response := performRequest(handler, http.MethodPatch, "/api/v1/tables/managed_alpha/rows/"+pathID, test.body)
			assertHTTPErrorCode(t, response, test.status, test.code)
		})
	}
}

func TestModifyAutoFillCannotTargetThePrimaryKey(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(strings.Replace(
		validPolicyPayload("managed_alpha"),
		`"allow_modify":false`,
		`"allow_modify":true`,
		1,
	), `"mutation_policy_config":{}`, `"mutation_policy_config":{"auto_fill":{"modify":{"id":{"source":"literal","value":"43"}}}}`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create disabled Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	rejected := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", "")
	assertHTTPErrorCode(t, rejected, http.StatusUnprocessableEntity, "invalid_policy_config")
}

func TestAddRequiresCurrentNonNullableFieldsBeforeExecution(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	rejected := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{}}`)
	assertHTTPErrorCode(t, rejected, http.StatusBadRequest, "missing_required_field")
}

func TestAddRejectsGeneratedInputFields(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	rejected := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{"value":"created","generated_value":"client override"}}`)
	assertHTTPErrorCode(t, rejected, http.StatusBadRequest, "invalid_mutation_content")
}

func TestAddAutoFillCanSupplyARequiredField(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	payload := strings.Replace(strings.Replace(
		validPolicyPayload("managed_alpha"),
		`"allow_add":false`,
		`"allow_add":true`,
		1,
	), `"mutation_policy_config":{}`, `"mutation_policy_config":{"auto_fill":{"add":{"value":{"source":"operator"}}}}`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	added := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{}}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("Auto Fill did not supply required field: HTTP %d %s", added.Code, added.Body.String())
	}
}

func TestAddMapsDuplicateKeysToConflict(t *testing.T) {
	handler := newPolicyHTTPHandlerWithMutationExecutor(t, duplicateMutationExecutor{})
	payload := strings.Replace(validPolicyPayload("managed_alpha"), `"allow_add":false`, `"allow_add":true`, 1)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", payload); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	duplicate := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/rows", `{"content":{"value":"duplicate"}}`)
	assertHTTPErrorCode(t, duplicate, http.StatusConflict, "duplicate_key")
}

func TestManagedTableQueryDistinguishesMissingNullAndJSONStringConditionFields(t *testing.T) {
	handler := newPolicyHTTPHandler(t)
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies", validPolicyPayload("managed_alpha")); response.Code != http.StatusCreated {
		t.Fatalf("create Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	if response := performRequest(handler, http.MethodPost, "/api/v1/table-policies/managed_alpha/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	valid := []string{
		`{"conditions":[{"field":"value","operator":"exact","value":""}]}`,
		`{"conditions":[{"field":"value","operator":"contains","value":"a"}]}`,
		`{"conditions":[{"field":"id","operator":"open_range","from":"1"}]}`,
		`{"conditions":[{"field":"id","operator":"closed_range","to":"2"}]}`,
		`{"conditions":[{"field":"id","operator":"in","values":["1"]}]}`,
		`{"conditions":[{"field":"id","operator":"not_in","values":["1"]}]}`,
		`{"conditions":[{"field":"value","operator":"is_null"}]}`,
		`{"conditions":[{"field":"value","operator":"is_not_null"}]}`,
	}
	for _, body := range valid {
		response := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", body)
		if response.Code != http.StatusOK {
			t.Fatalf("valid condition rejected: HTTP %d %s for %s", response.Code, response.Body.String(), body)
		}
	}

	invalid := []string{
		`{"conditions":[{"field":"value","operator":"exact"}]}`,
		`{"conditions":[{"field":"value","operator":"exact","value":null}]}`,
		`{"conditions":[{"field":"value","operator":"exact","value":"a","from":"a"}]}`,
		`{"conditions":[{"field":"value","operator":"contains","value":"a","values":[]}]}`,
		`{"conditions":[{"field":"id","operator":"open_range"}]}`,
		`{"conditions":[{"field":"id","operator":"open_range","from":null,"to":"2"}]}`,
		`{"conditions":[{"field":"id","operator":"closed_range","from":"1","to":null}]}`,
		`{"conditions":[{"field":"id","operator":"closed_range","from":"1","value":"1"}]}`,
		`{"conditions":[{"field":"id","operator":"in"}]}`,
		`{"conditions":[{"field":"id","operator":"in","values":null}]}`,
		`{"conditions":[{"field":"id","operator":"in","values":[]}]}`,
		`{"conditions":[{"field":"id","operator":"not_in","values":[null]}]}`,
		`{"conditions":[{"field":"id","operator":"in","values":["1"],"to":"2"}]}`,
		`{"conditions":[{"field":"value","operator":"is_null","value":null}]}`,
		`{"conditions":[{"field":"value","operator":"is_not_null","values":null}]}`,
		`{"conditions":[{"field":"value","operator":"is_not_null","from":null}]}`,
	}
	for _, body := range invalid {
		response := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", body)
		assertHTTPErrorCode(t, response, http.StatusBadRequest, "invalid_query_condition")
	}

	unknownField := performRequest(handler, http.MethodPost, "/api/v1/tables/managed_alpha/query", `{"conditions":[{"field":"value","operator":"exact","value":"a","unexpected":true}]}`)
	assertHTTPErrorCode(t, unknownField, http.StatusBadRequest, "invalid_request")
}

func validPolicyPayload(tableName string) string {
	return `{"table_name":"` + tableName + `","query_policy":"mysql_page_query_v1","query_policy_config":{},"mutation_policy":"mysql_single_table_mutation_v1","mutation_policy_config":{},"allow_add":false,"allow_modify":false,"allow_delete":false}`
}

func validPolicyResponse(tableName string) string {
	return `{"table_name":"` + tableName + `","query_policy":"mysql_page_query_v1","query_policy_config":{},"mutation_policy":"mysql_single_table_mutation_v1","mutation_policy_config":{},"allow_add":false,"allow_modify":false,"allow_delete":false,"enabled":false}`
}

func assertHTTPErrorCode(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
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

func assertPolicyResponse(t *testing.T, policy map[string]any) {
	t.Helper()
	if _, exposed := policy["id"]; exposed {
		t.Fatalf("Policy exposed internal Catalog ID: %#v", policy)
	}
	if len(policy) != 9 {
		t.Fatalf("Policy exposed unexpected metadata: %#v", policy)
	}
	if policy["table_name"] != "managed_alpha" || policy["query_policy"] != "mysql_page_query_v1" || policy["mutation_policy"] != "mysql_single_table_mutation_v1" {
		t.Fatalf("unexpected Policy identity or strategies: %#v", policy)
	}
	if enabled, ok := policy["enabled"].(bool); !ok || enabled {
		t.Fatalf("new Policy must be disabled: %#v", policy)
	}
	if _, ok := policy["query_policy_config"].(map[string]any); !ok {
		t.Fatalf("query Policy config is not an object: %#v", policy)
	}
	if _, ok := policy["mutation_policy_config"].(map[string]any); !ok {
		t.Fatalf("mutation Policy config is not an object: %#v", policy)
	}
	for _, capability := range []string{"allow_add", "allow_modify", "allow_delete"} {
		if _, ok := policy[capability].(bool); !ok {
			t.Fatalf("Policy capability %s is not boolean: %#v", capability, policy)
		}
	}
	if policy["allow_add"] != true || policy["allow_modify"] != true || policy["allow_delete"] != false {
		t.Fatalf("unexpected Mutation capabilities: %#v", policy)
	}
}

func newPolicyHTTPHandler(t *testing.T) http.Handler {
	return newPolicyHTTPHandlerWithRouterOptions(t, httpinterface.RouterOptions{AuthDisabled: true})
}

func newPolicyHTTPHandlerWithMutationExecutor(t *testing.T, mutationExecutor application.MutationExecutor) http.Handler {
	return newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, httpinterface.RouterOptions{AuthDisabled: true}, mutationExecutor)
}

func newPolicyHTTPHandlerWithRouterOptions(t *testing.T, options httpinterface.RouterOptions) http.Handler {
	return newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t, options, memoryMutationExecutor{})
}

func newPolicyHTTPHandlerWithOptionsAndMutationExecutor(t *testing.T, options httpinterface.RouterOptions, mutationExecutor application.MutationExecutor) http.Handler {
	return newPolicyHTTPHandlerWithExecutors(t, options, memoryQueryExecutor{}, mutationExecutor)
}

func newPolicyHTTPHandlerWithExecutors(t *testing.T, options httpinterface.RouterOptions, queryExecutor application.QueryExecutor, mutationExecutor application.MutationExecutor) http.Handler {
	t.Helper()
	metadata := &memoryMetadata{tables: map[string]domain.DatabaseTable{
		"managed_alpha": domain.DescribeDatabaseTable("managed_alpha", "Alpha configuration", []string{"id"}, false, false),
		"incompatible":  domain.DescribeDatabaseTable("incompatible", "Incompatible", []string{"code"}, false, false),
	}}
	catalog := &memoryPolicyCatalog{policies: make(map[string]domain.TablePolicy)}
	registry, err := application.NewStrategyRegistry(
		[]application.QueryRegistration{{ID: "mysql_page_query_v1", Constructor: application.NewMySQLPageQueryStrategy}},
		[]application.MutationRegistration{{ID: "mysql_single_table_mutation_v1", Constructor: application.NewMySQLSingleTableMutationStrategy}},
	)
	if err != nil {
		t.Fatalf("build strategy registry: %v", err)
	}
	policies := application.NewTablePolicyManagement(metadata, catalog, registry, "integration-test")
	queries := application.NewManagedTableQuery(metadata, catalog, registry, queryExecutor)
	mutations := application.NewManagedTableMutation(metadata, catalog, registry, application.NewFixedOperatorProvider("integration-test"), mutationExecutor)
	return httpinterface.NewRouter(application.NewDatabaseTableDiscovery(metadata), readyAdapter{}, policies, queries, mutations, options)
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

type memoryMetadata struct {
	tables map[string]domain.DatabaseTable
}

func (metadata *memoryMetadata) ListDatabaseTables(context.Context) ([]domain.DatabaseTable, error) {
	tables := make([]domain.DatabaseTable, 0, len(metadata.tables))
	for _, table := range metadata.tables {
		tables = append(tables, table)
	}
	sort.Slice(tables, func(left, right int) bool { return tables[left].Name < tables[right].Name })
	return tables, nil
}

func (metadata *memoryMetadata) GetDatabaseTable(_ context.Context, tableName string) (domain.DatabaseTable, error) {
	if strings.HasPrefix(strings.ToLower(tableName), "rcc_") {
		return domain.DatabaseTable{}, application.ErrProtectedTable
	}
	table, found := metadata.tables[tableName]
	if !found {
		return domain.DatabaseTable{}, application.ErrDatabaseTableNotFound
	}
	return table, nil
}

func (metadata *memoryMetadata) GetTableSchema(ctx context.Context, tableName string) (domain.TableSchema, error) {
	table, err := metadata.GetDatabaseTable(ctx, tableName)
	if err != nil {
		return domain.TableSchema{}, err
	}
	return domain.TableSchema{
		Name: tableName,
		Columns: []domain.Column{
			{Name: "id", Type: domain.ColumnTypeUInt64, Nullable: false, AutoIncrement: true},
			{Name: "value", Type: domain.ColumnTypeString, Nullable: false},
			{Name: "defaulted_value", Type: domain.ColumnTypeString, Nullable: false, HasDefault: true},
			{Name: "nullable_value", Type: domain.ColumnTypeString, Nullable: true},
			{Name: "generated_value", Type: domain.ColumnTypeString, Nullable: true, Generated: true},
		},
		Compatible:            table.Compatible,
		IncompatibilityReason: table.IncompatibilityReason,
	}, nil
}

type memoryPolicyCatalog struct {
	mu       sync.Mutex
	policies map[string]domain.TablePolicy
}

func (catalog *memoryPolicyCatalog) Create(_ context.Context, policy domain.TablePolicy, _ string) error {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	if _, found := catalog.policies[policy.TableName]; found {
		return domain.ErrTablePolicyExists
	}
	catalog.policies[policy.TableName] = policy
	return nil
}

func (catalog *memoryPolicyCatalog) List(context.Context) ([]domain.TablePolicy, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	policies := make([]domain.TablePolicy, 0, len(catalog.policies))
	for _, policy := range catalog.policies {
		policies = append(policies, policy)
	}
	sort.Slice(policies, func(left, right int) bool { return policies[left].TableName < policies[right].TableName })
	return policies, nil
}

func (catalog *memoryPolicyCatalog) Get(_ context.Context, tableName string) (domain.TablePolicy, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	policy, found := catalog.policies[tableName]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	return policy, nil
}

func (catalog *memoryPolicyCatalog) Replace(_ context.Context, replacement domain.TablePolicy, _ string) (domain.TablePolicy, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	current, found := catalog.policies[replacement.TableName]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	replacement.Enabled = current.Enabled
	catalog.policies[replacement.TableName] = replacement
	return replacement, nil
}

func (catalog *memoryPolicyCatalog) SetEnabled(_ context.Context, tableName string, enabled bool, _ string) (domain.TablePolicy, error) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	policy, found := catalog.policies[tableName]
	if !found {
		return domain.TablePolicy{}, domain.ErrTablePolicyNotFound
	}
	policy.Enabled = enabled
	catalog.policies[tableName] = policy
	return policy, nil
}

type readyAdapter struct{}

func (readyAdapter) Ready(context.Context) error { return nil }

type memoryQueryExecutor struct{}

func (memoryQueryExecutor) ExecutePageQuery(context.Context, domain.PageQuery) (domain.QueryResult, error) {
	return domain.QueryResult{}, nil
}

type memoryMutationExecutor struct{}

func (memoryMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "42", nil
}

func (memoryMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	return 1, nil
}

func (memoryMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	return 1, nil
}

type duplicateMutationExecutor struct{}

func (duplicateMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "", application.ErrDuplicateKey
}

func (duplicateMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	return 0, application.ErrDuplicateKey
}

func (duplicateMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	return 0, application.ErrDuplicateKey
}

type missingRowMutationExecutor struct{}

func (missingRowMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	return "", application.ErrMutationRowNotFound
}

func (missingRowMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	return 0, application.ErrMutationRowNotFound
}

func (missingRowMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	return 0, application.ErrMutationRowNotFound
}

type panicMutationExecutor struct{}

func (panicMutationExecutor) InsertRow(context.Context, domain.RowInsert) (string, error) {
	panic("mutation executor must not be called")
}

func (panicMutationExecutor) UpdateRow(context.Context, domain.RowUpdate) (int64, error) {
	panic("mutation executor must not be called")
}

func (panicMutationExecutor) DeleteRow(context.Context, domain.RowDelete) (int64, error) {
	panic("mutation executor must not be called")
}

var _ domain.TablePolicyCatalog = (*memoryPolicyCatalog)(nil)
var _ application.TableMetadataReader = (*memoryMetadata)(nil)
var _ application.Readiness = readyAdapter{}
var _ application.QueryExecutor = memoryQueryExecutor{}
var _ application.MutationExecutor = memoryMutationExecutor{}
var _ application.MutationExecutor = duplicateMutationExecutor{}
var _ application.MutationExecutor = missingRowMutationExecutor{}
var _ application.MutationExecutor = panicMutationExecutor{}
