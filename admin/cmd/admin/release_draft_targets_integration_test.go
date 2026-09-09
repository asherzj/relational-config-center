//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// AC-003: saving a draft, including its first detail, protects the target until
// an explicit terminal action releases it. Conflicts identify the owner.
func TestDraftReservationsBeginOnSaveAndEndOnCancel(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	empty := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"empty","table_name":"mutation_delete_parents","items":[]}`, "targets-empty-create")
	if empty.Code != 201 {
		t.Fatalf("empty draft: %d %s", empty.Code, empty.Body)
	}
	var order struct {
		ID          string
		ApplicantID string `json:"applicant_id"`
	}
	if err := json.Unmarshal(empty.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	item := `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"proposed"}}`
	body := fmt.Sprintf(`{"title":"owner","table_name":"mutation_delete_parents","items":[%s]}`, item)
	path := "/api/v1/release-orders/" + order.ID
	saved := releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"title":"owner","table_name":"mutation_delete_parents","expected_version":"1","items":[%s]}`, item), "targets-first-detail")
	if saved.Code != 200 {
		t.Fatalf("first detail: %d %s", saved.Code, saved.Body)
	}
	conflict := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "targets-conflicting-draft")
	assertIntegrationErrorCode(t, conflict, 409, "release_target_conflict")
	var failure struct {
		Error struct {
			TableName   string `json:"table_name"`
			OrderID     string `json:"order_id"`
			ApplicantID string `json:"applicant_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.TableName != "mutation_delete_parents" || failure.Error.OrderID != order.ID || failure.Error.ApplicantID != order.ApplicantID {
		t.Fatalf("missing conflict owner: %s", conflict.Body)
	}
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"2","reason":"release stale draft"}`, "targets-cancel-owner")
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	retry := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "targets-conflicting-draft")
	if retry.Code != 201 {
		t.Fatalf("released target: %d %s", retry.Code, retry.Body)
	}
}

