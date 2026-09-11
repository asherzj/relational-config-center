//go:build integration

package main

import (
	"fmt"
	"strings"
	"testing"
)

// The formal eighth increment adds personal notifications without changing
// published SQL, the baseline boundary, or existing control/business facts.
func TestApprovalNotificationsSchemaUpgradeReadinessAndProtection(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 7), buildSchemaMigrationReleaseAt(t, 8)
	ctx, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	deliveryExec(t, db, `INSERT INTO business_marker VALUES(1,'notification upgrade preserves existing rows')`)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('00000000-0000-0000-0000-000000000081','notification.schema','notification.schema@example.test','Retained account','opaque-password-hash',31,3,'2026-01-01')`)
	before := baselineDataSnapshot(t, db)
	ledger := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	requireSchemaMigrationState(t, current, driver, "pending", "status")
	binary := buildIntegrationAdmin(t)
	requireSchemaStartupRejected(t, accountProcessCommand(t, binary, driver))
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,DELETE,REFERENCES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "migration_failed") {
		t.Fatalf("notification DDL failure: %v %s", err, output)
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	if _, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatal("ordinary up bypassed explicit recovery")
	}
	if baselineDataSnapshot(t, db) != before || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`) != ledger {
		t.Fatal("failed migration changed old facts")
	}
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	var table, definition string
	if err := db.QueryRow("SHOW CREATE TABLE rcc_approval_notifications").Scan(&table, &definition); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual MySQL 8.4 notification DDL:\n%s", definition)
	var version int
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 8 {
		t.Fatalf("new migration: %d %v", version, err)
	}
	after := strings.Replace(baselineDataSnapshot(t, db), "rcc_approval_notifications:\n", "", 1)
	if after != before || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=7 ORDER BY id`) != ledger {
		t.Fatal("upgrade rewrote existing facts")
	}
	deliveryExec(t, db, `CREATE DATABASE approval_notifications_fresh CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	fresh := owner
	fresh.DBName = "approval_notifications_fresh"
	requireSchemaMigrationState(t, current, &fresh, "current", "up")
	assertBaselinePhysicalSchemaEqual(t, db, deliveryDB(t, &fresh))
	// Keep the 7→8 historical facts proof separate from the subsequent cutover.
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	process := accountProcessCommand(t, binary, driver)
	process.ready(t)
	for _, fault := range []struct{ apply, restore string }{
		{`RENAME TABLE rcc_approval_notifications TO held_notifications`, `RENAME TABLE held_notifications TO rcc_approval_notifications`},
		{`ALTER TABLE rcc_approval_notifications MODIFY read_sequence INT UNSIGNED NOT NULL DEFAULT 0`, `ALTER TABLE rcc_approval_notifications MODIFY read_sequence BIGINT UNSIGNED NOT NULL DEFAULT 0`},
	} {
		deliveryExec(t, db, fault.apply)
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 503 {
			t.Fatalf("incompatible notifications ready: %d", code)
		}
		requireSchemaStartupRejected(t, accountProcessCommand(t, binary, driver))
		deliveryExec(t, db, fault.restore)
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 200 {
			t.Fatalf("restored notifications unready: %d", code)
		}
	}
	process.stop(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	discovered := releaseRequest(t, app, "GET", "/api/v1/database-tables", "", "")
	if discovered.Code != 200 || strings.Contains(discovered.Body.String(), "rcc_approval_notifications") {
		t.Fatal("personal control table exposed")
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "GET", "/api/v1/database-tables/rcc_approval_notifications", "", ""), 404, "database_table_not_found")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/tables/rcc_approval_notifications/query", `{"conditions":[]}`, ""), 403, "protected_table")
	input := fmt.Sprintf(`{"title":"personal notifications protected","items":[{"table_name":%q,"operation":"ADD","content":{}}]}`, "rcc_approval_notifications")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", input, "protected-personal-notifications"), 403, "protected_table")
}
