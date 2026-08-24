//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMutationPolicyAddsRowAndReturnsJSONStringID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"first","label":"created through Admin"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row: expected HTTP 201, got %d: %s", added.Code, added.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if created.ID != "1" {
		t.Fatalf("expected lossless string id 1, got %q", created.ID)
	}

	queried := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/query", `{
		"conditions":[{"field":"id","operator":"exact","value":"1"}]
	}`)
	if queried.Code != http.StatusOK {
		t.Fatalf("query inserted row: expected HTTP 200, got %d: %s", queried.Code, queried.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(queried.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["code"] == nil || *result.Rows[0]["code"] != "first" || result.Rows[0]["label"] == nil || *result.Rows[0]["label"] != "created through Admin" {
		t.Fatalf("inserted row was not observable through Admin query: %s", queried.Body.String())
	}
}

func TestMutationPolicyReturnsExplicitAutoIncrementID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	const explicitID = "9007199254740993"
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"id":"`+explicitID+`","code":"explicit-auto-id","label":"client supplied id"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with explicit auto-increment id: HTTP %d %s", added.Code, added.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if response.ID != explicitID {
		t.Fatalf("expected explicit lossless id %q, got %q", explicitID, response.ID)
	}

	row := queryMutationRow(t, app, "explicit-auto-id")
	assertMutationString(t, row, "id", explicitID)
}

func TestMutationPolicyReturnsARequiredNonAutoIncrementID(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_supplied_id_items", `{"allow_add":true}`)

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_supplied_id_items/rows", `{
		"content":{"id":"external-9007199254740993","label":"supplied identity"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with supplied id: HTTP %d %s", added.Code, added.Body.String())
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(added.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if response.ID != "external-9007199254740993" {
		t.Fatalf("expected supplied lossless id, got %q", response.ID)
	}
}

func TestMutationPolicyUsesDefaultsAndNullabilityAndRejectsInvalidInputFields(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"schema-rules","label":"valid","nullable_value":null,"quantity":"7","metadata":"{\"enabled\":true}"}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row using default and NULL semantics: HTTP %d %s", added.Code, added.Body.String())
	}

	queried := queryMutationRow(t, app, "schema-rules")
	if queried["defaulted_value"] == nil || *queried["defaulted_value"] != "database-default" {
		t.Fatalf("database default was not preserved: %#v", queried)
	}
	if queried["nullable_value"] != nil {
		t.Fatalf("explicit JSON null did not become SQL NULL: %#v", queried)
	}
	if queried["generated_value"] == nil || *queried["generated_value"] != "schema-rules:generated" {
		t.Fatalf("generated value was not produced by MySQL: %#v", queried)
	}

	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "unknown field", body: `{"content":{"code":"unknown","label":"bad","not_a_column":"x"}}`, code: "invalid_mutation_content"},
		{name: "generated field", body: `{"content":{"code":"generated","label":"bad","generated_value":"override"}}`, code: "invalid_mutation_content"},
		{name: "invalid integer", body: `{"content":{"code":"integer","label":"bad","quantity":"seven"}}`, code: "invalid_mutation_content"},
		{name: "invalid JSON", body: `{"content":{"code":"json","label":"bad","metadata":"not-json"}}`, code: "invalid_mutation_content"},
		{name: "null in non-null column", body: `{"content":{"code":"null","label":null}}`, code: "invalid_mutation_content"},
		{name: "non-string dynamic value", body: `{"content":{"code":"number","label":7}}`, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", test.body)
			assertIntegrationErrorCode(t, response, http.StatusBadRequest, test.code)
		})
	}
}

func TestMutationPolicyAutoFillOverridesClientValuesFromEverySupportedSource(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_auto_fill_items", `{
		"allow_add":true,
		"auto_fill":{"add":{
			"creator":{"source":"operator"},
			"occurred_at":{"source":"now"},
			"status":{"source":"literal","value":"server-owned"},
			"quantity":{"source":"literal","value":"9"}
		}}
	}`)

	before := time.Now().UTC().Add(-time.Second)
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_auto_fill_items/rows", `{
		"content":{
			"code":"auto-fill",
			"creator":"client",
			"occurred_at":"2000-01-01 00:00:00",
			"status":"client",
			"quantity":"1"
		}
	}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("add row with Auto Fill: HTTP %d %s", added.Code, added.Body.String())
	}
	after := time.Now().UTC().Add(time.Second)

	row := queryMutationTableRow(t, app, "mutation_auto_fill_items", "auto-fill")
	assertMutationString(t, row, "creator", "integration-test")
	assertMutationString(t, row, "status", "server-owned")
	assertMutationString(t, row, "quantity", "9")
	if row["occurred_at"] == nil {
		t.Fatalf("Auto Fill now produced SQL NULL: %#v", row)
	}
	occurredAt, err := time.ParseInLocation("2006-01-02 15:04:05.999999", *row["occurred_at"], time.UTC)
	if err != nil || occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("Auto Fill now value %q is outside request window [%s, %s]: %v", *row["occurred_at"], before, after, err)
	}
}