// AC-002/003: a configured key protects old and proposed values, while any
// unfinished table reference makes its definition immutable.
func TestDraftConcurrencyKeyProtectsValuesAndDefinition(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowAdd: true})
	policy := policyIntegrationRequest(t, app, "GET", "/api/v1/table-policies/mutation_delete_parents", "")
	var assignment map[string]any
	if err := json.Unmarshal(policy.Body.Bytes(), &assignment); err != nil {
		t.Fatal(err)
	}
	setKey := func(columns []string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"table_name": "mutation_delete_parents", "query_policy_code": assignment["query_policy_code"], "mutation_policy_code": assignment["mutation_policy_code"], "concurrency_key": columns})
		return policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/mutation_delete_parents", string(body))
	}
	if response := setKey([]string{"code"}); response.Code != 200 {
		t.Fatalf("configure key: %d %s", response.Code, response.Body)
	}
	owner := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"key owner","table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"Candidate"}}]}`, "key-owner-create")
	if owner.Code != 201 {
		t.Fatal(owner.Body)
	}
	conflict := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"equivalent key","table_name":"mutation_delete_parents","items":[{"operation":"ADD","content":{"id":"50","code":"candidate"}}]}`, "key-conflict-create")
	assertIntegrationErrorCode(t, conflict, 409, "release_target_conflict")
	assertIntegrationErrorCode(t, setKey([]string{}), 409, "concurrency_key_in_use")
	if response := setKey([]string{"code"}); response.Code != 200 {
		t.Fatalf("unchanged key: %s", response.Body)
	}
}

// AC-005: an incremental save changes only addressed stable details, and a
// failed candidate target or stale whole-order version preserves the old set.
func TestDraftIncrementalEditsReplaceTargetsAtomically(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE draft_keys(id INT PRIMARY KEY, code VARCHAR(80), label VARCHAR(80))`)
	deliveryExec(t, db, `INSERT INTO draft_keys VALUES(1,'A','one'),(2,'S','two')`)
	enableMutationPolicy(t, app, "draft_keys", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	setDraftTestKey(t, app, "draft_keys", []string{"code"})
	create := func(items, key string) map[string]any {
		r := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"incremental","table_name":"draft_keys","items":`+items+`}`, key)
		if r.Code != 201 {
			t.Fatalf("create: %d %s", r.Code, r.Body)
		}
		var result map[string]any
		_ = json.Unmarshal(r.Body.Bytes(), &result)
		return result
	}
	owner := create(`[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"B"}},{"operation":"MODIFY","id":"2","expected_record_version":"0","content":{"label":"saved second"}}]`, "incremental-owner")
	create(`[{"operation":"ADD","content":{"id":"99","code":"C"}}]`, "incremental-C-owner")
	items := owner["items"].([]any)
	first := items[0].(map[string]any)
	second := items[1].(map[string]any)
	firstID, ok := first["detail_id"].(string)
	if !ok || firstID == "" {
		t.Fatalf("missing stable detail: %#v", first)
	}
	path := "/api/v1/release-orders/" + owner["id"].(string)
	patch := func(value, version, key string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"table_name": "draft_keys", "title": "incremental", "expected_version": version, "changes": map[string]any{"upserts": []any{map[string]any{"detail_id": firstID, "operation": "MODIFY", "id": "1", "expected_record_version": "0", "content": map[string]string{"code": value}}}}})
		return releaseRequest(t, app, "PUT", path, string(body), key)
	}
	assertIntegrationErrorCode(t, patch("C", "1", "incremental-conflict"), 409, "release_target_conflict")
	current := releaseRequest(t, app, "GET", path, "", "")
	var unchanged map[string]any
	_ = json.Unmarshal(current.Body.Bytes(), &unchanged)
	if unchanged["version"] != "1" || unchanged["items"].([]any)[0].(map[string]any)["content"].(map[string]any)["code"] != "B" {
		t.Fatalf("failed replacement changed draft: %s", current.Body)
	}
	for _, code := range []string{"A", "B"} {
		r := releaseRequest(t, app, "POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"probe","table_name":"draft_keys","items":[{"operation":"ADD","content":{"id":"88","code":%q}}]}`, code), "incremental-probe-"+code)
		assertIntegrationErrorCode(t, r, 409, "release_target_conflict")
	}
	saved := patch("D", "1", "incremental-new-target")
	if saved.Code != 200 {
		t.Fatal(saved.Body)
	}
	assertIntegrationErrorCode(t, patch("E", "1", "incremental-stale"), 409, "release_version_conflict")
	create(`[{"operation":"ADD","content":{"id":"88","code":"B"}}]`, "incremental-B-released")
	reorder, _ := json.Marshal(map[string]any{"table_name": "draft_keys", "title": "incremental", "expected_version": "2", "changes": map[string]any{"detail_order": []any{second["detail_id"], firstID}}})
	r := releaseRequest(t, app, "PUT", path, string(reorder), "incremental-reorder")
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
	// Only the edited upsert is prepared, but its error points into the whole
	// newly ordered candidate, not index zero of the changed-page payload.
	bad, _ := json.Marshal(map[string]any{"table_name": "draft_keys", "title": "incremental", "expected_version": "3", "changes": map[string]any{"upserts": []any{map[string]any{"detail_id": firstID, "operation": "MODIFY", "id": "1", "expected_record_version": "0", "content": map[string]string{"missing_field": "invalid"}}}}})
	invalid := releaseRequest(t, app, "PUT", path, string(bad), "incremental-error-position")
	assertIntegrationErrorCode(t, invalid, 422, "invalid_mutation_content")
	batchEdgeIndex(t, invalid, 1)
	var reordered map[string]any
	_ = json.Unmarshal(r.Body.Bytes(), &reordered)
	if reordered["items"].([]any)[0].(map[string]any)["detail_id"] != second["detail_id"] || reordered["items"].([]any)[0].(map[string]any)["content"].(map[string]any)["label"] != "saved second" {
		t.Fatalf("reorder lost other detail: %s", r.Body)
	}
}

func setDraftTestKey(t *testing.T, app *adminApplication, table string, columns []string) {
	t.Helper()
	r := policyIntegrationRequest(t, app, "GET", "/api/v1/table-policies/"+table, "")
	var assignment map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &assignment); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"table_name": table, "query_policy_code": assignment["query_policy_code"], "mutation_policy_code": assignment["mutation_policy_code"], "concurrency_key": columns})
	r = policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/"+table, string(body))
	if r.Code != 200 {
		t.Fatalf("set key: %d %s", r.Code, r.Body)
	}
}

