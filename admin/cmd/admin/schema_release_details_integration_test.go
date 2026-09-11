//go:build integration

package main

import (
	"strings"
	"testing"
)

// #81 AC-015: each interrupted table is a whole previous or target schema.
// These are real privilege failures between DDL statements, not mocked status.
func TestReleaseSchemaStagesRecoverWithoutRewritingLegacyBusinessFacts(t *testing.T) {
	v3, v4, v5 := buildSchemaMigrationReleaseAt(t, 3), buildSchemaMigrationReleaseAt(t, 4), buildSchemaMigrationReleaseAt(t, 5)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, v3, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	first, second := strings.Repeat("a", 32), strings.Repeat("b", 32)
	for index, id := range []string{first, second} {
		deliveryExec(t, db, `INSERT INTO rcc_release_orders(id,table_name,applicant_id,state,version,document) VALUES(?,'legacy_table','legacy-actor','SUCCEEDED',4,JSON_OBJECT('id',?,'publication',JSON_OBJECT('untouched',true)))`, id, id)
		deliveryExec(t, db, `INSERT INTO rcc_publication_commands(table_name,sequence,order_id,document) VALUES('legacy_table',?,?,JSON_OBJECT('old','command','sequence',?))`, index+1, id, index+1)
		deliveryExec(t, db, `INSERT INTO rcc_refresh_notifications(order_id,table_name,table_version,document) VALUES(?,'legacy_table',?,JSON_OBJECT('id',?,'status','NOT_CONNECTED'))`, id, index+1, id)
	}
	snapshots := func() string {
		return baselineRows(t, db, `SELECT id,applicant_id,state,version,document FROM rcc_release_orders ORDER BY id`) + baselineRows(t, db, `SELECT table_name,sequence,order_id,document FROM rcc_publication_commands ORDER BY sequence`) + baselineRows(t, db, `SELECT order_id,table_name,table_version,document FROM rcc_refresh_notifications ORDER BY order_id`)
	}
	original := snapshots()
	publishedHistory := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,DELETE,ALTER ON rcc_test.* TO 'rcc_admin'@'%'`)
	for _, table := range []string{"rcc_release_details", "rcc_release_executions", "rcc_schema_migration_attempts", "rcc_goose_db_version"} {
		deliveryExec(t, db, "GRANT CREATE ON rcc_test."+table+" TO 'rcc_admin'@'%'")
	}
	if output, err := schemaMigrationCommand(v4, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("expected the final new-table CREATE to fail: %s", output)
	} else {
		t.Logf("additive interruption: %s", output)
	}
	requireSchemaMigrationState(t, v4, driver, "recovery_required", "status")
	var columns int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND column_name='execution_id' AND table_name IN ('rcc_publication_commands','rcc_refresh_notifications')`).Scan(&columns); err != nil || columns != 2 {
		t.Fatalf("additive stage not reached: %d %v", columns, err)
	}
	if snapshots() != original {
		t.Fatal("interrupted additive DDL changed legacy documents")
	}
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, v4, driver, "current", "recover")
	// Allow the notification PK to commit, then stop the subsequent header ALTER.
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,DELETE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE ON rcc_test.rcc_goose_db_version TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT ALTER ON rcc_test.rcc_refresh_notifications TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(v5, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("expected header ALTER to fail after notification key commit: %s", output)
	} else {
		t.Logf("ownership interruption: %s", output)
	}
	requireSchemaMigrationState(t, v5, driver, "recovery_required", "status")
	var key string
	if err := db.QueryRow(`SELECT GROUP_CONCAT(column_name ORDER BY seq_in_index) FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name='rcc_refresh_notifications' AND index_name='PRIMARY'`).Scan(&key); err != nil || key != "execution_id,table_name" {
		t.Fatalf("atomic notification key not committed: %s %v", key, err)
	}
	identities := baselineRows(t, db, `SELECT execution_id,order_id,table_name FROM rcc_refresh_notifications ORDER BY order_id`)
	if snapshots() != original {
		t.Fatal("identity backfill rewrote legacy business facts or removed notification multiplicity")
	}
	deliveryExec(t, db, `GRANT ALTER ON rcc_test.rcc_release_orders TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, v5, driver, "current", "recover")
	requireSchemaMigrationState(t, v5, driver, "current", "up")
	if snapshots() != original || baselineRows(t, db, `SELECT execution_id,order_id,table_name FROM rcc_refresh_notifications ORDER BY order_id`) != identities {
		t.Fatal("recovery changed legacy facts or stable technical IDs")
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=3 ORDER BY id`); got != publishedHistory {
		t.Fatal("published Goose history changed")
	}
	var valid int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_refresh_notifications WHERE execution_id=CONCAT('legacy:',order_id) AND LENGTH(execution_id)=39`).Scan(&valid); err != nil || valid != 2 {
		t.Fatalf("legacy technical identities/multiplicity: %d %v", valid, err)
	}
}
