//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTableApprovalFallbackMatrixAndEmptySnapshot(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	editor := registerAccount(t, app, "fallback.editor", "fallback.editor@example.com", "correct horse battery staple")
	member := registerAccount(t, app, "fallback.member", "fallback.member@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "fallback-editor-grant")
	role := tableApprovalRole(t, app, "暂缺成员")
	for index, scenario := range []string{"empty", "no-members", "disabled-role", "only-applicant"} {
		t.Run(scenario, func(t *testing.T) {
			switch scenario {
			case "empty":
				assignTableApproval(t, app, "mutation_add_items")
			case "no-members":
				assignTableApproval(t, app, "mutation_add_items", role)
			case "disabled-role":
				role = updateTableRole(t, app, role, "停用角色", false, member)
			case "only-applicant":
				role = updateTableRole(t, app, role, "仅申请人", true, editor)
			}
			code := fmt.Sprintf("fallback-matrix-%d", index)
			path := createTableApprovalDraft(t, app, editor, code)
			rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "submit-"+code), 200)
			self := confirmedApprovalBody(t, app, editor, path, "不能自审")
			assertIntegrationErrorCode(t, releaseActorRequest(t, app, editor, "POST", path+"/approve", self, "self-"+code), 403, "permission_denied")
			if scenario == "empty" {
				assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "后来负责人", member))
			}
			order := readTableApprovalOrder(t, app, admin, path)
			if order.ApprovalContext.Tables[0].Mode != "ADMIN" || len(order.ApprovalContext.ApprovableTables) != 1 {
				t.Fatalf("default eligibility: %+v", order)
			}
			if scenario == "empty" && len(order.Approvals[0].Roles) != 0 {
				t.Fatal("empty snapshot was overwritten")
			}
			approved := rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/approve", confirmedApprovalBody(t, app, admin, path, "默认独立核实"), "approve-"+code), 200)
			if approved.Approvals[0].Decision.Source != "ADMIN" || len(approved.Approvals[0].Decision.Roles) != 0 {
				t.Fatalf("default source missing: %+v", approved)
			}
		})
	}
	// A viewer without table membership cannot act even though it can read.
	legacy := registerAccount(t, app, "fallback.legacy", "fallback.legacy@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, legacy, `["VIEWER"]`, "1", "fallback-viewer-value")
	assignTableApproval(t, app, "mutation_add_items")
	path := createTableApprovalDraft(t, app, editor, "legacy-no-authority")
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "legacy-submit"), 200)
	body := confirmedApprovalBody(t, app, legacy, path, "旧全局审批值不授权")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, legacy, "POST", path+"/approve", body, "legacy-approve-denied"), 403, "permission_denied")
	list := releaseActorRequest(t, app, legacy, "GET", "/api/v1/release-orders?state=PENDING_APPROVAL", "", "")
	if list.Code != 200 || strings.Contains(list.Body.String(), `"approve"`) {
		t.Fatalf("list gave legacy permission: %s", list.Body)
	}
}