// AC-002: generated and publication-time fields cannot define a key. An
// auto-ID draft still references its table even when it has no concrete target.
func TestDraftKeyEligibilityDefaultsAndAutoIDReferences(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE draft_auto_keys(id INT AUTO_INCREMENT PRIMARY KEY, code VARCHAR(80) NULL DEFAULT 'default', generated_code VARCHAR(80) GENERATED ALWAYS AS (code) STORED, stamp DATETIME)`)
	enableMutationPolicy(t, app, "draft_auto_keys", mutationPolicyFixture{AllowAdd: true, CreateTimeField: stringPointer("stamp")})
	policy := policyIntegrationRequest(t, app, "GET", "/api/v1/table-policies/draft_auto_keys", "")
	var assignment map[string]any
	_ = json.Unmarshal(policy.Body.Bytes(), &assignment)
	set := func(fields []string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"table_name": "draft_auto_keys", "query_policy_code": assignment["query_policy_code"], "mutation_policy_code": assignment["mutation_policy_code"], "concurrency_key": fields})
		return policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/draft_auto_keys", string(body))
	}
	for _, fields := range [][]string{{"id"}, {"generated_code"}, {"stamp"}, {"missing"}, {"code", "code"}} {
		assertIntegrationErrorCode(t, set(fields), 422, "concurrency_key_invalid")
	}
	auto := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"no concrete target","table_name":"draft_auto_keys","items":[{"operation":"ADD","content":{}}]}`, "auto-no-target")
	if auto.Code != 201 {
		t.Fatal(auto.Body)
	}
	assertIntegrationErrorCode(t, set([]string{"code"}), 409, "concurrency_key_in_use")
	var order map[string]any
	_ = json.Unmarshal(auto.Body.Bytes(), &order)
	if cancelled := releaseRequest(t, app, "POST", "/api/v1/release-orders/"+order["id"].(string)+"/cancel", `{"expected_version":"1","reason":"admin cancelled abandoned draft"}`, "auto-cancel"); cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	if configured := set([]string{"code"}); configured.Code != 200 {
		t.Fatal(configured.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"default is ambiguous","table_name":"draft_auto_keys","items":[{"operation":"ADD","content":{}}]}`, "auto-default-missing"), 422, "concurrency_key_value_required")
	null := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"explicit null","table_name":"draft_auto_keys","items":[{"operation":"ADD","content":{"code":null}}]}`, "auto-explicit-null")
	if null.Code != 201 {
		t.Fatal(null.Body)
	}
}

// AC-004: expected equivalence comes from independent literal examples of
// database comparison. Distinct tables and NULL/empty/tuple boundaries survive.
func TestDraftConcurrencyKeyDatabaseEquality(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	cases := []struct {
		name, definition             string
		first, equivalent, different any
	}{
		{"decimal", "DECIMAL(12,3)", "+01.0", "1.000", "1.001"},
		{"integer", "INT", "+01", "1", "2"},
		{"float", "FLOAT", "-0", "0", "0.0001"},
		{"double", "DOUBLE", "-0", "0", "1.0000001"},
		{"text", "VARCHAR(80) COLLATE utf8mb4_0900_ai_ci", "Café", "CAFE", "Café "},
		{"pad", "VARCHAR(80) COLLATE utf8mb4_unicode_ci", "A", "a ", "B"},
		{"char", "CHAR(80) COLLATE utf8mb4_0900_ai_ci", "A", "a ", "B"},
		{"null", "VARCHAR(80)", nil, nil, ""},
		{"time", "TIME(3)", "12:00:00", "12:00:00.000", "12:00:00.001"},
		{"timestamp", "TIMESTAMP(3)", "2026-09-09T12:00:00Z", "2026-09-09T12:00:00.000Z", "2026-09-09T12:00:00.001Z"},
		{"datetime", "DATETIME(3)", "2026-09-09 12:00:00", "2026-09-09 12:00:00.000", "2026-09-09 12:00:00.001"},
		{"enum", "ENUM('A','B') COLLATE utf8mb4_0900_ai_ci", "A", "a", "B"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			table := "draft_equal_" + test.name
			deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, code "+test.definition+" NULL)")
			enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true})
			setDraftTestKey(t, app, table, []string{"code"})
			create := func(id int, value any, key string) *httptest.ResponseRecorder {
				body, _ := json.Marshal(map[string]any{"title": "database equality", "table_name": table, "items": []any{map[string]any{"operation": "ADD", "content": map[string]any{"id": fmt.Sprint(id), "code": value}}}})
				return releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), key+"-"+test.name)
			}
			if r := create(1, test.first, "equality-first"); r.Code != 201 {
				t.Fatal(r.Body)
			}
			assertIntegrationErrorCode(t, create(2, test.equivalent, "equality-equivalent"), 409, "release_target_conflict")
			if r := create(3, test.different, "equality-different"); r.Code != 201 {
				t.Fatal(r.Body)
			}
		})
	}
	t.Run("stored-time-duration", func(t *testing.T) {
		deliveryExec(t, db, `CREATE TABLE draft_time_duration(id INT PRIMARY KEY,code TIME(3),label VARCHAR(32))`)
		deliveryExec(t, db, `INSERT INTO draft_time_duration VALUES(1,'-25:00:00.001','a'),(2,'-25:00:00.001','b')`)
		enableMutationPolicy(t, app, "draft_time_duration", mutationPolicyFixture{AllowModify: true})
		setDraftTestKey(t, app, "draft_time_duration", []string{"code"})
		for index, id := range []string{"1", "2"} {
			response := releaseRequest(t, app, "POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"stored time","table_name":"draft_time_duration","items":[{"operation":"MODIFY","id":%q,"expected_record_version":"0","content":{"label":"changed"}}]}`, id), "stored-duration-"+id)
			if index == 0 {
				rollbackOrderResponse(t, response, 201)
			} else {
				assertIntegrationErrorCode(t, response, 409, "release_target_conflict")
			}
		}
	})
	for _, table := range []string{"draft_tuple_a", "draft_tuple_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY,a VARCHAR(80),b VARCHAR(80))")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true})
		setDraftTestKey(t, app, table, []string{"a", "b"})
		for i, pair := range [][2]any{{"ab", "c"}, {"a", "bc"}, {nil, ""}, {"", nil}, {"", ""}} {
			body, _ := json.Marshal(map[string]any{"title": "tuple boundary", "table_name": table, "items": []any{map[string]any{"operation": "ADD", "content": map[string]any{"id": fmt.Sprint(i + 1), "a": pair[0], "b": pair[1]}}}})
			if r := releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), fmt.Sprintf("tuple-%s-%d", table, i)); r.Code != 201 {
				t.Fatal(r.Body)
			}
		}
	}
}

