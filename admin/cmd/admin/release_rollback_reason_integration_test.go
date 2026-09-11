//go:build integration

package main

import (
	"database/sql"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// #81 AC-013: only the recorded rollback executor or an administrator can
// append corrections, and those audit writes cannot rewrite execution facts.
func TestRollbackReasonCanBeCorrectedByExecutorOrAdministrator(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'reason-target','before')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})

	applicant := releaseReasonAccount(t, app, "reason.applicant", "EDITOR")
	reviewer := registerAccount(t, app, "reason.reviewer", "reason.reviewer@example.com", "correct horse battery staple")
	forwardPublisher := releaseReasonAccount(t, app, "reason.forward", "PUBLISHER")
	rollbackExecutor := releaseReasonAccount(t, app, "reason.rollback", "PUBLISHER")
	unrelated := registerAccount(t, app, "reason.unrelated", "reason.unrelated@example.com", "correct horse battery staple")

	created := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", `{"items":[{"content":{"label":"published"},"expected_record_version":"0","id":"10","operation":"MODIFY","table_name":"mutation_add_items"}],"title":"回滚原因留痕"}`, "reason-create"), 201)
	configurePublicationReviewer(t, app, reviewer, "mutation_add_items")
	path := "/api/v1/release-orders/" + created.ID
	submitted := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, "reason-submit"), 200)
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, reviewer, path, "independent review"), "reason-approve"), 200)
	published := rollbackOrderResponse(t, releaseActorRequest(t, app, forwardPublisher, "POST", path+"/execute", `{"expected_version":"3"}`, "reason-publish"), 200)
	preview := readQuickPreview(t, app, rollbackExecutor, path, published.Version)
	rolled := rollbackOrderResponse(t, releaseActorRequest(t, app, rollbackExecutor, "POST", path+"/quick-rollback", quickRollbackBody(published.Version, preview.Digest, ""), "reason-rollback"), 200)
	if submitted.ApplicantID != accountID(t, applicant) || approved.ApplicantID != accountID(t, applicant) || len(rolled.Executions) < 2 || rolled.Executions[1].ActorID != accountID(t, rollbackExecutor) || rolled.Executions[0].ActorID != accountID(t, forwardPublisher) {
		t.Fatal("fixture identities did not remain distinct")
	}

	// The named executor keeps this right even after losing PUBLISHER. The
	// endpoint must not inherit a generic publisher/editor middleware gate.
	grantReleaseRole(t, app, rollbackExecutor, `["VIEWER"]`, "2", "reason-rollback-viewer")
	detail := releaseActorReadAllDetails(t, app, rollbackExecutor, "GET", path, "", "")
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), `"edit-rollback-reason"`) {
		t.Fatalf("recorded executor action missing after role change: %d %s", detail.Code, detail.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, rollbackExecutor, "POST", path+"/rollback-reason", fmt.Sprintf(`{"reason":%q}`, strings.Repeat("界", 667)), "reason-too-long"), 422, "release_invalid")

	before := rollbackReasonFacts(t, db, rolled.ID)
	originalRequests := baselineRows(t, db, `SELECT * FROM rcc_release_requests WHERE operation NOT LIKE 'rollback-reason:%'`)
	immutableTables := map[string]string{}
	for _, table := range []string{"mutation_add_items", "rcc_release_details", "rcc_release_executions", "rcc_release_targets", "rcc_release_table_references", "rcc_publication_commands", "rcc_refresh_notifications", "rcc_record_versions", "rcc_table_publications"} {
		immutableTables[table] = baselineRows(t, db, "SELECT * FROM "+table)
	}
	assertStoredFacts := func() {
		t.Helper()
		if baselineRows(t, db, `SELECT * FROM rcc_release_requests WHERE operation NOT LIKE 'rollback-reason:%'`) != originalRequests {
			t.Fatal("reason correction rewrote original request actor, digest or saved result")
		}
		for table, original := range immutableTables {
			if baselineRows(t, db, "SELECT * FROM "+table) != original {
				t.Fatalf("reason correction rewrote stored %s facts", table)
			}
		}
	}
	firstBody := `{"reason":"数据库约束冲突，恢复上一版"}`
	firstResponse := releaseActorRequest(t, app, rollbackExecutor, "POST", path+"/rollback-reason", firstBody, "reason-first")
	first := rollbackOrderResponse(t, firstResponse, 200)
	if first.Version != rolled.Version || first.UpdatedAt != rolled.UpdatedAt {
		t.Fatal("reason edit advanced the workflow identity", first.Version, first.UpdatedAt)
	}

	for name, actor := range map[string]*httptest.ResponseRecorder{
		"applicant":         applicant,
		"forward-publisher": forwardPublisher,
		"unrelated":         unrelated,
	} {
		t.Run("deny-"+name, func(t *testing.T) {
			assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/rollback-reason", `{"reason":"must not write"}`, "reason-deny-"+name), 403, "permission_denied")
		})
	}

	correctedBody := `{"reason":"确认是外键约束冲突，已恢复上一版"}`
	correctedResponse := releaseRequest(t, app, "POST", path+"/rollback-reason", correctedBody, "reason-admin-correction")
	rollbackOrderResponse(t, correctedResponse, 200)
	if replay := releaseRequest(t, app, "POST", path+"/rollback-reason", correctedBody, "reason-admin-correction"); replay.Code != 200 || replay.Body.String() != correctedResponse.Body.String() {
		t.Fatalf("same reason request did not replay exactly: %d %s", replay.Code, replay.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/rollback-reason", `{"reason":"different"}`, "reason-admin-correction"), 409, "idempotency_conflict")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/rollback-reason", `{"reason":""}`, "reason-admin-clear"), 200)

	current := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
	for _, order := range []domain.ReleaseOrder{rolled, current} {
		context := order.ApprovalContext
		if len(context.Revision) != 64 || len(context.ApprovableTables) != 0 || len(context.Tables) != 1 || context.Tables[0].TableName != "mutation_add_items" || context.Tables[0].Mode != "COMPLETED" || context.Tables[0].CanApprove {
			t.Fatalf("completed rollback advertised live approval authority: %+v", context)
		}
	}
	if current.ApprovalContext.Revision == rolled.ApprovalContext.Revision {
		t.Fatal("current approval context did not distinguish changed actor/qualification")
	}
	expected := rolled
	expected.ApprovalContext = current.ApprovalContext
	if !reflect.DeepEqual(withoutRollbackReasons(current), expected) {
		t.Fatal("reason history rewrote immutable order or execution facts")
	}
	assertStoredFacts()
	revisions := []domain.ReleaseEvent{}
	for _, event := range current.History {
		if event.Action == "ROLLBACK_REASON" {
			revisions = append(revisions, event)
		}
	}
	if len(revisions) != 3 || revisions[0].ActorID != accountID(t, rollbackExecutor) || revisions[0].Reason != "数据库约束冲突，恢复上一版" || revisions[1].ActorID == revisions[0].ActorID || revisions[1].Reason != "确认是外键约束冲突，已恢复上一版" || revisions[2].ActorID != revisions[1].ActorID || revisions[2].Reason != "" {
		t.Fatal("reason corrections did not retain real actor and content", revisions)
	}
	for _, event := range revisions {
		if event.At == "" || event.Version != rolled.Version || event.ExecutionID != rolled.Executions[1].ID {
			t.Fatal("reason correction is not tied to the immutable rollback execution", event)
		}
	}
	after := rollbackReasonFacts(t, db, rolled.ID)
	if before.business != after.business || before.detail != after.detail || before.execution != after.execution || before.versions != after.versions || before.commands != after.commands || before.notifications != after.notifications || before.targets != after.targets {
		t.Fatal("reason correction changed publication facts", before, after)
	}
	if after.requests != before.requests+3 {
		t.Fatal("only the three committed logical reason requests should persist", before.requests, after.requests)
	}

	// A failed audit write leaves neither fabricated history nor an occupied
	// request key. The same original key/body can be submitted manually later.
	deliveryExec(t, db, `CREATE TRIGGER fail_reason_history BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='reason history unavailable'`)
	failed := releaseActorRequest(t, app, rollbackExecutor, "POST", path+"/rollback-reason", `{"reason":"故障后仍保留的输入"}`, "reason-storage-failure")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	deliveryExec(t, db, `DROP TRIGGER fail_reason_history`)
	unchanged := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(unchanged, current) {
		t.Fatal("failed persistence fabricated rollback reason history")
	}
	if count := rollbackReasonRequestCount(t, db, accountID(t, rollbackExecutor), rolled.ID, "reason-storage-failure"); count != 0 {
		t.Fatal("failed persistence occupied the request key", count)
	}
	succeeded := rollbackOrderResponse(t, releaseActorRequest(t, app, rollbackExecutor, "POST", path+"/rollback-reason", `{"reason":"故障后仍保留的输入"}`, "reason-storage-failure"), 200)
	if got := succeeded.History[len(succeeded.History)-1]; got.Action != "ROLLBACK_REASON" || got.Reason != "故障后仍保留的输入" {
		t.Fatal("manual retry did not append the original input", got)
	}
	if facts := rollbackReasonFacts(t, db, rolled.ID); facts.requests != after.requests+1 || facts.business != before.business || facts.detail != before.detail || facts.execution != before.execution || facts.versions != before.versions || facts.commands != before.commands || facts.notifications != before.notifications || facts.targets != before.targets {
		t.Fatal("manual retry changed anything besides audited reason state", facts)
	}
	assertStoredFacts()
}

