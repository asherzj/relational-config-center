//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

type approvalRoleResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Version     string `json:"version"`
	Referenced  bool   `json:"referenced"`
	Members     []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"members"`
}

func approvalRoleRequest(t *testing.T, f maintenanceFixture, actor *httptest.ResponseRecorder, method, path, body, key string, status int) *httptest.ResponseRecorder {
	t.Helper()
	response := accountRequestFrom(f.app, method, path, body, actor.Result().Cookies(), sessionCSRF(t, actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	if response.Code != status {
		t.Fatalf("%s %s: got %d %s; want %d", method, path, response.Code, response.Body, status)
	}
	return response
}

func decodeApprovalRole(t *testing.T, response *httptest.ResponseRecorder) approvalRoleResult {
	t.Helper()
	var role approvalRoleResult
	if err := json.Unmarshal(response.Body.Bytes(), &role); err != nil {
		t.Fatal(err)
	}
	return role
}

// #92 AC-001: real members retain their identities and global grants across groups.
func TestApprovalRoleMembershipPersistsWithoutGrantingAccountRoles(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "approval.admin", "approval.admin@example.com", "correct horse battery staple")
	member := registerAccount(t, f.app, "approval.member", "approval.member@example.com", "correct horse battery staple")
	second := registerAccount(t, f.app, "approval.second", "approval.second@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	search := approvalRoleRequest(t, f, admin, "GET", "/api/v1/account-roles?q=approval.member", "", "", 200)
	if !containsJSONID(search.Body.Bytes(), accountID(t, member)) {
		t.Fatal("member search omitted the real identity")
	}
	body := fmt.Sprintf(`{"name":"运营审批","description":"商品负责人","enabled":true,"member_ids":[%q,%q,%q]}`, accountID(t, member), accountID(t, second), accountID(t, member))
	created := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", body, "approval-create-0001", 201))
	if len(created.ID) != 36 || created.Version != "1" || len(created.Members) != 2 {
		t.Fatalf("create: %+v", created)
	}
	approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", fmt.Sprintf(`{"name":"财务审批","description":"价格负责人","enabled":true,"member_ids":[%q]}`, accountID(t, member)), "approval-create-0002", 201)
	updated := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "PUT", "/api/v1/approval-roles/"+created.ID, fmt.Sprintf(`{"name":"商品运营","description":"调整说明","enabled":true,"member_ids":[%q],"expected_version":"1"}`, accountID(t, member)), "approval-update-0001", 200))
	if updated.ID != created.ID || updated.Version != "2" || updated.Name != "商品运营" || len(updated.Members) != 1 {
		t.Fatalf("update: %+v", updated)
	}
	admin = loginAccount(t, f.app, "approval.admin", "correct horse battery staple")
	read := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "GET", "/api/v1/approval-roles/"+created.ID, "", "", 200))
	if read.ID != created.ID || read.Name != "商品运营" || read.Description != "调整说明" || len(read.Members) != 1 || read.Members[0].ID != accountID(t, member) {
		t.Fatalf("relogin: %+v", read)
	}
	list := approvalRoleRequest(t, f, admin, "GET", "/api/v1/approval-roles", "", "", 200)
	var catalog struct {
		Roles []approvalRoleResult `json:"roles"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Roles) != 2 || len(catalog.Roles[0].Members) != 1 || len(catalog.Roles[1].Members) != 1 {
		t.Fatalf("many to many: %s", list.Body)
	}
	current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", member.Result().Cookies(), "")
	var identity struct {
		Account struct {
			Roles []string `json:"roles"`
		} `json:"account"`
	}
	if err := json.Unmarshal(current.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(identity.Account.Roles) != "[VIEWER]" {
		t.Fatalf("membership changed global capabilities: %s", current.Body)
	}
}

func containsJSONID(raw []byte, id string) bool {
	var result struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return false
	}
	for _, account := range result.Accounts {
		if account.ID == id {
			return true
		}
	}
	return false
}