func TestTableApprovalScopeConcurrencyAndRejection(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	goods := registerAccount(t, app, "scope.goods", "scope.goods@example.com", "correct horse battery staple")
	prices := registerAccount(t, app, "scope.prices", "scope.prices@example.com", "correct horse battery staple")
	goodsRole := tableApprovalRole(t, app, "商品", goods)
	priceRole := tableApprovalRole(t, app, "价格", prices)
	assignTableApproval(t, app, "mutation_add_items", goodsRole)
	assignTableApproval(t, app, "mutation_supplied_id_items", priceRole)
	create := func(code string) string {
		body := fmt.Sprintf(`{"title":"范围竞争","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"intent"}},{"table_name":"mutation_supplied_id_items","operation":"ADD","content":{"id":%q,"label":"intent"}}]}`, code, code)
		created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "scope-create-"+code), 201)
		path := "/api/v1/release-orders/" + created.ID
		rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "scope-submit-"+code), 200)
		return path
	}
	path := create("range-expanded")
	original := confirmedApprovalBody(t, app, goods, path, "仅确认商品表")
	priceRole = updateTableRole(t, app, priceRole, "价格", true, prices, goods)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", original, "scope-expanded-original"), 409, "release_approval_conflict")
	updated := confirmedApprovalBody(t, app, goods, path, "明确确认两张表")
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", updated, "scope-expanded-reviewed"), 200)
	if approved.State != "APPROVED" || len(approved.History[len(approved.History)-1].TableNames) != 2 {
		t.Fatal("one action did not cover all eligible tables")
	}
	priceRole = updateTableRole(t, app, priceRole, "价格", true, prices)
	for index, actions := range [][2]string{{"approve", "approve"}, {"approve", "reject"}, {"approve", "cancel"}} {
		code := fmt.Sprintf("race-%d", index)
		path := create(code)
		actors := []*httptest.ResponseRecorder{goods, prices}
		bodies := []string{confirmedApprovalBody(t, app, goods, path, "商品意见"), confirmedApprovalBody(t, app, prices, path, "价格意见")}
		if actions[1] == "cancel" {
			actors[1] = admin
			bodies[1] = `{"expected_version":"2","reason":"取消竞争"}`
		}
		start := make(chan struct{})
		responses := make(chan *httptest.ResponseRecorder, 2)
		for i := range 2 {
			actor := actors[i]
			cookies, csrf := actor.Result().Cookies(), sessionCSRF(t, actor)
			go func(i int) {
				<-start
				responses <- accountRequestFrom(app, "POST", path+"/"+actions[i], bodies[i], cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("%s-decision-%d", code, i)})
			}(i)
		}
		close(start)
		statuses := map[int]int{}
		for range 2 {
			response := <-responses
			statuses[response.Code]++
			if response.Code != 200 && response.Code != 409 {
				t.Fatalf("race response: %d %s", response.Code, response.Body)
			}
		}
		if statuses[200] != 1 || statuses[409] != 1 {
			t.Fatalf("decision order not serialized: %v", statuses)
		}
		current := readTableApprovalOrder(t, app, admin, path)
		if current.Version != "3" || len(current.History) != 3 {
			t.Fatalf("race duplicated transition: %+v", current)
		}
	}
	path = create("valid-rejection")
	rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", confirmedApprovalBody(t, app, goods, path, "先通过商品"), "rejection-first-approval"), 200)
	rejected := rollbackOrderResponse(t, releaseActorRequest(t, app, prices, "POST", path+"/reject", confirmedApprovalBody(t, app, prices, path, "价格不可接受"), "rejection-valid"), 200)
	if rejected.State != "REJECTED" || rejected.Approvals[0].State != "APPROVED" || rejected.Approvals[1].State != "REJECTED" || rejected.Approvals[1].Decision.Reason != "价格不可接受" {
		t.Fatalf("rejection erased history: %+v", rejected)
	}
	// The stopped order releases its known target so another draft can reuse it.
	raw, _ := json.Marshal(map[string]any{"title": "拒绝后重新申请", "items": []any{map[string]any{"table_name": "mutation_supplied_id_items", "operation": "ADD", "content": map[string]string{"id": "valid-rejection", "label": "new"}}}})
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", string(raw), "rejection-target-reused"), 201)
}

