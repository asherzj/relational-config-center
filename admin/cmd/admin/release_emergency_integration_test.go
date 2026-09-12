//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// AC-012: selecting the order type is an explicit draft save; every participating
// table changes together and a replay retains the originally saved instances.
func TestReleaseEmergencyAC012SwitchReplacesWholeDraft(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	saved := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "emergency-switch-create"), 201)
	path := "/api/v1/release-orders/" + saved.ID
	body := `{"title":"应急整单切换","release_type":"EMERGENCY","expected_version":"1","changes":{"upserts":[],"delete_detail_ids":[]}}`
	switched := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "emergency-switch"), 200)
	if switched.ReleaseType != "EMERGENCY" || switched.Version != "2" || len(switched.TableFlows) != 2 || len(switched.MissingFlowTables) != 0 {
		t.Fatalf("whole draft did not switch: %+v", switched)
	}
	for i, flow := range switched.TableFlows {
		if flow.ReleaseType != "EMERGENCY" || len(flow.Nodes) != 2 || flow.Nodes[0].Type != "PUBLICATION" || flow.InstanceID == saved.TableFlows[i].InstanceID {
			t.Fatalf("mixed/old flow: %+v", flow)
		}
	}
	replay := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "emergency-switch"), 200)
	if !reflect.DeepEqual(switched.TableFlows, replay.TableFlows) || replay.Version != "2" {
		t.Fatal("replay replaced instances", replay)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, body, "emergency-stale"), 409, "release_version_conflict")
	back := flowResponse(t, releaseRequest(t, app, "PUT", path, `{"title":"切回常规","release_type":"STANDARD","expected_version":"2","changes":{"upserts":[],"delete_detail_ids":[]}}`, "emergency-back"), 200)
	if back.ReleaseType != "STANDARD" || len(back.TableFlows) != 2 || back.TableFlows[0].InstanceID == saved.TableFlows[0].InstanceID {
		t.Fatal("standard switch failed", back)
	}
}

// AC-015: a reason is required and submission creates publication readiness,
// never business values or an imaginary approval responsibility/decision.
func TestReleaseEmergencyAC015SubmitNeedsReasonAndDoesNotPublish(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	editor := registerAccount(t, app, "emergency.editor", "emergency.editor@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "emergency-editor-role")
	body := strings.Replace(flowDraftBody, `"title":`, `"release_type":"EMERGENCY","title":`, 1)
	created := flowResponse(t, releaseActorRequest(t, app, editor, "POST", "/api/v1/release-orders", body, "emergency-submit-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	for i, reason := range []string{"", "  \n\t", strings.Repeat("长", 2001)} {
		response := releaseActorRequest(t, app, editor, "POST", path+"/submit", fmt.Sprintf(`{"expected_version":"1","emergency_reason":%q}`, reason), fmt.Sprintf("emergency-bad-reason-%d", i))
		assertIntegrationErrorCode(t, response, 422, "release_emergency_reason")
	}
	before := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if before.State != "DRAFT" || before.Version != "1" {
		t.Fatal("rejected reason changed draft", before)
	}
	submitBody := `{"expected_version":"1","emergency_reason":"修复生产配置错误"}`
	response := releaseActorRequest(t, app, editor, "POST", path+"/submit", submitBody, "emergency-submit")
	saved := rollbackOrderResponse(t, response, 200)
	var payload map[string]any
	if json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload["emergency_reason"] != "修复生产配置错误" {
		t.Fatal("reason not retained", response.Body)
	}
	if saved.State != "PENDING_PUBLICATION" || len(saved.Approvals) != 0 || len(saved.ApprovalContext.Tables) != 0 || len(saved.Executions) != 0 {
		t.Fatal("submission fabricated approval/publication", saved)
	}
	for _, event := range saved.History {
		if event.Action == "APPROVE" || event.Action == "EXECUTE" {
			t.Fatal("false event", event)
		}
	}
	for _, flow := range saved.TableFlows {
		if flow.Nodes[0].State != "ACTIVE" || flow.Nodes[0].ActorID != "" {
			t.Fatal("publication node has false actor", flow)
		}
	}
	for _, table := range []string{"policy_alpha", "policy_beta"} {
		r := releaseRequest(t, app, "POST", "/api/v1/tables/"+table+"/query", `{}`, "")
		var rows struct {
			Rows []map[string]any `json:"rows"`
		}
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &rows) != nil || len(rows.Rows) != 0 {
			t.Fatalf("submission wrote data %s %d %s", table, r.Code, r.Body)
		}
	}
	list := releaseRequest(t, app, "GET", "/api/v1/release-orders?state=PENDING_PUBLICATION", "", "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), saved.ID) {
		t.Fatal("new state cannot be listed", list.Body)
	}
	replay := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", submitBody, "emergency-submit"), 200)
	if !reflect.DeepEqual(saved.TableFlows, replay.TableFlows) || replay.Version != saved.Version {
		t.Fatal("submit replay replaced instance", replay)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "PUT", path, `{"title":"too late","release_type":"STANDARD","expected_version":"2","changes":{"upserts":[]}}`, "emergency-switch-after-submit"), 422, "release_state_invalid")
}

