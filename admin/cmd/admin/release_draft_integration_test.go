//go:build integration

package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func releaseRequest(t *testing.T, app *adminApplication, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	session := integrationAdminSession(t, app)
	return accountRequestFrom(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
}

// AC-010: the public draft contract persists the applicant's readable title.
func TestReleaseDraftTitlePersistsAndReloads(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"items":[{"content":{"code":"proposed"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"渠道配置 🚀 变更"}`
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-title-create")
	if created.Code != 201 {
		t.Fatalf("create titled draft: %d %s", created.Code, created.Body)
	}
	var order struct{ ID, Title string }
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	if order.ID == "" || order.Title != "渠道配置 🚀 变更" {
		t.Fatalf("titled draft: %s", created.Body)
	}
	read := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders/"+order.ID, "", "")
	if read.Code != 200 || read.Body.String() != created.Body.String() {
		t.Fatalf("reload titled draft: %d %s", read.Code, read.Body)
	}
	list := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders?limit=20", "", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), `"title":"渠道配置 🚀 变更"`) {
		t.Fatalf("list titled draft: %d %s", list.Code, list.Body)
	}
}

// AC-010: title length is counted in Unicode code points and blank titles are invalid.
func TestReleaseDraftTitleUnicodeValidation(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	request := func(title, key string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]any{
			"title": title,
			"items": []any{map[string]any{"table_name": "mutation_delete_parents", "operation": "MODIFY", "id": "1", "expected_record_version": "0", "content": map[string]string{"code": "proposed"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), key)
	}
	assertIntegrationErrorCode(t, request(" \t ", "draft-title-blank"), 422, "release_title_invalid")
	assertIntegrationErrorCode(t, request(strings.Repeat("界", 101), "draft-title-long"), 422, "release_title_invalid")
	if accepted := request(strings.Repeat("🚀", 100), "draft-title-boundary"); accepted.Code != 201 {
		t.Fatalf("100-character title rejected: %d %s", accepted.Code, accepted.Body)
	}
}

// AC-010: title and item changes share one draft CAS and freeze together on submit.
func TestReleaseDraftTitleEditCASAndFreeze(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"first"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"原始标题"}`, "draft-title-cas-create")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var draft struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + draft.ID
	updated := releaseRequest(t, app, "PUT", path, `{"expected_version":"1","items":[{"content":{"code":"second"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"审批前的新标题"}`, "draft-title-cas-update")
	if updated.Code != 200 || !strings.Contains(updated.Body.String(), `"title":"审批前的新标题"`) || !strings.Contains(updated.Body.String(), `"code":"second"`) {
		t.Fatalf("update title and content: %d %s", updated.Code, updated.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"1","items":[{"content":{"code":"stale"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"过期窗口标题"}`, "draft-title-cas-stale"), 409, "release_version_conflict")
	current := releaseReadAllDetails(t, app, "GET", path, "", "")
	if current.Body.String() != updated.Body.String() {
		t.Fatalf("stale update changed title or detail: %s", current.Body)
	}
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"2"}`, "draft-title-submit")
	if submitted.Code != 200 || !strings.Contains(submitted.Body.String(), `"title":"审批前的新标题"`) {
		t.Fatalf("submit titled draft: %d %s", submitted.Code, submitted.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"3","items":[{"content":{"code":"changed"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"提交后篡改"}`, "draft-title-frozen"), 422, "release_state_invalid")
}

// AC-012: the authenticated public API persists an intent, never a business write.
func TestReleaseDraftSaveAndReload(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"proposed"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "draft-create-0001")
	if saved.Code != 201 {
		t.Fatalf("save draft: %d %s", saved.Code, saved.Body)
	}
	var order struct {
		ID, State, Version string
		Items              []struct {
			Before  map[string]*string
			Content map[string]*string
		}
	}
	if err := json.Unmarshal(saved.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	if order.ID == "" || order.State != "DRAFT" || order.Version != "1" || len(order.Items) != 1 || *order.Items[0].Before["code"] != "delete-rollback" {
		t.Fatalf("draft: %s", saved.Body)
	}
	read := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders/"+order.ID, "", "")
	if read.Code != 200 || read.Body.String() != saved.Body.String() {
		t.Fatalf("reload: %s", read.Body)
	}
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if *row["code"] != "delete-rollback" || version != "0" {
		t.Fatalf("draft wrote business data: %v %s", row, version)
	}
}

// AC-013/016/017: stale windows cannot overwrite; retries preserve the original result.
func TestReleaseDraftCASCancelAndIdempotency(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	body := `{"items":[{"content":{"code":"first"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	first := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-create-0002")
	if first.Code != 201 {
		t.Fatal(first.Body)
	}
	repeat := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-create-0002")
	if repeat.Body.String() != first.Body.String() {
		t.Fatalf("different retry result: %s", repeat.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(first.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	update := `{"expected_version":"1","items":[{"content":{"code":"second"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	saved := releaseRequest(t, app, "PUT", path, update, "draft-update-0001")
	if saved.Code != 200 {
		t.Fatalf("update: %d %s", saved.Code, saved.Body)
	}
	retry := releaseRequest(t, app, "PUT", path, update, "draft-update-0001")
	if retry.Body.String() != saved.Body.String() {
		t.Fatalf("old-version retry: %s", retry.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, update, "draft-update-0002"), 409, "release_version_conflict")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", update, "draft-create-0002"), 409, "idempotency_conflict")
	cancel := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"2","reason":"no longer needed"}`, "draft-cancel-0001")
	if cancel.Code != 200 {
		t.Fatalf("cancel: %d %s", cancel.Code, cancel.Body)
	}
	if again := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"2","reason":"no longer needed"}`, "draft-cancel-0001"); again.Body.String() != cancel.Body.String() {
		t.Fatal(again.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"3","reason":"again"}`, "draft-cancel-0002"), 422, "release_state_invalid")
	read := releaseReadAllDetails(t, app, "GET", path, "", "")
	if read.Body.String() != cancel.Body.String() {
		t.Fatal(read.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "DELETE", path, "", ""), 400, "method_not_allowed")
}

// AC-014: a saved diff preserves each field state and defers all automatic values.
func TestReleaseDraftDiffAndServerBaseline(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	body := `{"items":[{"content":{"code":"draft","label":"","metadata":"null","nullable_value":null},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	r := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-semantics-01")
	if r.Code != 201 {
		t.Fatalf("ADD draft: %d %s", r.Code, r.Body)
	}
	var order struct {
		Items []struct {
			Before any
			Fields []struct {
				Name, Type, BeforeState, ProposedState string
				Proposed                               *string
			}
		}
	}
	// Use explicit public names rather than relying on case-insensitive struct matching.
	var raw map[string]any
	_ = json.Unmarshal(r.Body.Bytes(), &raw)
	item := raw["items"].([]any)[0].(map[string]any)
	if item["before"] != nil || item["id"] != nil {
		t.Fatalf("invented initial row: %s", r.Body)
	}
	states := map[string]string{}
	for _, v := range item["fields"].([]any) {
		f := v.(map[string]any)
		states[f["name"].(string)] = f["proposed_state"].(string)
		if f["before_state"] != "absent" {
			t.Fatal(f)
		}
	}
	if states["label"] != "value" || states["nullable_value"] != "sql_null" || states["defaulted_value"] != "omitted" || states["generated_value"] != "generated" {
		t.Fatalf("states: %v", states)
	}
	_ = order
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{AllowAdd: true, CreateOperatorField: releaseString("creator"), CreateTimeField: releaseString("occurred_at")})
	auto := `{"items":[{"content":{"code":"auto","quantity":"1","status":"active"},"operation":"ADD","table_name":"mutation_auto_fill_items"}],"title":"集成测试发布单"}`
	r = releaseRequest(t, app, "POST", "/api/v1/release-orders", auto, "draft-auto-fill-01")
	if r.Code != 201 {
		t.Fatalf("automatic draft: %d %s", r.Code, r.Body)
	}
	_ = json.Unmarshal(r.Body.Bytes(), &raw)
	item = raw["items"].([]any)[0].(map[string]any)
	for _, v := range item["fields"].([]any) {
		f := v.(map[string]any)
		if f["name"] == "creator" || f["name"] == "occurred_at" {
			if f["proposed_state"] != "automatic" || f["proposed"] != nil {
				t.Fatalf("premature auto-fill: %v", f)
			}
		}
	}
	forged := `{"items":[{"content":{"code":"auto","creator":"forged","quantity":"1","status":"active"},"operation":"ADD","table_name":"mutation_auto_fill_items"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", forged, "draft-forged-auto"), 422, "invalid_mutation_content")
	before := `{"items":[{"before":{"label":"forged"},"content":{"code":"draft","label":""},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", before, "draft-forged-before"), 400, "invalid_request")
}

func releaseString(s string) *string { return &s }

// A missing known ADD id keeps the database's comparison identity, tombstone,
// and maintenance generation. Draft reads must not initialize version resources.
func TestReleaseDraftMissingIdentityAndTombstone(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/010-record-identity-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for _, test := range []struct{ table, first, equivalent string }{
		{"record_identity_ci", "Résumé", "RESUME"}, {"record_identity_pad", "Code ", "CÓDE"}, {"record_identity_decimal", "+001.00", "1.0000"}, {"record_identity_timestamp", "2026-09-07T00:00:00Z", "2026-09-07T00:00:00.000000Z"},
	} {
		t.Run(test.table, func(t *testing.T) {
			enableMutationPolicy(t, app, test.table, mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
			body, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": []any{map[string]any{"table_name": test.table, "operation": "ADD", "content": map[string]string{"id": test.first, "label": "draft"}}}})
			draft := releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "draft-identity-initial-"+test.table)
			if draft.Code != 201 {
				t.Fatalf("missing identity: %d %s", draft.Code, draft.Body)
			}
			var saved map[string]any
			_ = json.Unmarshal(draft.Body.Bytes(), &saved)
			if saved["items"].([]any)[0].(map[string]any)["expected_record_version"] != "0" {
				t.Fatal(draft.Body)
			}
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM rcc_record_versions WHERE table_name=?", test.table).Scan(&count); err != nil || count != 0 {
				t.Fatalf("draft allocated versions: %d %v", count, err)
			}
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+saved["id"].(string)+"/cancel", `{"expected_version":"1","reason":"end missing baseline fixture"}`, "end-initial-"+test.table), 200)
			addBody, _ := json.Marshal(map[string]any{"content": map[string]string{"id": test.first, "label": "real"}})
			add := publicationFixtureRequest(t, app, "ADD", test.table, "", string(addBody))
			if add.Code != 200 {
				t.Fatal(add.Body)
			}
			deleted := publicationFixtureRequest(t, app, "DELETE", test.table, test.equivalent, `{"expected_version":"1"}`)
			assertMutationAffected(t, deleted)
			body, _ = json.Marshal(map[string]any{"title": "集成测试发布单", "items": []any{map[string]any{"table_name": test.table, "operation": "ADD", "content": map[string]string{"id": test.equivalent, "label": "recreated draft"}}}})
			draft = releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "draft-identity-tombstone-"+test.table)
			if draft.Code != 201 {
				t.Fatal(draft.Body)
			}
			_ = json.Unmarshal(draft.Body.Bytes(), &saved)
			if saved["items"].([]any)[0].(map[string]any)["expected_record_version"] != "2" {
				t.Fatalf("lost tombstone: %s", draft.Body)
			}
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+saved["id"].(string)+"/cancel", `{"expected_version":"1","reason":"end tombstone baseline fixture"}`, "end-tombstone-"+test.table), 200)
			if _, err := db.Exec("INSERT INTO rcc_record_versions VALUES(?,X'',9007199254740993)", test.table); err != nil {
				t.Fatal(err)
			}
			draft = releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "draft-identity-floor-"+test.table)
			if draft.Code != 201 {
				t.Fatal(draft.Body)
			}
			_ = json.Unmarshal(draft.Body.Bytes(), &saved)
			if saved["items"].([]any)[0].(map[string]any)["expected_record_version"] != "9007199254740993" {
				t.Fatalf("lost floor: %s", draft.Body)
			}
		})
	}
}

// AC-013/015: authorization uses the current session and the stable applicant;
// list/detail history is readable even after live schema or policy changes.
func TestReleaseDraftCurrentAuthorizationAndListing(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	admin := integrationAdminSession(t, app)
	editor := registerAccount(t, app, "draft.editor", "draft.editor@example.com", "correct horse battery staple")
	other := registerAccount(t, app, "draft.other", "draft.other@example.com", "correct horse battery staple")
	assign := func(account *httptest.ResponseRecorder, roles, version, key string) {
		t.Helper()
		response := accountRequestFrom(app, "PUT", "/api/v1/account-roles/"+accountID(t, account), `{"roles":`+roles+`,"expected_version":"`+version+`"}`, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
		if response.Code != 200 {
			t.Fatal(response.Body)
		}
	}
	req := func(account *httptest.ResponseRecorder, method, path, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, method, path, body, account.Result().Cookies(), sessionCSRF(t, account), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	}
	body := `{"items":[{"expected_record_version":"0","id":"1","operation":"DELETE","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, req(editor, "POST", "/api/v1/release-orders", body, "draft-role-0001"), 403, "permission_denied")
	assign(editor, `["PUBLISHER"]`, "1", "draft-publisher-001")
	assertIntegrationErrorCode(t, req(editor, "POST", "/api/v1/release-orders", body, "draft-role-0001"), 403, "permission_denied")
	assign(editor, `["EDITOR"]`, "2", "draft-editor-0001")
	saved := req(editor, "POST", "/api/v1/release-orders", body, "draft-role-0001")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct {
		ID          string
		ApplicantID string   `json:"applicant_id"`
		Actions     []string `json:"allowed_actions"`
	}
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	if order.ApplicantID != accountID(t, editor) || len(order.Actions) != 3 {
		t.Fatal(saved.Body)
	}
	path := "/api/v1/release-orders/" + order.ID
	assign(other, `["EDITOR"]`, "1", "draft-other-00001")
	update := `{"expected_version":"1","items":[{"content":{"code":"intruder"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, req(other, "PUT", path, update, "draft-other-edit-01"), 403, "permission_denied")
	assertIntegrationErrorCode(t, req(other, "POST", path+"/cancel", `{"expected_version":"1","reason":"intruder"}`, "draft-other-cancel"), 403, "permission_denied")
	read := req(other, "GET", path, "", "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"allowed_actions":[]`) {
		t.Fatal(read.Body)
	}
	assign(editor, `["VIEWER"]`, "3", "draft-revoke-001")
	assertIntegrationErrorCode(t, req(editor, "POST", "/api/v1/release-orders", body, "draft-role-0001"), 403, "permission_denied")
	assertIntegrationErrorCode(t, req(editor, "PUT", path, update, "draft-revoked-edit"), 403, "permission_denied")
	list := req(other, "GET", "/api/v1/release-orders?table_name=mutation_delete_parents&state=DRAFT&applicant_id="+order.ApplicantID+"&limit=1", "", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), order.ID) {
		t.Fatal(list.Body)
	}
	after := req(other, "GET", "/api/v1/release-orders?after="+order.ID, "", "")
	if after.Code != 200 || !strings.Contains(after.Body.String(), `"orders":[]`) {
		t.Fatal(after.Body)
	}
	missing := req(other, "GET", "/api/v1/release-orders/00000000000000000000000000000000", "", "")
	assertIntegrationErrorCode(t, missing, 404, "release_not_found")
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"administrator stopped"}`, "draft-admin-cancel")
	if cancelled.Code != 200 || !strings.Contains(cancelled.Body.String(), `"state":"CANCELLED"`) {
		t.Fatal(cancelled.Body)
	}
	// Removing a policy assignment never removes its existing release history.
	disabled := policyIntegrationRequest(t, app, "POST", "/api/v1/table-policies/mutation_delete_parents/disable", "")
	if disabled.Code != 200 {
		t.Fatal(disabled.Body)
	}
	read = releaseActorReadAllDetails(t, app, other, "GET", path, "", "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), "delete-rollback") {
		t.Fatal(read.Body)
	}
}

func TestReleaseDraftSchemaReadiness(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	if _, err := db.Exec("DROP TABLE rcc_release_requests"); err != nil {
		t.Fatal(err)
	}
	if app.mysql.Ready(ctx) == nil {
		t.Fatal("ready without durable release request storage")
	}
	migration, err := os.ReadFile("../../../deploy/mysql/migrations/010-release-drafts.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		for _, statement := range strings.Split(string(migration), ";") {
			if strings.TrimSpace(statement) != "" {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if err := app.mysql.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE rcc_release_requests ENGINE=MyISAM"); err != nil {
		t.Fatal(err)
	}
	if app.mysql.Ready(ctx) == nil {
		t.Fatal("ready with nontransactional release request storage")
	}
}

func TestReleaseDraftKnownAddUpdateRequiresOriginalBaseline(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	body := `{"items":[{"content":{"code":"draft","id":"7","label":"proposed"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-add-original")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	// External maintenance can invalidate an unpublished missing-row baseline.
	deliveryExec(t, db, `INSERT INTO rcc_record_versions(table_name,record_key,lock_version) SELECT table_name,record_key,2 FROM rcc_release_targets WHERE order_id=?`, order.ID)

	update := `{"expected_version":"1","items":[{"content":{"code":"draft","id":"7","label":"edited"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", "/api/v1/release-orders/"+order.ID, update, "draft-add-missing"), 422, "record_version_required")
	update = strings.Replace(update, `"operation":"ADD"`, `"operation":"ADD","expected_record_version":"0"`, 1)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", "/api/v1/release-orders/"+order.ID, update, "draft-add-stale"), 409, "record_version_conflict")
}

func TestReleaseDraftPreviewRebuildsMissingAddBaseline(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	db := deliveryDB(t, driver)
	if _, err := db.Exec("INSERT INTO rcc_record_versions VALUES('mutation_add_items',X'',9)"); err != nil {
		t.Fatal(err)
	}
	body := `{"items":[{"content":{"code":"preview","id":"7","label":"retained"},"expected_record_version":"0","operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	preview := releaseRequest(t, app, "POST", "/api/v1/release-orders/preview", body, "")
	if preview.Code != 200 {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body)
	}
	var result struct {
		Items []struct {
			Version string `json:"expected_record_version"`
		}
	}
	_ = json.Unmarshal(preview.Body.Bytes(), &result)
	if len(result.Items) != 1 || result.Items[0].Version != "9" {
		t.Fatal(preview.Body)
	}
	list := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders", "", "")
	if !strings.Contains(list.Body.String(), `"orders":[]`) {
		t.Fatal("preview persisted an order")
	}
	rebuilt := strings.Replace(body, `"expected_record_version":"0"`, `"expected_record_version":"9"`, 1)
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", rebuilt, "draft-rebuilt-add")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
}

// AC-013/017: real request and order locks converge competing retries and CAS.
// Failed persistence rolls back the order, history and request identity together.
func TestReleaseDraftConcurrentAndAtomicStorage(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	_ = integrationAdminSession(t, app)
	body := `{"items":[{"content":{"code":"retry"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	responses := make(chan *httptest.ResponseRecorder, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			responses <- releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-concurrent-create")
		}()
	}
	close(start)
	a, b := <-responses, <-responses
	if a.Code != 201 || b.Code != 201 || a.Body.String() != b.Body.String() {
		t.Fatalf("concurrent request results: %d %s / %d %s", a.Code, a.Body, b.Code, b.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(a.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	update := `{"expected_version":"1","items":[{"content":{"code":"next"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	start = make(chan struct{})
	for _, key := range []string{"draft-race-window1", "draft-race-window2"} {
		go func(key string) { <-start; responses <- releaseRequest(t, app, "PUT", path, update, key) }(key)
	}
	close(start)
	a, b = <-responses, <-responses
	if a.Code == 409 {
		a, b = b, a
	}
	if a.Code != 200 {
		t.Fatal(a.Body)
	}
	assertIntegrationErrorCode(t, b, 409, "release_version_conflict")
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	deliveryExec(t, owner, `INSERT INTO mutation_delete_parents(id,code) VALUES(2,'storage-fault-fixture')`)
	if _, err := owner.Exec(`CREATE TRIGGER reject_release_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.result IS NOT NULL AND OLD.result IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected release storage failure'; END IF; END`); err != nil {
		t.Fatal(err)
	}
	// A disjoint target reaches the storage fault after the earlier draft acquired id 1.
	body = strings.Replace(body, `"id":"1"`, `"id":"2"`, 1)
	failure := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-storage-failure")
	assertIntegrationErrorCode(t, failure, 503, "release_unavailable")
	list := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders", "", "")
	var result struct{ Orders []json.RawMessage }
	_ = json.Unmarshal(list.Body.Bytes(), &result)
	if len(result.Orders) != 1 {
		t.Fatalf("failed create remained: %s", list.Body)
	}
	if _, err := owner.Exec("DROP TRIGGER reject_release_result"); err != nil {
		t.Fatal(err)
	}
	recovered := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-storage-failure")
	if recovered.Code != 201 {
		t.Fatal(recovered.Body)
	}
	// A fresh application instance reads durable history and replays the original result.
	fresh, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fresh.Close() })
	session := integrationAdminSession(t, app)
	read := accountRequest(fresh, "GET", path, "", session.Result().Cookies(), "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"version":"2"`) {
		t.Fatal(read.Body)
	}
	repeat := accountRequestFrom(fresh, "POST", "/api/v1/release-orders", body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", map[string]string{"Idempotency-Key": "draft-storage-failure"})
	if repeat.Code != 201 || repeat.Body.String() != recovered.Body.String() {
		t.Fatal(repeat.Body)
	}
	for _, table := range []string{"rcc_release_orders", "rcc_release_requests"} {
		assertIntegrationErrorCode(t, policyIntegrationRequest(t, app, "POST", "/api/v1/tables/"+table+"/query", `{}`), 403, "protected_table")
	}
}

func TestReleaseDraftRejectsLossySnapshotAndRetainsSavedSchema(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS draft_binary(id INT PRIMARY KEY, payload BLOB)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO draft_binary VALUES(1,X'FF00')"); err != nil {
		t.Fatal(err)
	}
	enableMutationPolicy(t, app, "draft_binary", mutationPolicyFixture{AllowDelete: true})
	r := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"expected_record_version":"0","id":"1","operation":"DELETE","table_name":"draft_binary"}],"title":"集成测试发布单"}`, "draft-binary-reject")
	assertIntegrationErrorCode(t, r, 422, "incompatible_table")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"items":[{"content":{"code":"saved history"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-history-schema")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	if _, err := db.Exec("ALTER TABLE mutation_delete_parents RENAME COLUMN code TO renamed_code"); err != nil {
		t.Fatal(err)
	}
	read := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders/"+order.ID, "", "")
	if read.Code != 200 || read.Body.String() != saved.Body.String() {
		t.Fatalf("historical schema was re-read: %s", read.Body)
	}
	if _, err := db.Exec("ALTER TABLE mutation_auto_fill_items MODIFY creator VARCHAR(8) NOT NULL"); err != nil {
		t.Fatal(err)
	}
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{AllowAdd: true, CreateOperatorField: stringPointer("creator"), CreateTimeField: stringPointer("occurred_at")})
	invalid := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"invalid-auto","quantity":"1","status":"active"},"operation":"ADD","table_name":"mutation_auto_fill_items"}],"title":"集成测试发布单"}`, "draft-operator-capacity")
	assertIntegrationErrorCode(t, invalid, 422, "operator_field_incompatible")

}

func TestReleaseDraftReplayUsesCurrentActionsAndRejectsChangedDigest(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"items":[{"content":{"code":"same request"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-replay-current")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	cancelled := releaseRequest(t, app, "POST", "/api/v1/release-orders/"+order.ID+"/cancel", `{"expected_version":"1","reason":"stop"}`, "draft-replay-cancel")
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	replay := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-replay-current")
	if replay.Code != 201 || !strings.Contains(replay.Body.String(), `"version":"1"`) || !strings.Contains(replay.Body.String(), `"allowed_actions":["copy"]`) {
		t.Fatalf("original business result must not offer stale actions: %s", replay.Body)
	}
	changed := strings.Replace(body, `"same request"`, `"changed request"`, 1)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", changed, "draft-replay-current"), 409, "idempotency_conflict")
}
