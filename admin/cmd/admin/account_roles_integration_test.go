//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// AC-001: registration order does not confer permission to change the catalog.
func TestRegisteredAccountIsViewer(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	registered := registerAccount(t, app, "roles.viewer", "roles.viewer@example.com", "correct horse battery staple")
	cookies, csrf := registered.Result().Cookies(), sessionCSRF(t, registered)
	read := accountRequest(app, "GET", "/api/v1/query-policies", "", cookies, "")
	if read.Code != 200 {
		t.Fatalf("viewer read: %d %s", read.Code, read.Body.String())
	}
	write := accountRequest(app, "POST", "/api/v1/query-policies", `{}`, cookies, csrf)
	if write.Code != 403 || !strings.Contains(write.Body.String(), `"permission_denied"`) {
		t.Fatalf("viewer catalog write: got %d %s; want 403 permission_denied", write.Code, write.Body.String())
	}
}

// AC-002: a maintainer explicitly bootstraps one administrator, who assigns a combination.
func TestMaintainerBootstrapsAdministratorAndAssignsRoles(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "roles.admin", "admin@example.com", "correct horse battery staple")
	editor := registerAccount(t, f.app, "roles.editor", "editor@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	list := accountRequest(f.app, "GET", "/api/v1/account-roles?q=roles.editor", "", admin.Result().Cookies(), "")
	if list.Code != 200 || !strings.Contains(list.Body.String(), accountID(t, editor)) {
		t.Fatalf("account search: %d %s", list.Code, list.Body.String())
	}
	path := "/api/v1/account-roles/" + accountID(t, editor)
	result := accountRequestFrom(f.app, "PUT", path, `{"roles":["EDITOR","APPROVER"],"expected_version":"1"}`, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": "roles-assign-0001"})
	if result.Code != 200 {
		t.Fatalf("assign roles: %d %s", result.Code, result.Body.String())
	}
	current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", editor.Result().Cookies(), "")
	if current.Code != 200 || !strings.Contains(current.Body.String(), `"roles":["EDITOR","APPROVER"]`) {
		t.Fatalf("current roles: %d %s", current.Code, current.Body.String())
	}
}

// AC-005: the saved result and immutable history make response-loss retries unambiguous.
func TestRoleChangeIsAuditedAndIdempotent(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "audit.admin", "audit.admin@example.com", "correct horse battery staple")
	target := registerAccount(t, f.app, "audit.target", "audit.target@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	path := "/api/v1/account-roles/" + accountID(t, target)
	body := `{"roles":["EDITOR","PUBLISHER"],"expected_version":"1"}`
	change := func(value string) *httptest.ResponseRecorder {
		return accountRequestFrom(f.app, "PUT", path, value, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": "audited-roles-001"})
	}
	first := change(body)
	second := change(body)
	if first.Code != 200 || second.Code != 200 || first.Body.String() != second.Body.String() {
		t.Fatalf("repeat result: %d %s / %d %s", first.Code, first.Body, second.Code, second.Body)
	}
	conflict := change(`{"roles":["VIEWER"],"expected_version":"1"}`)
	if conflict.Code != 409 || !strings.Contains(conflict.Body.String(), "idempotency_conflict") {
		t.Fatalf("different intent: %d %s", conflict.Code, conflict.Body)
	}
	history := accountRequest(f.app, "GET", path+"/history", "", admin.Result().Cookies(), "")
	if history.Code != 200 || !strings.Contains(history.Body.String(), `"before_roles":["VIEWER"]`) || !strings.Contains(history.Body.String(), `"after_roles":["EDITOR","PUBLISHER"]`) || !strings.Contains(history.Body.String(), accountID(t, admin)) {
		t.Fatalf("history: %d %s", history.Code, history.Body)
	}
	var records struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.Unmarshal(history.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records.Events) != 1 {
		t.Fatalf("duplicate audit entries: %d", len(records.Events))
	}
}

// AC-006: both role administration and offline account maintenance retain a usable administrator.
func TestLastEnabledAdministratorCannotBeRemovedOrDisabled(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "last.admin", "last.admin@example.com", "correct horse battery staple")
	id := accountID(t, admin)
	f.run(t, "", "grant-admin", "--id", id)
	change := accountRequestFrom(f.app, "PUT", "/api/v1/account-roles/"+id, `{"roles":["VIEWER"],"expected_version":"2"}`, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": "last-admin-remove"})
	if change.Code != 409 || !strings.Contains(change.Body.String(), "last_administrator") {
		t.Fatalf("last administrator demotion: %d %s", change.Code, change.Body)
	}
	output, err := f.command("", "disable", "--id", id).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "last enabled administrator") {
		t.Fatalf("last administrator disable: %v %s", err, output)
	}
	if current := accountRequest(f.app, "GET", "/api/v1/account-roles", "", admin.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("administrator lost: %d %s", current.Code, current.Body)
	}
}

// AC-001/002: existing accounts keep their identity and default to read-only;
// an interrupted or repeated migration must never reset an explicit grant.
func TestAccountRoleUpgradePreservesAccountsAndGrants(t *testing.T) {
	f := newMaintenanceFixture(t)
	account := registerAccount(t, f.app, "upgrade.roles", "upgrade.roles@example.com", "correct horse battery staple")
	deliveryExec(t, f.databaseOwner, "DROP TABLE rcc_account_role_history")
	deliveryExec(t, f.databaseOwner, "ALTER TABLE rcc_accounts DROP COLUMN roles, DROP COLUMN role_version")
	if err := f.app.mysql.Ready(t.Context()); err == nil {
		t.Fatal("Admin is ready with missing role control schema")
	}
	applyRoleMigration(t, f.databaseOwner)
	if err := f.app.mysql.Ready(t.Context()); err != nil {
		t.Fatal(err)
	}
	current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", account.Result().Cookies(), "")
	if current.Code != 200 || !strings.Contains(current.Body.String(), `"roles":["VIEWER"]`) {
		t.Fatalf("old account default: %d %s", current.Code, current.Body)
	}
	f.run(t, "", "grant-admin", "--id", accountID(t, account))
	applyRoleMigration(t, f.databaseOwner)
	current = accountRequest(f.app, "GET", "/api/v1/account-roles", "", account.Result().Cookies(), "")
	if current.Code != 200 {
		t.Fatalf("migration reset granted role: %d %s", current.Code, current.Body)
	}
}

func applyRoleMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	data, err := os.ReadFile("../../../deploy/mysql/migrations/008-account-roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, statement := range strings.Split(string(data), ";") {
		if strings.TrimSpace(statement) != "" {
			if _, err := conn.ExecContext(t.Context(), statement); err != nil {
				t.Fatalf("role migration: %v", err)
			}
		}
	}
}

