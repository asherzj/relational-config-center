//go:build integration

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// AC-021: the formal upgrade contracts current grants, never historical facts.
func TestLegacyApproverFormalCutover(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 8), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	for roles := 1; roles <= 31; roles++ {
		for _, enabled := range []bool{true, false} {
			id := cutoverAccountID(roles, enabled)
			deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,enabled,created_at) VALUES(?,?,?,?,?,?,7,5,?,'2026-01-01')`, id, fmt.Sprintf("legacy-%d-%t", roles, enabled), id+"@example.test", "Retained", "opaque", roles, enabled)
		}
	}

	// Existing sessions, old-format order JSON, role assignments, request facts,
	// personal progress and even expired auth records must survive this upgrade.
	for _, roles := range []int{4, 12, 20, 31} {
		id := cutoverAccountID(roles, true)
		deliveryExec(t, db, `INSERT INTO rcc_login_sessions(token_hash,account_id,csrf_hash,password_version,session_version,created_at,last_active_at,expires_at) VALUES(SHA2(?,256),?,SHA2('cutover-csrf',256),1,5,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)+INTERVAL 8 HOUR)`, fmt.Sprintf("old-cutover-session-%d", roles), id)
		deliveryExec(t, db, `INSERT INTO rcc_approval_notifications(account_id,order_id,sequence,pending,pending_sequence,result_sequence,read_sequence) VALUES(?,'old-order',11,1,9,11,7)`, id)
	}
	deliveryExec(t, db, `INSERT INTO rcc_auth_rate_limits(bucket_key,attempts,in_flight,expires_at) VALUES('old-expired-window',3,0,'2000-01-01')`)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	deliveryExec(t, db, `INSERT INTO business_marker VALUES(1,'retained old business value')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES('old-order',?,'PUBLISHED',7,'{"old_format":true,"unicode":"旧事实","original":7}')`, cutoverAccountID(12, true))
	deliveryExec(t, db, `INSERT INTO rcc_approval_roles(id,name,description,creator,modifier,created_at,updated_at) VALUES('00000000-0000-4000-8000-000000000098','Existing role','Retained role',?,?,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))`, cutoverAccountID(20, true), cutoverAccountID(20, true))
	deliveryExec(t, db, `INSERT INTO rcc_approval_role_members(role_id,account_id) VALUES('00000000-0000-4000-8000-000000000098',?)`, cutoverAccountID(4, true))
	deliveryExec(t, db, `INSERT INTO rcc_table_approval_assignments(table_name,version,role_ids) VALUES('business_marker',3,'["00000000-0000-4000-8000-000000000098"]')`)
	actor, target := cutoverAccountID(20, true), cutoverAccountID(12, true)
	for _, prior := range []struct {
		key            string
		roles, version int
	}{{"legacy-grant-key", 12, 7}, {"normal-grant-key", 10, 6}} {
		result, _ := json.Marshal(map[string]any{"ID": target, "Username": "legacy-12-true", "DisplayName": "Retained", "Enabled": true, "Roles": prior.roles, "RoleVersion": prior.version})
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%d:%d", actor, target, prior.roles, prior.version-1))))
		deliveryExec(t, db, `INSERT INTO rcc_account_role_history(actor_kind,actor_id,account_id,before_roles,after_roles,version,request_key,request_digest,result,created_at) VALUES('account',?,?,4,?,?,?,?,?,'2026-01-01')`, actor, target, prior.roles, prior.version, prior.key, digest, string(result))
	}
	preserved := cutoverPreservedSnapshot(t, db)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if cutoverPreservedSnapshot(t, db) != preserved {
		t.Fatal("cutover changed immutable history, request result/digest/actor, session, notification, role membership, old order or business facts")
	}
	for roles := 1; roles <= 31; roles++ {
		for _, enabled := range []bool{true, false} {
			var actual, version, session int
			var active bool
			if err := db.QueryRow(`SELECT roles,role_version,session_version,enabled FROM rcc_accounts WHERE id=?`, cutoverAccountID(roles, enabled)).Scan(&actual, &version, &session, &active); err != nil {
				t.Fatal(err)
			}
			// Fixed expected outcomes, independent of the migration expression.
			expected := []int{0, 1, 2, 3, 1, 1, 2, 3, 8, 9, 10, 11, 8, 9, 10, 11, 16, 17, 18, 19, 16, 17, 18, 19, 24, 25, 26, 27, 24, 25, 26, 27}[roles]
			expectedVersion := 7
			if expected != roles {
				expectedVersion = 8
			}
			if actual != expected || version != expectedVersion || session != 5 || active != enabled {
				t.Fatalf("old roles %d enabled %t: got roles/version/session/enabled %d/%d/%d/%t want %d/%d/5/%t", roles, enabled, actual, version, session, active, expected, expectedVersion, enabled)
			}
		}
	}
	before := baselineDataSnapshot(t, db)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if baselineDataSnapshot(t, db) != before {
		t.Fatal("repeated upgrade changed account or unrelated facts")
	}
	process := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	process.ready(t)
	for _, sample := range []struct {
		old   int
		names string
	}{{4, `["VIEWER"]`}, {12, `["PUBLISHER"]`}, {20, `["ADMIN"]`}, {31, `["VIEWER","EDITOR","PUBLISHER","ADMIN"]`}} {
		cookies := cutoverCookies(sample.old)
		status, _, body := process.request(t, "GET", "/api/v1/auth/session", "", cookies, "")
		if status != 200 || !strings.Contains(string(body), `"roles":`+sample.names) {
			t.Fatalf("old cookie %d: %d %s", sample.old, status, body)
		}
		status, _, body = process.request(t, "GET", "/api/v1/account-roles", "", cookies, "")
		if (sample.old == 4 || sample.old == 12) && status != 403 {
			t.Fatalf("non-admin gained management: %d %s", status, body)
		}
		if sample.old == 20 || sample.old == 31 {
			if status != 200 || strings.Contains(string(body), "APPROVER") {
				t.Fatalf("current account list: %d %s", status, body)
			}
		}
	}
	status, _, history := process.request(t, "GET", "/api/v1/account-roles/"+target+"/history", "", cutoverCookies(20), "")
	if status != 200 || !strings.Contains(string(history), `"before_roles":["APPROVER"]`) || !strings.Contains(string(history), `"after_roles":["APPROVER","PUBLISHER"]`) {
		t.Fatalf("historical decoder: %d %s", status, history)
	}
	status, _, body := process.requestWithKey(t, "PUT", "/api/v1/account-roles/"+target, `{"roles":["APPROVER","PUBLISHER"],"expected_version":"6"}`, cutoverCookies(20), "cutover-csrf", "legacy-grant-key")
	if status != 422 {
		t.Fatalf("legacy request was translated/regranted: %d %s", status, body)
	}
	oldResult := baselineRows(t, db, `SELECT * FROM rcc_account_role_history`)
	status, _, body = process.requestWithKey(t, "PUT", "/api/v1/account-roles/"+target, `{"roles":["EDITOR","PUBLISHER"],"expected_version":"5"}`, cutoverCookies(20), "cutover-csrf", "normal-grant-key")
	if status != 200 || !strings.Contains(string(body), `"roles":["EDITOR","PUBLISHER"]`) || !strings.Contains(string(body), `"version":"6"`) {
		t.Fatalf("valid historical request did not retain its saved result: %d %s", status, body)
	}
	if baselineRows(t, db, `SELECT * FROM rcc_account_role_history`) != oldResult {
		t.Fatal("replay changed immutable stored history")
	}
	var currentRoles, currentVersion int
	if err := db.QueryRow(`SELECT roles,role_version FROM rcc_accounts WHERE id=?`, target).Scan(&currentRoles, &currentVersion); err != nil || currentRoles != 8 || currentVersion != 8 {
		t.Fatal("saved-result replay re-applied old authority")
	}
	status, _, body = process.requestWithKey(t, "PUT", "/api/v1/account-roles/"+target, `{"roles":["VIEWER"],"expected_version":"7"}`, cutoverCookies(20), "cutover-csrf", "stale-cutover-form")
	if status != 409 || !strings.Contains(string(body), "account_roles_conflict") {
		t.Fatalf("old form overwrote cutover: %d %s", status, body)
	}
	process.stop(t)

}

// AC-021: a retired grant is rejected at the public input boundary, while
// immutable event values retain their original meaning.
func TestLegacyApproverCurrentInputRejectsRetiredGrants(t *testing.T) {
	f := newMaintenanceFixture(t)
	admin := registerAccount(t, f.app, "cutover.admin", "cutover.admin@example.test", "correct horse battery staple")
	target := registerAccount(t, f.app, "cutover.target", "cutover.target@example.test", "correct horse battery staple")
	f.run(t, "", "grant-admin", "--id", accountID(t, admin))
	path := "/api/v1/account-roles/" + accountID(t, target)
	for _, roles := range []string{`["APPROVER"]`, `["EDITOR","APPROVER"]`, `["APPROVER","ADMIN"]`} {
		got := accountRequestFrom(f.app, "PUT", path, `{"roles":`+roles+`,"expected_version":"1"}`, admin.Result().Cookies(), sessionCSRF(t, admin), "192.0.2.1:1234", map[string]string{"Idempotency-Key": "retired-grant-request"})
		if got.Code != 422 {
			t.Fatalf("retired role request: %d %s", got.Code, got.Body)
		}
	}
}

func cutoverAccountID(roles int, enabled bool) string {
	status := 0
	if enabled {
		status = 1
	}
	return fmt.Sprintf("00000000-0000-4000-8%d00-%012d", status, roles)
}
func cutoverCookies(roles int) []*http.Cookie {
	return []*http.Cookie{{Name: "rcc-session-dev", Value: fmt.Sprintf("old-cutover-session-%d", roles)}}
}
func cutoverPreservedSnapshot(t *testing.T, db *sql.DB) string {
	all := baselineDataSnapshot(t, db)
	accounts := baselineRows(t, db, `SELECT * FROM rcc_accounts`)
	// Compare every other table in full, and every account field except the two
	// explicitly changed current-grant fields.
	return strings.Replace(all, "rcc_accounts:"+accounts+"\n", "", 1) + baselineRows(t, db, `SELECT id,username,email,display_name,password_hash,enabled,password_version,session_version,created_at FROM rcc_accounts`)
}

// AC-022: data failure and both durable bookkeeping boundaries require explicit
// recovery; repeated execution never advances a contracted account twice.
func TestLegacyApproverCutoverRecovery(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 8), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	for index, fault := range []struct {
		name, trigger string
		applied       bool
	}{
		{"statement_rollback", `CREATE TRIGGER cutover_fault BEFORE UPDATE ON rcc_accounts FOR EACH ROW BEGIN IF OLD.roles=31 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='cutover write failure'; END IF; END`, false},
		{"data_committed_before_goose", `CREATE TRIGGER cutover_fault BEFORE INSERT ON rcc_goose_db_version FOR EACH ROW BEGIN IF NEW.version_id=9 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='goose write failure'; END IF; END`, true},
		{"goose_committed_before_confirmation", `CREATE TRIGGER cutover_fault BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.target_version=9 AND NEW.state='SUCCEEDED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='confirmation failure'; END IF; END`, true},
	} {
		t.Run(fault.name, func(t *testing.T) {
			name := fmt.Sprintf("cutover_recovery_%d", index)
			deliveryExec(t, db, "CREATE DATABASE "+name+" CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
			isolated := owner
			isolated.DBName = name
			target := deliveryDB(t, &isolated)
			requireSchemaMigrationState(t, previous, &isolated, "current", "up")
			deliveryExec(t, target, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('first','first','first@example.test','Retained','opaque',4,7,'2026-01-01'),('second','second','second@example.test','Retained','opaque',31,7,'2026-01-01')`)
			before := baselineDataSnapshot(t, target)
			deliveryExec(t, target, fault.trigger)
			if output, err := schemaMigrationCommand(current, &isolated, "up").CombinedOutput(); err == nil {
				t.Fatalf("fault succeeded: %s", output)
			}
			requireSchemaMigrationState(t, current, &isolated, "recovery_required", "status")
			after := baselineDataSnapshot(t, target)
			if !fault.applied && after != before {
				t.Fatal("failed UPDATE partially modified accounts")
			}
			if fault.applied {
				var rows int
				if err := target.QueryRow(`SELECT COUNT(*) FROM rcc_accounts WHERE roles IN (1,27) AND role_version=8`).Scan(&rows); err != nil || rows != 2 {
					t.Fatalf("statement was not durably applied: %d %v", rows, err)
				}
			}
			if _, err := schemaMigrationCommand(current, &isolated, "up").CombinedOutput(); err == nil {
				t.Fatal("normal up bypassed explicit recovery")
			}
			deliveryExec(t, target, `DROP TRIGGER cutover_fault`)
			if index == 2 {
				deliveryExec(t, target, `UPDATE rcc_accounts SET roles=4 WHERE id='first'`)
				damaged := baselineDataSnapshot(t, target)
				if _, err := schemaMigrationCommand(current, &isolated, "recover").CombinedOutput(); err == nil {
					t.Fatal("recorded cutover accepted legacy current data")
				}
				if baselineDataSnapshot(t, target) != damaged {
					t.Fatal("confirmation recovery repaired invalid current data")
				}
				deliveryExec(t, target, `UPDATE rcc_accounts SET roles=1 WHERE id='first'`)
			}
			requireSchemaMigrationState(t, current, &isolated, "current", "recover")
			requireSchemaMigrationState(t, current, &isolated, "current", "up")
			var rows int
			if err := target.QueryRow(`SELECT COUNT(*) FROM rcc_accounts WHERE roles IN (1,27) AND role_version=8`).Scan(&rows); err != nil || rows != 2 {
				t.Fatalf("recovery duplicated/lost contraction: %d %v", rows, err)
			}
			if fault.applied && baselineDataSnapshot(t, target) != after {
				t.Fatal("recovery rewrote already-committed data")
			}
		})
	}
}

func TestLegacyApproverInvalidDataAndMaintenanceBoundary(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 8), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('00000000-0000-4000-8000-000000000298','old','old@example.test','Retained','opaque',20,7,'2026-01-01')`)
	maintenance := filepath.Join(t.TempDir(), "account-maintain")
	build := exec.Command("go", "build", "-o", maintenance, "../account-maintain")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build maintenance: %v %s", err, output)
	}
	command := func(action string) *exec.Cmd {
		cmd := exec.Command(maintenance, action, "--id", "00000000-0000-4000-8000-000000000298")
		cmd.Env = integrationEnvironment(driver, "invalid-http-address")
		return cmd
	}
	for _, roles := range []int{4, 20, 31} {
		deliveryExec(t, db, `UPDATE rcc_accounts SET roles=?`, roles)
		before := baselineDataSnapshot(t, db)
		output, err := command("grant-admin").CombinedOutput()
		if err == nil || !strings.Contains(string(output), "schema_not_ready") {
			t.Fatalf("maintenance bypassed cutover for %d: %v %s", roles, err, output)
		}
		if baselineDataSnapshot(t, db) != before {
			t.Fatal("rejected maintenance changed account or auth records")
		}
		if output, err := command("lookup").CombinedOutput(); err != nil {
			t.Fatalf("read-only maintenance globally coupled to Ready: %v %s", err, output)
		}
	}
	for _, roles := range []int{0, 32} {
		deliveryExec(t, db, `UPDATE rcc_accounts SET roles=?`, roles)
		before := baselineDataSnapshot(t, db) + baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts`)
		if _, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
			t.Fatal("invalid legacy data auto-repaired")
		}
		if baselineDataSnapshot(t, db)+baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts`) != before {
			t.Fatal("rejected precheck wrote data")
		}
	}
	deliveryExec(t, db, `UPDATE rcc_accounts SET roles=4,role_version=18446744073709551615`)
	before := baselineDataSnapshot(t, db)
	if _, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatal("overflow silently wrapped/clamped role version")
	}
	if baselineDataSnapshot(t, db) != before {
		t.Fatal("overflow changed current grants")
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	// Explicit fixture repair establishes a recoverable positive version; recovery
	// itself does not invent or reset an overflowed version.
	deliveryExec(t, db, `UPDATE rcc_accounts SET role_version=7`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	if output, err := command("grant-admin").CombinedOutput(); err != nil {
		t.Fatalf("current maintenance cannot grant: %v %s", err, output)
	}
	var roles, version int
	if err := db.QueryRow(`SELECT roles,role_version FROM rcc_accounts`).Scan(&roles, &version); err != nil || roles != 17 || version != 9 {
		t.Fatalf("current grant lost viewer or duplicated cutover: %d %d %v", roles, version, err)
	}
	next := buildNextSchemaMigrationRelease(t)
	deliveryExec(t, db, `CREATE TRIGGER future_cutover_fault BEFORE INSERT ON rcc_goose_db_version FOR EACH ROW BEGIN IF NEW.version_id=10 THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='future ledger failure'; END IF; END`)
	if _, err := schemaMigrationCommand(next, driver, "up").CombinedOutput(); err == nil {
		t.Fatal("future fault did not fire")
	}
	deliveryExec(t, db, `DROP TRIGGER future_cutover_fault`)
	deliveryExec(t, db, `UPDATE rcc_accounts SET roles=4`)
	damaged := baselineDataSnapshot(t, db)
	if _, err := schemaMigrationCommand(next, driver, "recover").CombinedOutput(); err == nil {
		t.Fatal("future recovering migration inherited the v9 legacy exception")
	}
	if baselineDataSnapshot(t, db) != damaged {
		t.Fatal("future recovery changed invalid current data")
	}
	deliveryExec(t, db, `UPDATE rcc_accounts SET roles=17`)
	requireSchemaMigrationState(t, next, driver, "current", "recover")

}

// v8 and v9 have identical business/control structures. Build valid approval
// facts via public HTTP, copy that fixture into an explicitly installed v8
// database, then set its historical current grant. The copied database has its
// own authentic v8 migration ledger; no current ledger is downgraded or edited.
func TestLegacyApproverCutoverPreservesLiveResponsibilityAndCompletedApproval(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	admin := integrationAdminSession(t, app)
	member := registerAccount(t, app, "cutover.member", "cutover.member@example.test", "correct horse battery staple")
	assignTableApproval(t, app, "mutation_add_items", tableApprovalRole(t, app, "Retained responsibility", member))
	submitted := func(key string) string {
		path := createTableApprovalDraft(t, app, admin, key)
		rollbackOrderResponse(t, releaseActorRequest(t, app, admin, "POST", path+"/submit", `{"expected_version":"1"}`, key+"-submit"), 200)
		return path
	}
	completed := submitted("cutover-approved")
	saved := rollbackOrderResponse(t, releaseActorRequest(t, app, member, "POST", completed+"/approve", confirmedApprovalBody(t, app, member, completed, "Valid independent decision"), "cutover-approve"), 200)
	pending := submitted("cutover-pending")
	assertApprovalCounts(t, app, member, 1, 1)
	beforePending := readApprovalProgress(t, app, member, pending)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE DATABASE cutover_live_v8 CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	previous := owner
	previous.DBName = "cutover_live_v8"
	requireSchemaMigrationState(t, buildSchemaMigrationReleaseAt(t, 8), &previous, "current", "up")
	target := deliveryDB(t, &previous)
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' AND table_name NOT IN ('rcc_goose_db_version','rcc_schema_migration_attempts') ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	conn, err := target.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=0`); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		quoted := "`" + strings.ReplaceAll(table, "`", "``") + "`"
		if !strings.HasPrefix(table, "rcc_") {
			if _, err := conn.ExecContext(ctx, "CREATE TABLE "+quoted+" LIKE rcc_test."+quoted); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := conn.ExecContext(ctx, "DELETE FROM "+quoted); err != nil {
			t.Fatal(err)
		}
		// Generated business columns recompute from the copied real inputs; MySQL
		// rejects even an empty INSERT SELECT that explicitly assigns them.
		columns, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND generation_expression='' ORDER BY ordinal_position`, table)
		if err != nil {
			t.Fatal(err)
		}
		var writable []string
		for columns.Next() {
			var name string
			if err := columns.Scan(&name); err != nil {
				t.Fatal(err)
			}
			writable = append(writable, "`"+strings.ReplaceAll(name, "`", "``")+"`")
		}
		if err := columns.Err(); err != nil {
			t.Fatal(err)
		}
		columns.Close()
		list := strings.Join(writable, ",")
		if _, err := conn.ExecContext(ctx, "INSERT INTO "+quoted+"("+list+") SELECT "+list+" FROM rcc_test."+quoted); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS=1`); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	deliveryExec(t, target, `UPDATE rcc_accounts SET roles=4 WHERE id=?`, accountID(t, member))
	deliveryExec(t, target, `UPDATE rcc_accounts SET roles=20 WHERE id=?`, accountID(t, admin))
	preserved := cutoverPreservedSnapshot(t, target)
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), &previous, "current", "up")
	if cutoverPreservedSnapshot(t, target) != preserved {
		t.Fatal("cutover rewrote actual approval/request/notification facts")
	}
	upgraded, err := newApplication(ctx, integrationConfig(&previous))
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	assertApprovalCounts(t, upgraded, member, 1, 1)
	if got := readApprovalProgress(t, upgraded, member, pending); got != beforePending {
		t.Fatalf("old member notification changed: %+v %+v", beforePending, got)
	}
	if got := readTableApprovalOrder(t, upgraded, member, completed); !reflect.DeepEqual(got.Approvals, saved.Approvals) {
		t.Fatal("legitimate completed approval changed during cutover")
	}
	available := readTableApprovalOrder(t, upgraded, member, pending)
	if len(available.ApprovalContext.ApprovableTables) != 1 || available.ApprovalContext.Tables[0].Mode != "ROLE" {
		t.Fatalf("migrated viewer lost existing responsibility: %+v", available.ApprovalContext)
	}
	rollbackOrderResponse(t, releaseActorRequest(t, upgraded, member, "POST", pending+"/approve", confirmedApprovalBody(t, upgraded, member, pending, "Still assigned after upgrade"), "cutover-after-approve"), 200)
	assertApprovalCounts(t, upgraded, member, 0, 0)
}