func TestMutationPolicyRejectsMissingRequiredFieldsAndRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	missing := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"missing-label"}
	}`)
	assertIntegrationErrorCode(t, missing, http.StatusBadRequest, "missing_required_field")
	assertMutationRowAbsent(t, app, "mutation_add_items", "missing-label")

	failed := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"rollback","label":"rollback"}
	}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	assertMutationRowAbsent(t, app, "mutation_add_items", "rollback")
}

func TestMutationPolicyMapsMySQLUniqueKeyViolationsToConflict(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	first := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"duplicate","label":"first"}
	}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("create first unique row: HTTP %d %s", first.Code, first.Body.String())
	}
	duplicate := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{
		"content":{"code":"duplicate","label":"second"}
	}`)
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "duplicate_key")
	row := queryMutationRow(t, app, "duplicate")
	assertMutationString(t, row, "label", "first")
}

func TestMutationPolicyPatchesOnlySubmittedFieldsWithJSONStringSemantics(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_modify":true}`)
	id := addMutationPatchFixtureRow(t, app, "patch-semantics", "original")

	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":"changed"}
	}`)
	assertMutationAffected(t, modified)
	row := queryMutationRow(t, app, "patch-semantics")
	assertMutationString(t, row, "label", "changed")
	assertMutationString(t, row, "nullable_value", "preserved")
	assertMutationString(t, row, "quantity", "7")
	assertMutationString(t, row, "defaulted_value", "database-default")

	nulled := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"nullable_value":null}
	}`)
	assertMutationAffected(t, nulled)
	if row = queryMutationRow(t, app, "patch-semantics"); row["nullable_value"] != nil {
		t.Fatalf("PATCH JSON null did not become SQL NULL: %#v", row)
	}

	emptied := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":""}
	}`)
	assertMutationAffected(t, emptied)
	row = queryMutationRow(t, app, "patch-semantics")
	assertMutationString(t, row, "label", "")

	unchanged := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{
		"content":{"label":""}
	}`)
	assertMutationAffected(t, unchanged)
}

func TestMutationPolicyPatchRejectsInvalidAndNonWritableFields(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_modify":true}`)
	id := addMutationPatchFixtureRow(t, app, "patch-invalid", "original")

	tests := []struct {
		name string
		body string
		code string
	}{
		{name: "invalid integer", body: `{"content":{"quantity":"seven"}}`, code: "invalid_mutation_content"},
		{name: "primary key", body: `{"content":{"id":"99"}}`, code: "invalid_mutation_content"},
		{name: "generated field", body: `{"content":{"generated_value":"override"}}`, code: "invalid_mutation_content"},
		{name: "unknown field", body: `{"content":{"not_a_column":"value"}}`, code: "invalid_mutation_content"},
		{name: "null in non-null column", body: `{"content":{"label":null}}`, code: "invalid_mutation_content"},
		{name: "empty patch", body: `{"content":{}}`, code: "invalid_mutation_content"},
		{name: "non-string dynamic value", body: `{"content":{"quantity":7}}`, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, test.body)
			assertIntegrationErrorCode(t, response, http.StatusBadRequest, test.code)
			row := queryMutationRow(t, app, "patch-invalid")
			assertMutationString(t, row, "label", "original")
			assertMutationString(t, row, "quantity", "7")
		})
	}

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `unsupported_value` blob NULL"); err != nil {
		t.Fatalf("add unsupported live field: %v", err)
	}
	unsupported := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"unsupported_value":"bytes"}}`)
	assertIntegrationErrorCode(t, unsupported, http.StatusBadRequest, "invalid_mutation_content")
}

func TestMutationPolicyPatchUsesLatestPolicyAndLiveSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_modify":false}`)
	id := addMutationPatchFixtureRow(t, app, "patch-latest", "original")

	forbidden := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"forbidden"}}`)
	assertIntegrationErrorCode(t, forbidden, http.StatusForbidden, "mutation_not_allowed")

	replacement := integrationPolicyPayload("mutation_add_items", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{"allow_add":true,"allow_modify":true}`)
	if response := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/mutation_add_items", replacement); response.Code != http.StatusOK {
		t.Fatalf("replace current Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `future_note` varchar(64) NULL DEFAULT 'future-default'"); err != nil {
		t.Fatalf("add supported live field: %v", err)
	}

	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"latest"}}`)
	assertMutationAffected(t, modified)
	row := queryMutationRow(t, app, "patch-latest")
	assertMutationString(t, row, "label", "latest")
	assertMutationString(t, row, "future_note", "future-default")
	assertMutationString(t, row, "defaulted_value", "database-default")

	newField := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"future_note":"live-schema"}}`)
	assertMutationAffected(t, newField)
	row = queryMutationRow(t, app, "patch-latest")
	assertMutationString(t, row, "future_note", "live-schema")
	assertMutationString(t, row, "defaulted_value", "database-default")

	missing := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/999999", `{"content":{"label":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
}

