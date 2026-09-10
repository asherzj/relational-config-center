//go:build integration

package main

import (
	"strings"
	"testing"
)

// The released Goose v2 and main's field-policy feature meet at migration 00003.
// Exercise the real command at both failure boundaries, preserving published data.
func TestSchemaMigrationAddsFieldPoliciesWithoutChangingPublishedState(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 2), buildSchemaMigrationReleaseAt(t, 3)
	_, driver := startIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	loadHistoricalBaselineData(t, db)
	before := baselineDataSnapshot(t, db)
	oldHistory := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	requireSchemaMigrationState(t, current, driver, "pending", "status")
	requireSchemaStartupRejected(t, accountProcessCommand(t, buildIntegrationAdmin(t), driver))

	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("field-table creation without permission succeeded: %s", output)
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "recovery_required") {
		t.Fatalf("ordinary upgrade retried the failed migration: %v %s", err, output)
	}
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `CREATE TRIGGER block_field_migration_confirmation BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.target_version=3 AND NEW.state='SUCCEEDED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test confirmation boundary'; END IF; END`)
	if output, err := schemaMigrationCommand(current, driver, "recover").CombinedOutput(); err == nil {
		t.Fatalf("unconfirmed field-table migration reported success: %s", output)
	}
	var committed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version WHERE version_id=3 AND is_applied=1`).Scan(&committed); err != nil || committed != 1 {
		t.Fatalf("expected committed field-table version: %d %v", committed, err)
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	deliveryExec(t, db, `DROP TRIGGER block_field_migration_confirmation`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if got := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=2 ORDER BY id`); got != oldHistory {
		t.Fatal("upgrade changed the previously published migration history")
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_table_field_policies`); got != "" {
		t.Fatal("migration seeded field policies")
	}
	if got := strings.Replace(baselineDataSnapshot(t, db), "rcc_table_field_policies:\n", "", 1); got != before {
		t.Fatal("field-table upgrade/recovery changed accounts, sessions, policies, release history, versions or business rows")
	}
	// The isolated v2→v3 proof above stays pinned to the published field release.
	// Current Admin also needs the subsequent main-order schema stages.
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	p := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	p.ready(t)
	p.stop(t)
}