// AC-003: a business key may be referenced by several details in one order.
// Omitted MODIFY fields use the saved real old value; only the last removal frees it.
func TestDraftSharedKeyReferencesReleaseOnlyAfterLastDetail(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE draft_shared(id INT PRIMARY KEY,code VARCHAR(80),label VARCHAR(80))`)
	deliveryExec(t, db, `INSERT INTO draft_shared VALUES(1,'shared','one'),(2,'SHARED','two')`)
	enableMutationPolicy(t, app, "draft_shared", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	setDraftTestKey(t, app, "draft_shared", []string{"code"})
	owner := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"shared references","table_name":"draft_shared","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"edit one"}},{"operation":"MODIFY","id":"2","expected_record_version":"0","content":{"label":"edit two"}}]}`, "shared-owner")
	if owner.Code != 201 {
		t.Fatal(owner.Body)
	}
	var order map[string]any
	_ = json.Unmarshal(owner.Body.Bytes(), &order)
	probe := func(key string) *httptest.ResponseRecorder {
		return releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"shared probe","table_name":"draft_shared","items":[{"operation":"ADD","content":{"id":"3","code":"Shared"}}]}`, key)
	}
	for index, item := range order["items"].([]any) {
		assertIntegrationErrorCode(t, probe(fmt.Sprintf("shared-probe-%d", index)), 409, "release_target_conflict")
		body, _ := json.Marshal(map[string]any{"title": "shared references", "table_name": "draft_shared", "expected_version": fmt.Sprint(index + 1), "changes": map[string]any{"delete_detail_ids": []any{item.(map[string]any)["detail_id"]}}})
		if r := releaseRequest(t, app, "PUT", "/api/v1/release-orders/"+order["id"].(string), string(body), fmt.Sprintf("shared-remove-%d", index)); r.Code != 200 {
			t.Fatal(r.Body)
		}
	}
	if r := probe("shared-final-probe"); r.Code != 201 {
		t.Fatal(r.Body)
	}
}

// AC-002: both HTTP writers start behind the same real database guard. Exactly
// one may establish the definition/reference pair; the other must observe it.
func TestDraftSaveAndKeyDefinitionRaceCannotBypassReferences(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for round := 0; round < 4; round++ {
		table := fmt.Sprintf("draft_guard_%d", round)
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT AUTO_INCREMENT PRIMARY KEY,code VARCHAR(80) DEFAULT 'implicit')")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true})
		policy := policyIntegrationRequest(t, app, "GET", "/api/v1/table-policies/"+table, "")
		var assignment map[string]any
		_ = json.Unmarshal(policy.Body.Bytes(), &assignment)
		definition, _ := json.Marshal(map[string]any{"table_name": table, "query_policy_code": assignment["query_policy_code"], "mutation_policy_code": assignment["mutation_policy_code"], "concurrency_key": []string{"code"}})
		blocker, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		var locked string
		if err := blocker.QueryRow("SELECT table_name FROM rcc_table_policies WHERE table_name=? FOR UPDATE", table).Scan(&locked); err != nil {
			t.Fatal(err)
		}
		saved, defined := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
		go func() {
			saved <- releaseRequest(t, app, "POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"guard race","table_name":%q,"items":[{"operation":"ADD","content":{}}]}`, table), fmt.Sprintf("draft-guard-save-%d", round))
		}()
		go func() {
			defined <- policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/"+table, string(definition))
		}()
		// Both writers must remain blocked until the database guard is released.
		select {
		case r := <-saved:
			t.Fatalf("draft bypassed guard: %s", r.Body)
		case r := <-defined:
			t.Fatalf("definition bypassed guard: %s", r.Body)
		case <-time.After(100 * time.Millisecond):
		}
		if err := blocker.Commit(); err != nil {
			t.Fatal(err)
		}
		a, b := <-saved, <-defined
		if a.Code == 201 {
			assertIntegrationErrorCode(t, b, 409, "concurrency_key_in_use")
		} else {
			if b.Code != 200 {
				t.Fatalf("neither guard writer succeeded: %d %s / %d %s", a.Code, a.Body, b.Code, b.Body)
			}
			assertIntegrationErrorCode(t, a, 422, "concurrency_key_value_required")
		}
	}
}