// #92 AC-002: a lost response is recovered with the original request, not another change.
func TestApprovalRoleRetriesKeepOriginalResultAndRejectChangedIntent(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "retry.admin", "retry.admin@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	body := `{"name":"运营审批","description":"初始","enabled":true,"member_ids":[]}`
	first := approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", body, "approval-retry-create", 201)
	again := approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", body, "approval-retry-create", 201)
	if first.Body.String() != again.Body.String() {
		t.Fatalf("duplicate create changed result: %s / %s", first.Body, again.Body)
	}
	role := decodeApprovalRole(t, first)
	path := "/api/v1/approval-roles/" + role.ID
	update := `{"name":"运营审批","description":"新说明","enabled":false,"member_ids":[],"expected_version":"1"}`
	saved := approvalRoleRequest(t, f, admin, "PUT", path, update, "approval-retry-update", 200)
	retry := approvalRoleRequest(t, f, admin, "PUT", path, update, "approval-retry-update", 200)
	if saved.Body.String() != retry.Body.String() {
		t.Fatal("same request did not return its original result")
	}
	conflict := approvalRoleRequest(t, f, admin, "PUT", path, `{"name":"其他意图","description":"新说明","enabled":false,"member_ids":[],"expected_version":"1"}`, "approval-retry-update", 409)
	if !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
		t.Fatalf("wrong conflict: %s", conflict.Body)
	}
	read := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "GET", path, "", "", 200))
	if read.Version != "2" || read.Description != "新说明" {
		t.Fatalf("retries changed persistent result: %+v", read)
	}
}

func TestApprovalRoleUnreferencedLifecycle(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "lifecycle.admin", "lifecycle.admin@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	role := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", `{"name":"误建角色","description":"可停用后恢复","enabled":true,"member_ids":[]}`, "role-lifecycle-create", 201))
	path := "/api/v1/approval-roles/" + role.ID
	for i, enabled := range []bool{false, true} {
		result := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "PUT", path, fmt.Sprintf(`{"name":"误建角色","description":"可停用后恢复","enabled":%t,"member_ids":[],"expected_version":"%d"}`, enabled, i+1), fmt.Sprintf("lifecycle-enabled-%d", i), 200))
		if result.Enabled != enabled {
			t.Fatalf("enabled state: %+v", result)
		}
	}
	deleted := approvalRoleRequest(t, f, admin, "DELETE", path, `{"expected_version":"3"}`, "role-lifecycle-delete", 200)
	again := approvalRoleRequest(t, f, admin, "DELETE", path, `{"expected_version":"3"}`, "role-lifecycle-delete", 200)
	if deleted.Body.String() != again.Body.String() {
		t.Fatal("delete retry lost result")
	}
	approvalRoleRequest(t, f, admin, "GET", path, "", "", 404)
}