// AC-003/004: the same authenticated session follows each changed global grant.
func TestCurrentSessionUsesRolePermissionMatrix(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "matrix.admin", "matrix.admin@example.com", "correct horse battery staple")
	target := registerAccount(t, f.app, "matrix.target", "matrix.target@example.com", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	version := 1
	for _, role := range []string{"VIEWER", "EDITOR", "APPROVER", "PUBLISHER", "ADMIN", "VIEWER"} {
		body := fmt.Sprintf(`{"roles":[%q],"expected_version":"%d"}`, role, version)
		changed := accountRequestFrom(f.app, "PUT", "/api/v1/account-roles/"+accountID(t, target), body, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("matrix-change-%02d", version)})
		if changed.Code != 200 {
			t.Fatalf("grant %s: %d %s", role, changed.Code, changed.Body)
		}
		version++
		tests := []struct {
			method, path string
			allowed      bool
		}{
			{"GET", "/api/v1/query-policies", true},
			{"GET", "/api/v1/account-roles", role == "ADMIN"},
			{"POST", "/api/v1/tables/not_managed/query", true},
			{"POST", "/api/v1/tables/not_managed/rows", role == "EDITOR" || role == "ADMIN"},
			{"PATCH", "/api/v1/tables/not_managed/rows/1", role == "EDITOR" || role == "ADMIN"},
			{"DELETE", "/api/v1/tables/not_managed/rows/1", role == "EDITOR" || role == "ADMIN"},
		}
		for _, resource := range []string{"query-policies", "mutation-policies", "table-policies"} {
			for _, action := range []struct{ method, suffix string }{{"POST", ""}, {"PUT", "/missing_v1"}} {
				tests = append(tests, struct {
					method, path string
					allowed      bool
				}{action.method, "/api/v1/" + resource + action.suffix, role == "ADMIN"})
			}
		}
		for _, resource := range []string{"query-policies", "mutation-policies"} {
			for _, suffix := range []string{"activate", "deprecate", "metadata"} {
				method := "POST"
				if suffix == "metadata" {
					method = "PATCH"
				}
				tests = append(tests, struct {
					method, path string
					allowed      bool
				}{method, "/api/v1/" + resource + "/missing_v1/" + suffix, role == "ADMIN"})
			}
			tests = append(tests, struct {
				method, path string
				allowed      bool
			}{"DELETE", "/api/v1/" + resource + "/missing_v1", role == "ADMIN"})
		}
		for _, suffix := range []string{"enable", "disable"} {
			tests = append(tests, struct {
				method, path string
				allowed      bool
			}{"POST", "/api/v1/table-policies/missing/" + suffix, role == "ADMIN"})
		}
		for _, test := range tests {
			response := accountRequestFrom(f.app, test.method, test.path, `{}`, target.Result().Cookies(), sessionCSRF(t, target), "192.0.2.1:1234", map[string]string{"X-RCC-Roles": "ADMIN", "X-RCC-Account-ID": accountID(t, admin)})
			denied := response.Code == 403 && strings.Contains(response.Body.String(), "permission_denied")
			if denied == test.allowed || response.Code >= 500 {
				t.Fatalf("%s %s %s: %d %s", role, test.method, test.path, response.Code, response.Body)
			}
		}
	}
	discovered := accountRequest(f.app, "GET", "/api/v1/database-tables", "", admin.Result().Cookies(), "")
	if discovered.Code != 200 || strings.Contains(discovered.Body.String(), "rcc_account_role_history") {
		t.Fatalf("control discovery: %d %s", discovered.Code, discovered.Body)
	}
	for _, path := range []string{"/api/v1/tables/rcc_account_role_history/query", "/api/v1/tables/rcc_account_role_history/rows"} {
		response := accountRequest(f.app, "POST", path, `{}`, admin.Result().Cookies(), sessionCSRF(t, admin))
		if response.Code < 400 || response.Code >= 500 {
			t.Fatalf("control access: %d %s", response.Code, response.Body)
		}
	}
}

