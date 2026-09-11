//go:build integration

package main

import (
	"fmt"
	"strings"
	"testing"
)

// #92 AC-022: the published role schema upgrades through the real command.
// An interrupted DDL keeps existing facts and requires explicit recovery.
func TestTableApprovalSchemaRecoversUpgradeWithoutChangingExistingFacts(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 6), buildSchemaMigrationReleaseAt(t, 7)
	ctx, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	for _, statement := range []string{
		`CREATE TABLE business_marker(id int PRIMARY KEY,note text)`,
		`INSERT INTO business_marker VALUES(1,'preserved table approval upgrade')`,
		`INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('00000000-0000-0000-0000-000000000061','schema.owner','schema.owner@example.test','Retained owner','opaque-password-hash',31,3,'2026-01-01')`,
		`INSERT INTO rcc_approval_roles(id,name,description,enabled,version,creator,modifier,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000062','Retained reviewers','Existing role description',1,4,'00000000-0000-0000-0000-000000000061','00000000-0000-0000-0000-000000000061','2026-01-01','2026-01-02')`,
		`INSERT INTO rcc_approval_role_members(role_id,account_id) VALUES('00000000-0000-0000-0000-000000000062','00000000-0000-0000-0000-000000000061')`,
		`INSERT INTO rcc_approval_role_requests(actor_id,request_key,request_digest,result,created_at) VALUES('00000000-0000-0000-0000-000000000061','retained-role-request',REPEAT('a',64),JSON_OBJECT('retained',true),'2026-01-02')`,
		`INSERT INTO rcc_approval_role_references(role_id,reference_kind,reference_key,role_name,created_at) VALUES('00000000-0000-0000-0000-000000000062','table','business_marker','Original reviewers','2026-01-02')`,
		`INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES('retained-table-approval-order','00000000-0000-0000-0000-000000000061','DRAFT',3,JSON_OBJECT('retained',true))`,
	} {
		deliveryExec(t, db, statement)
	}
	newTables := []string{"rcc_table_approval_assignments", "rcc_table_approval_requests"}
	existingData := func() string {
		snapshot := strings.Replace(baselineDataSnapshot(t, db), "rcc_approval_notifications:\n", "", 1)
		for _, table := range newTables {
			// Only empty additive tables are excluded; seeded rows still fail this comparison.
			snapshot = strings.Replace(snapshot, table+":\n", "", 1)
		}
		return snapshot
	}
	before := existingData()
	ledger := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	attempts := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	assertExistingFacts := func() {
		t.Helper()
		if existingData() != before || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=6 ORDER BY id`) != ledger || baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts WHERE target_version<=6 ORDER BY id`) != attempts {
			t.Fatal("table approval upgrade changed existing data or published migration history")
		}
	}
	requireSchemaMigrationState(t, current, driver, "pending", "status")
	binary := buildIntegrationAdmin(t)
	requireSchemaStartupRejected(t, accountProcessCommand(t, binary, driver))

	// Permit the first new table but reject the second CREATE after the first DDL commits.
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,DELETE,REFERENCES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_table_approval_assignments TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "migration_failed") {
		t.Fatalf("partial table approval DDL must fail: %v %s", err, output)
	}
	var first, second, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_table_approval_assignments'`).Scan(&first); err != nil || first != 1 {
		t.Fatalf("first table did not commit before failure: %d %v", first, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_table_approval_requests'`).Scan(&second); err != nil || second != 0 {
		t.Fatalf("second table unexpectedly committed: %d %v", second, err)
	}
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("failed DDL advanced version: %d %v", version, err)
	}
	assertExistingFacts()
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	failedLedger := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	failedAttempts := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "recovery_required") {
		t.Fatalf("ordinary up retried partial table approval DDL: %v %s", err, output)
	}
	if baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`) != failedLedger || baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`) != failedAttempts {
		t.Fatal("refused ordinary up changed migration ledgers")
	}
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 7 {
		t.Fatalf("table approval upgrade version: %d %v", version, err)
	}
	assertExistingFacts()

	// Compare the physical structures produced by recovery and a clean installation.
	deliveryExec(t, db, `CREATE DATABASE table_approval_fresh CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	fresh := owner
	fresh.DBName = "table_approval_fresh"
	requireSchemaMigrationState(t, current, &fresh, "current", "up")
	assertBaselinePhysicalSchemaEqual(t, db, deliveryDB(t, &fresh))

	// The table approval migration remains pinned to 7; current processes require later increments.
	preservedBeforeCutover := cutoverPreservedSnapshot(t, db)
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	if strings.Replace(cutoverPreservedSnapshot(t, db), "rcc_approval_notifications:\n", "", 1) != preservedBeforeCutover {
		t.Fatal("current cutover changed facts outside current account roles")
	}
	var currentRoles, currentRoleVersion int
	if err := db.QueryRow(`SELECT roles,role_version FROM rcc_accounts WHERE username='schema.owner'`).Scan(&currentRoles, &currentRoleVersion); err != nil || currentRoles != 27 || currentRoleVersion != 4 {
		t.Fatalf("current cutover role mapping: roles=%d version=%d error=%v", currentRoles, currentRoleVersion, err)
	}
	currentData := baselineDataSnapshot(t, db)
	process := accountProcessCommand(t, binary, driver)
	process.ready(t)
	for _, table := range newTables {
		deliveryExec(t, db, "RENAME TABLE "+table+" TO held_table_approval")
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 503 {
			t.Fatalf("missing %s still ready: %d", table, code)
		}
		requireSchemaStartupRejected(t, accountProcessCommand(t, binary, driver))
		deliveryExec(t, db, "RENAME TABLE held_table_approval TO "+table)
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 200 {
			t.Fatalf("restored %s unready: %d", table, code)
		}
	}
	process.stop(t)
	if baselineDataSnapshot(t, db) != currentData || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=6 ORDER BY id`) != ledger || baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts WHERE target_version<=6 ORDER BY id`) != attempts {
		t.Fatal("readiness checks changed current data or published migration history")
	}

	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	discovered := releaseRequest(t, app, "GET", "/api/v1/database-tables", "", "")
	if discovered.Code != 200 || !strings.Contains(discovered.Body.String(), "business_marker") {
		t.Fatalf("ordinary table discovery failed: %d %s", discovered.Code, discovered.Body)
	}
	for _, table := range newTables {
		if strings.Contains(discovered.Body.String(), table) {
			t.Fatalf("control table exposed in discovery: %s", table)
		}
		assertIntegrationErrorCode(t, releaseRequest(t, app, "GET", "/api/v1/database-tables/"+table, "", ""), 404, "database_table_not_found")
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/tables/"+table+"/query", `{"conditions":[]}`, ""), 403, "protected_table")
		input := fmt.Sprintf(`{"title":"control data must stay protected","items":[{"table_name":%q,"operation":"ADD","content":{}}]}`, table)
		assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", input, "protected-"+table), 403, "protected_table")
	}
}