func TestMutationPolicyPatchAutoFillOverridesClientValues(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_auto_fill_items", `{
		"allow_add":true,
		"allow_modify":true,
		"auto_fill":{"modify":{
			"creator":{"source":"operator"},
			"occurred_at":{"source":"now"},
			"status":{"source":"literal","value":"server-owned"},
			"quantity":{"source":"literal","value":"9"}
		}}
	}`)
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_auto_fill_items/rows", `{
		"content":{"code":"patch-auto-fill","creator":"initial","occurred_at":"2000-01-01 00:00:00","status":"initial","quantity":"1"}
	}`)
	id := mutationResponseID(t, added)

	before := time.Now().UTC().Add(-time.Second)
	modified := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_auto_fill_items/rows/"+id, `{
		"content":{"creator":"client","occurred_at":"2001-01-01 00:00:00","status":"client","quantity":"2"}
	}`)
	assertMutationAffected(t, modified)
	after := time.Now().UTC().Add(time.Second)

	row := queryMutationTableRow(t, app, "mutation_auto_fill_items", "patch-auto-fill")
	assertMutationString(t, row, "creator", "integration-test")
	assertMutationString(t, row, "status", "server-owned")
	assertMutationString(t, row, "quantity", "9")
	if row["occurred_at"] == nil {
		t.Fatalf("MODIFY Auto Fill now produced SQL NULL: %#v", row)
	}
	occurredAt, err := time.ParseInLocation("2006-01-02 15:04:05.999999", *row["occurred_at"], time.UTC)
	if err != nil || occurredAt.Before(before) || occurredAt.After(after) {
		t.Fatalf("MODIFY Auto Fill now value %q is outside request window [%s, %s]: %v", *row["occurred_at"], before, after, err)
	}
}

func TestMutationPolicyPatchRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_modify":true}`)
	id := addMutationPatchFixtureRow(t, app, "patch-rollback", "original")

	failed := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"rollback","quantity":"99"}}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row := queryMutationRow(t, app, "patch-rollback")
	assertMutationString(t, row, "label", "original")
	assertMutationString(t, row, "quantity", "7")
}

func TestMutationPolicyPatchMapsUniqueKeyConflictsAndRollsBack(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_modify":true}`)
	addMutationPatchFixtureRow(t, app, "patch-unique-first", "first")
	secondID := addMutationPatchFixtureRow(t, app, "patch-unique-second", "second")

	conflict := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+secondID, `{"content":{"code":"patch-unique-first","label":"changed"}}`)
	assertIntegrationErrorCode(t, conflict, http.StatusConflict, "duplicate_key")
	row := queryMutationRow(t, app, "patch-unique-second")
	assertMutationString(t, row, "label", "second")
}