func TestRoleChangesAndAccountMaintenanceAreConcurrentSafe(t *testing.T) {
	f := newMaintenanceFixture(t)
	first := registerAccount(t, f.app, "race.first", "race.first@example.com", "correct horse battery staple")
	second := registerAccount(t, f.app, "race.second", "race.second@example.com", "correct horse battery staple")
	firstID, secondID := accountID(t, first), accountID(t, second)
	f.run(t, "", "grant-admin", "--id", firstID)
	f.run(t, "", "grant-admin", "--id", secondID)
	start := make(chan struct{})
	results := make(chan bool, 2)
	secondCSRF := sessionCSRF(t, second)
	go func() { <-start; err := f.command("", "disable", "--id", firstID).Run(); results <- err == nil }()
	go func() {
		<-start
		r := accountRequestFrom(f.app, "PUT", "/api/v1/account-roles/"+secondID, `{"roles":["VIEWER"],"expected_version":"2"}`, second.Result().Cookies(), secondCSRF, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "race-demote-admin"})
		results <- r.Code == 200
	}()
	close(start)
	successes := 0
	for range 2 {
		if <-results {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent administrator removals: %d succeeded", successes)
	}
	var count int
	if err := f.db.QueryRow("SELECT COUNT(*) FROM rcc_accounts WHERE enabled=TRUE AND roles & 16<>0").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("enabled administrators after race: %d", count)
	}
	// A database-authorized operator can restore availability without resetting credentials.
	f.run(t, "", "enable", "--id", firstID)
	f.run(t, "", "grant-admin", "--id", firstID)
	restored := loginAccount(t, f.app, "race.first", "correct horse battery staple")
	if list := accountRequest(f.app, "GET", "/api/v1/account-roles", "", restored.Result().Cookies(), ""); list.Code != 200 {
		t.Fatalf("maintenance recovery: %d %s", list.Code, list.Body)
	}
}

func TestRoleChangeRaceAndAuditFailureDoNotLeavePartialGrants(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "atomic.admin", "atomic.admin@example.com", "correct horse battery staple")
	target := registerAccount(t, f.app, "atomic.target", "atomic.target@example.com", "correct horse battery staple")
	adminID, targetID := accountID(t, admin), accountID(t, target)
	f.run(t, "", "grant-admin", "--id", adminID)
	cookies, csrf := admin.Result().Cookies(), sessionCSRF(t, admin)
	change := func(role, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(f.app, "PUT", "/api/v1/account-roles/"+targetID, fmt.Sprintf(`{"roles":[%q],"expected_version":"1"}`, role), cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	}
	deliveryExec(t, f.databaseOwner, `CREATE TRIGGER reject_role_audit BEFORE INSERT ON rcc_account_role_history FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected audit failure'`)
	failed := change("EDITOR", "atomic-audit-failure")
	if failed.Code != 503 || strings.Contains(failed.Body.String(), "injected") {
		t.Fatalf("audit failure response: %d %s", failed.Code, failed.Body)
	}
	current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", target.Result().Cookies(), "")
	if current.Code != 200 || !strings.Contains(current.Body.String(), `"roles":["VIEWER"]`) {
		t.Fatalf("partial role grant: %d %s", current.Code, current.Body)
	}
	deliveryExec(t, f.databaseOwner, "DROP TRIGGER reject_role_audit")
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	go func() { <-start; results <- change("EDITOR", "atomic-audit-failure") }()
	go func() { <-start; results <- change("PUBLISHER", "atomic-race-other") }()
	close(start)
	statuses := map[int]int{}
	for range 2 {
		response := <-results
		statuses[response.Code]++
	}
	if statuses[200] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent version results: %v", statuses)
	}
	history := accountRequest(f.app, "GET", "/api/v1/account-roles/"+targetID+"/history", "", cookies, "")
	var payload struct {
		Events []struct {
			Version string `json:"version"`
		} `json:"events"`
	}
	if history.Code != 200 {
		t.Fatalf("history after failure: %d %s", history.Code, history.Body)
	}
	if err := json.Unmarshal(history.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Events) != 1 || payload.Events[0].Version != "2" {
		t.Fatalf("failure advanced version/audit: %s", history.Body)
	}
}