// AC-016: authorization is current at execution, including the applicant.
func TestReleaseEmergencyAC016PublisherPermissionIsCurrent(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	editor := registerAccount(t, app, "emergency.publisher", "emergency.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "emergency-publisher-editor")
	body := strings.Replace(flowDraftBody, `"title":`, `"release_type":"EMERGENCY","title":`, 1)
	created := flowResponse(t, releaseActorRequest(t, app, editor, "POST", "/api/v1/release-orders", body, "emergency-permission-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	flowResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1","emergency_reason":"紧急修正"}`, "emergency-permission-submit"), 200)
	executeBody := `{"expected_version":"2"}`
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/execute", executeBody, "emergency-permission-execute"), 403, "permission_denied")
	grantReleaseRole(t, app, editor, `["EDITOR","PUBLISHER"]`, "2", "emergency-publisher-grant")
	eligible := flowResponse(t, releaseActorRequest(t, app, editor, "GET", path, "", ""), 200)
	if !slices.Contains(eligible.AllowedActions, "execute") {
		t.Fatal("current publisher applicant cannot execute", eligible)
	}
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "3", "emergency-publisher-revoke")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/execute", executeBody, "emergency-permission-execute"), 403, "permission_denied")
	grantReleaseRole(t, app, editor, `["EDITOR","PUBLISHER"]`, "4", "emergency-publisher-regrant")
	published := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/execute", executeBody, "emergency-permission-execute"), 200)
	if published.State != "SUCCEEDED" || len(published.Executions) != 1 || published.Executions[0].ActorID != accountID(t, editor) || len(published.Approvals) != 0 {
		t.Fatal("manual applicant publication failed", published)
	}
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "5", "emergency-publisher-revoke-replay")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/execute", executeBody, "emergency-permission-execute"), 403, "permission_denied")
	grantReleaseRole(t, app, editor, `["EDITOR","PUBLISHER"]`, "6", "emergency-publisher-regrant-replay")
	replay := rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/execute", executeBody, "emergency-permission-execute"), 200)
	if !reflect.DeepEqual(published.Executions, replay.Executions) || !reflect.DeepEqual(published.TableFlows, replay.TableFlows) {
		t.Fatal("manual original replay changed execution", replay)
	}
}

// AC-017/018: emergency uses the existing whole-order executor and protected
// completion lifecycle. These behaviors already exist; their real regression
// proves the new readiness state reaches that same transaction.
func TestReleaseEmergencyAC017AC018WholeOrderExecutionAndCompletion(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE emergency_parents(id INT PRIMARY KEY,label VARCHAR(80))`)
	deliveryExec(t, db, `CREATE TABLE emergency_children(id INT PRIMARY KEY,parent_id INT NOT NULL,label VARCHAR(80),FOREIGN KEY(parent_id) REFERENCES emergency_parents(id))`)
	deliveryExec(t, db, `INSERT INTO emergency_parents VALUES(1,'old parent')`)
	deliveryExec(t, db, `INSERT INTO emergency_children VALUES(1,1,'old child')`)
	for _, table := range []string{"emergency_parents", "emergency_children"} {
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	}
	body := `{"title":"应急跨表迁移","release_type":"EMERGENCY","items":[{"table_name":"emergency_parents","operation":"ADD","content":{"id":"2","label":"new parent"}},{"table_name":"emergency_children","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"parent_id":"2"}},{"table_name":"emergency_parents","operation":"DELETE","id":"1","expected_record_version":"0","content":{}}]}`
	created := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "emergency-whole-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	submitted := flowResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1","emergency_reason":"跨表紧急修正"}`, "emergency-whole-submit"), 200)
	if !slices.Contains(submitted.AllowedActions, "execute") {
		t.Fatal("publisher not offered execution", submitted)
	}
	t.Run("execution_failure_rolls_back_all_tables_and_request_result", func(t *testing.T) {
		deliveryExec(t, db, `CREATE TRIGGER reject_emergency_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'execute:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='reject entire emergency commit'; END IF; END`)
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"2"}`, "emergency-whole-execute"), 503, "release_unavailable")
		deliveryExec(t, db, `DROP TRIGGER reject_emergency_result`)
		row, v := recordVersionRow(t, app, "emergency_children", "1")
		if *row["parent_id"] != "1" || v != "0" {
			t.Fatal("partial child write", row, v)
		}
		row, v = recordVersionRow(t, app, "emergency_parents", "1")
		if *row["label"] != "old parent" || v != "0" {
			t.Fatal("partial parent write", row, v)
		}
		current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
		if current.State != "PENDING_PUBLICATION" || len(current.Executions) != 0 || current.Version != "2" {
			t.Fatal("failed transaction recorded success", current)
		}
		found := false
		for _, event := range current.History {
			found = found || event.Action == "EXECUTE_FAILED"
		}
		if !found {
			t.Fatal("known failure absent")
		}
	})
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"2"}`, "emergency-whole-execute"), 200)
	if published.State != "SUCCEEDED" || published.Version != "3" || len(published.Executions) != 1 || len(published.Executions[0].TableVersions) != 2 {
		t.Fatal("whole-order publication missing", published)
	}
	commands := executionCommands(published, "PUBLICATION")
	if len(commands) != 3 || commands[0].TableName != "emergency_parents" || commands[1].TableName != "emergency_children" || commands[2].TableName != "emergency_parents" {
		t.Fatal("global order lost", commands)
	}
	row, v := recordVersionRow(t, app, "emergency_children", "1")
	if *row["parent_id"] != "2" || v != "1" {
		t.Fatal("missing final child", row, v)
	}
	for _, flow := range published.TableFlows {
		if flow.Nodes[0].State != "COMPLETED" || flow.Nodes[0].ActorID != integrationAccountID(t, app) || flow.Nodes[1].State != "ACTIVE" {
			t.Fatal("false real node progress", flow)
		}
	}
	contender := `{"title":"保护中的目标","release_type":"EMERGENCY","items":[{"table_name":"emergency_children","operation":"MODIFY","id":"1","expected_record_version":"1","content":{"label":"later"}}]}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", contender, "emergency-target-after-publication"), 409, "release_target_conflict")
	preview := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"3"}`, fmt.Sprintf("preview-%d", publicationFixtureSequence.Add(1)))
	if preview.Code != 200 {
		t.Fatal("protected rollback entrance disappeared", preview.Body)
	}
	completed := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/complete", `{"expected_version":"4"}`, "emergency-whole-complete"), 200)
	if completed.State != "COMPLETED" || len(completed.Executions) != 1 {
		t.Fatal("completion invented execution", completed)
	}
	row, v = recordVersionRow(t, app, "emergency_children", "1")
	if *row["parent_id"] != "2" || v != "1" {
		t.Fatal("completion changed business", row, v)
	}
	for _, flow := range completed.TableFlows {
		if flow.Nodes[1].State != "COMPLETED" || flow.Nodes[1].ActorID != integrationAccountID(t, app) || flow.Nodes[1].At == "" {
			t.Fatal("completion lacks actual actor/time", flow)
		}
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"5"}`, fmt.Sprintf("preview-%d", publicationFixtureSequence.Add(1))), 422, "release_state_invalid")
	flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", contender, "emergency-target-after-publication"), 201)
	old := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"2"}`, "emergency-whole-execute"), 200)
	if old.State != "SUCCEEDED" || old.Version != "3" || !reflect.DeepEqual(old.Executions, published.Executions) {
		t.Fatal("original result lost", old)
	}
	current := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "COMPLETED" || current.Version != "5" {
		t.Fatal("historical replay changed current", current)
	}
}