func withoutRollbackReasons(order domain.ReleaseOrder) domain.ReleaseOrder {
	history := make([]domain.ReleaseEvent, 0, len(order.History))
	for _, event := range order.History {
		if event.Action != "ROLLBACK_REASON" {
			history = append(history, event)
		}
	}
	order.History = history
	return order
}

func releaseReasonAccount(t *testing.T, app *adminApplication, username, role string) *httptest.ResponseRecorder {
	t.Helper()
	account := registerAccount(t, app, username, username+"@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, account, fmt.Sprintf(`[%q]`, role), "1", username+"-role")
	return account
}

type rollbackReasonSnapshot struct {
	business, detail, execution, versions      string
	commands, notifications, targets, requests int
}

func rollbackReasonFacts(t *testing.T, db *sql.DB, orderID string) rollbackReasonSnapshot {
	t.Helper()
	return rollbackReasonSnapshot{
		business:      rollbackReasonScalar(t, db, `SELECT CONCAT(label,':',COALESCE((SELECT lock_version FROM rcc_record_versions WHERE table_name='mutation_add_items' AND record_key<>X'' LIMIT 1),0)) FROM mutation_add_items WHERE id=10`),
		detail:        rollbackReasonScalar(t, db, `SELECT GROUP_CONCAT(CONCAT(position,':',SHA2(CAST(application AS CHAR),256),':',SHA2(CAST(publication AS CHAR),256),':',SHA2(CAST(rollback AS CHAR),256)) ORDER BY position) FROM rcc_release_details WHERE order_id=?`, orderID),
		execution:     rollbackReasonScalar(t, db, `SELECT GROUP_CONCAT(CONCAT(kind,':',HEX(execution_id),':',SHA2(CAST(document AS CHAR),256)) ORDER BY kind) FROM rcc_release_executions WHERE order_id=?`, orderID),
		versions:      rollbackReasonScalar(t, db, `SELECT GROUP_CONCAT(CONCAT(HEX(table_name),':',table_version,':',command_cursor) ORDER BY table_name) FROM rcc_table_publications`),
		commands:      rollbackReasonCount(t, db, `SELECT COUNT(*) FROM rcc_publication_commands WHERE order_id=?`, orderID),
		notifications: rollbackReasonCount(t, db, `SELECT COUNT(*) FROM rcc_refresh_notifications WHERE order_id=?`, orderID),
		targets:       rollbackReasonCount(t, db, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id=?`, orderID),
		requests:      rollbackReasonCount(t, db, `SELECT COUNT(*) FROM rcc_release_requests`),
	}
}

func rollbackReasonScalar(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	var value sql.NullString
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value.String
}

func rollbackReasonCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var value int
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func rollbackReasonRequestCount(t *testing.T, db *sql.DB, actor, orderID, key string) int {
	t.Helper()
	return rollbackReasonCount(t, db, `SELECT COUNT(*) FROM rcc_release_requests WHERE actor_id=? AND operation=? AND request_key=?`, actor, "rollback-reason:"+orderID, key)
}