// AC-002: authorization is checked on every request, including saved-result retries.
func TestApprovalRoleAuthorizationValidationAndConcurrentEdits(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "secure.admin", "secure.admin@example.com", "correct horse battery staple")
	other := registerAccount(t, f.app, "secure.other", "secure.other@example.com", "correct horse battery staple")
	viewer := registerAccount(t, f.app, "secure.viewer", "secure.viewer@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	f.run(t, "", "grant-admin", "--id", accountID(t, other))
	body := `{"name":"财务审批","description":"原说明","enabled":true,"member_ids":[]}`
	role := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", body, "secure-create-0001", 201))
	path := "/api/v1/approval-roles/" + role.ID
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		target := path
		if method == "POST" {
			target = "/api/v1/approval-roles"
		}
		approvalRoleRequest(t, f, viewer, method, target, body, "viewer-forbidden-key", 403)
	}
	if got := accountRequest(f.app, "GET", path, "", nil, ""); got.Code != 401 {
		t.Fatalf("anonymous: %d", got.Code)
	}
	if got := accountRequest(f.app, "DELETE", path, `{"expected_version":"1"}`, admin.Result().Cookies(), ""); got.Code != 403 {
		t.Fatalf("missing CSRF: %d", got.Code)
	}
	if got := accountRequest(f.app, "POST", "/api/v1/approval-roles", body, other.Result().Cookies(), sessionCSRF(t, admin)); got.Code != 403 {
		t.Fatalf("different account CSRF: %d", got.Code)
	}
	for i, invalid := range []string{
		`{"name":" ","description":"","enabled":true,"member_ids":[]}`,
		`{"name":"名称","description":"","member_ids":[]}`,
		`{"name":"名称","description":"","enabled":false}`,
		`{"name":"名称","description":"","enabled":true,"member_ids":["display-name"]}`,
		`{"name":"名称","description":"","enabled":true,"member_ids":[],"actor_id":"forged"}`,
	} {
		approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", invalid, fmt.Sprintf("invalid-request-%d", i), 422)
	}
	approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", `{"name":"无效人员","description":"","enabled":true,"member_ids":["00000000-0000-4000-8000-000000000000"]}`, "unknown-member-key", 404)
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i, actor := range []*httptest.ResponseRecorder{admin, other} {
		go func(i int, actor *httptest.ResponseRecorder) {
			responses <- accountRequestFrom(f.app, "PUT", path, fmt.Sprintf(`{"name":"财务审批","description":"并发%d","enabled":true,"member_ids":[],"expected_version":"1"}`, i), actor.Result().Cookies(), sessionCSRF(t, actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("concurrent-role-%d", i)})
		}(i, actor)
	}
	statuses := map[int]int{}
	for range 2 {
		response := <-responses
		statuses[response.Code]++
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent edits: %v", statuses)
	}
	current := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "GET", path, "", "", 200))
	if current.Version != "2" {
		t.Fatalf("concurrent version: %s", current.Version)
	}
	// A historical request result cannot grant revoked management capability.
	approvalRoleRequest(t, f, other, "PUT", "/api/v1/account-roles/"+accountID(t, admin), `{"roles":["VIEWER"],"expected_version":"2"}`, "demote-admin-for-retry", 200)
	approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", body, "secure-create-0001", 403)
}

func TestApprovalRoleFailedRequestRecordRollsBackMembersAndVersion(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "atomic.admin", "atomic.admin@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	role := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "POST", "/api/v1/approval-roles", `{"name":"事务角色","description":"原值","enabled":true,"member_ids":[]}`, "atomic-role-create", 201))
	path := "/api/v1/approval-roles/" + role.ID
	deliveryExec(t, f.databaseOwner, `CREATE TRIGGER reject_approval_request BEFORE INSERT ON rcc_approval_role_requests FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected request result failure'`)
	body := fmt.Sprintf(`{"name":"事务角色","description":"新值","enabled":false,"member_ids":[%q],"expected_version":"1"}`, accountID(t, admin))
	failed := approvalRoleRequest(t, f, admin, "PUT", path, body, "atomic-role-update", 503)
	if !strings.Contains(failed.Body.String(), `"code":"approval_role_not_saved"`) {
		t.Fatalf("confirmed rollback must be a definite failure: %s", failed.Body.String())
	}
	if strings.Contains(failed.Body.String(), "injected") {
		t.Fatal("storage failure leaked internals")
	}
	read := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "GET", path, "", "", 200))
	if read.Version != "1" || read.Description != "原值" || !read.Enabled || len(read.Members) != 0 {
		t.Fatalf("partial save: %+v", read)
	}
	deliveryExec(t, f.databaseOwner, `DROP TRIGGER reject_approval_request`)
	saved := decodeApprovalRole(t, approvalRoleRequest(t, f, admin, "PUT", path, body, "atomic-role-update", 200))
	if saved.Version != "2" || len(saved.Members) != 1 {
		t.Fatalf("retry after rollback: %+v", saved)
	}
}