func TestMutationPolicyPatchFailsClosedForCurrentPolicyAndSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	missing := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/1", `{"content":{"label":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "table_policy_not_found")
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", integrationPolicyPayload(
		"mutation_add_items",
		"mysql_page_query_v1",
		`{}`,
		"mysql_single_table_mutation_v1",
		`{"allow_add":true,"allow_modify":true}`,
	))
	if created.Code != http.StatusCreated {
		t.Fatalf("create disabled Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	disabled := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/1", `{"content":{"label":"disabled"}}`)
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	if enabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/mutation_add_items/enable", ""); enabled.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}
	id := addMutationPatchFixtureRow(t, app, "patch-fail-closed", "original")

	protected := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/rcc_table_policies/rows/1", `{"content":{"modifier":"x"}}`)
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'unknown_mutation' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("make current Policy invalid: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"unknown"}}`)
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_strategy")

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'mysql_single_table_mutation_v1' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("restore current Policy: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` MODIFY `id` bigint unsigned NOT NULL, DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"incompatible"}}`)
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestMutationPolicyDeleteDefaultsToDeniedThenUsesTheCurrentReplacement(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)
	id := addMutationPatchFixtureRow(t, app, "delete-current", "preserved until permitted")

	denied := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, denied, http.StatusForbidden, "mutation_not_allowed")
	assertMutationString(t, queryMutationRow(t, app, "delete-current"), "label", "preserved until permitted")

	replacement := integrationPolicyPayload(
		"mutation_add_items",
		"mysql_page_query_v1",
		`{}`,
		"mysql_single_table_mutation_v1",
		`{"allow_add":true,"allow_delete":true}`,
	)
	if response := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/mutation_add_items", replacement); response.Code != http.StatusOK {
		t.Fatalf("replace current Policy: HTTP %d %s", response.Code, response.Body.String())
	}

	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertMutationAffected(t, deleted)
	assertMutationRowAbsent(t, app, "mutation_add_items", "delete-current")
}

func TestMutationPolicyDeleteFailsClosedBeforeExecution(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)

	missingPolicy := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/1", "")
	assertIntegrationErrorCode(t, missingPolicy, http.StatusNotFound, "table_policy_not_found")

	policy := integrationPolicyPayload(
		"mutation_add_items",
		"mysql_page_query_v1",
		`{}`,
		"mysql_single_table_mutation_v1",
		`{"allow_add":true,"allow_delete":true}`,
	)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create disabled Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	disabled := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/1", "")
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/mutation_add_items/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	id := addMutationPatchFixtureRow(t, app, "delete-invalid-id", "must remain")

	invalidID := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/not-an-integer", "")
	assertIntegrationErrorCode(t, invalidID, http.StatusBadRequest, "invalid_mutation_content")
	assertMutationString(t, queryMutationRow(t, app, "delete-invalid-id"), "id", id)

	protected := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/rcc_table_policies/rows/1", "")
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")
}

func TestMutationPolicyDeleteMapsMissingRowsToNotFound(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_delete":true}`)

	missing := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/999999", "")
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
}

func TestMutationPolicyDeleteRollsBackDatabaseFailures(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	enableMutationPolicy(t, app, "mutation_delete_parents", `{"allow_delete":true}`)

	failed := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_delete_parents/rows/1", "")
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row := queryMutationTableRow(t, app, "mutation_delete_parents", "delete-rollback")
	assertMutationString(t, row, "id", "1")
}

func TestMutationPolicyDeleteIgnoresUnrelatedUnsupportedLiveColumns(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_delete":true}`)
	id := addMutationPatchFixtureRow(t, app, "delete-blob", "delete despite blob")

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` ADD COLUMN `unsupported_payload` BLOB NULL"); err != nil {
		t.Fatalf("add unsupported unrelated column: %v", err)
	}

	queryRejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/query", `{}`)
	assertIntegrationErrorCode(t, queryRejected, http.StatusUnprocessableEntity, "incompatible_table")
	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertMutationAffected(t, deleted)

	var count int
	if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM `mutation_add_items` WHERE `id` = ?", id).Scan(&count); err != nil {
		t.Fatalf("verify deleted row directly: %v", err)
	}
	if count != 0 {
		t.Fatalf("row with unsupported unrelated column was not deleted: count=%d", count)
	}
}

func TestMutationPolicyDeleteDoesNotApplyAutoFillSchemaRules(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_auto_fill_items", `{
		"allow_delete":true,
		"auto_fill":{"modify":{"creator":{"source":"operator"}}}
	}`)

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, `
INSERT INTO mutation_auto_fill_items (id, code, creator, occurred_at, status, quantity)
VALUES (1, 'delete-stale-autofill', 'initial', '2026-01-01 00:00:00', 'initial', 1)`); err != nil {
		t.Fatalf("seed Auto Fill delete row: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_auto_fill_items` DROP COLUMN `creator`"); err != nil {
		t.Fatalf("remove now-unrelated Auto Fill column: %v", err)
	}

	deleted := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_auto_fill_items/rows/1", "")
	assertMutationAffected(t, deleted)
	assertMutationRowAbsent(t, app, "mutation_auto_fill_items", "delete-stale-autofill")
}

