//go:build integration

package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestTableReleaseSchemaMigratesExistingTablesAndPreservesSelections(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 5), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE association_business_marker(id INT PRIMARY KEY,note VARCHAR(30))`)
	deliveryExec(t, db, `INSERT INTO association_business_marker VALUES(1,'preserved')`)
	deliveryExec(t, db, `INSERT INTO rcc_table_policies(id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES(101,'association_business_marker','query','mutation',1,'original-admin','original-admin'),(102,'disabled_business','query','mutation',0,'original-admin','original-admin')`)
	before := baselineRows(t, db, `SELECT id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier,created_at,updated_at,concurrency_key FROM rcc_table_policies ORDER BY id`)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	var associations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_table_release_templates`).Scan(&associations); err != nil || associations != 4 {
		t.Fatalf("both existing table rules need default associations: count=%d err=%v", associations, err)
	}
	if got := baselineRows(t, db, `SELECT id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier,created_at,updated_at,concurrency_key FROM rcc_table_policies ORDER BY id`); got != before {
		t.Fatal("migration changed existing table rule configuration or audit")
	}
	var marker string
	if err := db.QueryRow(`SELECT note FROM association_business_marker WHERE id=1`).Scan(&marker); err != nil || marker != "preserved" {
		t.Fatalf("business data changed: %q %v", marker, err)
	}
	deliveryExec(t, db, `INSERT INTO rcc_release_templates(code,name,description,release_type,node_list,monitor_list,enabled,creator,modifier) SELECT 'chosen_emergency_v1','Chosen emergency',description,release_type,node_list,monitor_list,1,'admin-choice','admin-choice' FROM rcc_release_templates WHERE code='default_emergency_v1'`)
	deliveryExec(t, db, `UPDATE rcc_table_release_templates SET template_id=(SELECT id FROM rcc_release_templates WHERE code='chosen_emergency_v1'),version=2,modifier='admin-choice' WHERE table_policy_id=101 AND release_type='EMERGENCY'`)
	deliveryExec(t, db, `UPDATE rcc_table_release_templates SET enabled=0,version=2,modifier='admin-choice' WHERE table_policy_id=101 AND release_type='STANDARD'`)
	selected := baselineDataSnapshot(t, db)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if got := baselineDataSnapshot(t, db); got != selected {
		t.Fatal("repeated upgrade replaced administrator template selections or audit")
	}
	fresh := createSchemaComparisonDatabase(t, driver)
	requireSchemaMigrationState(t, current, fresh, "current", "up")
	freshDB := deliveryDB(t, fresh)
	counter := regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`)
	for _, name := range []string{"rcc_table_policies", "rcc_release_templates", "rcc_table_release_templates"} {
		var table, upgraded, installed string
		if err := db.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &upgraded); err != nil {
			t.Fatal(err)
		}
		if err := freshDB.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &installed); err != nil {
			t.Fatal(err)
		}
		if counter.ReplaceAllString(upgraded, "") != counter.ReplaceAllString(installed, "") {
			t.Fatalf("fresh and upgraded association structure differs for %s", name)
		}
	}
}

func TestTableReleaseSchemaReadinessRejectsMissingEmergencyAssociationReadOnly(t *testing.T) {
	_, driver := startCurrentIntegrationMySQL(t)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `INSERT INTO rcc_table_policies(id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES(501,'managed_marker','query','mutation',1,'admin','admin')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_templates(code,name,description,release_type,node_list,monitor_list,enabled,creator,modifier) SELECT 'readiness_emergency_v1','Readiness custom',description,release_type,node_list,monitor_list,1,'admin','admin' FROM rcc_release_templates WHERE code='default_emergency_v1'`)
	deliveryExec(t, db, `INSERT INTO rcc_table_release_templates(table_policy_id,release_type,template_id,creator,modifier) SELECT 501,'EMERGENCY',id,'admin','admin' FROM rcc_release_templates WHERE code='readiness_emergency_v1'`)
	deliveryExec(t, db, `CREATE USER 'association_reader'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT ON rcc_test.* TO 'association_reader'@'%'`)
	reader := *driver
	reader.User = "association_reader"
	p := accountProcessCommand(t, buildIntegrationAdmin(t), &reader)
	p.ready(t)
	var nodes string
	if err := db.QueryRow(`SELECT node_list FROM rcc_release_templates WHERE code='readiness_emergency_v1'`).Scan(&nodes); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []struct{ name, statement string }{
		{"wrong_role", `UPDATE rcc_release_templates SET node_list=JSON_SET(node_list,'$[0].required_role','EDITOR') WHERE code='readiness_emergency_v1'`},
		{"wrong_node_type", `UPDATE rcc_release_templates SET node_list=JSON_SET(node_list,'$[0].type','APPROVAL') WHERE code='readiness_emergency_v1'`},
		{"duplicate_node_code", `UPDATE rcc_release_templates SET node_list=JSON_SET(node_list,'$[1].code',JSON_UNQUOTE(JSON_EXTRACT(node_list,'$[0].code'))) WHERE code='readiness_emergency_v1'`},
	} {
		t.Run(fault.name, func(t *testing.T) {
			deliveryExec(t, db, fault.statement)
			t.Cleanup(func() {
				deliveryExec(t, db, `UPDATE rcc_release_templates SET node_list=? WHERE code='readiness_emergency_v1'`, nodes)
			})
			before := baselineDataSnapshot(t, db)
			if status, _, body := p.request(t, "GET", "/health/ready", "", nil, ""); status != 503 {
				t.Fatalf("invalid referenced emergency nodes must reject readiness: %d %s", status, body)
			}
			if got := baselineDataSnapshot(t, db); got != before {
				t.Fatal("invalid-node readiness check changed persisted data")
			}
		})
	}
	deliveryExec(t, db, `DELETE FROM rcc_table_release_templates WHERE table_policy_id=501`)
	before := baselineDataSnapshot(t, db)
	status, _, body := p.request(t, "GET", "/health/ready", "", nil, "")
	if status != 503 {
		t.Fatalf("managed table without emergency association must not be ready: %d %s", status, body)
	}
	if got := baselineDataSnapshot(t, db); got != before {
		t.Fatal("readiness repaired or changed stored state")
	}
	requireSchemaStartupRejected(t, accountProcessCommand(t, buildIntegrationAdmin(t), &reader))
	deliveryExec(t, db, `UPDATE rcc_table_policies SET enabled=0 WHERE id=501`)
	if status, _, body := p.request(t, "GET", "/health/ready", "", nil, ""); status != 200 {
		t.Fatalf("an unmanaged disabled table must not prevent readiness: %d %s", status, body)
	}
}

func TestTableReleaseSchemaDoesNotRestoreDisabledOrDeletedStandardDefaults(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 9), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	for _, mode := range []string{"disabled", "deleted"} {
		t.Run(mode, func(t *testing.T) {
			name := "standard_" + mode
			deliveryExec(t, db, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
			target := owner
			target.DBName = name
			requireSchemaMigrationState(t, previous, &target, "current", "up")
			old := deliveryDB(t, &target)
			deliveryExec(t, old, `INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES('managed_table','query','mutation',1,'admin','admin')`)
			if mode == "disabled" {
				deliveryExec(t, old, `UPDATE rcc_release_templates SET enabled=0,version=2,modifier='choice' WHERE code='default_standard_v1'`)
			} else {
				deliveryExec(t, old, `DELETE FROM rcc_release_templates WHERE code='default_standard_v1'`)
			}
			before := baselineRows(t, old, `SELECT * FROM rcc_release_templates ORDER BY id`)
			requireSchemaMigrationState(t, current, &target, "current", "up")
			if got := baselineRows(t, old, `SELECT * FROM rcc_release_templates ORDER BY id`); got != before {
				t.Fatal("upgrade restored or re-enabled an intentionally unavailable standard default")
			}
			var count, emergency int
			if err := old.QueryRow(`SELECT COUNT(*),SUM(release_type='EMERGENCY' AND enabled=1) FROM rcc_table_release_templates`).Scan(&count, &emergency); err != nil || count != 1 || emergency != 1 {
				t.Fatalf("only emergency association should be initialized: %d/%d %v", count, emergency, err)
			}
		})
	}
}

func TestTableReleaseSchemaRecoveryKeepsExistingSelections(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 9), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `INSERT INTO rcc_table_policies(id,table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES(701,'recovery_table','query','mutation',1,'admin','admin')`)
	deliveryExec(t, db, `CREATE USER 'association_migrator'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT,CREATE,ALTER,INDEX,REFERENCES ON rcc_test.* TO 'association_migrator'@'%'`)
	deliveryExec(t, db, `GRANT INSERT,UPDATE ON rcc_test.rcc_goose_db_version TO 'association_migrator'@'%'`)
	deliveryExec(t, db, `GRANT INSERT,UPDATE ON rcc_test.rcc_schema_migration_attempts TO 'association_migrator'@'%'`)
	limited := *driver
	limited.User = "association_migrator"
	output, err := schemaMigrationCommand(current, &limited, "up").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "migration_failed") {
		t.Fatalf("association seed without INSERT must leave recoverable failure: %v %s", err, output)
	}
	requireSchemaMigrationState(t, current, &limited, "recovery_required", "status")
	if output, err := schemaMigrationCommand(current, &limited, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "recovery_required") {
		t.Fatalf("ordinary retry unexpectedly repeated incomplete migration: %v %s", err, output)
	}
	deliveryExec(t, db, `INSERT INTO rcc_release_templates(code,name,description,release_type,node_list,monitor_list,enabled,creator,modifier) SELECT 'recovered_emergency_v1','Recovered choice',description,release_type,node_list,monitor_list,1,'admin-choice','admin-choice' FROM rcc_release_templates WHERE code='default_emergency_v1'`)
	deliveryExec(t, db, `INSERT INTO rcc_table_release_templates(table_policy_id,release_type,template_id,enabled,version,creator,modifier) SELECT 701,release_type,id,1,7,'admin-choice','admin-choice' FROM rcc_release_templates WHERE code='recovered_emergency_v1'`)
	chosen := baselineRows(t, db, `SELECT * FROM rcc_table_release_templates WHERE release_type='EMERGENCY' ORDER BY id`)
	deliveryExec(t, db, `GRANT INSERT ON rcc_test.rcc_table_release_templates TO 'association_migrator'@'%'`)
	requireSchemaMigrationState(t, current, &limited, "current", "recover")
	if got := baselineRows(t, db, `SELECT * FROM rcc_table_release_templates WHERE release_type='EMERGENCY' ORDER BY id`); got != chosen {
		t.Fatal("recovery overwrote the chosen emergency template, version or audit")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_table_release_templates WHERE table_policy_id=701`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("recovery did not fill only the missing standard association: %d %v", count, err)
	}
}
