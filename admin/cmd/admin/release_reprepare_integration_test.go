//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// AC-009: the approved original and its targets are replaced atomically by a
// newly reviewed draft owned by the actual initiator. The approval never moves.
func TestReleaseReprepareReplacesApprovedOrderWithEditableDraft(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	applicant := registerAccount(t, app, "reprepare.applicant", "reprepare.applicant@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "reprepare.reviewer", "reprepare.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR","APPROVER"]`, "1", "reprepare-applicant-role")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "reprepare-reviewer-role")

	created := releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", `{"title":"继承后仍可编辑的标题","table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepared"}}]}`, "reprepare-create")
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var original struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + original.ID
	if submitted := releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, "reprepare-submit"); submitted.Code != 200 {
		t.Fatalf("submit: %d %s", submitted.Code, submitted.Body)
	}
	if approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"已独立核对"}`, "reprepare-approve"); approved.Code != 200 {
		t.Fatalf("approve: %d %s", approved.Code, approved.Body)
	}

	preview := releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders/preview", `{"title":"继承后仍可编辑的标题","table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepared"}}]}`, "")
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"expected_record_version":"0"`) {
		t.Fatalf("preview latest configuration: %d %s", preview.Code, preview.Body)
	}
	body := `{"expected_version":"3","confirmed":true,"items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepared"}}]}`
	reprepared := releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", body, "reprepare-apply")
	if reprepared.Code != 201 {
		t.Fatalf("reprepare: %d %s", reprepared.Code, reprepared.Body)
	}
	var draft struct {
		ID, Title, State, Version string
		ApplicantID               string `json:"applicant_id"`
		CopiedFromID              string `json:"copied_from_id"`
		History                   []struct {
			Action         string
			ActorID        string `json:"actor_id"`
			RelatedOrderID string `json:"related_order_id"`
		}
	}
	if err := json.Unmarshal(reprepared.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if draft.ID == "" || draft.ID == original.ID || draft.Title != "继承后仍可编辑的标题" || draft.ApplicantID != accountID(t, applicant) || draft.State != "DRAFT" || draft.Version != "1" || draft.CopiedFromID != original.ID || len(draft.History) != 1 || draft.History[0].Action != "REPREPARE" || draft.History[0].RelatedOrderID != original.ID {
		t.Fatalf("replacement draft: %s", reprepared.Body)
	}

	old := releaseActorRequest(t, app, applicant, "GET", path, "", "")
	if old.Code != 200 || !strings.Contains(old.Body.String(), `"state":"CANCELLED"`) || !strings.Contains(old.Body.String(), `"version":"4"`) || !strings.Contains(old.Body.String(), `"action":"REPREPARE"`) || !strings.Contains(old.Body.String(), `"related_order_id":"`+draft.ID+`"`) || strings.Contains(old.Body.String(), `"execute"`) {
		t.Fatalf("original cancellation and history: %d %s", old.Code, old.Body)
	}

	draftPath := "/api/v1/release-orders/" + draft.ID
	edited := releaseActorRequest(t, app, applicant, "PUT", draftPath, `{"title":"重新准备后修改的标题","table_name":"mutation_delete_parents","expected_version":"1","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepared"}}]}`, "reprepare-edit")
	if edited.Code != 200 || !strings.Contains(edited.Body.String(), `"title":"重新准备后修改的标题"`) {
		t.Fatalf("edit replacement title: %d %s", edited.Code, edited.Body)
	}
	if submitted := releaseActorRequest(t, app, applicant, "POST", draftPath+"/submit", `{"expected_version":"2"}`, "reprepare-resubmit"); submitted.Code != 200 {
		t.Fatalf("submit replacement: %d %s", submitted.Code, submitted.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, applicant, "POST", draftPath+"/approve", `{"expected_version":"3","reason":"不能沿用或自批"}`, "reprepare-self-approve"), 403, "permission_denied")
}