func TestMutationPolicyDeleteFailsClosedForInvalidCurrentPolicyAndLiveTable(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true,"allow_delete":true}`)
	id := addMutationPatchFixtureRow(t, app, "delete-invalid-policy", "must remain")
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'unknown_mutation' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("make current strategy unknown: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_strategy")
	assertDirectMutationRowCount(t, ctx, database, "mutation_add_items", id, 1)

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'mysql_single_table_mutation_v1', `mutation_policy_config` = '{' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("make current config malformed: %v", err)
	}
	malformed := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/"+id, "")
	assertIntegrationErrorCode(t, malformed, http.StatusUnprocessableEntity, "invalid_policy_config")
	assertDirectMutationRowCount(t, ctx, database, "mutation_add_items", id, 1)

	enableMutationPolicy(t, app, "mutation_supplied_id_items", `{"allow_delete":true}`)
	if _, err := database.ExecContext(ctx, "INSERT INTO `mutation_supplied_id_items` (`id`, `label`) VALUES ('incompatible-id', 'must remain')"); err != nil {
		t.Fatalf("seed incompatible Schema row: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_supplied_id_items` DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_supplied_id_items/rows/incompatible-id", "")
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
	assertDirectMutationRowCount(t, ctx, database, "mutation_supplied_id_items", "incompatible-id", 1)

	enableMutationPolicy(t, app, "mutation_auto_fill_items", `{"allow_delete":true}`)
	if _, err := database.ExecContext(ctx, "DROP TABLE `mutation_auto_fill_items`"); err != nil {
		t.Fatalf("drop current physical table: %v", err)
	}
	missingTable := policyIntegrationRequest(app, http.MethodDelete, "/api/v1/tables/mutation_auto_fill_items/rows/1", "")
	assertIntegrationErrorCode(t, missingTable, http.StatusNotFound, "database_table_not_found")
}

func TestMutationAddFailsClosedAndUsesTheLatestPolicySnapshot(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)

	missing := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"missing","label":"x"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "table_policy_not_found")

	policy := integrationPolicyPayload("mutation_add_items", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{"allow_add":false}`)
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", policy); response.Code != http.StatusCreated {
		t.Fatalf("create disabled Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	disabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"disabled","label":"x"}}`)
	assertIntegrationErrorCode(t, disabled, http.StatusForbidden, "table_policy_disabled")
	if response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/mutation_add_items/enable", ""); response.Code != http.StatusOK {
		t.Fatalf("enable Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	forbidden := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"forbidden","label":"x"}}`)
	assertIntegrationErrorCode(t, forbidden, http.StatusForbidden, "mutation_not_allowed")

	replacement := integrationPolicyPayload("mutation_add_items", "mysql_page_query_v1", `{}`, "mysql_single_table_mutation_v1", `{"allow_add":true}`)
	if response := policyIntegrationRequest(app, http.MethodPut, "/api/v1/table-policies/mutation_add_items", replacement); response.Code != http.StatusOK {
		t.Fatalf("replace current Policy: HTTP %d %s", response.Code, response.Body.String())
	}
	added := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"current","label":"latest Policy"}}`)
	if added.Code != http.StatusCreated {
		t.Fatalf("latest Policy Snapshot did not permit ADD: HTTP %d %s", added.Code, added.Body.String())
	}

	protected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/rcc_table_policies/rows", `{"content":{}}`)
	assertIntegrationErrorCode(t, protected, http.StatusForbidden, "protected_table")
}

func TestMutationAddFailsClosedForInvalidLivePolicyAndSchema(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", `{"allow_add":true}`)

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'unknown_mutation' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("make current Policy invalid: %v", err)
	}
	unknown := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"unknown-policy","label":"x"}}`)
	assertIntegrationErrorCode(t, unknown, http.StatusUnprocessableEntity, "unknown_policy_strategy")

	if _, err := database.ExecContext(ctx, "UPDATE `rcc_table_policies` SET `mutation_policy` = 'mysql_single_table_mutation_v1' WHERE `table_name` = 'mutation_add_items'"); err != nil {
		t.Fatalf("restore current Policy: %v", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE `mutation_add_items` MODIFY `id` bigint unsigned NOT NULL, DROP PRIMARY KEY"); err != nil {
		t.Fatalf("make live Schema incompatible: %v", err)
	}
	incompatible := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"incompatible","label":"x"}}`)
	assertIntegrationErrorCode(t, incompatible, http.StatusUnprocessableEntity, "incompatible_table")
}

func TestMutationAddFailsClosedWhenPolicyCatalogIsUnavailable(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/006-mutation-fixture.sql",
	)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })

	database, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.ExecContext(context.Background(), "DROP TABLE `rcc_table_policies`"); err != nil {
		t.Fatalf("make Policy Catalog unavailable: %v", err)
	}

	rejected := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"unavailable","label":"x"}}`)
	assertIntegrationErrorCode(t, rejected, http.StatusServiceUnavailable, "policy_catalog_unavailable")
}