// AC-003/005: a control-storage failure after target reconciliation leaves both
// the acknowledged draft and its old target set intact, including request reuse.
func TestDraftTargetReplacementRollsBackOnPersistenceFailure(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE draft_atomic(id INT PRIMARY KEY,code VARCHAR(80))`)
	deliveryExec(t, db, `INSERT INTO draft_atomic VALUES(1,'A')`)
	enableMutationPolicy(t, app, "draft_atomic", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	setDraftTestKey(t, app, "draft_atomic", []string{"code"})
	r := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"atomic","table_name":"draft_atomic","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"B"}}]}`, "atomic-owner")
	if r.Code != 201 {
		t.Fatal(r.Body)
	}
	var owner map[string]any
	_ = json.Unmarshal(r.Body.Bytes(), &owner)
	rootDriver := *driver
	rootDriver.User = "root"
	root := deliveryDB(t, &rootDriver)
	deliveryExec(t, root, `CREATE TRIGGER reject_draft_request BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.result IS NOT NULL AND OLD.result IS NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected request failure'; END IF; END`)
	body := `{"title":"atomic","table_name":"draft_atomic","expected_version":"1","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"C"}}]}`
	path := "/api/v1/release-orders/" + owner["id"].(string)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, body, "atomic-replace"), 503, "release_unavailable")
	deliveryExec(t, root, `DROP TRIGGER reject_draft_request`)
	current := releaseRequest(t, app, "GET", path, "", "")
	if current.Body.String() != r.Body.String() {
		t.Fatalf("failure changed saved draft: %s", current.Body)
	}
	for _, code := range []string{"A", "B"} {
		probe := releaseRequest(t, app, "POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"atomic probe","table_name":"draft_atomic","items":[{"operation":"ADD","content":{"id":"2","code":%q}}]}`, code), "atomic-probe-"+code)
		assertIntegrationErrorCode(t, probe, 409, "release_target_conflict")
	}
	if retry := releaseRequest(t, app, "PUT", path, body, "atomic-replace"); retry.Code != 200 {
		t.Fatal(retry.Body)
	}
}

