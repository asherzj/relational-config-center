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

// AC-012: the authenticated public API persists an intent, never a business write.
func TestReleaseDraftSaveAndReload(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"proposed"}}]}`, "draft-create-0001")
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
	read := releaseRequest(t, app, "GET", "/api/v1/release-orders/"+order.ID, "", "")
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
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	body := `{"table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"first"}}]}`
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
	update := `{"table_name":"mutation_delete_parents","expected_version":"1","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"second"}}]}`
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
	read := releaseRequest(t, app, "GET", path, "", "")
	if read.Body.String() != cancel.Body.String() {
		t.Fatal(read.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "DELETE", path, "", ""), 400, "method_not_allowed")
}

// AC-014: a saved diff preserves each field state and defers all automatic values.
func TestReleaseDraftDiffAndServerBaseline(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	body := `{"table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"draft","label":"","nullable_value":null,"metadata":"null"}}]}`
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
	auto := `{"table_name":"mutation_auto_fill_items","items":[{"operation":"ADD","content":{"code":"auto","status":"active","quantity":"1"}}]}`
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
	forged := `{"table_name":"mutation_auto_fill_items","items":[{"operation":"ADD","content":{"creator":"forged","code":"auto","status":"active","quantity":"1"}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", forged, "draft-forged-auto"), 422, "invalid_mutation_content")
	before := `{"table_name":"mutation_add_items","items":[{"operation":"ADD","before":{"label":"forged"},"content":{"code":"draft","label":""}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", before, "draft-forged-before"), 400, "invalid_request")
}

func releaseString(s string) *string { return &s }

// A missing known ADD id keeps the database's comparison identity, tombstone,
// and maintenance generation. Draft reads must not initialize version resources.
func TestReleaseDraftMissingIdentityAndTombstone(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/010-record-identity-fixture.sql")
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
			body, _ := json.Marshal(map[string]any{"table_name": test.table, "items": []any{map[string]any{"operation": "ADD", "content": map[string]string{"id": test.first, "label": "draft"}}}})
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
			addBody, _ := json.Marshal(map[string]any{"content": map[string]string{"id": test.first, "label": "real"}})
			add := publicationFixtureRequest(t, app, "ADD", test.table, "", string(addBody))
			if add.Code != 200 {
				t.Fatal(add.Body)
			}
			deleted := publicationFixtureRequest(t, app, "DELETE", test.table, test.equivalent, `{"expected_version":"1"}`)
			assertMutationAffected(t, deleted)
			body, _ = json.Marshal(map[string]any{"table_name": test.table, "items": []any{map[string]any{"operation": "ADD", "content": map[string]string{"id": test.equivalent, "label": "recreated draft"}}}})
			draft = releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "draft-identity-tombstone-"+test.table)
			if draft.Code != 201 {
				t.Fatal(draft.Body)
			}
			_ = json.Unmarshal(draft.Body.Bytes(), &saved)
			if saved["items"].([]any)[0].(map[string]any)["expected_record_version"] != "2" {
				t.Fatalf("lost tombstone: %s", draft.Body)
			}
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
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
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
	body := `{"table_name":"mutation_delete_parents","items":[{"operation":"DELETE","id":"1","expected_record_version":"0"}]}`
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
	update := `{"table_name":"mutation_delete_parents","expected_version":"1","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"intruder"}}]}`
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
	read = req(other, "GET", path, "", "")
	if read.Code != 200 || !strings.Contains(read.Body.String(), "delete-rollback") {
		t.Fatal(read.Body)
	}
}

func TestReleaseDraftSchemaReadiness(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
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
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	body := `{"table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"id":"7","code":"draft","label":"proposed"}}]}`
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-add-original")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	add := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"7","code":"other","label":"other"}}`)
	if add.Code != 200 {
		t.Fatal(add.Body)
	}
	assertMutationAffected(t, publicationFixtureRequest(t, app, "DELETE", "mutation_add_items", "7", `{"expected_version":"1"}`))
	update := `{"table_name":"mutation_add_items","expected_version":"1","items":[{"operation":"ADD","content":{"id":"7","code":"draft","label":"edited"}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", "/api/v1/release-orders/"+order.ID, update, "draft-add-missing"), 422, "record_version_required")
	update = strings.Replace(update, `"operation":"ADD"`, `"operation":"ADD","expected_record_version":"0"`, 1)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", "/api/v1/release-orders/"+order.ID, update, "draft-add-stale"), 409, "record_version_conflict")
}

func TestReleaseDraftPreviewRebuildsMissingAddBaseline(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
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
	body := `{"table_name":"mutation_add_items","items":[{"operation":"ADD","expected_record_version":"0","content":{"id":"7","code":"preview","label":"retained"}}]}`
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
	list := releaseRequest(t, app, "GET", "/api/v1/release-orders", "", "")
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
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	_ = integrationAdminSession(t, app)
	body := `{"table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"retry"}}]}`
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
	update := `{"table_name":"mutation_delete_parents","expected_version":"1","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"next"}}]}`
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
	if _, err := owner.Exec(`CREATE TRIGGER reject_release_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.result IS NOT NULL AND OLD.result IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected release storage failure'; END IF; END`); err != nil {
		t.Fatal(err)
	}
	failure := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-storage-failure")
	assertIntegrationErrorCode(t, failure, 503, "release_unavailable")
	list := releaseRequest(t, app, "GET", "/api/v1/release-orders", "", "")
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
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
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
	r := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"table_name":"draft_binary","items":[{"operation":"DELETE","id":"1","expected_record_version":"0"}]}`, "draft-binary-reject")
	assertIntegrationErrorCode(t, r, 422, "incompatible_table")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"saved history"}}]}`
	saved := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "draft-history-schema")
	if saved.Code != 201 {
		t.Fatal(saved.Body)
	}
	var order struct{ ID string }
	_ = json.Unmarshal(saved.Body.Bytes(), &order)
	if _, err := db.Exec("ALTER TABLE mutation_delete_parents RENAME COLUMN code TO renamed_code"); err != nil {
		t.Fatal(err)
	}
	read := releaseRequest(t, app, "GET", "/api/v1/release-orders/"+order.ID, "", "")
	if read.Code != 200 || read.Body.String() != saved.Body.String() {
		t.Fatalf("historical schema was re-read: %s", read.Body)
	}
	if _, err := db.Exec("ALTER TABLE mutation_auto_fill_items MODIFY creator VARCHAR(8) NOT NULL"); err != nil {
		t.Fatal(err)
	}
	enableMutationPolicy(t, app, "mutation_auto_fill_items", mutationPolicyFixture{AllowAdd: true, CreateOperatorField: stringPointer("creator"), CreateTimeField: stringPointer("occurred_at")})
	invalid := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"table_name":"mutation_auto_fill_items","items":[{"operation":"ADD","content":{"code":"invalid-auto","status":"active","quantity":"1"}}]}`, "draft-operator-capacity")
	assertIntegrationErrorCode(t, invalid, 422, "operator_field_incompatible")

}

func TestReleaseDraftReplayUsesCurrentActionsAndRejectsChangedDigest(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"same request"}}]}`
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
	changed := strings.Replace(body, `"table_name":`, `"expected_version":"unexpected","table_name":`, 1)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", changed, "draft-replay-current"), 409, "idempotency_conflict")
}