// AC-023/012: real dependency failures during a switch roll back content,
// instances and result together; the same original request remains replayable.
func TestReleaseEmergencyAC023SwitchFailureAndOriginalResults(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	created := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "emergency-failure-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	body := `{"title":"失败也保留整单输入","release_type":"EMERGENCY","expected_version":"1","changes":{"upserts":[],"delete_detail_ids":[]}}`
	unchanged := func() {
		t.Helper()
		current := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
		if current.Version != "1" || current.ReleaseType != "STANDARD" || current.Title != created.Title || !reflect.DeepEqual(current.TableFlows, created.TableFlows) {
			t.Fatal("failed switch changed draft", current)
		}
	}
	deliveryExec(t, db, `RENAME TABLE rcc_release_templates TO unavailable_emergency_templates`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, body, "emergency-failed-switch"), 503, "release_unavailable")
	unchanged()
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", strings.Replace(flowDraftBody, `"title":`, `"release_type":"EMERGENCY","title":`, 1), "emergency-failed-create"), 503, "release_unavailable")
	deliveryExec(t, db, `RENAME TABLE unavailable_emergency_templates TO rcc_release_templates`)
	deliveryExec(t, db, `CREATE TRIGGER reject_emergency_instance BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='instance save unavailable'`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, body, "emergency-failed-switch"), 503, "release_unavailable")
	unchanged()
	deliveryExec(t, db, `DROP TRIGGER reject_emergency_instance`)
	deliveryExec(t, db, `CREATE TRIGGER reject_emergency_switch_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'edit:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='switch original result unavailable'; END IF; END`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, body, "emergency-failed-switch"), 503, "release_unavailable")
	unchanged()
	deliveryExec(t, db, `DROP TRIGGER reject_emergency_switch_result`)
	switched := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "emergency-failed-switch"), 200)
	if switched.ReleaseType != "EMERGENCY" || switched.Version != "2" {
		t.Fatal(switched)
	}
	submit := `{"expected_version":"2","emergency_reason":"原包恢复应急原因"}`
	deliveryExec(t, db, `CREATE TRIGGER reject_emergency_submit_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'submit:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='submit original result unavailable'; END IF; END`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/submit", submit, "emergency-failed-submit"), 503, "release_unavailable")
	current := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "DRAFT" || current.Version != "2" || current.EmergencyReason != "" {
		t.Fatal("failed submit froze partial state", current)
	}
	deliveryExec(t, db, `DROP TRIGGER reject_emergency_submit_result`)
	submitted := flowResponse(t, releaseRequest(t, app, "POST", path+"/submit", submit, "emergency-failed-submit"), 200)
	// A later template edit cannot change the original switch or submit result.
	update := strings.TrimSuffix(strings.ReplaceAll(emergencyReleaseTemplateBody, "urgent_release_v1", "default_emergency_v1"), "}") + `,"expected_version":"1"}`
	if r := templateRequest(t, app, "PUT", "/api/v1/release-templates/default_emergency_v1", update, "emergency-after-commit-template"); r.Code != 200 {
		t.Fatal(r.Body)
	}
	replay := flowResponse(t, releaseRequest(t, app, "PUT", path, body, "emergency-failed-switch"), 200)
	if !reflect.DeepEqual(replay.TableFlows, switched.TableFlows) || replay.State != "DRAFT" || replay.Version != "2" {
		t.Fatal("historical switch replaced", replay)
	}
	replay = flowResponse(t, releaseRequest(t, app, "POST", path+"/submit", submit, "emergency-failed-submit"), 200)
	if !reflect.DeepEqual(replay.TableFlows, submitted.TableFlows) || replay.Version != "3" || replay.State != "PENDING_PUBLICATION" {
		t.Fatal("historical submit replaced", replay)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/submit", strings.ReplaceAll(submit, "原包恢复应急原因", "不同意图"), "emergency-failed-submit"), 409, "idempotency_conflict")
}

// AC-012: the two HTTP requests overlap at real database locks. Each ordering
// admits exactly one versioned transition and rejects the stale other window.
func TestReleaseEmergencyAC012SwitchAndSubmitCompetition(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	reviewer := publicationFixtureReviewer(t, app)
	configurePublicationReviewer(t, app, reviewer, "policy_alpha", "policy_beta")
	admin := integrationAdminSession(t, app)
	cookies, csrf := admin.Result().Cookies(), sessionCSRF(t, admin)
	request := func(method, path, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, method, path, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	}
	for _, first := range []string{"switch", "submit"} {
		t.Run(first+"_wins", func(t *testing.T) {
			created := flowResponse(t, request("POST", "/api/v1/release-orders", flowDraftBody, "emergency-race-create-"+first), 201)
			path := "/api/v1/release-orders/" + created.ID
			switchBody := `{"title":"切换与提交竞争","release_type":"EMERGENCY","expected_version":"1","changes":{"upserts":[],"delete_detail_ids":[]}}`
			gate, err := db.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			var got int
			if err = gate.QueryRowContext(ctx, `SELECT GET_LOCK('emergency_race_gate',5)`).Scan(&got); err != nil || got != 1 {
				t.Fatal(err)
			}
			defer gate.ExecContext(ctx, `SELECT RELEASE_LOCK('emergency_race_gate')`)
			deliveryExec(t, db, `CREATE TRIGGER pause_emergency_race BEFORE UPDATE ON rcc_release_orders FOR EACH ROW BEGIN SET @entered=GET_LOCK('emergency_race_entered',0); SET @wait=GET_LOCK('emergency_race_gate',12); SET @done=RELEASE_LOCK('emergency_race_entered'); SET @opened=RELEASE_LOCK('emergency_race_gate'); END`)
			defer deliveryExec(t, db, `DROP TRIGGER IF EXISTS pause_emergency_race`)
			method, url, input := "PUT", path, switchBody
			otherMethod, otherURL, otherInput := "POST", path+"/submit", `{"expected_version":"1"}`
			if first == "submit" {
				method, otherMethod = otherMethod, method
				url, otherURL = otherURL, url
				input, otherInput = otherInput, input
			}
			winner := make(chan *httptest.ResponseRecorder, 1)
			loser := make(chan *httptest.ResponseRecorder, 1)
			go func() { winner <- request(method, url, input, "emergency-race-first-"+first) }()
			awaitFlowDatabaseCondition(t, db, `SELECT IF(IS_USED_LOCK('emergency_race_entered') IS NULL,0,1)`)
			go func() { loser <- request(otherMethod, otherURL, otherInput, "emergency-race-second-"+first) }()
			awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_auth_control_lock")
			if err = gate.QueryRowContext(ctx, `SELECT RELEASE_LOCK('emergency_race_gate')`).Scan(&got); err != nil || got != 1 {
				t.Fatal(err)
			}
			saved := flowResponse(t, receiveFlowResponse(t, winner), 200)
			assertIntegrationErrorCode(t, receiveFlowResponse(t, loser), 409, "release_version_conflict")
			current := flowResponse(t, request("GET", path, "", ""), 200)
			if current.Version != "2" || !reflect.DeepEqual(current.TableFlows, saved.TableFlows) {
				t.Fatal("competing request overwrote saved flows", current)
			}
			if first == "switch" {
				if current.State != "DRAFT" || current.ReleaseType != "EMERGENCY" {
					t.Fatal(current)
				}
			} else {
				if current.State != "PENDING_APPROVAL" || current.ReleaseType != "STANDARD" {
					t.Fatal(current)
				}
			}
			for _, flow := range current.TableFlows {
				if flow.ReleaseType != current.ReleaseType {
					t.Fatal("mixed type", current)
				}
			}
		})
	}
}

// New pending publication is an unfinished order in all existing consumers:
// list, cancellation/copy, and reprepare preserve identity and target rules.
func TestReleaseEmergencyPendingConsumersAndDerivedDrafts(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	body := strings.Replace(flowDraftBody, `"title":`, `"release_type":"EMERGENCY","title":`, 1)
	created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "emergency-derived-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	submitted := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1","emergency_reason":"紧急修正"}`, "emergency-derived-submit"), 200)
	list := releaseRequest(t, app, "GET", "/api/v1/release-orders?state=PENDING_PUBLICATION", "", "")
	var catalog struct {
		Orders []struct {
			ID        string   `json:"id"`
			Allowed   []string `json:"allowed_actions"`
			Approvals []any    `json:"approvals"`
		} `json:"orders"`
	}
	if list.Code != 200 || json.Unmarshal(list.Body.Bytes(), &catalog) != nil || len(catalog.Orders) != 1 || !slices.Contains(catalog.Orders[0].Allowed, "execute") || !slices.Contains(catalog.Orders[0].Allowed, "cancel") || !slices.Contains(catalog.Orders[0].Allowed, "reprepare") || len(catalog.Orders[0].Approvals) != 0 {
		t.Fatal("pending consumers lost actions/type", list.Body)
	}
	prepared := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/reprepare", derivedDraftBody(t, submitted), "emergency-derived-reprepare"), 201)
	if prepared.State != "DRAFT" || prepared.ReleaseType != "EMERGENCY" || prepared.EmergencyReason != "" || prepared.TableFlows[0].InstanceID == submitted.TableFlows[0].InstanceID || len(prepared.Approvals) != 0 {
		t.Fatal("reprepare inherited reason or instance", prepared)
	}
	original := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if original.State != "CANCELLED" {
		t.Fatal(original)
	}
	path = "/api/v1/release-orders/" + prepared.ID
	flowResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1","emergency_reason":"新的应急原因"}`, "emergency-derived-resubmit"), 200)
	cancelled := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"2","reason":"重新考虑"}`, "emergency-derived-cancel"), 200)
	copied := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/copy", derivedDraftBody(t, cancelled), "emergency-derived-copy"), 201)
	if copied.State != "DRAFT" || copied.ReleaseType != "EMERGENCY" || copied.EmergencyReason != "" || copied.TableFlows[0].InstanceID == cancelled.TableFlows[0].InstanceID {
		t.Fatal("copy inherited committed workflow", copied)
	}
}

// The emergency reason belongs only to submission. Other public operations
// retain their version-only or explicitly declared request contract.
func TestReleaseEmergencyReasonOnlyAcceptedBySubmit(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	created := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"动作字段边界","items":[]}`, "emergency-contract-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	for _, action := range []string{"execute", "complete", "quick-rollback/preview"} {
		response := releaseRequest(t, app, "POST", path+"/"+action, `{"expected_version":"1","emergency_reason":"只能在提交时提供"}`, "emergency-contract-"+strings.ReplaceAll(action, "/", "-"))
		assertIntegrationErrorCode(t, response, 400, "invalid_request")
	}
}
