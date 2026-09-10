//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

// AC-018: submitting freezes the verified intent, without writing configuration.
func TestReleaseSubmitFreezesIntent(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	body := `{"items":[{"content":{"code":"proposed"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "approval-create-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var draft struct{ ID string }
	_ = json.Unmarshal(created.Body.Bytes(), &draft)
	path := "/api/v1/release-orders/" + draft.ID
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "approval-submit-01")
	if submitted.Code != 200 {
		t.Fatalf("submit: %d %s", submitted.Code, submitted.Body)
	}
	var order struct {
		State, Version string
		FrozenDigest   string `json:"frozen_digest"`
	}
	_ = json.Unmarshal(submitted.Body.Bytes(), &order)
	if order.State != "PENDING_APPROVAL" || order.Version != "2" || len(order.FrozenDigest) != 64 {
		t.Fatalf("not frozen: %s", submitted.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"2","items":[{"content":{"code":"changed"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "approval-edit-01"), 422, "release_state_invalid")
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if *row["code"] != "delete-rollback" || version != "0" {
		t.Fatalf("submit wrote business row: %v %s", row, version)
	}
}

// AC-019/022: independent drafts compete for one actual record identity.
func TestReleaseTargetsCompeteAndCancelReleases(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/010-record-identity-fixture.sql")
	enableMutationPolicy(t, app, "record_identity_ci", mutationPolicyFixture{AllowAdd: true})
	bodies := make([]string, 2)
	for i, id := range []string{"Résumé", "RESUME"} {
		raw, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": []any{map[string]any{"table_name": "record_identity_ci", "operation": "ADD", "content": map[string]string{"id": id, "label": "draft"}}}})
		bodies[i] = string(raw)
	}
	type outcome struct {
		index    int
		response *httptest.ResponseRecorder
	}
	responses := make(chan outcome, 2)
	session := integrationAdminSession(t, app)
	csrf, cookies := sessionCSRF(t, session), session.Result().Cookies()
	start := make(chan struct{})
	for i := range bodies {
		go func(i int) {
			<-start
			responses <- outcome{i, accountRequestFrom(app, "POST", "/api/v1/release-orders", bodies[i], cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("target-create-%d", i)})}
		}(i)
	}
	close(start)
	winner, loser := "", -1
	for range 2 {
		result := <-responses
		if result.response.Code == 201 {
			winner = "/api/v1/release-orders/" + rollbackOrderResponse(t, result.response, 201).ID
		} else {
			assertIntegrationErrorCode(t, result.response, 409, "release_target_conflict")
			loser = result.index
		}
	}
	if winner == "" || loser == -1 {
		t.Fatal("expected one saved draft and one rejected competing save")
	}
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", winner+"/submit", `{"expected_version":"1"}`, "target-submit-01"), 200)
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", winner+"/cancel", `{"expected_version":"2","reason":"stop"}`, "target-cancel-01"), 200)
	// The same logical failed save can succeed once cancellation releases its target.
	saved := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", bodies[loser], fmt.Sprintf("target-create-%d", loser)), 201)
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+saved.ID+"/submit", `{"expected_version":"1"}`, "target-submit-released"), 200)
}

func releaseActorRequest(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	return accountRequestFrom(app, method, path, body, actor.Result().Cookies(), sessionCSRF(t, actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
}
func grantReleaseRole(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, roles, version, key string) {
	t.Helper()
	r := releaseRequest(t, app, "PUT", "/api/v1/account-roles/"+accountID(t, actor), `{"roles":`+roles+`,"expected_version":"`+version+`"}`, key)
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
}

// AC-011: any viewer can resolve only the permanent account IDs already exposed
// by one order, and profile changes affect the current display without rewriting history.
func TestReleasePeopleResolveCurrentNamesWithoutAccountAdmin(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	editor := registerAccount(t, app, "people.editor", "people.editor@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "people.reviewer", "people.reviewer@example.com", "correct horse battery staple")
	publisher := registerAccount(t, app, "people.publisher", "people.publisher@example.com", "correct horse battery staple")
	viewer := registerAccount(t, app, "people.viewer", "people.viewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "people-editor-role")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "people-reviewer-role")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "people-publisher-role")
	created := releaseActorRequest(t, app, editor, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"people","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"验证人员归属"}`, "people-create")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + order.ID
	if submitted := releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "people-submit"); submitted.Code != 200 {
		t.Fatal(submitted.Body)
	}
	if approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"人员独立审批"}`, "people-approve"); approved.Code != 200 {
		t.Fatal(approved.Body)
	}
	if published := releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "people-execute"); published.Code != 200 {
		t.Fatal(published.Body)
	}
	for _, change := range []struct {
		actor *httptest.ResponseRecorder
		name  string
	}{{editor, "当前申请人"}, {reviewer, "当前审批人"}, {publisher, "当前发布人"}} {
		response := releaseActorRequest(t, app, change.actor, "PATCH", "/api/v1/auth/profile", fmt.Sprintf(`{"display_name":%q}`, change.name), "")
		if response.Code != 200 {
			t.Fatalf("rename %s: %d %s", change.name, response.Code, response.Body)
		}
	}
	people := releaseActorReadAllDetails(t, app, viewer, "GET", path+"/people", "", "")
	if people.Code != 200 {
		t.Fatalf("viewer reads related people: %d %s", people.Code, people.Body)
	}
	var result struct {
		People map[string]string `json:"people"`
	}
	if err := json.Unmarshal(people.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{accountID(t, editor): "当前申请人", accountID(t, reviewer): "当前审批人", accountID(t, publisher): "当前发布人"}
	if !reflect.DeepEqual(result.People, want) {
		t.Fatalf("related people: %#v, want %#v", result.People, want)
	}
	assertIntegrationErrorCode(t, releaseActorReadAllDetails(t, app, viewer, "GET", "/api/v1/account-roles?limit=20", "", ""), 403, "permission_denied")
}

// AC-020/024/025: a current, independent approver decides once; a historical
// approval survives revocation while new requests still require current grants.
func TestReleaseApprovalCurrentRolesAndHistory(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	reviewer := registerAccount(t, app, "release.reviewer", "release.reviewer@example.com", "correct horse battery staple")
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"approved","label":"new"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "review-create-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "review-submit-01")
	if submitted.Code != 200 {
		t.Fatal(submitted.Body)
	}
	body := `{"expected_version":"2","reason":"reviewed change"}`
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, admin, "POST", path+"/approve", body, "review-self-01"), 403, "permission_denied")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", body, "review-approve-01"), 403, "permission_denied")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "review-grant-01")
	for _, action := range []string{"approve", "reject"} {
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/"+action, `{"expected_version":"2","reason":"  "}`, "review-empty-"+action), 422, "release_invalid")
	}
	approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", body, "review-approve-01")
	if approved.Code != 200 || !strings.Contains(approved.Body.String(), `"state":"APPROVED"`) || !strings.Contains(approved.Body.String(), accountID(t, reviewer)) {
		t.Fatalf("approve: %d %s", approved.Code, approved.Body)
	}
	replay := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", body, "review-approve-01")
	if replay.Body.String() != approved.Body.String() {
		t.Fatalf("approval replay changed: %s", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"different"}`, "review-approve-01"), 409, "idempotency_conflict")
	grantReleaseRole(t, app, reviewer, `["VIEWER"]`, "2", "review-revoke-01")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", body, "review-approve-01"), 403, "permission_denied")
	read := releaseActorReadAllDetails(t, app, reviewer, "GET", path, "", "")
	if read.Body.String() != approved.Body.String() {
		t.Fatalf("revocation changed historical approval: %s", read.Body)
	}
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"3","reason":"stop approved order"}`, "review-cancel-01")
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "3", "review-restore-01")
	replay = releaseActorRequest(t, app, reviewer, "POST", path+"/approve", body, "review-approve-01")
	if replay.Code != 200 || !strings.Contains(replay.Body.String(), `"state":"APPROVED"`) {
		t.Fatalf("old successful result lost after cancellation: %s", replay.Body)
	}
	current := releaseActorReadAllDetails(t, app, reviewer, "GET", path, "", "")
	if !strings.Contains(current.Body.String(), `"state":"CANCELLED"`) {
		t.Fatal(current.Body)
	}
}

// AC-021: copying a rejected order requires an explicitly reviewed current
// baseline and retains the source decision without inheriting its approval.
func TestReleaseRejectedCopyRechecksBaseline(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	reviewer := registerAccount(t, app, "copy.reviewer", "copy.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "copy-reviewer-01")
	original := `{"items":[{"content":{"code":"proposal"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"调整删除保护配置"}`
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", original, "copy-original-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	r := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "copy-submit-01")
	if r.Code != 200 {
		t.Fatal(r.Body)
	}
	rejected := releaseActorRequest(t, app, reviewer, "POST", path+"/reject", `{"expected_version":"2","reason":"needs another review"}`, "copy-reject-01")
	if rejected.Code != 200 {
		t.Fatal(rejected.Body)
	}
	updated := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"title":"刷新复制基线","expected_version":"0","content":{"code":"new baseline"}}`)
	assertMutationAffected(t, updated)
	body := `{"expected_version":"3","confirmed":true,"items":[{"table_name":"mutation_delete_parents","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"proposal"}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/copy", body, "copy-request-01"), 409, "record_version_conflict")
	fresh := releaseRequest(t, app, "POST", "/api/v1/release-orders/preview", original, "")
	if fresh.Code != 200 || !strings.Contains(fresh.Body.String(), "new baseline") {
		t.Fatal(fresh.Body)
	}
	confirmed := strings.Replace(body, `"expected_record_version":"0"`, `"expected_record_version":"1"`, 1)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/copy", strings.Replace(confirmed, `"confirmed":true`, `"confirmed":false`, 1), "copy-unconfirmed-01"), 422, "release_invalid")
	copied := releaseRequest(t, app, "POST", path+"/copy", confirmed, "copy-request-01")
	if copied.Code != 201 {
		t.Fatalf("copy: %d %s", copied.Code, copied.Body)
	}
	var result struct {
		ID, Title, State, Version string
		CopiedFromID              string `json:"copied_from_id"`
		History                   []struct{ Action string }
		Items                     []struct{ Before map[string]*string }
	}
	json.Unmarshal(copied.Body.Bytes(), &result)
	if result.ID == order.ID || result.Title != "调整删除保护配置" || result.State != "DRAFT" || result.Version != "1" || result.CopiedFromID != order.ID || len(result.History) != 1 || result.History[0].Action != "COPY" || *result.Items[0].Before["code"] != "new baseline" {
		t.Fatalf("incorrect copied draft: %s", copied.Body)
	}
	replay := releaseRequest(t, app, "POST", path+"/copy", confirmed, "copy-request-01")
	if replay.Body.String() != copied.Body.String() {
		t.Fatal(replay.Body)
	}
	current := releaseActorReadAllDetails(t, app, reviewer, "GET", path, "", "")
	var linked domain.ReleaseOrder
	if current.Code != 200 || json.Unmarshal(current.Body.Bytes(), &linked) != nil || linked.State != "REJECTED" || len(linked.History) != 4 || linked.History[3].Action != "COPY" || linked.History[3].RelatedOrderID != result.ID {
		t.Fatalf("copy did not preserve and link source rejection: %s", current.Body)
	}
}

// AC-019/023: a storage failure rolls back state, request, history and targets;
// competing decisions then produce exactly one new workflow event.
func TestReleaseWorkflowAtomicityAndCompetition(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	reviewer := registerAccount(t, app, "race.reviewer", "race.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "race-grant-01")
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"proposal"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`, "atomic-create-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	if _, err := owner.Exec(`CREATE TRIGGER reject_release_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected storage failure'`); err != nil {
		t.Fatal(err)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "atomic-submit-01"), 503, "release_unavailable")
	var targets, requests int
	if err := owner.QueryRow("SELECT COUNT(*) FROM rcc_release_targets").Scan(&targets); err != nil || targets != 1 {
		t.Fatalf("failed submit changed saved targets: %d %v", targets, err)
	}
	if err := owner.QueryRow("SELECT COUNT(*) FROM rcc_release_requests WHERE operation=?", "submit:"+order.ID).Scan(&requests); err != nil || requests != 0 {
		t.Fatalf("partial request: %d %v", requests, err)
	}
	read := releaseReadAllDetails(t, app, "GET", path, "", "")
	if read.Body.String() != created.Body.String() {
		t.Fatal(read.Body)
	}
	if _, err := owner.Exec("DROP TRIGGER reject_release_result"); err != nil {
		t.Fatal(err)
	}
	submitted := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "atomic-submit-01")
	if submitted.Code != 200 {
		t.Fatal(submitted.Body)
	}
	admin := integrationAdminSession(t, app)
	responses := make(chan *httptest.ResponseRecorder, 3)
	for _, action := range []string{"approve", "reject", "cancel"} {
		actor := reviewer
		if action == "cancel" {
			actor = admin
		}
		cookies, csrf := actor.Result().Cookies(), sessionCSRF(t, actor)
		go func(action string) {
			responses <- accountRequestFrom(app, "POST", path+"/"+action, `{"expected_version":"2","reason":"concurrent decision"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "race-" + action + "-01"})
		}(action)
	}
	successes := 0
	for range 3 {
		r := <-responses
		if r.Code == 200 {
			successes++
		} else {
			assertIntegrationErrorCode(t, r, 409, "release_version_conflict")
		}
	}
	if successes != 1 {
		t.Fatalf("successful decisions: %d", successes)
	}
	current := releaseReadAllDetails(t, app, "GET", path, "", "")
	var result struct {
		State, Version string
		History        []struct{ Action string }
	}
	json.Unmarshal(current.Body.Bytes(), &result)
	if result.Version != "3" || len(result.History) != 3 {
		t.Fatalf("inconsistent history: %s", current.Body)
	}
	if err := owner.QueryRow("SELECT COUNT(*) FROM rcc_release_targets WHERE order_id=?", order.ID).Scan(&targets); err != nil {
		t.Fatal(err)
	}
	expected := 0
	if result.State == "APPROVED" {
		expected = 1
	}
	if targets != expected {
		t.Fatalf("state %s targets %d", result.State, targets)
	}
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if *row["code"] != "delete-rollback" || version != "0" {
		t.Fatal("workflow wrote configuration")
	}
}

// AC-018: freezing records execution definitions, not changing table statistics
// or display descriptions. Every observed digest comes from the public submit.
func TestReleaseFreezeTracksExecutionSemantics(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	sequence := 0
	freeze := func() string {
		t.Helper()
		sequence++
		created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"proposal","label":"intent"},"detail_id":"0123456789abcdef0123456789abcdef","operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, fmt.Sprintf("schema-create-%02d", sequence))
		if created.Code != 201 {
			t.Fatal(created.Body)
		}
		var draft struct{ ID string }
		json.Unmarshal(created.Body.Bytes(), &draft)
		r := releaseRequest(t, app, "POST", "/api/v1/release-orders/"+draft.ID+"/submit", `{"expected_version":"1"}`, fmt.Sprintf("schema-submit-%02d", sequence))
		if r.Code != 200 {
			t.Fatal(r.Body)
		}
		var result struct {
			FrozenDigest string `json:"frozen_digest"`
		}
		json.Unmarshal(r.Body.Bytes(), &result)
		if strings.Contains(r.Body.String(), `"frozen":`) || strings.Contains(r.Body.String(), `"frozen_tables":`) || strings.Contains(r.Body.String(), `"record_table"`) || strings.Contains(r.Body.String(), `"record_key"`) {
			t.Fatalf("internal metadata leaked: %s", r.Body)
		}
		var frozen string
		if err := owner.QueryRow("SELECT JSON_EXTRACT(document,'$.frozen_tables.mutation_add_items') FROM rcc_release_orders WHERE id=?", draft.ID).Scan(&frozen); err != nil {
			t.Fatal(err)
		}

		for _, value := range []string{"mysql-8.4-execution-v1", "defaulted_value", "checks", "STRICT_TRANS_TABLES"} {
			if !strings.Contains(frozen, value) {
				t.Fatalf("missing stored execution evidence %s", value)
			}
		}
		return result.FrozenDigest
	}
	original := freeze()
	for _, statement := range []string{`INSERT INTO mutation_add_items(id,code,label) VALUES(200,'unrelated','other')`, `ANALYZE TABLE mutation_add_items`, `UPDATE rcc_mutation_policies SET name='Renamed rule',description='description only' WHERE code=(SELECT mutation_policy_code FROM rcc_table_policies WHERE table_name='mutation_add_items')`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if got := freeze(); got != original {
		t.Fatal("mutable statistics, auto-increment cursor or descriptions changed frozen semantics")
	}
	previous := original
	for _, statement := range []string{`ALTER TABLE mutation_add_items ALTER COLUMN defaulted_value SET DEFAULT 'changed-default'`, `ALTER TABLE mutation_add_items MODIFY generated_value varchar(128) GENERATED ALWAYS AS (concat(code,':changed')) STORED`, `CREATE TRIGGER final_label BEFORE INSERT ON mutation_add_items FOR EACH ROW SET NEW.label=CONCAT(NEW.label,'!')`, `ALTER TABLE mutation_add_items ADD UNIQUE KEY unique_label(label)`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
		current := freeze()
		if current == previous {
			t.Fatalf("execution change not captured: %s", statement)
		}
		previous = current
	}
	// Test a description-only DDL on a table with no stored expressions. MySQL
	// may otherwise rewrite literal character sets while ALTER reserializes them.
	if _, err := owner.Exec("DROP TRIGGER final_label"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec("ALTER TABLE mutation_add_items DROP COLUMN generated_value, DROP CHECK chk_mutation_add_items_label"); err != nil {
		t.Fatal(err)
	}
	plain := freeze()
	if _, err := owner.Exec("ALTER TABLE mutation_add_items COMMENT='description only'"); err != nil {
		t.Fatal(err)
	}
	if got := freeze(); got != plain {
		t.Fatal("description-only table comment changed execution semantics")
	}
	preview := releaseRequest(t, app, "POST", "/api/v1/release-orders/preview", `{"items":[{"content":{"code":"known","id":"201","label":"known"},"detail_id":"0123456789abcdef0123456789abcdef","operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "")
	if preview.Code != 200 || strings.Contains(preview.Body.String(), `"record_table"`) || strings.Contains(preview.Body.String(), `"record_key"`) {
		t.Fatalf("preview leaked internal identity: %s", preview.Body)
	}
}

// Metadata must be provably visible. A global TRIGGER grant with a schema
// restriction must not make hidden triggers look absent, and schema pattern
// matching must remain correct under NO_BACKSLASH_ESCAPES.
func TestReleaseFreezeMetadataVisibility(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"permission","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "metadata-create-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var draft struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &draft)
	path := "/api/v1/release-orders/" + draft.ID
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	for _, statement := range []string{`CREATE TRIGGER visible_target BEFORE INSERT ON mutation_add_items FOR EACH ROW SET NEW.label=CONCAT(NEW.label,'!')`, `SET GLOBAL sql_mode=CONCAT(@@global.sql_mode,',NO_BACKSLASH_ESCAPES')`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	sqlModeApp, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlModeApp.Close()
	submitted := releaseRequest(t, sqlModeApp, "POST", path+"/submit", `{"expected_version":"1"}`, "metadata-submit-01")
	if submitted.Code != 200 {
		t.Fatalf("explicit schema grant with NO_BACKSLASH_ESCAPES: %s", submitted.Body)
	}
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"2","reason":"permission test"}`, "metadata-cancel-01")
	if cancelled.Code != 200 {
		t.Fatal(cancelled.Body)
	}
	created = releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"restricted","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "metadata-create-02")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	json.Unmarshal(created.Body.Bytes(), &draft)
	path = "/api/v1/release-orders/" + draft.ID
	for _, statement := range []string{`SET GLOBAL partial_revokes=ON`, `CREATE USER 'metadata_limited'@'%' IDENTIFIED BY 'rcc_password'`, `GRANT SELECT,INSERT,UPDATE,DELETE ON rcc_test.* TO 'metadata_limited'@'%'`, `GRANT TRIGGER ON *.* TO 'metadata_limited'@'%'`, `REVOKE TRIGGER ON rcc_test.* FROM 'metadata_limited'@'%'`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	limitedDriver := *driver
	limitedDriver.User = "metadata_limited"
	limited := deliveryDB(t, &limitedDriver)
	var hidden int
	if err := limited.QueryRow("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE EVENT_OBJECT_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE='mutation_add_items'").Scan(&hidden); err != nil || hidden != 0 {
		t.Fatalf("trigger fixture not hidden: %d %v", hidden, err)
	}
	restricted, err := newApplication(ctx, integrationConfig(&limitedDriver))
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	assertIntegrationErrorCode(t, releaseRequest(t, restricted, "POST", path+"/submit", `{"expected_version":"1"}`, "restricted-submit-01"), 422, "release_metadata_permission")
	// Restore a directly provable table grant without a global privilege ambiguity.
	for _, statement := range []string{`REVOKE TRIGGER ON *.* FROM 'metadata_limited'@'%'`, `GRANT TRIGGER ON rcc_test.mutation_add_items TO 'metadata_limited'@'%'`} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	success := releaseRequest(t, restricted, "POST", path+"/submit", `{"expected_version":"1"}`, "restricted-submit-01")
	if success.Code != 200 {
		t.Fatalf("explicit restored grant: %s", success.Body)
	}
}

// AC-018: a submitted baseline and current rules must both still be valid.
func TestReleaseSubmitRevalidatesBaselineAndRules(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	body := `{"items":[{"content":{"code":"proposal"},"expected_record_version":"0","id":"1","operation":"MODIFY","table_name":"mutation_delete_parents"}],"title":"集成测试发布单"}`
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "revalidate-create-01")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var order struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &order)
	path := "/api/v1/release-orders/" + order.ID
	disabled := policyIntegrationRequest(t, app, "POST", "/api/v1/table-policies/mutation_delete_parents/disable", "")
	if disabled.Code != 200 {
		t.Fatal(disabled.Body)
	}
	r := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "revalidate-submit-01")
	assertIntegrationErrorCode(t, r, 403, "table_policy_disabled")
	enabled := policyIntegrationRequest(t, app, "POST", "/api/v1/table-policies/mutation_delete_parents/enable", "")
	if enabled.Code != 200 {
		t.Fatal(enabled.Body)
	}
	// An external maintenance writer can bypass platform reservations; submit must
	// still compare the real old row before freezing.
	deliveryExec(t, db, `UPDATE mutation_delete_parents SET code='later' WHERE id=1`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "revalidate-submit-01"), 409, "record_version_conflict")
	read := releaseReadAllDetails(t, app, "GET", path, "", "")
	if read.Body.String() != created.Body.String() {
		t.Fatal(read.Body)
	}
}

// The execution metadata read holds the actual table definition until the
// owning transaction completes, even for an ADD with no known record identity.
func TestReleaseExecutionSchemaHoldsMetadataLock(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	ddl, err := owner.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ddl.Close()
	if _, err := ddl.ExecContext(ctx, "SET SESSION lock_wait_timeout=1"); err != nil {
		t.Fatal(err)
	}
	acquired, finish, done := make(chan error, 1), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- app.mysql.ExecuteReleaseOrder(ctx, func(s application.ReleaseOrderSession) error {
			_, err := s.LockAndReadTableExecutionSchema(ctx, "mutation_add_items")
			acquired <- err
			<-finish
			return err
		})
	}()
	if err := <-acquired; err != nil {
		close(finish)
		<-done
		t.Fatal(err)
	}
	_, ddlErr := ddl.ExecContext(ctx, "ALTER TABLE mutation_add_items COMMENT='blocked until transaction completes'")
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(ddlErr, &mysqlError) || mysqlError.Number != 1205 {
		t.Fatalf("DDL crossed frozen metadata transaction: %v", ddlErr)
	}
	if _, err := ddl.ExecContext(ctx, "ALTER TABLE mutation_add_items COMMENT='allowed after transaction'"); err != nil {
		t.Fatal(err)
	}
}

// A zero AUTO_INCREMENT input is not necessarily an actual identity: MySQL's
// session mode decides whether it generates an id or inserts the literal zero.
func TestReleaseAutoIncrementZeroIdentity(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	actual, err := owner.Exec(`INSERT INTO mutation_add_items(id,code,label) VALUES(0,'database-generated-zero','proof')`)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := actual.LastInsertId()
	if err != nil || generated == 0 {
		t.Fatalf("fixture did not generate identity: %d %v", generated, err)
	}
	for i, code := range []string{"first zero proposal", "second zero proposal"} {
		body, _ := json.Marshal(map[string]any{"title": "集成测试发布单", "items": []any{map[string]any{"table_name": "mutation_add_items", "operation": "ADD", "content": map[string]string{"id": "0", "code": code, "label": "intent"}}}})
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), fmt.Sprintf("zero-create-%d", i)), 422, "release_auto_id_ambiguous")
	}
	var targets int
	if err := owner.QueryRow("SELECT COUNT(*) FROM rcc_release_targets").Scan(&targets); err != nil || targets != 0 {
		t.Fatalf("generated zero reserved: %d %v", targets, err)
	}
	if _, err := owner.Exec(`SET GLOBAL sql_mode=CONCAT(@@global.sql_mode,',NO_AUTO_VALUE_ON_ZERO')`); err != nil {
		t.Fatal(err)
	}
	exact, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer exact.Close()
	body := `{"items":[{"content":{"code":"literal-zero-0","id":"0","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	first := rollbackOrderResponse(t, releaseRequest(t, exact, "POST", "/api/v1/release-orders", body, "literal-zero-create-0"), 201)
	paths := []string{"/api/v1/release-orders/" + first.ID}
	assertIntegrationErrorCode(t, releaseRequest(t, exact, "POST", "/api/v1/release-orders", strings.Replace(body, "literal-zero-0", "literal-zero-1", 1), "literal-zero-create-1"), 409, "release_target_conflict")
	rollbackOrderResponse(t, releaseRequest(t, exact, "POST", paths[0]+"/submit", `{"expected_version":"1"}`, "literal-zero-submit-0"), 200)

	exactDB := deliveryDB(t, driver)
	if _, err := exactDB.Exec(`INSERT INTO mutation_add_items(id,code,label) VALUES(0,'literal-zero-proof','proof')`); err != nil {
		t.Fatal(err)
	}
	var actualZero int
	if err := exactDB.QueryRow("SELECT COUNT(*) FROM mutation_add_items WHERE id=0").Scan(&actualZero); err != nil || actualZero != 1 {
		t.Fatalf("literal zero proof: %d %v", actualZero, err)
	}
	// Remove the external identity proof, then publish the frozen literal zero.
	deliveryExec(t, exactDB, `DELETE FROM mutation_add_items WHERE id=0`)
	approved := releaseActorRequest(t, exact, publicationFixtureReviewer(t, exact), "POST", paths[0]+"/approve", `{"expected_version":"2","reason":"literal zero is the verified identity"}`, "literal-zero-approve")
	if approved.Code != 200 {
		t.Fatal(approved.Body)
	}
	command := publishedFixtureCommand(t, releaseRequest(t, exact, "POST", paths[0]+"/execute", `{"expected_version":"3"}`, "literal-zero-execute"))
	if command.ID != "0" || command.RecordVersion != "1" {
		t.Fatal("literal zero was treated as a generated id")
	}

}

// A grant on a different case-sensitive object cannot prove trigger visibility.
func TestReleaseFreezeMetadataGrantNameIdentity(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	var lowerCase int
	if err := owner.QueryRow("SELECT @@lower_case_table_names").Scan(&lowerCase); err != nil || lowerCase != 0 {
		t.Fatalf("case-sensitive fixture: %d %v", lowerCase, err)
	}
	for _, statement := range []string{
		`SET GLOBAL partial_revokes=ON`,
		`CREATE DATABASE RCC_TEST`,
		`CREATE TABLE Mutation_Add_Items LIKE mutation_add_items`,
		`CREATE TRIGGER case_target BEFORE INSERT ON mutation_add_items FOR EACH ROW SET NEW.label=CONCAT(NEW.label,'!')`,
		`CREATE USER 'case_limited'@'%' IDENTIFIED BY 'rcc_password'`,
		`CREATE USER 'CASE_LIMITED'@'%' IDENTIFIED BY 'rcc_password'`,
		`GRANT SELECT,INSERT,UPDATE,DELETE ON rcc_test.* TO 'case_limited'@'%'`,
		`GRANT SELECT ON mysql.* TO 'case_limited'@'%'`,
	} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	limitedDriver := *driver
	limitedDriver.User = "case_limited"
	limited := deliveryDB(t, &limitedDriver)
	restricted, err := newApplication(ctx, integrationConfig(&limitedDriver))
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	for _, tc := range []struct {
		name, object, grantee string
		partialRevokes        bool
	}{
		{"schema", "RCC_TEST.*", "'case_limited'@'%'", true},
		{"schema-pattern", "RCC_TEST.*", "'case_limited'@'%'", false},
		{"table", "rcc_test.Mutation_Add_Items", "'case_limited'@'%'", true},
		{"account", "rcc_test.mutation_add_items", "'CASE_LIMITED'@'%'", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := owner.Exec("SET GLOBAL partial_revokes=?", tc.partialRevokes); err != nil {
				t.Fatal(err)
			}
			if _, err := owner.Exec("GRANT TRIGGER ON " + tc.object + " TO " + tc.grantee); err != nil {
				t.Fatal(err)
			}
			defer owner.Exec("REVOKE TRIGGER ON " + tc.object + " FROM " + tc.grantee)
			var hidden int
			if err := limited.QueryRow("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE EVENT_OBJECT_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE='mutation_add_items'").Scan(&hidden); err != nil || hidden != 0 {
				t.Fatalf("target trigger must actually be hidden: %d %v", hidden, err)
			}
			created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"集成测试发布单","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"case-`+tc.name+`","label":"intent"}}]}`, "case-create-"+tc.name)
			if created.Code != 201 {
				t.Fatal(created.Body)
			}
			var draft struct{ ID string }
			json.Unmarshal(created.Body.Bytes(), &draft)
			path := "/api/v1/release-orders/" + draft.ID + "/submit"
			key := "case-submit-" + tc.name
			assertIntegrationErrorCode(t, releaseRequest(t, restricted, "POST", path, `{"expected_version":"1"}`, key), 422, "release_metadata_permission")
			if _, err := owner.Exec(`GRANT TRIGGER ON rcc_test.mutation_add_items TO 'case_limited'@'%'`); err != nil {
				t.Fatal(err)
			}
			defer owner.Exec(`REVOKE TRIGGER ON rcc_test.mutation_add_items FROM 'case_limited'@'%'`)
			if success := releaseRequest(t, restricted, "POST", path, `{"expected_version":"1"}`, key); success.Code != 200 {
				t.Fatalf("actual target grant retry: %s", success.Body)
			}
		})
	}

	t.Run("quoted-username-containing-at", func(t *testing.T) {
		for _, statement := range []string{
			`CREATE USER 'case@''\\limited'@'%' IDENTIFIED BY 'rcc_password'`,
			`GRANT SELECT,INSERT,UPDATE,DELETE ON rcc_test.* TO 'case@''\\limited'@'%'`,
			`GRANT TRIGGER ON rcc_test.mutation_add_items TO 'case@''\\limited'@'%'`,
		} {
			if _, err := owner.Exec(statement); err != nil {
				t.Fatal(err)
			}
		}
		atDriver := *driver
		atDriver.User = `case@'\limited`
		atApp, err := newApplication(ctx, integrationConfig(&atDriver))
		if err != nil {
			t.Fatal(err)
		}
		defer atApp.Close()
		created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"account-at","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "at-account-create")
		if created.Code != 201 {
			t.Fatal(created.Body)
		}
		var draft struct{ ID string }
		json.Unmarshal(created.Body.Bytes(), &draft)
		if success := releaseRequest(t, atApp, "POST", "/api/v1/release-orders/"+draft.ID+"/submit", `{"expected_version":"1"}`, "at-account-submit"); success.Code != 200 {
			t.Fatalf("exact account including at sign: %s", success.Body)
		}
	})
}

// A case-insensitive deployment still accepts a directly granted table identity.
func TestReleaseFreezeMetadataCaseInsensitiveNames(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("rcc_test"), tcmysql.WithUsername("rcc_admin"), tcmysql.WithPassword("rcc_password"),
		testcontainers.WithCmd("--lower-case-table-names=1"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Error(err)
		}
	}()
	driver, err := mysqldriver.ParseDSN(container.MustConnectionString(ctx, "parseTime=true"))
	if err != nil {
		t.Fatal(err)
	}
	initializeCurrentIntegrationSchema(t, ctx, driver, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	queryCode, mutationCode := createPolicyDefinitions(t, app, "mutation_add_items", queryPolicyFixture{}, mutationPolicyFixture{AllowAdd: true}, 1)
	assigned := policyIntegrationRequest(t, app, "POST", "/api/v1/table-policies", tablePolicyCodePayload("MUTATION_ADD_ITEMS", queryCode, mutationCode))
	if assigned.Code != 201 {
		t.Fatalf("uppercase policy assignment: %d %s", assigned.Code, assigned.Body.String())
	}
	setPolicyAssignmentEnabled(t, app, "MUTATION_ADD_ITEMS", true)
	duplicateAliases := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"same physical target","items":[{"table_name":"MUTATION_ADD_ITEMS","operation":"ADD","content":{"id":"99","code":"upper","label":"upper"}},{"table_name":"mutation_add_items","operation":"ADD","content":{"id":"99","code":"lower","label":"lower"}}]}`, "case-alias-duplicate")
	assertIntegrationErrorCode(t, duplicateAliases, 422, "release_duplicate_target")
	setDraftTestKey(t, app, "mutation_add_items", []string{"code"})
	aliasOwner := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"physical key owner","items":[{"table_name":"MUTATION_ADD_ITEMS","operation":"ADD","content":{"id":"98","code":"Alias-Key","label":"owner"}}]}`, "case-alias-owner"), 201)
	if len(aliasOwner.TableNames) != 1 || aliasOwner.TableNames[0] != "mutation_add_items" || aliasOwner.Items[0].TableName != "mutation_add_items" {
		t.Fatal("physical name not retained")
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"physical key contender","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"id":"97","code":"alias-key","label":"contender"}}]}`, "case-alias-contender"), 409, "release_target_conflict")
	assignment := policyIntegrationRequest(t, app, "GET", "/api/v1/table-policies/mutation_add_items", "")
	var policy map[string]any
	_ = json.Unmarshal(assignment.Body.Bytes(), &policy)
	changed, _ := json.Marshal(map[string]any{"table_name": "MUTATION_ADD_ITEMS", "query_policy_code": policy["query_policy_code"], "mutation_policy_code": policy["mutation_policy_code"], "concurrency_key": []string{}})
	assertIntegrationErrorCode(t, policyIntegrationRequest(t, app, "PUT", "/api/v1/table-policies/MUTATION_ADD_ITEMS", string(changed)), 409, "concurrency_key_in_use")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+aliasOwner.ID+"/cancel", `{"expected_version":"1","reason":"end physical identity fixture"}`, "case-alias-cancel"), 200)
	setDraftTestKey(t, app, "mutation_add_items", nil)
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	var mode int
	if err := owner.QueryRow("SELECT @@lower_case_table_names").Scan(&mode); err != nil || mode != 1 {
		t.Fatalf("case-insensitive fixture: %d %v", mode, err)
	}
	for _, statement := range []string{
		`SET GLOBAL partial_revokes=ON`,
		`CREATE USER 'case_insensitive'@'%' IDENTIFIED BY 'rcc_password'`,
		`GRANT SELECT,INSERT,UPDATE,DELETE ON rcc_test.* TO 'case_insensitive'@'%'`,
		`GRANT TRIGGER ON RCC_TEST.MUTATION_ADD_ITEMS TO 'case_insensitive'@'%'`,
		`CREATE TRIGGER same_case_target BEFORE INSERT ON mutation_add_items FOR EACH ROW SET NEW.label=CONCAT(NEW.label,'!')`,
	} {
		if _, err := owner.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	limitedDriver := *driver
	limitedDriver.User = "case_insensitive"
	limited := deliveryDB(t, &limitedDriver)
	var visible int
	if err := limited.QueryRow("SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE EVENT_OBJECT_SCHEMA=DATABASE() AND EVENT_OBJECT_TABLE='mutation_add_items'").Scan(&visible); err != nil || visible != 1 {
		t.Fatalf("actual same-table trigger visible: %d %v", visible, err)
	}
	restricted, err := newApplication(ctx, integrationConfig(&limitedDriver))
	if err != nil {
		t.Fatal(err)
	}
	defer restricted.Close()
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"code":"same-case","label":"intent"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "same-case-create")
	if created.Code != 201 {
		t.Fatal(created.Body)
	}
	var draft struct{ ID string }
	json.Unmarshal(created.Body.Bytes(), &draft)
	if success := releaseRequest(t, restricted, "POST", "/api/v1/release-orders/"+draft.ID+"/submit", `{"expected_version":"1"}`, "same-case-submit"); success.Code != 200 {
		t.Fatalf("same table direct grant: %s", success.Body)
	}
}
