//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"net/http/httptest"
	"strings"
	"testing"
)

// #92 AC-003/004: assignments have an independent version and a permanent reference.
func TestTableApprovalAssignmentRetainsReferenceAndOtherPolicy(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	roleResponse := releaseRequest(t, app, "POST", "/api/v1/approval-roles", `{"name":"运营审批","description":"","enabled":true,"member_ids":[]}`, "assignment-create-role")
	if roleResponse.Code != 201 {
		t.Fatal(roleResponse.Body)
	}
	role := decodeApprovalRole(t, roleResponse)
	path := "/api/v1/table-policies/mutation_add_items/approval-roles"
	before := releaseRequest(t, app, "GET", "/api/v1/table-policies/mutation_add_items", "", "")
	original := releaseRequest(t, app, "GET", path, "", "")
	if original.Code != 200 {
		t.Fatalf("assignment read: %d %s", original.Code, original.Body)
	}
	var initial struct {
		Version string
		RoleIDs []string `json:"role_ids"`
	}
	if json.Unmarshal(original.Body.Bytes(), &initial) != nil || initial.Version != "0" || len(initial.RoleIDs) != 0 {
		t.Fatal(original.Body)
	}
	saved := releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"expected_version":"0","role_ids":[%q]}`, role.ID), "assignment-save-role")
	if saved.Code != 200 {
		t.Fatalf("assignment save: %d %s", saved.Code, saved.Body)
	}
	replay := releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"expected_version":"0","role_ids":[%q]}`, role.ID), "assignment-save-role")
	if replay.Code != 200 || replay.Body.String() != saved.Body.String() {
		t.Fatalf("assignment replay changed result: %s", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"0","role_ids":[]}`, "assignment-save-role"), 409, "idempotency_conflict")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"expected_version":"1","role_ids":[%q,%q]}`, role.ID, role.ID), "assignment-duplicate-roles"), 422, "invalid_approval_role")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"1","role_ids":["00000000-0000-4000-8000-000000000000"]}`, "assignment-unknown-role"), 404, "approval_role_not_found")
	viewer := registerAccount(t, app, "assignment.viewer", "assignment.viewer@example.com", "correct horse battery staple")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, viewer, "GET", path, "", ""), 403, "permission_denied")

	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, `{"expected_version":"0","role_ids":[]}`, "assignment-stale-save"), 409, "table_approval_conflict")
	clear := releaseRequest(t, app, "PUT", path, `{"expected_version":"1","role_ids":[]}`, "assignment-clear-role")
	if clear.Code != 200 {
		t.Fatal(clear.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "DELETE", "/api/v1/approval-roles/"+role.ID, `{"expected_version":"1"}`, "assignment-delete-role"), 409, "approval_role_referenced")
	after := releaseRequest(t, app, "GET", "/api/v1/table-policies/mutation_add_items", "", "")
	if before.Body.String() != after.Body.String() {
		t.Fatalf("approval assignment changed business policy: %s / %s", before.Body, after.Body)
	}
}

// #92 AC-009: the sole administrator cannot submit work only they could review.
func TestReleaseSubmitRequiresIndependentApprover(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"无人可审批","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"no-reviewer","label":"intent"}}]}`, "no-approver-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	response := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "no-approver-submit")
	assertIntegrationErrorCode(t, response, 422, "release_approver_unavailable")
	if !strings.Contains(response.Body.String(), "mutation_add_items") {
		t.Fatal("missing problem table", response.Body)
	}
	read := rollbackOrderResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if read.State != "DRAFT" || read.Version != "1" {
		t.Fatalf("failed submit changed draft: %+v", read)
	}
	cancelled := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"结束未提交草稿"}`, "no-approver-cancel"), 200)
	if cancelled.State != "CANCELLED" {
		t.Fatal("unsubmitted draft cancellation lost")
	}
}

func tableApprovalRole(t *testing.T, app *adminApplication, name string, members ...*httptest.ResponseRecorder) approvalRoleResult {
	t.Helper()
	ids := []string{}
	for _, member := range members {
		ids = append(ids, accountID(t, member))
	}
	payload, _ := json.Marshal(map[string]any{"name": name, "description": "", "enabled": true, "member_ids": ids})
	response := releaseRequest(t, app, "POST", "/api/v1/approval-roles", string(payload), fmt.Sprintf("table-role-%d", publicationFixtureSequence.Add(1)))
	if response.Code != 201 {
		t.Fatal(response.Body)
	}
	return decodeApprovalRole(t, response)
}
func assignTableApproval(t *testing.T, app *adminApplication, table string, roles ...approvalRoleResult) {
	t.Helper()
	path := "/api/v1/table-policies/" + table + "/approval-roles"
	response := releaseRequest(t, app, "GET", path, "", "")
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	var assignment struct{ Version string }
	_ = json.Unmarshal(response.Body.Bytes(), &assignment)
	ids := []string{}
	for _, role := range roles {
		ids = append(ids, role.ID)
	}
	payload, _ := json.Marshal(map[string]any{"expected_version": assignment.Version, "role_ids": ids})
	response = releaseRequest(t, app, "PUT", path, string(payload), fmt.Sprintf("table-assignment-%d", publicationFixtureSequence.Add(1)))
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
}
func confirmedApprovalBody(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, path, reason string) string {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "GET", path, "", "")
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	var order domain.ReleaseHeader
	if json.Unmarshal(response.Body.Bytes(), &order) != nil {
		t.Fatal(response.Body)
	}
	payload, _ := json.Marshal(map[string]any{"expected_version": order.Version, "reason": reason, "confirmed_tables": order.ApprovalContext.ApprovableTables, "expected_approval_revision": order.ApprovalContext.Revision})
	return string(payload)
}

// #92 AC-005/006: each table needs one eligible decision, and the whole order needs all tables.
func TestTableApprovalPartialProgressAndFinalApproval(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	goods := registerAccount(t, app, "table.goods", "table.goods@example.com", "correct horse battery staple")
	finance := registerAccount(t, app, "table.finance", "table.finance@example.com", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "商品角色", goods))
	assignTableApproval(t, app, "mutation_delete_parents", tableApprovalRole(t, app, "价格角色", finance))
	created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"分别审批","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"separate","label":"intent"}},{"table_name":"mutation_delete_parents","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"updated"}}]}`, "partial-create-order"), 201)
	path := "/api/v1/release-orders/" + created.ID
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "partial-submit-order"), 200)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", `{"expected_version":"2","reason":"缺少确认范围"}`, "partial-missing-confirmation"), 422, "release_invalid")
	body := confirmedApprovalBody(t, app, goods, path, "商品已核实")
	partial := rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", body, "partial-goods-approve"), 200)
	if partial.State != "PENDING_APPROVAL" || len(partial.Approvals) != 2 || partial.Approvals[0].State != "APPROVED" || partial.Approvals[1].State != "PENDING" {
		t.Fatalf("wrong partial progress: %+v", partial)
	}
	replay := rollbackOrderResponse(t, releaseActorRequest(t, app, goods, "POST", path+"/approve", body, "partial-goods-approve"), 200)
	if replay.Version != partial.Version || len(replay.History) != len(partial.History) {
		t.Fatal("replay duplicated decision")
	}
	denied := releaseActorRequest(t, app, goods, "POST", path+"/reject", confirmedApprovalBody(t, app, goods, path, "不能再处理商品"), "partial-repeat-reject")
	assertIntegrationErrorCode(t, denied, 403, "permission_denied")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "partial-no-execution"), 422, "release_state_invalid")
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, finance, "POST", path+"/approve", confirmedApprovalBody(t, app, finance, path, "价格已核实"), "partial-finance-approve"), 200)
	if approved.State != "APPROVED" || approved.Approvals[1].State != "APPROVED" || approved.History[len(approved.History)-1].Reason != "价格已核实" {
		t.Fatalf("missing final approval/history: %+v", approved)
	}
}

