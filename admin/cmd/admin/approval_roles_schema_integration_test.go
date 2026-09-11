//go:build integration

package main

import (
	"strings"
	"testing"
)

func TestApprovalRoleSchemaPreservesFrozenBaselineAndRecoversUpgrade(t *testing.T) {
	current := buildSchemaMigrationReleaseAt(t, 6)
	_, driver := startIntegrationMySQL(t, "testdata/pre-goose-8b5cd859.sql", "testdata/006-mutation-fixture.sql")
	owner := *driver
	owner.User = "root"
	owner.MultiStatements = true
	owner.Params = map[string]string{"charset": "utf8mb4"}
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	loadHistoricalBaselineData(t, db)
	applyUnmanagedCurrentMigrations(t, db)
	before := baselineDataSnapshot(t, db)
	requireSchemaMigrationState(t, current, driver, "pending", "baseline")
	var version int
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("baseline moved: %d %v", version, err)
	}
	requireSchemaStartupRejected(t, accountProcessCommand(t, buildIntegrationAdmin(t), driver))
	history := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	requireSchemaMigrationState(t, current, driver, "pending", "baseline")
	if baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`) != history {
		t.Fatal("repeat baseline changed history")
	}
	// Fail after the first new table has committed; ordinary up must not guess.
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,DELETE,REFERENCES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_approval_roles TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("partial DDL succeeded: %s", output)
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	if _, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatal("ordinary up retried partial DDL")
	}
	var roles int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_approval_roles'`).Scan(&roles); err != nil || roles != 1 {
		t.Fatalf("first DDL did not commit: %d %v", roles, err)
	}
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `CREATE TRIGGER block_approval_schema_confirmation BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.target_version=6 AND NEW.state='SUCCEEDED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='confirmation boundary'; END IF; END`)
	if _, err := schemaMigrationCommand(current, driver, "recover").CombinedOutput(); err == nil {
		t.Fatal("unconfirmed DDL reported success")
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 6 {
		t.Fatalf("DDL did not complete: %d %v", version, err)
	}
	deliveryExec(t, db, `DROP TRIGGER block_approval_schema_confirmation`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if got := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=5 ORDER BY id`); got != history {
		t.Fatal("upgrade changed frozen version history")
	}
	after := baselineDataSnapshot(t, db)
	for _, table := range []string{"rcc_approval_roles", "rcc_approval_role_members", "rcc_approval_role_requests", "rcc_approval_role_references"} {
		after = strings.Replace(after, table+":\n", "", 1)
	}
	if after != before {
		t.Fatal("upgrade changed existing business or control data")
	}
	// A new installation and an adopted/forward-upgraded one converge on real DDL.
	deliveryExec(t, db, `CREATE DATABASE approval_fresh CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	fresh := owner
	fresh.DBName = "approval_fresh"
	requireSchemaMigrationState(t, current, &fresh, "current", "up")
	assertBaselinePhysicalSchemaEqual(t, db, deliveryDB(t, &fresh))
	// Role migration evidence stays pinned to 6; current Admin requires later stages.
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	process := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	process.ready(t)
	for _, fault := range []struct{ apply, restore string }{
		{`RENAME TABLE rcc_approval_role_members TO held_approval_members`, `RENAME TABLE held_approval_members TO rcc_approval_role_members`},
		{`ALTER TABLE rcc_approval_roles ALTER CHECK chk_approval_role_version NOT ENFORCED`, `ALTER TABLE rcc_approval_roles ALTER CHECK chk_approval_role_version ENFORCED`},
	} {
		deliveryExec(t, db, fault.apply)
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 503 {
			t.Fatalf("damaged role structure ready: %d", code)
		}
		deliveryExec(t, db, fault.restore)
		if code, _, _ := process.request(t, "GET", "/health/ready", "", nil, ""); code != 200 {
			t.Fatalf("restored role structure unready: %d", code)
		}
	}
	process.stop(t)
}
