//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// #59 AC-001: success protects its records while unrelated records keep moving.
func TestReleaseCompletionRetainsKnownTargets(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'first','old'),(200,'second','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-known")
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-known-execute"), 200)
	if published.State != "SUCCEEDED" {
		t.Fatal("ordinary publication must await completion")
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"items":[{"content":{"label":"overlap"},"expected_record_version":"1","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-overlap-create"), 409, "release_target_conflict")
	other := approvePublication(t, app, reviewer, `{"items":[{"content":{"label":"unrelated"},"expected_record_version":"0","id":"200","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-other")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", other+"/execute", `{"expected_version":"3"}`, "completion-other-execute"), 200)
	row, version := recordVersionRow(t, app, "mutation_add_items", "200")
	if *row["label"] != "unrelated" || version != "1" {
		t.Fatal(fmt.Sprint("unrelated publication blocked: ", row, version))
	}
}

// #59 AC-007/008: ordinary rollback starts only after completion and requires
// independent approval; its successful reverse result is immediately terminal.
func TestReleaseCompletionPermanentlyClosesRollbackWindow(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'closed-window','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"完结不再回滚"}`, "closed-window")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "closed-publish"), 200)
	actor := integrationAdminSession(t, app)
	preview := readQuickPreview(t, app, actor, path, "4")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/complete", fmt.Sprintf(`{"expected_version":%q}`, preview.ExpectedVersion), "closed-complete"), 200)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"6"}`, fmt.Sprintf("preview-%d", publicationFixtureSequence.Add(1))), 422, "release_state_invalid")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback", quickRollbackBody("6", preview.Digest, ""), "closed-rollback"), 422, "release_state_invalid")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback", `{"expected_version":"6","reason":"old path"}`, "closed-legacy"), 404, "route_not_found")
	row, version := recordVersionRow(t, app, "mutation_add_items", "100")
	if *row["label"] != "published" || version != "1" {
		t.Fatal("closed window altered configuration")
	}
}

// #59 AC-007: a current publisher completes another publisher's result without
// another configuration write, publication version or notification.
func TestReleaseCompletionReleasesWithoutRepublishing(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'completion','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-release")
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-release-execute"), 200)
	publisher := registerAccount(t, app, "completion.publisher", "completion.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "completion-publisher-role")
	response := releaseActorRequest(t, app, publisher, "POST", path+"/complete", `{"expected_version":"4"}`, "completion-release-complete")
	completed := rollbackOrderResponse(t, response, 200)
	if completed.State != "COMPLETED" || completed.Version != "5" || !reflect.DeepEqual(executionCommands(published, "PUBLICATION"), executionCommands(completed, "PUBLICATION")) {
		t.Fatal("completion must preserve the exact publication and advance only workflow", response.Body)
	}
	event := completed.History[len(completed.History)-1]
	if event.Action != "COMPLETE" || event.ActorID != accountID(t, publisher) || event.Reason != "" {
		t.Fatal("completion did not record the current publisher")
	}
	row, version := recordVersionRow(t, app, "mutation_add_items", "100")
	if *row["label"] != "published" || version != "1" {
		t.Fatal("completion changed configuration", row, version)
	}
	draft := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"label":"next"},"expected_record_version":"1","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-after")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", draft+"/execute", `{"expected_version":"3"}`, "completion-after-execute"), 200)
	listed := releaseReadAllDetails(t, app, "GET", "/api/v1/release-orders?state=COMPLETED", "", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), completed.ID) {
		t.Fatal("completed state is not queryable", listed.Body)
	}
}

// #59 AC-002: generated identities and deleted identities remain protected.
func TestReleaseCompletionProtectsActualAndDeletedIDs(t *testing.T) {
	app, _ := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'deleted','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"code":"generated","label":"published"},"operation":"ADD","table_name":"mutation_add_items"},{"content":{},"expected_record_version":"0","id":"100","operation":"DELETE","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-identities")
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-identities-execute"), 200)
	bodies := []string{}
	for index, item := range []string{
		fmt.Sprintf(`{"table_name":"mutation_add_items","operation":"MODIFY","id":%q,"expected_record_version":"1","content":{"label":"overlap"}}`, executionCommands(published, "PUBLICATION")[0].ID),
		`{"table_name":"mutation_add_items","operation":"ADD","content":{"id":"100","code":"recreated","label":"new"}}`,
	} {
		body := `{"title":"集成测试发布单","items":[` + item + `]}`
		bodies = append(bodies, body)
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, fmt.Sprintf("completion-identity-create-%d", index)), 409, "release_target_conflict")
	}
	completePublicationFixture(t, app, path, "completion-identities-complete")
	for index, body := range bodies {
		draft := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, fmt.Sprintf("completion-identity-create-%d", index)), 201)
		rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+draft.ID+"/submit", `{"expected_version":"1"}`, fmt.Sprintf("completion-identity-submit-%d", index)), 200)
	}

}