// AC-009: current ownership is checked at action time. An ADMIN may replace
// another applicant's approved order, and becomes the new applicant.
func TestReleaseReprepareRequiresCurrentApplicantEditorOrAdmin(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	applicant := registerAccount(t, app, "reprepare.owner", "reprepare.owner@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "reprepare.owner.reviewer", "reprepare.owner.reviewer@example.com", "correct horse battery staple")
	outsider := registerAccount(t, app, "reprepare.outsider", "reprepare.outsider@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR"]`, "1", "reprepare-owner-role")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "reprepare-owner-reviewer-role")
	grantReleaseRole(t, app, outsider, `["EDITOR"]`, "1", "reprepare-outsider-role")

	path := approvedOrderForReprepare(t, app, applicant, reviewer, "reprepare-owner")
	body := `{"expected_version":"3","confirmed":true,"items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepare-owner"}}]}`
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, outsider, "POST", path+"/reprepare", body, "reprepare-outsider"), 403, "permission_denied")
	grantReleaseRole(t, app, applicant, `["VIEWER"]`, "2", "reprepare-owner-revoked")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", body, "reprepare-revoked"), 403, "permission_denied")

	admin := integrationAdminSession(t, app)
	reprepared := releaseActorRequest(t, app, admin, "POST", path+"/reprepare", body, "reprepare-admin")
	if reprepared.Code != 201 || !strings.Contains(reprepared.Body.String(), `"applicant_id":"`+accountID(t, admin)+`"`) {
		t.Fatalf("admin replacement ownership: %d %s", reprepared.Code, reprepared.Body)
	}
	var draft struct{ ID string }
	if err := json.Unmarshal(reprepared.Body.Bytes(), &draft); err != nil {
		t.Fatal(err)
	}
	if submitted := releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders/"+draft.ID+"/submit", `{"expected_version":"1"}`, "reprepare-admin-submit"); submitted.Code != 200 {
		t.Fatal(submitted.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, admin, "POST", "/api/v1/release-orders/"+draft.ID+"/approve", `{"expected_version":"2","reason":"管理员也不能自批"}`, "reprepare-admin-self-approve"), 403, "permission_denied")
}

// AC-009: every explicit storage failure rolls back the old cancellation,
// target release, new draft, both histories and the idempotency record.
func TestReleaseReprepareFailureKeepsApprovedOrderAndTarget(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	applicant := registerAccount(t, app, "reprepare.atomic", "reprepare.atomic@example.com", "correct horse battery staple")
	reviewer := registerAccount(t, app, "reprepare.atomic.reviewer", "reprepare.atomic.reviewer@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, applicant, `["EDITOR"]`, "1", "reprepare-atomic-role")
	grantReleaseRole(t, app, reviewer, `["APPROVER"]`, "1", "reprepare-atomic-reviewer-role")
	path := approvedOrderForReprepare(t, app, applicant, reviewer, "reprepare-atomic")
	approved := releaseActorRequest(t, app, applicant, "GET", path, "", "")

	deliveryExec(t, owner, `CREATE TRIGGER reject_reprepared_draft BEFORE INSERT ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected reprepare failure'`)
	body := `{"expected_version":"3","confirmed":true,"items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"reprepare-atomic"}}]}`
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", body, "reprepare-atomic-apply"), 503, "release_unavailable")
	unchanged := releaseActorRequest(t, app, applicant, "GET", path, "", "")
	if unchanged.Body.String() != approved.Body.String() {
		t.Fatalf("explicit failure changed approved order: %s", unchanged.Body)
	}
	assertReprepareStorageCounts(t, owner, 1, 1, 0)
	deliveryExec(t, owner, `DROP TRIGGER reject_reprepared_draft`)

	recovered := releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", body, "reprepare-atomic-apply")
	if recovered.Code != 201 {
		t.Fatalf("retry after explicit failure: %d %s", recovered.Code, recovered.Body)
	}
	replay := releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", body, "reprepare-atomic-apply")
	if replay.Code != 201 || replay.Body.String() != recovered.Body.String() {
		t.Fatalf("idempotent recovery changed draft: %d %s", replay.Code, replay.Body)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, applicant, "POST", path+"/reprepare", strings.Replace(body, "reprepare-atomic", "changed-intent", 1), "reprepare-atomic-apply"), 409, "idempotency_conflict")
	assertReprepareStorageCounts(t, owner, 2, 0, 1)
}

func approvedOrderForReprepare(t *testing.T, app *adminApplication, applicant, reviewer *httptest.ResponseRecorder, key string) string {
	t.Helper()
	created := releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", `{"title":"待重新准备的批准单","table_name":"mutation_delete_parents","items":[{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"`+key+`"}}]}`, key+"-create")
	if created.Code != 201 {
		t.Fatalf("create approved fixture: %d %s", created.Code, created.Body)
	}
	var order struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + order.ID
	if submitted := releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, key+"-submit"); submitted.Code != 200 {
		t.Fatalf("submit approved fixture: %d %s", submitted.Code, submitted.Body)
	}
	if approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", `{"expected_version":"2","reason":"独立审批"}`, key+"-approve"); approved.Code != 200 {
		t.Fatalf("approve fixture: %d %s", approved.Code, approved.Body)
	}
	return path
}

func assertReprepareStorageCounts(t *testing.T, db *sql.DB, orders, targets, requests int) {
	t.Helper()
	for query, want := range map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                      orders,
		`SELECT COUNT(*) FROM rcc_release_targets`:                                     targets,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'reprepare:%'`: requests,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d, want %d (%v)", query, got, want, err)
		}
	}
}