// Every terminal route must release both primary and supplementary targets,
// plus the table reference that protects the definition.
func TestDraftAllTargetTypesReleaseOnEveryTerminalAction(t *testing.T) {
	for _, action := range []string{"cancel", "reject", "complete", "quick-rollback"} {
		t.Run(action, func(t *testing.T) {
			app, db := batchEdgeApplication(t, `CREATE TABLE terminal_keys(id INT PRIMARY KEY,code VARCHAR(32),label VARCHAR(32))`, `INSERT INTO terminal_keys VALUES(1,'old','original')`)
			enableMutationPolicy(t, app, "terminal_keys", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
			setDraftTestKey(t, app, "terminal_keys", []string{"code"})
			created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"terminal target owner","table_name":"terminal_keys","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"new"}}]}`, "terminal-create"), 201)
			path := "/api/v1/release-orders/" + created.ID
			batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 3, `SELECT COUNT(*) FROM rcc_release_table_references`: 1})
			if action != "cancel" {
				rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "terminal-submit"), 200)
				reviewer := publicationFixtureReviewer(t, app)
				if action == "reject" {
					rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/reject", `{"expected_version":"2","reason":"end proposal"}`, "terminal-reject"), 200)
				} else {
					rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"reviewed"}`, "terminal-approve"), 200)
					rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "terminal-execute"), 200)
					batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 3, `SELECT COUNT(*) FROM rcc_release_table_references`: 1})
					if action == "complete" {
						completePublicationFixture(t, app, path, "terminal-complete")
					} else {
						quickRestoreFixture(t, app, path, "terminal-restore")
					}
				}
			} else {
				rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"end proposal"}`, "terminal-cancel"), 200)
			}
			batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 0, `SELECT COUNT(*) FROM rcc_release_table_references`: 0})
			setDraftTestKey(t, app, "terminal_keys", []string{})
		})
	}
}

// AC-002/003: live DDL can change database equality without changing row values
// or the configured field names. Submit must not freeze a new unreserved key.
func TestDraftSubmitRejectsChangedTargetIdentity(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE draft_ddl_keys(id INT PRIMARY KEY,code VARCHAR(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin)`)
	enableMutationPolicy(t, app, "draft_ddl_keys", mutationPolicyFixture{AllowAdd: true})
	setDraftTestKey(t, app, "draft_ddl_keys", []string{"code"})
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"saved binary key","table_name":"draft_ddl_keys","items":[{"operation":"ADD","content":{"id":"1","code":"Alpha"}}]}`, "ddl-original")
	original := batchEdgeOrder(t, created, 201)
	path := "/api/v1/release-orders/" + original.ID
	deliveryExec(t, db, `ALTER TABLE draft_ddl_keys MODIFY code VARCHAR(80) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	blocker := batchEdgeOrder(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"current equivalent owner","table_name":"draft_ddl_keys","items":[{"operation":"ADD","content":{"id":"2","code":"ALPHA"}}]}`, "ddl-current-owner"), 201)
	targets := func() string {
		t.Helper()
		var result string
		if err := db.QueryRow(`SELECT GROUP_CONCAT(CONCAT(HEX(record_key),':',order_id) ORDER BY record_key SEPARATOR ',') FROM rcc_release_targets`).Scan(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	beforeTargets := targets()
	refused := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "ddl-submit")
	assertIntegrationErrorCode(t, refused, 409, "record_version_conflict")
	batchEdgeIndex(t, refused, 0)
	if current := releaseRequest(t, app, "GET", path, "", ""); current.Body.String() != created.Body.String() || targets() != beforeTargets {
		t.Fatalf("refused submission changed draft or targets: %s", current.Body)
	}
	// Explicit saving performs the normal atomic replacement. A conflict retains
	// the old set; ending the owner then permits repeating the same logical save.
	edit, _ := json.Marshal(map[string]any{"title": original.Title, "table_name": original.TableName, "expected_version": "1", "changes": map[string]any{"upserts": []any{map[string]any{"detail_id": original.Items[0].DetailID, "operation": "ADD", "expected_record_version": "0", "content": map[string]string{"id": "1", "code": "Alpha"}}}}})
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, string(edit), "ddl-resave"), 409, "release_target_conflict")
	if targets() != beforeTargets {
		t.Fatal("failed replacement changed target ownership")
	}
	batchEdgeOrder(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+blocker.ID+"/cancel", `{"expected_version":"1","reason":"end current owner"}`, "ddl-cancel-owner"), 200)
	batchEdgeOrder(t, releaseRequest(t, app, "PUT", path, string(edit), "ddl-resave"), 200)
	batchEdgeOrder(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"2"}`, "ddl-submit-refreshed"), 200)
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 2, `SELECT COUNT(*) FROM rcc_release_table_references`: 1})
}