func updateTableRole(t *testing.T, app *adminApplication, role approvalRoleResult, name string, enabled bool, members ...*httptest.ResponseRecorder) approvalRoleResult {
	t.Helper()
	ids := []string{}
	for _, member := range members {
		ids = append(ids, accountID(t, member))
	}
	payload, _ := json.Marshal(map[string]any{"name": name, "description": "", "enabled": enabled, "member_ids": ids, "expected_version": role.Version})
	response := releaseRequest(t, app, "PUT", "/api/v1/approval-roles/"+role.ID, string(payload), fmt.Sprintf("table-role-update-%d", publicationFixtureSequence.Add(1)))
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	return decodeApprovalRole(t, response)
}
func readTableApprovalOrder(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, path string) domain.ReleaseHeader {
	t.Helper()
	response := releaseActorRequest(t, app, actor, "GET", path, "", "")
	if response.Code != 200 {
		t.Fatal(response.Body)
	}
	var order domain.ReleaseHeader
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	return order
}
func createTableApprovalDraft(t *testing.T, app *adminApplication, actor *httptest.ResponseRecorder, code string) string {
	t.Helper()
	body := fmt.Sprintf(`{"title":"表审批验收","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"intent"}}]}`, code)
	created := rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", "/api/v1/release-orders", body, "draft-"+code), 201)
	return "/api/v1/release-orders/" + created.ID
}
func TestTableApprovalSnapshotMembershipFallbackAndHistory(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	original := registerAccount(t, app, "snapshot.original", "snapshot.original@example.com", "correct horse battery staple")
	replacement := registerAccount(t, app, "snapshot.replacement", "snapshot.replacement@example.com", "correct horse battery staple")
	later := registerAccount(t, app, "snapshot.later", "snapshot.later@example.com", "correct horse battery staple")
	fallback := registerAccount(t, app, "snapshot.fallback", "snapshot.fallback@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, fallback, `["ADMIN"]`, "1", "snapshot-fallback-admin")
	role := tableApprovalRole(t, app, "提交时角色", original)
	laterRole := tableApprovalRole(t, app, "后来改绑角色", later)
	assignTableApproval(t, app, "mutation_add_items", role)
	path := createTableApprovalDraft(t, app, admin, "snapshot-fallback")
	rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, "snapshot-submit"), 200)
	oldBody := confirmedApprovalBody(t, app, original, path, "原成员意见保留")
	assignTableApproval(t, app, "mutation_add_items", laterRole)
	if order := readTableApprovalOrder(t, app, later, path); len(order.ApprovalContext.ApprovableTables) != 0 || order.Approvals[0].Roles[0].ID != role.ID {
		t.Fatalf("rebinding rewrote snapshot: %+v", order)
	}
	if order := readTableApprovalOrder(t, app, fallback, path); len(order.ApprovalContext.ApprovableTables) != 0 {
		t.Fatal("admin bypassed eligible role members")
	}
	role = updateTableRole(t, app, role, "当前改名角色", true)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, original, "POST", path+"/approve", oldBody, "snapshot-removed-member"), 409, "release_approval_conflict")
	fallbackBody := confirmedApprovalBody(t, app, fallback, path, "默认接手意见")
	if order := readTableApprovalOrder(t, app, fallback, path); order.ApprovalContext.Tables[0].Mode != "ADMIN" || len(order.ApprovalContext.ApprovableTables) != 1 {
		t.Fatalf("fallback missing: %+v", order)
	}
	role = updateTableRole(t, app, role, "当前改名角色", true, replacement)
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, fallback, "POST", path+"/approve", fallbackBody, "snapshot-fallback-stale"), 409, "release_approval_conflict")
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, replacement, "POST", path+"/approve", confirmedApprovalBody(t, app, replacement, path, "新成员核实"), "snapshot-new-member"), 200)
	decision := approved.Approvals[0].Decision
	if approved.State != "APPROVED" || decision.ActorID != accountID(t, replacement) || decision.Source != "ROLE" || decision.Roles[0].Name != "当前改名角色" || approved.Approvals[0].Roles[0].Name != "提交时角色" {
		t.Fatalf("qualification history not traceable: %+v", approved.Approvals)
	}
	updateTableRole(t, app, role, "停用后的名字", false)
	assignTableApproval(t, app, "mutation_add_items")
	historical := readTableApprovalOrder(t, app, admin, path)
	if historical.Approvals[0].Decision.Reason != "新成员核实" || historical.Approvals[0].State != "APPROVED" {
		t.Fatal("role changes revoked history")
	}
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "snapshot-execute"), 200)
	if published.State != "SUCCEEDED" {
		t.Fatal("approval-only configuration broke frozen business execution")
	}
}