func queryMutationRow(t *testing.T, app *adminApplication, code string) map[string]*string {
	return queryMutationTableRow(t, app, "mutation_add_items", code)
}

func addMutationPatchFixtureRow(t *testing.T, app *adminApplication, code, label string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"content": map[string]any{
			"code":           code,
			"label":          label,
			"nullable_value": "preserved",
			"quantity":       "7",
		},
	})
	if err != nil {
		t.Fatalf("encode mutation fixture row: %v", err)
	}
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", string(body))
	return mutationResponseID(t, response)
}

func mutationResponseID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	if response.Code != http.StatusCreated {
		t.Fatalf("add mutation fixture row: HTTP %d %s", response.Code, response.Body.String())
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode mutation fixture id: %v", err)
	}
	if body.ID == "" {
		t.Fatalf("mutation fixture returned empty id: %s", response.Body.String())
	}
	return body.ID
}

func assertMutationAffected(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("modify row: expected HTTP 200, got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Affected int64 `json:"affected"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode modify response: %v", err)
	}
	if body.Affected != 1 {
		t.Fatalf("modify row: expected affected 1, got %d: %s", body.Affected, response.Body.String())
	}
}

func queryMutationTableRow(t *testing.T, app *adminApplication, tableName, code string) map[string]*string {
	t.Helper()
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/"+tableName+"/query", `{
		"conditions":[{"field":"code","operator":"exact","value":"`+code+`"}]
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query mutation row %q: HTTP %d %s", code, response.Code, response.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected one row for %q, got %s", code, response.Body.String())
	}
	return result.Rows[0]
}

func assertMutationString(t *testing.T, row map[string]*string, field, expected string) {
	t.Helper()
	if row[field] == nil || *row[field] != expected {
		t.Fatalf("expected %s=%q, got %#v", field, expected, row[field])
	}
}

func assertMutationRowAbsent(t *testing.T, app *adminApplication, tableName, code string) {
	t.Helper()
	response := policyIntegrationRequest(app, http.MethodPost, "/api/v1/tables/"+tableName+"/query", `{
		"conditions":[{"field":"code","operator":"exact","value":"`+code+`"}]
	}`)
	if response.Code != http.StatusOK {
		t.Fatalf("query absent mutation row %q: HTTP %d %s", code, response.Code, response.Body.String())
	}
	var result struct {
		Rows []map[string]*string `json:"rows"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode query response: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("mutation row %q should be absent after rejection/rollback: %s", code, response.Body.String())
	}
}

func assertDirectMutationRowCount(t *testing.T, ctx context.Context, database *sql.DB, tableName, id string, expected int) {
	t.Helper()
	var count int
	query := "SELECT COUNT(*) FROM `" + tableName + "` WHERE `id` = ?"
	if err := database.QueryRowContext(ctx, query, id).Scan(&count); err != nil {
		t.Fatalf("count direct mutation row: %v", err)
	}
	if count != expected {
		t.Fatalf("expected direct row count %d, got %d", expected, count)
	}
}

func enableMutationPolicy(t *testing.T, app *adminApplication, tableName, mutationConfig string) {
	t.Helper()
	created := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies", integrationPolicyPayload(
		tableName,
		"mysql_page_query_v1",
		`{}`,
		"mysql_single_table_mutation_v1",
		mutationConfig,
	))
	if created.Code != http.StatusCreated {
		t.Fatalf("create Table Policy: HTTP %d %s", created.Code, created.Body.String())
	}
	enabled := policyIntegrationRequest(app, http.MethodPost, "/api/v1/table-policies/"+tableName+"/enable", "")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable Table Policy: HTTP %d %s", enabled.Code, enabled.Body.String())
	}
}