func TestTableApprovalAccountAvailabilityAndDurableHistory(t *testing.T) {
	f := newMaintenanceFixture(t)
	fixture, err := os.ReadFile("testdata/006-mutation-fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, f.databaseOwner, statement)
		}
	}
	app := f.app
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "account.member", "account.member@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "account.fallback", "account.fallback@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "1", "account-fallback-grant")
	role := tableApprovalRole(t, app, "实时启停成员", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "account-availability")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "account-submit"), 200)
	f.run(t, "", "disable", "--id", accountID(t, member))
	if order := readTableApprovalOrder(t, app, fallback, path); order.ApprovalContext.Tables[0].Mode != "ADMIN" {
		t.Fatal("disabled members did not activate fallback")
	}
	oldDefault := confirmedApprovalBody(t, app, fallback, path, "默认资格确认")
	grantReleaseRole(t, app, fallback, `["VIEWER"]`, "2", "account-admin-revoke")
	if order := readTableApprovalOrder(t, app, admin, path); order.State != "PENDING_APPROVAL" || order.Version != "2" || order.ApprovalContext.Tables[0].Mode != "UNAVAILABLE" {
		t.Fatalf("in-flight lost reviewers changed progress: %+v", order)
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, fallback, "POST", path+"/approve", oldDefault, "account-revoked-default"), 409, "release_approval_conflict")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "3", "account-admin-restore")
	// No member is eligible: a successful default decision survives subsequent role/ADMIN removal.
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, fallback, "POST", path+"/approve", confirmedApprovalBody(t, app, fallback, path, "默认审批完成"), "account-default-approve"), 200)
	if approved.Approvals[0].Decision.Source != "ADMIN" {
		t.Fatal("lost default source")
	}
	grantReleaseRole(t, app, fallback, `["VIEWER"]`, "4", "account-default-demote")
	f.run(t, "", "enable", "--id", accountID(t, member))
	member = loginAccount(t, app, "account.member", "correct horse battery staple")
	historical := readTableApprovalOrder(t, app, member, path)
	if historical.Approvals[0].Decision.ActorID != accountID(t, fallback) || historical.Approvals[0].State != "APPROVED" || len(historical.ApprovalContext.ApprovableTables) != 0 {
		t.Fatal("role recovery reopened completed fallback")
	}
	// Another pending application switches back to members when the account is enabled.
	path = createTableApprovalDraft(t, app, admin, "account-restored")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "account-restored-submit"), 200)
	if order := readTableApprovalOrder(t, app, member, path); order.ApprovalContext.Tables[0].Mode != "ROLE" || len(order.ApprovalContext.ApprovableTables) != 1 {
		t.Fatal("enabled account could not take over")
	}
}

func TestTableApprovalReadAndResultFailuresDoNotGrantOrCommit(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ownerConfig := *driver
	ownerConfig.User = "root"
	owner := deliveryDB(t, &ownerConfig)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	editor := registerAccount(t, app, "atomic.editor", "atomic.editor@example.com", "correct horse battery staple")
	member := registerAccount(t, app, "atomic.member", "atomic.member@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, editor, `["EDITOR"]`, "1", "atomic-editor-grant")
	role := tableApprovalRole(t, app, "真实读取", member)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, editor, "failure-no-grant")
	rollbackOrderResponse(t, releaseActorRequest(t, app, editor, "POST", path+"/submit", `{"expected_version":"1"}`, "failure-submit"), 200)
	body := confirmedApprovalBody(t, app, member, path, "结果须原子保存")
	adminBody := confirmedApprovalBody(t, app, admin, path, "读取故障不能授权")
	deliveryExec(t, owner, `RENAME TABLE rcc_approval_role_members TO unavailable_approval_members`)
	denied := releaseActorRequest(t, app, admin, "POST", path+"/approve", adminBody, "failure-no-fallback")
	assertIntegrationErrorCode(t, denied, 503, "release_unavailable")
	deliveryExec(t, owner, `RENAME TABLE unavailable_approval_members TO rcc_approval_role_members`)
	deliveryExec(t, owner, `CREATE TRIGGER reject_table_approval_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected durable result failure'`)
	failed := releaseActorRequest(t, app, member, "POST", path+"/approve", body, "failure-atomic-decision")
	assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
	read := readTableApprovalOrder(t, app, member, path)
	if read.State != "PENDING_APPROVAL" || read.Version != "2" || read.Approvals[0].Decision != nil || len(read.History) != 2 {
		t.Fatalf("result failure left partial decision: %+v", read)
	}
	deliveryExec(t, owner, `DROP TRIGGER reject_table_approval_result`)
	saved := rollbackOrderResponse(t, releaseActorRequest(t, app, member, "POST", path+"/approve", body, "failure-atomic-decision"), 200)
	if saved.Version != "3" || len(saved.History) != 3 {
		t.Fatal("retry after rollback failed")
	}
	// Same request identity with different intent is rejected without changing history.
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, member, "POST", path+"/approve", strings.Replace(body, "结果须原子保存", "不同意见", 1), "failure-atomic-decision"), 409, "idempotency_conflict")
}