// #59 AC-003/006/016: current roles, observed versions and original request keys
// remain authoritative even when another publisher competes or access changes.
func TestReleaseCompletionRolesConcurrencyAndReplay(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"code":"race","label":"published"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-race")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/complete", `{"expected_version":"3"}`, "completion-unpublished"), 422, "release_state_invalid")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-race-execute"), 200)
	actor := registerAccount(t, app, "completion.roles", "completion.roles@example.com", "correct horse battery staple")
	for index, roles := range []string{`["VIEWER"]`, `["EDITOR"]`} {
		if index > 0 {
			grantReleaseRole(t, app, actor, roles, fmt.Sprint(index), fmt.Sprintf("completion-role-%d", index))
		}
		assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"4"}`, "completion-roles"), 403, "permission_denied")
		view := releaseActorReadAllDetails(t, app, actor, "GET", path, "", "")
		if strings.Contains(view.Body.String(), `"allowed_actions":["complete"]`) {
			t.Fatal("forbidden completion action advertised")
		}
	}
	var roles, roleVersion int
	if err := db.QueryRow(`SELECT roles,role_version FROM rcc_accounts WHERE id=?`, accountID(t, actor)).Scan(&roles, &roleVersion); err != nil || roles != 2 || roleVersion != 2 {
		t.Fatalf("current EDITOR matrix grant: roles=%d version=%d error=%v", roles, roleVersion, err)
	}
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "2", "completion-role-publisher")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"3"}`, "completion-stale"), 409, "release_version_conflict")
	cookies, csrf := actor.Result().Cookies(), sessionCSRF(t, actor)
	type outcome struct {
		key      string
		response *httptest.ResponseRecorder
	}
	results := make(chan outcome, 2)
	for _, key := range []string{"completion-race-one", "completion-race-two"} {
		go func(key string) {
			results <- outcome{key, accountRequestFrom(app, "POST", path+"/complete", `{"expected_version":"4"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})}
		}(key)
	}
	var winner outcome
	successes, conflicts := 0, 0
	for range 2 {
		result := <-results
		if result.response.Code == 200 {
			successes++
			winner = result
		} else {
			assertIntegrationErrorCode(t, result.response, 409, "release_version_conflict")
			conflicts++
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("completion did not converge", successes, conflicts)
	}
	replay := releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"4"}`, winner.key)
	if replay.Code != 200 || replay.Body.String() != winner.response.Body.String() {
		t.Fatal("original completion result changed", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"5"}`, winner.key), 409, "idempotency_conflict")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"5"}`, "completion-terminal"), 422, "release_state_invalid")
	current := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
	if current.Version != "5" || len(current.History) != 5 {
		t.Fatal("duplicate completion history")
	}
	grantReleaseRole(t, app, actor, `["VIEWER"]`, "3", "completion-role-revoke")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/complete", `{"expected_version":"4"}`, winner.key), 403, "permission_denied")
}

// #59 fault decisions: real MySQL failures at every completion persistence edge
// leave the publication, workflow, versions and public conflict behavior intact.
func TestReleaseCompletionPersistenceFailureIsAtomic(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'atomic-complete','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-atomic")
	original := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-atomic-execute"), 200)
	conflictBody := `{"items":[{"content":{"label":"next"},"expected_record_version":"1","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", conflictBody, "completion-atomic-conflict"), 409, "release_target_conflict")

	reviewer := publicationFixtureReviewer(t, app)
	beforeNotice := readApprovalProgress(t, app, reviewer, path)
	for _, failure := range []struct{ table, event, condition string }{{"rcc_approval_notifications", "UPDATE", "TRUE"}, {"rcc_release_targets", "DELETE", "TRUE"}, {"rcc_release_orders", "UPDATE", "NEW.state='COMPLETED'"}, {"rcc_release_requests", "UPDATE", "NEW.result IS NOT NULL"}} {
		t.Run(failure.table, func(t *testing.T) {
			deliveryExec(t, db, fmt.Sprintf("CREATE TRIGGER fail_completion BEFORE %s ON %s FOR EACH ROW BEGIN IF %s THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='completion storage boundary'; END IF; END", failure.event, failure.table, failure.condition))
			assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/complete", `{"expected_version":"4"}`, "completion-atomic-complete"), 503, "release_unavailable")
			deliveryExec(t, db, `DROP TRIGGER fail_completion`)
			current := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
			if readApprovalProgress(t, app, reviewer, path) != beforeNotice {
				t.Fatal("failed completion changed personal result notification")
			}
			if !reflect.DeepEqual(original, current) {
				t.Fatal("failed completion changed order")
			}
			assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", conflictBody, "completion-atomic-conflict"), 409, "release_target_conflict")
			row, version := recordVersionRow(t, app, "mutation_add_items", "100")
			if *row["label"] != "published" || version != "1" {
				t.Fatal("completion failure changed configuration")
			}
		})
	}
	completePublicationFixture(t, app, path, "completion-atomic-complete")
	assertReleaseNotificationAdvance(t, app, reviewer, path, beforeNotice)
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", conflictBody, "completion-atomic-conflict"), 201)
}

func TestReleaseCompletionRequiresTrustedPublicationOnReadAndReplay(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"code":"integrity","label":"trusted"},"operation":"ADD","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-integrity")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-integrity-execute"), 200)
	completed := completePublicationFixture(t, app, path, "completion-integrity-complete")
	var stored []byte
	if err := db.QueryRow(`SELECT publication FROM rcc_release_details WHERE order_id=? AND position=0`, completed.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, db, `UPDATE rcc_release_details SET publication=NULL WHERE order_id=?`, completed.ID)
	for _, read := range []string{path} {
		assertIntegrationErrorCode(t, releaseReadAllDetails(t, app, "GET", read, "", ""), 503, "release_unavailable")
	}
	deliveryExec(t, db, `UPDATE rcc_release_details SET publication=? WHERE order_id=? AND position=0`, stored, completed.ID)
	deliveryExec(t, db, `UPDATE rcc_release_requests SET result=JSON_REMOVE(result,'$.executions') WHERE operation=?`, "complete:"+completed.ID)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/complete", `{"expected_version":"4"}`, "completion-integrity-complete"), 503, "release_unavailable")
}

// Long-lived pre-submit history is a persistence fixture. Completion and the
// terminal action must fit at the accepted boundary and close rollback.
func TestReleaseCompletionAtPublicationCapacityClosesRollbackWindow(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(100,'completion-capacity','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	reviewer := publicationFixtureReviewer(t, app)
	path := approvePublication(t, app, reviewer, `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"100","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"集成测试发布单"}`, "completion-capacity")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "completion-capacity-execute"), 200)
	var stored application.ReleaseOrder
	err := app.mysql.ExecuteReleaseOrder(t.Context(), func(session application.ReleaseOrderSession) error {
		var readErr error
		stored, readErr = session.GetReleaseOrder(t.Context(), strings.TrimPrefix(path, "/api/v1/release-orders/"))
		return readErr
	})
	if err != nil {
		t.Fatal(err)
	}
	stored = seedLongPublishedHistory(t, db, stored)
	// A valid ASCII approval opinion fills the remaining gap without changing
	// the executed configuration, actor, ordering or permitted reason length.
	limit := (8 << 20) - (64 << 10)
	for i := range stored.History {
		if stored.History[i].Action == "APPROVE" {
			stored.History[i].Reason = ""
			encoded, _ := json.Marshal(stored)
			padding := limit - len(encoded) - 16
			if padding < 0 || padding > 2000 {
				t.Fatalf("unreachable approval size: %d", padding)
			}
			stored.History[i].Reason = strings.Repeat("x", padding)
		}
	}
	encoded, _ := json.Marshal(stored)
	if len(encoded) != limit-16 {
		t.Fatalf("publication boundary not reached: %d", len(encoded))
	}
	deliveryExec(t, db, `UPDATE rcc_release_orders SET document=JSON_SET(document,'$.history',CAST(? AS JSON)) WHERE id=?`, mustJSONHistory(stored.History), stored.ID)
	body := fmt.Sprintf(`{"expected_version":%q}`, stored.Version)
	response := releaseRequest(t, app, "POST", path+"/complete", body, "completion-capacity-complete")
	if response.Code != 200 {
		t.Fatalf("valid published order cannot complete: %d %s", response.Code, response.Body.String())
	}
	completed := rollbackOrderResponse(t, response, 200)
	if completed.State != "COMPLETED" || !reflect.DeepEqual(executionCommands(completed, "PUBLICATION"), executionCommands(stored, "PUBLICATION")) {
		t.Fatal("completion changed publication")
	}
	replay := releaseRequest(t, app, "POST", path+"/complete", body, "completion-capacity-complete")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("capacity completion lost original result")
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", fmt.Sprintf(`{"expected_version":%q}`, completed.Version), fmt.Sprintf("preview-%d", publicationFixtureSequence.Add(1))), 422, "release_state_invalid")
}
