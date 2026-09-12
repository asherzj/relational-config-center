//go:build integration

package main

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// AC-019: a real multi-table publication notifies permanent participants,
// including default ADMIN decisions, after their eligibility has changed.
func TestReleaseNotificationsPublicationUsesActualParticipants(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	admin := integrationAdminSession(t, app)
	applicant := registerAccount(t, app, "result.applicant", "result.applicant@example.com", "correct horse battery staple")
	goods := registerAccount(t, app, "result.goods", "result.goods@example.com", "correct horse battery staple")
	replacement := registerAccount(t, app, "result.replacement", "result.replacement@example.com", "correct horse battery staple")
	publisher := registerAccount(t, app, "result.publisher", "result.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR","PUBLISHER"]`, "1", "result-editor")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "result-publisher")
	first := tableApprovalRole(t, app, "真实审批一", goods)
	second := tableApprovalRole(t, app, "真实审批二", goods)
	assignTableApproval(t, app, "mutation_add_items", first, second)
	order := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", `{"title":"多表真实参与提醒","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"participant","label":"actual"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":"participant","label":"actual"}}]}`, "result-create"), 201)
	path := "/api/v1/release-orders/" + order.ID
	rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, "result-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "商品已审"), "result-goods"), 200)
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/approve", confirmedApprovalBody(t, app, admin, path, "默认审批已审"), "result-default"), 200)
	if approved.Approvals[1].Decision == nil || approved.Approvals[1].Decision.Source != "ADMIN" {
		t.Fatalf("missing real default decision: %+v", approved.Approvals)
	}
	updateTableRole(t, app, first, "原角色新成员", true, replacement)
	updateTableRole(t, app, second, "原角色停用", false, goods)
	before := map[*httptest.ResponseRecorder]domain.ApprovalNotification{}
	for _, account := range []*httptest.ResponseRecorder{applicant, goods, admin, replacement, publisher} {
		before[account] = readApprovalProgress(t, app, account, path)
	}
	response := releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"4"}`, "result-execute")
	result := rollbackOrderResponse(t, response, 200)
	if result.State != "SUCCEEDED" || result.Version != "5" || len(result.Executions) != 1 || len(result.Executions[0].TableVersions) != 2 {
		t.Fatalf("publication result: %+v", result)
	}
	for _, account := range []*httptest.ResponseRecorder{applicant, goods, admin} {
		assertReleaseNotificationAdvance(t, app, account, path, before[account])
	}
	for _, account := range []*httptest.ResponseRecorder{replacement, publisher} {
		if got := readApprovalProgress(t, app, account, path); got != before[account] {
			t.Fatalf("unrelated or self reminder: %+v -> %+v", before[account], got)
		}
	}
	assertNotificationOrders(t, readNotificationPage(t, app, goods, "view=handled&unread=true"), path)
	beforeReplay := readApprovalProgress(t, app, goods, path)
	replay := releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"4"}`, "result-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() || readApprovalProgress(t, app, goods, path) != beforeReplay {
		t.Fatal("replay duplicated publication or reminder")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_approval_notifications WHERE order_id='` + order.ID + `'`: 3, `SELECT COUNT(*) FROM rcc_refresh_notifications WHERE JSON_UNQUOTE(JSON_EXTRACT(document,'$.status'))='NOT_CONNECTED'`: 2, `SELECT COUNT(*) FROM rcc_publication_commands`: 2})
}

func assertReleaseNotificationAdvance(t *testing.T, app *adminApplication, account *httptest.ResponseRecorder, path string, before domain.ApprovalNotification) domain.ApprovalNotification {
	t.Helper()
	var previous uint64
	if _, err := fmt.Sscan(before.Sequence, &previous); err != nil {
		t.Fatal(err)
	}
	got := readApprovalProgress(t, app, account, path)
	if got.Sequence != fmt.Sprint(previous+1) || !got.Unread || got.Pending {
		t.Fatalf("result reminder: before=%+v got=%+v", before, got)
	}
	return got
}

// Completion and original-order rollback each advance the same personal result
// aggregate once. A participant's own operation preserves their earlier unread.
func TestReleaseNotificationsTerminalResultsPreserveOwnUnread(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	for _, action := range []string{"complete", "quick-rollback"} {
		t.Run(action, func(t *testing.T) {
			path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "terminal-"+action)
			original := rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "terminal-publish-"+action), 200)
			beforeApplicant := readApprovalProgress(t, app, applicant, path)
			beforeReviewer := readApprovalProgress(t, app, reviewer, path)
			beforePublisher := readApprovalProgress(t, app, publisher, path)
			body := `{"expected_version":"4"}`
			if action == "quick-rollback" {
				preview := readQuickPreview(t, app, reviewer, path, "4")
				body = quickRollbackBody(preview.ExpectedVersion, preview.Digest, "还原真实配置")
			}
			response := releaseActorRequest(t, app, reviewer, "POST", path+"/"+action, body, "terminal-"+action)
			terminal := rollbackOrderResponse(t, response, 200)
			afterApplicant := assertReleaseNotificationAdvance(t, app, applicant, path, beforeApplicant)
			if got := readApprovalProgress(t, app, reviewer, path); got != beforeReviewer {
				t.Fatalf("own action consumed or duplicated earlier result: %+v -> %+v", beforeReviewer, got)
			}
			if got := readApprovalProgress(t, app, publisher, path); got != beforePublisher {
				t.Fatal("publisher alone became a recipient")
			}
			expectedExecutions := 1
			expectedState := "COMPLETED"
			expectedRows := 1
			if action == "quick-rollback" {
				expectedExecutions = 2
				expectedState = "ROLLED_BACK"
				expectedRows = 0
			}
			expectedVersion := "5"
			if action == "quick-rollback" {
				expectedVersion = "6"
			}
			if terminal.Version != expectedVersion || terminal.State != expectedState || len(terminal.Executions) != expectedExecutions {
				t.Fatalf("terminal result: %+v", terminal)
			}
			for table, notice := range original.Executions[0].Notifications {
				if terminal.Executions[0].Notifications[table] != notice || notice.Status != "NOT_CONNECTED" {
					t.Fatal("personal notification changed downstream publication")
				}
			}
			batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items WHERE code='terminal-` + action + `'`: expectedRows, `SELECT COUNT(*) FROM mutation_supplied_id_items WHERE id='terminal-` + action + `'`: expectedRows, `SELECT COUNT(*) FROM rcc_release_executions WHERE order_id='` + terminal.ID + `'`: expectedExecutions, `SELECT COUNT(*) FROM rcc_publication_commands WHERE order_id='` + terminal.ID + `'`: 2 * expectedExecutions, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + terminal.ID + `'`: 0})
			replay := releaseActorRequest(t, app, reviewer, "POST", path+"/"+action, body, "terminal-"+action)
			if replay.Code != 200 || replay.Body.String() != response.Body.String() || readApprovalProgress(t, app, applicant, path) != afterApplicant {
				t.Fatal("terminal replay created another fact")
			}
			acknowledgeApprovalProgress(t, app, applicant, path, beforeApplicant.Sequence)
			if !readApprovalProgress(t, app, applicant, path).Unread {
				t.Fatal("old detail swallowed terminal result")
			}
		})
	}
}