func TestTableApprovalConcurrentFirstReferenceAndDelete(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	for i := range 3 {
		role := tableApprovalRole(t, app, fmt.Sprintf("首次引用竞争%d", i))
		path := "/api/v1/table-policies/mutation_add_items/approval-roles"
		current := releaseRequest(t, app, "GET", path, "", "")
		var assignment struct{ Version string }
		_ = json.Unmarshal(current.Body.Bytes(), &assignment)
		bodies := []string{fmt.Sprintf(`{"expected_version":%q,"role_ids":[%q]}`, assignment.Version, role.ID), `{"expected_version":"1"}`}
		methods := []string{"PUT", "DELETE"}
		paths := []string{path, "/api/v1/approval-roles/" + role.ID}
		start := make(chan struct{})
		responses := make(chan struct {
			index    int
			response *httptest.ResponseRecorder
		}, 2)
		cookies, csrf := admin.Result().Cookies(), sessionCSRF(t, admin)
		for index := range 2 {
			go func(index int) {
				<-start
				responses <- struct {
					index    int
					response *httptest.ResponseRecorder
				}{index, accountRequestFrom(app, methods[index], paths[index], bodies[index], cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("reference-race-%d-%d", i, index)})}
			}(index)
		}
		close(start)
		statuses := [2]int{}
		for range 2 {
			result := <-responses
			statuses[result.index] = result.response.Code
		}
		if statuses != [2]int{200, 409} && statuses != [2]int{404, 200} {
			t.Fatalf("invalid reference/delete order: %v", statuses)
		}
		read := releaseRequest(t, app, "GET", path, "", "")
		if read.Code != 200 {
			t.Fatalf("dangling assignment after race: %s", read.Body)
		}
		assignTableApproval(t, app, "mutation_add_items")
		if statuses[0] == 200 {
			assertIntegrationErrorCode(t, releaseRequest(t, app, "DELETE", "/api/v1/approval-roles/"+role.ID, `{"expected_version":"1"}`, fmt.Sprintf("permanent-reference-%d", i)), 409, "approval_role_referenced")
		}
	}
}

