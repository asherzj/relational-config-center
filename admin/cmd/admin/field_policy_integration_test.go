//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// AC-001 observes the management HTTP contract with real MySQL persistence.
func TestFieldPolicyAC001RoundTrip(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/002-discovery-fixture.sql")
	path := "/api/v1/table-field-policies/managed_alpha"
	first := policyIntegrationRequest(t, app, http.MethodGet, path, "")
	if first.Code != 200 {
		t.Fatalf("read real fields: %d %s", first.Code, first.Body.String())
	}
	const body = `{"policies":[{"field_name":"value","display_name":"配置值","description":"业务说明","display_order":2,"is_visible":true,"is_queryable":true,"query_operators":["exact","in"],"ui_type":"select","ui_options":{"options":[{"label":"开启","value":"on"},{"label":"关闭","value":"off"}]},"editable_on_add":true,"editable_on_modify":true,"is_required":true,"default_value":"on","enabled":true}]}`
	for i := 0; i < 2; i++ {
		saved := policyIntegrationRequest(t, app, http.MethodPut, path, body)
		if saved.Code != 200 {
			t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
		}
	}
	got := policyIntegrationRequest(t, app, http.MethodGet, path, "")
	var response struct {
		Fields []struct {
			FieldName string         `json:"field_name"`
			State     string         `json:"state"`
			Policy    map[string]any `json:"policy"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(got.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Fields) != 2 {
		t.Fatalf("real fields duplicated or missing: %s", got.Body.String())
	}
	for _, f := range response.Fields {
		if f.FieldName == "value" {
			if f.State != "active" || f.Policy["display_name"] != "配置值" || f.Policy["default_value"] != "on" || f.Policy["ui_type"] != "select" {
				t.Fatalf("incomplete roundtrip: %s", got.Body.String())
			}
			return
		}
	}
	t.Fatal("value field missing")
}

// AC-002: rejected complete replacements cannot overwrite any stored field.
func TestFieldPolicyAC002InvalidConfigurationIsAtomic(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/002-discovery-fixture.sql")
	path := "/api/v1/table-field-policies/managed_alpha"
	valid := `{"policies":[{"field_name":"value","display_name":"原名称","display_order":0,"is_visible":true,"is_queryable":true,"query_operators":["exact"],"ui_type":"text","ui_options":{"options":[]},"editable_on_add":true,"editable_on_modify":true,"is_required":false,"enabled":true}]}`
	saved := policyIntegrationRequest(t, app, "PUT", path, valid)
	if saved.Code != 200 {
		t.Fatal(saved.Body.String())
	}
	cases := []string{
		strings.Replace(valid, `"ui_type":"text"`, `"ui_type":"auto"`, 1),
		strings.Replace(valid, `["exact"]`, `["arbitrary_sql"]`, 1),
		strings.Replace(valid, `["exact"]`, `[]`, 1),
		strings.Replace(valid, `"ui_type":"text"`, `"ui_type":"number"`, 1),
		strings.Replace(valid, `"ui_options":{"options":[]}`, `"ui_options":{"options":[{"label":"A","value":"x"}]}`, 1),
		strings.Replace(valid, `"ui_type":"text","ui_options":{"options":[]}`, `"ui_type":"radio","ui_options":{"options":[{"label":"A","value":"x"},{"label":"B","value":"x"}]}`, 1),
		strings.Replace(valid, `"enabled":true`, `"default_value":null,"enabled":true`, 1),
		strings.Replace(valid, `"enabled":true`, `"default_value":42,"enabled":true`, 1),
		strings.Replace(valid, `"editable_on_add":true`, `"editable_on_add":false`, 1),
		strings.Replace(valid, `"field_name":"value"`, `"field_name":"invented"`, 1),
	}
	for _, body := range cases {
		result := policyIntegrationRequest(t, app, "PUT", path, body)
		if result.Code != 422 || !strings.Contains(result.Body.String(), `"invalid_field_policy"`) {
			t.Fatalf("want configuration rejection, got %d %s for %s", result.Code, result.Body.String(), body)
		}
		got := policyIntegrationRequest(t, app, "GET", path, "")
		if got.Body.String() != saved.Body.String() {
			t.Fatalf("rejected replacement changed catalog: %s", got.Body.String())
		}
	}
	viewer := registerAccount(t, app, "field.viewer", "field.viewer@example.com", "correct horse battery staple")
	denied := accountRequest(app, "PUT", path, valid, viewer.Result().Cookies(), sessionCSRF(t, viewer))
	if denied.Code != 403 {
		t.Fatalf("viewer save: %d %s", denied.Code, denied.Body.String())
	}
}

// AC-020 uses the same isolated database before/after the additive upgrade.
func TestFieldPolicyAC020MigrationPreservesExistingData(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, localManagedTableFixture)
	db := deliveryDB(t, driver)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	session := integrationAdminSession(t, app)
	draft := releaseActorRequest(t, app, session, "POST", "/api/v1/release-orders", `{"title":"迁移前保留发布单","table_name":"notification_templates","items":[{"operation":"ADD","content":{"template_key":"migration-keep","channel":"EMAIL","subject":"keep","body":"keep","enabled":"1","priority":"1","metadata":"{}"}}]}`, "field-migration-draft")
	if draft.Code != 201 {
		t.Fatalf("prepare old draft: %d %s", draft.Code, draft.Body.String())
	}
	var table, initial string
	if err := db.QueryRow("SHOW CREATE TABLE rcc_table_field_policies").Scan(&table, &initial); err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, db, "DROP TABLE rcc_table_field_policies")
	sqlBytes, err := os.ReadFile("../../../deploy/mysql/migrations/014-table-field-policies.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		deliveryExec(t, db, string(sqlBytes))
	}
	var upgraded string
	if err := db.QueryRow("SHOW CREATE TABLE rcc_table_field_policies").Scan(&table, &upgraded); err != nil {
		t.Fatal(err)
	}
	if initial != upgraded {
		t.Fatalf("fresh/upgrade schema differs:\n%s\n%s", initial, upgraded)
	}
	var drafts, rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_release_orders").Scan(&drafts); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM notification_templates").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if drafts != 1 || rows != 3 {
		t.Fatalf("upgrade changed old data: drafts=%d business rows=%d", drafts, rows)
	}
	deliveryExec(t, db, `INSERT INTO rcc_table_field_policies(table_name,field_name,display_name,creator,modifier) VALUES('notification_templates','channel','渠道','test','test')`)
	if _, err := db.Exec(`INSERT INTO rcc_table_field_policies(table_name,field_name,display_name,creator,modifier) VALUES('notification_templates','channel','渠道','test','test')`); err == nil {
		t.Fatal("missing table/field unique constraint")
	}
	if _, err := db.Exec(`UPDATE rcc_table_field_policies SET is_visible=2`); err == nil {
		t.Fatal("missing boolean CHECK")
	}
}

func TestFieldPolicyReadDistinguishesDisabledSchemaDriftAndFailure(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, localManagedTableFixture)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	db := deliveryDB(t, driver)
	path := "/api/v1/table-field-policies/notification_templates"
	body := `{"policies":[{"field_name":"priority","display_name":"优先级","is_visible":true,"is_queryable":true,"query_operators":["exact"],"ui_type":"number","editable_on_add":true,"editable_on_modify":true,"enabled":true}]}`
	saved := policyIntegrationRequest(t, app, "PUT", path, body)
	if saved.Code != 200 {
		t.Fatal(saved.Body.String())
	}
	deliveryExec(t, db, "ALTER TABLE notification_templates MODIFY priority varchar(32) NOT NULL DEFAULT '1'")
	drift := policyIntegrationRequest(t, app, "GET", path, "")
	if drift.Code != 200 || !strings.Contains(drift.Body.String(), `"state":"incompatible"`) {
		t.Fatalf("schema drift: %d %s", drift.Code, drift.Body.String())
	}
	disable := policyIntegrationRequest(t, app, "PUT", path, strings.Replace(body, `"enabled":true`, `"enabled":false`, 1))
	if disable.Code != 200 {
		t.Fatalf("disable unchanged incompatible rule: %d %s", disable.Code, disable.Body.String())
	}
	editedDisabled := policyIntegrationRequest(t, app, "PUT", path, strings.Replace(strings.Replace(body, `"enabled":true`, `"enabled":false`, 1), `"display_name":"优先级"`, `"display_name":"改过名称"`, 1))
	if editedDisabled.Code != 422 {
		t.Fatalf("editing incompatible disabled rule bypassed validation: %d %s", editedDisabled.Code, editedDisabled.Body.String())
	}
	newDisabled := policyIntegrationRequest(t, app, "PUT", path, strings.Replace(strings.Replace(body, `"enabled":true`, `"enabled":false`, 1), `"field_name":"priority"`, `"field_name":"subject"`, 1))
	if newDisabled.Code != 422 {
		t.Fatalf("new incompatible disabled rule bypassed validation: %d %s", newDisabled.Code, newDisabled.Body.String())
	}
	reenabling := policyIntegrationRequest(t, app, "PUT", path, body)
	if reenabling.Code != 422 {
		t.Fatalf("reenable incompatible rule: %d %s", reenabling.Code, reenabling.Body.String())
	}
	disabled := policyIntegrationRequest(t, app, "GET", path, "")
	if disabled.Code != 200 || !strings.Contains(disabled.Body.String(), `"state":"disabled"`) {
		t.Fatalf("disabled: %d %s", disabled.Code, disabled.Body.String())
	}
	// A fresh read is driven by the real schema: stale catalog rows never create fields.
	deliveryExec(t, db, "ALTER TABLE notification_templates DROP COLUMN priority, ADD COLUMN recovery_note varchar(64) NULL")
	current := policyIntegrationRequest(t, app, "GET", path, "")
	var metadata struct {
		Fields []struct {
			Name      string `json:"field_name"`
			State     string `json:"state"`
			Effective struct {
				UIType    string   `json:"ui_type"`
				Operators []string `json:"query_operators"`
			} `json:"effective"`
		} `json:"fields"`
	}
	if current.Code != 200 || json.Unmarshal(current.Body.Bytes(), &metadata) != nil {
		t.Fatalf("read changed schema: %d %s", current.Code, current.Body.String())
	}
	foundNew := false
	for _, field := range metadata.Fields {
		if field.Name == "priority" {
			t.Fatal("removed field manufactured from stale catalog")
		}
		if field.Name == "recovery_note" {
			foundNew = true
			if field.State != "missing" || field.Effective.UIType != "text" || len(field.Effective.Operators) != 1 || field.Effective.Operators[0] != "exact" {
				t.Fatalf("new field defaults: %s", current.Body.String())
			}
		}
	}
	if !foundNew {
		t.Fatal("new real field omitted")
	}
	deliveryExec(t, db, "RENAME TABLE rcc_table_field_policies TO unavailable_field_policies")
	failed := policyIntegrationRequest(t, app, "GET", path, "")
	if failed.Code != 503 || !strings.Contains(failed.Body.String(), `"field_policy_unavailable"`) {
		t.Fatalf("failed read falsely became missing: %d %s", failed.Code, failed.Body.String())
	}
	deliveryExec(t, db, "RENAME TABLE unavailable_field_policies TO rcc_table_field_policies")
	retried := policyIntegrationRequest(t, app, "GET", path, "")
	if retried.Code != 200 || retried.Body.String() != current.Body.String() {
		t.Fatalf("independent retry did not restore metadata: %d %s", retried.Code, retried.Body.String())
	}
}

func TestFieldPolicyDefaultsNumericOptionsAndAudit(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, localManagedTableFixture)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	db := deliveryDB(t, driver)
	path := "/api/v1/table-field-policies/notification_templates"
	policy := map[string]any{"field_name": "subject", "display_name": "标题", "display_order": 4294967295, "is_visible": true, "is_queryable": true, "query_operators": []string{"exact"}, "ui_type": "text", "ui_options": map[string]any{"options": []any{}}, "editable_on_add": true, "editable_on_modify": true, "is_required": false, "enabled": true}
	save := func(p map[string]any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"policies": []any{p}})
		return policyIntegrationRequest(t, app, "PUT", path, string(body))
	}
	// subject is nullable: omission, JSON null and the empty string must survive storage distinctly.
	for _, mode := range []string{"none", "null", "empty"} {
		delete(policy, "default_value")
		if mode == "null" {
			policy["default_value"] = nil
		}
		if mode == "empty" {
			policy["default_value"] = ""
		}
		response := save(policy)
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", mode, response.Code, response.Body.String())
		}
		var sqlNull bool
		var jsonType sql.NullString
		if err := db.QueryRow("SELECT default_value IS NULL,JSON_TYPE(default_value) FROM rcc_table_field_policies WHERE field_name='subject'").Scan(&sqlNull, &jsonType); err != nil {
			t.Fatal(err)
		}
		if mode == "none" && !sqlNull || mode == "null" && (sqlNull || jsonType.String != "NULL") || mode == "empty" && (sqlNull || jsonType.String != "STRING") {
			t.Fatalf("lost default distinction %s %v %#v", mode, sqlNull, jsonType)
		}
		var result struct {
			Fields []struct {
				FieldName string                     `json:"field_name"`
				Policy    map[string]json.RawMessage `json:"policy"`
			} `json:"fields"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		for _, f := range result.Fields {
			if f.FieldName == "subject" {
				value, present := f.Policy["default_value"]
				if mode == "none" && present || mode == "null" && string(value) != "null" || mode == "empty" && string(value) != `""` {
					t.Fatalf("HTTP default distinction lost: %s", response.Body.String())
				}
			}
		}
	}
	var creator, created string
	var id uint64
	if err := db.QueryRow("SELECT id,creator,CAST(created_at AS CHAR) FROM rcc_table_field_policies").Scan(&id, &creator, &created); err != nil {
		t.Fatal(err)
	}
	policy["display_name"] = strings.Repeat("名", 200)
	response := save(policy)
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var laterID uint64
	var laterCreator, laterCreated string
	if err := db.QueryRow("SELECT id,creator,CAST(created_at AS CHAR) FROM rcc_table_field_policies").Scan(&laterID, &laterCreator, &laterCreated); err != nil {
		t.Fatal(err)
	}
	if id != laterID || creator != laterCreator || created != laterCreated {
		t.Fatal("replacement rewrote identity or creation audit")
	}
	for _, name := range []string{" ", strings.Repeat("名", 201)} {
		policy["display_name"] = name
		if r := save(policy); r.Code != 422 {
			t.Fatalf("bad display name: %d %s", r.Code, r.Body.String())
		}
	}
	policy["display_name"] = "标题"
	policy["display_order"] = -1
	if r := save(policy); r.Code != 422 {
		t.Fatalf("negative order: %d", r.Code)
	}
	policy["display_order"] = 4294967296
	if r := save(policy); r.Code != 422 {
		t.Fatalf("oversized order: %d", r.Code)
	}
	policy["display_order"] = 0
	policy["field_name"] = "priority"
	policy["ui_type"] = "number"
	policy["ui_options"] = map[string]any{"options": []any{}, "min": "0", "max": "20", "step": "2"}
	policy["default_value"] = "10"
	if r := save(policy); r.Code != 200 {
		t.Fatalf("numeric constraints: %d %s", r.Code, r.Body.String())
	}
	for _, options := range []map[string]any{{"min": "21", "max": "20"}, {"step": "0"}, {"step": "3"}, {"min": "11"}, {"max": "9"}, {"min": "invalid"}} {
		policy["ui_options"] = options
		if r := save(policy); r.Code != 422 {
			t.Fatalf("bad numeric constraints %#v: %d %s", options, r.Code, r.Body.String())
		}
	}
}

func TestFieldPolicyDatabaseFailureRollsBackWholeReplacement(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/002-discovery-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	db := deliveryDB(t, driver)
	path := "/api/v1/table-field-policies/managed_alpha"
	body := `{"policies":[{"field_name":"id","display_name":"编号","is_visible":true,"is_queryable":false,"ui_type":"text","editable_on_add":false,"enabled":true},{"field_name":"value","display_name":"原值","is_visible":true,"is_queryable":false,"ui_type":"text","editable_on_add":true,"enabled":true}]}`
	saved := policyIntegrationRequest(t, app, "PUT", path, body)
	if saved.Code != 200 {
		t.Fatal(saved.Body.String())
	}
	// A real database boundary failure on the second field occurs after the first UPDATE.
	deliveryExec(t, db, `ALTER TABLE rcc_table_field_policies ADD CONSTRAINT injected_field_failure CHECK (field_name <> 'value' OR display_name <> '新值')`)
	rejected := policyIntegrationRequest(t, app, "PUT", path, strings.ReplaceAll(strings.ReplaceAll(body, "编号", "新编号"), "原值", "新值"))
	if rejected.Code != 503 {
		t.Fatalf("want unavailable: %d %s", rejected.Code, rejected.Body.String())
	}
	got := policyIntegrationRequest(t, app, "GET", path, "")
	if got.Body.String() != saved.Body.String() {
		t.Fatal("database failure left partial replacement")
	}
}

// AC-009/018: interaction flags and static options never become execution authority.
func TestFieldPolicyAC009AC018KeepDatabaseDefaultsAndExecutionBoundary(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	body := `{"policies":[{"field_name":"defaulted_value","display_name":"Web不可编辑","display_order":0,"is_visible":false,"is_queryable":false,"query_operators":[],"ui_type":"select","ui_options":{"options":[{"label":"建议值","value":"suggested"}]},"editable_on_add":false,"editable_on_modify":false,"is_required":true,"enabled":true}]}`
	saved := policyIntegrationRequest(t, app, "PUT", "/api/v1/table-field-policies/mutation_add_items", body)
	if saved.Code != 200 {
		t.Fatalf("field configuration: %d %s", saved.Code, saved.Body.String())
	}
	for _, item := range []struct{ code, content, expected string }{
		{"field-default", `"nullable_value":null`, "database-default"},
		{"field-custom", `"defaulted_value":"outside-options","nullable_value":null`, "outside-options"},
		{"field-empty", `"defaulted_value":"","nullable_value":""`, ""},
	} {
		result := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"`+item.code+`","label":"valid",`+item.content+`}}`)
		if result.Code != 200 {
			t.Fatalf("publish legal content despite Web flags: %d %s", result.Code, result.Body.String())
		}
		row := queryMutationRow(t, app, item.code)
		assertMutationString(t, row, "defaulted_value", item.expected)
		if item.code == "field-empty" {
			assertMutationString(t, row, "nullable_value", "")
		} else if row["nullable_value"] != nil {
			t.Fatal("explicit NULL was lost")
		}
	}
	for _, item := range []struct{ operation, content, code string }{
		{"ADD", `{"code":"bad-number","label":"valid","quantity":"not-a-number"}`, "invalid_mutation_content"},
		{"ADD", `{"code":"bad-generated","label":"valid","generated_value":"override"}`, "invalid_mutation_content"},
		{"MODIFY", `{"label":"illegal"}`, "mutation_not_allowed"},
	} {
		result := publicationFixtureRequest(t, app, item.operation, "mutation_add_items", "1", `{"content":`+item.content+`}`)
		status := http.StatusUnprocessableEntity
		if item.code == "mutation_not_allowed" {
			status = http.StatusForbidden
		}
		assertIntegrationErrorCode(t, result, status, item.code)
	}
}