func releaseNotificationActors(t *testing.T, app *adminApplication) (*httptest.ResponseRecorder, *httptest.ResponseRecorder, *httptest.ResponseRecorder) {
	t.Helper()
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	applicant := registerAccount(t, app, "lifecycle.applicant", "lifecycle.applicant@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "lifecycle.reviewer", "lifecycle.reviewer@example.com", "correct horse battery staple")
	publisher := registerAccount(t, app, "lifecycle.publisher", "lifecycle.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR","PUBLISHER"]`, "1", "lifecycle-applicant")
	grantReleaseRole(t, app, reviewer, `["PUBLISHER"]`, "1", "lifecycle-reviewer")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "1", "lifecycle-publisher")
	role := tableApprovalRole(t, app, "生命周期审批", reviewer)
	assignTableApproval(t, app, "mutation_add_items", role)
	assignTableApproval(t, app, "mutation_supplied_id_items", role)
	return applicant, reviewer, publisher
}
func approvedReleaseNotificationOrder(t *testing.T, app *adminApplication, applicant, reviewer *httptest.ResponseRecorder, code string) string {
	t.Helper()
	body := fmt.Sprintf(`{"title":"多表生命周期 %s","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"actual"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":%q,"label":"actual"}}]}`, code, code, code)
	order := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", body, code+"-create"), 201)
	path := "/api/v1/release-orders/" + order.ID
	rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, code+"-submit"), 200)
	rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, reviewer, path, "两表通过"), code+"-approve"), 200)
	return path
}

// Reprepare is an independent workflow: only the source applicant receives its
// cancellation; the replacement is a draft without a submitted responsibility.
func TestReleaseNotificationsReprepareCancelsSourceAtomically(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, _ := releaseNotificationActors(t, app)
	admin := integrationAdminSession(t, app)
	path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "reprepare-notice")
	source := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, applicant, "GET", path, "", ""), 200)
	body := derivedDraftBody(t, source)
	before := readApprovalProgress(t, app, applicant, path)
	reviewerBefore := readApprovalProgress(t, app, reviewer, path)
	// Fail the existing applicant aggregate after saving both business orders.
	deliveryExec(t, db, `CREATE TRIGGER fail_reprepare_notice BEFORE UPDATE ON rcc_approval_notifications FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='source cancellation notice failure'`)
	failed := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-notice")
	deliveryExec(t, db, `DROP TRIGGER fail_reprepare_notice`)
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	current := readTableApprovalOrder(t, app, applicant, path)
	if current.State != "APPROVED" || current.Version != "3" || len(current.History) != len(source.History) || readApprovalProgress(t, app, applicant, path) != before {
		t.Fatal("failed reprepare retired source or changed notice")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'reprepare:%'`: 0, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + source.ID + `'`: 1})
	response := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-notice")
	replacement := rollbackOrderResponse(t, response, 201)
	latest := assertReleaseNotificationAdvance(t, app, applicant, path, before)
	if replacement.State != "DRAFT" || replacement.ApplicantID != accountID(t, admin) {
		t.Fatal("replacement inherited submitted approval")
	}
	current = readTableApprovalOrder(t, app, applicant, path)
	event := current.History[len(current.History)-1]
	if current.State != "CANCELLED" || event.Action != "REPREPARE" || event.RelatedOrderID != replacement.ID {
		t.Fatal("missing source cancellation relation")
	}
	if readApprovalProgress(t, app, reviewer, path) != reviewerBefore {
		t.Fatal("reprepare notified reviewer as release result")
	}
	for _, actor := range []*httptest.ResponseRecorder{admin, applicant, reviewer} {
		if got := readApprovalProgress(t, app, actor, "/api/v1/release-orders/"+replacement.ID); got.Sequence != "0" || got.Unread || got.Pending {
			t.Fatalf("draft fabricated notification: %+v", got)
		}
	}
	replay := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-notice")
	if replay.Code != 201 || replay.Body.String() != response.Body.String() || readApprovalProgress(t, app, applicant, path) != latest {
		t.Fatal("reprepare replay repeated source notification")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 2, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + source.ID + `'`: 0, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + replacement.ID + `'`: 1, `SELECT COUNT(*) FROM rcc_approval_notifications WHERE order_id='` + replacement.ID + `'`: 0})
}