// #92 AC-010: database-boundary audit timestamps prove that a successful
// decision commits its qualification before a competing revocation can win.
func TestTableApprovalQualificationChangesSerializeWithDecisions(t *testing.T) {
	f := newMaintenanceFixture(t)
	fixture, err := os.ReadFile("testdata/006-mutation-fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, f.databaseOwner, statement)
		}
	}
	app := f.app
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "racequal.member", "racequal.member@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "racequal.fallback", "racequal.fallback@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "1", "racequal-admin-grant")
	role := tableApprovalRole(t, app, "竞争资格", member)
	deliveryExec(t, f.databaseOwner, `CREATE TABLE qualification_audit(kind varchar(20),stamp datetime(6))`)
	deliveryExec(t, f.databaseOwner, `CREATE TRIGGER audit_role_qualification AFTER UPDATE ON rcc_approval_roles FOR EACH ROW INSERT INTO qualification_audit VALUES('qualification',UTC_TIMESTAMP(6))`)
	deliveryExec(t, f.databaseOwner, `CREATE TRIGGER audit_account_qualification AFTER UPDATE ON rcc_accounts FOR EACH ROW BEGIN IF OLD.enabled<>NEW.enabled OR OLD.roles<>NEW.roles THEN INSERT INTO qualification_audit VALUES('qualification',UTC_TIMESTAMP(6)); END IF; END`)
	deliveryExec(t, f.databaseOwner, `CREATE TRIGGER audit_approval_commit AFTER UPDATE ON rcc_release_orders FOR EACH ROW BEGIN IF NEW.state='APPROVED' AND OLD.state<>NEW.state THEN INSERT INTO qualification_audit VALUES('decision',UTC_TIMESTAMP(6)); END IF; END`)
	for _, mode := range []string{"membership", "role-disabled", "account-disabled", "admin-revoked"} {
		t.Run(mode, func(t *testing.T) {
			role = updateTableRole(t, app, role, "竞争资格", true, member)
			assignTableApproval(t, app, "mutation_add_items", role)
			actor := member
			if mode == "admin-revoked" {
				assignTableApproval(t, app, "mutation_add_items")
				actor = fallback
			}
			path := createTableApprovalDraft(t, app, admin, "racequal-"+mode)
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "racequal-submit-"+mode), 200)
			body := confirmedApprovalBody(t, app, actor, path, "变更竞争前确认")
			mutationPath := "/api/v1/approval-roles/" + role.ID
			mutationBody := fmt.Sprintf(`{"name":"竞争资格","description":"","enabled":true,"member_ids":[],"expected_version":%q}`, role.Version)
			if mode == "role-disabled" {
				mutationBody = fmt.Sprintf(`{"name":"竞争资格","description":"","enabled":false,"member_ids":[%q],"expected_version":%q}`, accountID(t, member), role.Version)
			}
			if mode == "admin-revoked" {
				mutationPath = "/api/v1/account-roles/" + accountID(t, fallback)
				mutationBody = `{"roles":["VIEWER"],"expected_version":"2"}`
			}
			deliveryExec(t, f.databaseOwner, `DELETE FROM qualification_audit`)
			start := make(chan struct{})
			decided := make(chan *httptest.ResponseRecorder, 1)
			changed := make(chan error, 1)
			actorCookies, actorCSRF := actor.Result().Cookies(), sessionCSRF(t, actor)
			adminCookies, adminCSRF := admin.Result().Cookies(), sessionCSRF(t, admin)
			go func() {
				<-start
				decided <- accountRequestFrom(app, "POST", path+"/approve", body, actorCookies, actorCSRF, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "racequal-decide-" + mode})
			}()
			command := f.command("", "disable", "--id", accountID(t, member))
			go func() {
				<-start
				if mode == "account-disabled" {
					out, err := command.CombinedOutput()
					if err != nil {
						changed <- fmt.Errorf("maintenance: %v %s", err, out)
					} else {
						changed <- nil
					}
					return
				}
				response := accountRequestFrom(app, "PUT", mutationPath, mutationBody, adminCookies, adminCSRF, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "racequal-change-" + mode})
				if response.Code != 200 {
					changed <- fmt.Errorf("qualification update: %d %s", response.Code, response.Body)
				} else {
					changed <- nil
				}
			}()
			close(start)
			response := <-decided
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 && response.Code != 409 && !(mode == "account-disabled" && (response.Code == 401 || response.Code == 403)) {
				t.Fatalf("unexpected competition: %d %s", response.Code, response.Body)
			}
			var changes, decisions, inversions int
			if err := f.databaseOwner.QueryRow(`SELECT SUM(kind='qualification'),SUM(kind='decision'),COALESCE((SELECT COUNT(*) FROM qualification_audit q JOIN qualification_audit d ON q.kind='qualification' AND d.kind='decision' AND q.stamp<=d.stamp),0) FROM qualification_audit`).Scan(&changes, &decisions, &inversions); err != nil {
				t.Fatal(err)
			}
			if changes != 1 || inversions != 0 || (response.Code == 200) != (decisions == 1) {
				t.Fatalf("qualification bypass: status=%d changes=%d decisions=%d inversions=%d", response.Code, changes, decisions, inversions)
			}
			read := readTableApprovalOrder(t, app, admin, path)
			if response.Code == 200 && read.State != "APPROVED" || response.Code != 200 && (read.State != "PENDING_APPROVAL" || read.Version != "2") {
				t.Fatalf("unexpected durable result: %+v", read)
			}
			if mode != "admin-revoked" {
				role = decodeApprovalRole(t, releaseRequest(t, app, "GET", "/api/v1/approval-roles/"+role.ID, "", ""))
			}
		})
	}
}
