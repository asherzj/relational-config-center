//go:build integration

package main

import (
	"strings"
	"testing"
)

func TestTemplateApprovalSchemaPreservesPublishedRolesAndChecksCompleteReadiness(t *testing.T) {
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, buildSchemaMigrationReleaseAt(t, 7), driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	for _, statement := range []string{
		`CREATE TABLE integration_business_marker(id INT PRIMARY KEY,note TEXT)`,
		`INSERT INTO integration_business_marker VALUES(1,'preserve published role facts')`,
		`INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('00000000-0000-0000-0000-000000000071','integration.owner','integration@example.test','Retained owner','opaque-password-hash',31,3,'2026-01-01')`,
		`INSERT INTO rcc_approval_roles(id,name,description,enabled,version,creator,modifier,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000072','Retained reviewers','Preserved description',1,4,'00000000-0000-0000-0000-000000000071','00000000-0000-0000-0000-000000000071','2026-01-01','2026-01-02')`,
		`INSERT INTO rcc_approval_role_members(role_id,account_id) VALUES('00000000-0000-0000-0000-000000000072','00000000-0000-0000-0000-000000000071')`,
		`INSERT INTO rcc_approval_role_requests(actor_id,request_key,request_digest,result,created_at) VALUES('00000000-0000-0000-0000-000000000071','retained-role-request',REPEAT('a',64),JSON_OBJECT('retained',true),'2026-01-02')`,
		`INSERT INTO rcc_approval_role_references(role_id,reference_kind,reference_key,role_name,created_at) VALUES('00000000-0000-0000-0000-000000000072','table','integration_business_marker','Original reviewers','2026-01-02')`,
		`INSERT INTO rcc_table_approval_assignments(table_name,version,role_ids) VALUES('integration_business_marker',7,JSON_ARRAY('00000000-0000-0000-0000-000000000072'))`,
		`INSERT INTO rcc_table_approval_requests(actor_id,request_key,digest,result) VALUES('00000000-0000-0000-0000-000000000071','retained-table-request',UNHEX(REPEAT('b',64)),JSON_OBJECT('retained',true))`,
		`INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES('integration_business_marker','query','mutation',1,'original-admin','original-admin')`,
	} {
		deliveryExec(t, db, statement)
	}
	beforeCutover := cutoverPreservedSnapshot(t, db)
	requireSchemaMigrationState(t, buildSchemaMigrationReleaseAt(t, 9), driver, "current", "up")
	if strings.Replace(cutoverPreservedSnapshot(t, db), "rcc_approval_notifications:\n", "", 1) != beforeCutover {
		t.Fatal("published cutover changed retained role, membership, assignment, request or business facts")
	}
	var roles, roleVersion int
	if err := db.QueryRow(`SELECT roles,role_version FROM rcc_accounts WHERE username='integration.owner'`).Scan(&roles, &roleVersion); err != nil || roles != 27 || roleVersion != 4 {
		t.Fatalf("published role cutover was not preserved: %d/%d %v", roles, roleVersion, err)
	}
	before := preTemplateDataSnapshot(t, db)
	current := buildSchemaMigrationCommand(t)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if after := preTemplateDataSnapshot(t, db); after != before {
		t.Fatal("template upgrade changed published role, membership, assignment, request, audit or business facts")
	}
	fresh := createSchemaComparisonDatabase(t, &owner)
	requireSchemaMigrationState(t, current, fresh, "current", "up")
	assertBaselinePhysicalSchemaEqual(t, db, deliveryDB(t, fresh))

	deliveryExec(t, db, `CREATE USER 'integration_reader'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT ON rcc_test.* TO 'integration_reader'@'%'`)
	reader := *driver
	reader.User = "integration_reader"
	binary := buildIntegrationAdmin(t)
	process := accountProcessCommand(t, binary, &reader)
	process.ready(t)
	for _, table := range []string{"rcc_approval_roles", "rcc_approval_role_members", "rcc_approval_role_requests", "rcc_approval_role_references", "rcc_table_approval_assignments", "rcc_table_approval_requests"} {
		t.Run(table, func(t *testing.T) {
			deliveryExec(t, db, "RENAME TABLE "+table+" TO held_integration_control")
			defer deliveryExec(t, db, "RENAME TABLE held_integration_control TO "+table)
			snapshot := baselineDataSnapshot(t, db)
			if status, _, body := process.request(t, "GET", "/health/ready", "", nil, ""); status != 503 {
				t.Fatalf("current cumulative manifest omitted published %s: %d %s", table, status, body)
			}
			requireSchemaStartupRejected(t, accountProcessCommand(t, binary, &reader))
			if baselineDataSnapshot(t, db) != snapshot {
				t.Fatal("read-only readiness or rejected startup changed data")
			}
		})
		if status, _, body := process.request(t, "GET", "/health/ready", "", nil, ""); status != 200 {
			t.Fatalf("restored published control structure remains unready: %d %s", status, body)
		}
	}
	process.stop(t)
}

// Formal v9 is the deployed prefix. Fail the candidate catalog after its CREATE
// commits, then recover only v10 before explicitly applying the association v11.
func TestTemplateApprovalSchemaRecoversCatalogSeedFromFormalNine(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 9), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE catalog_business_marker(id INT PRIMARY KEY,note TEXT)`)
	deliveryExec(t, db, `INSERT INTO catalog_business_marker VALUES(1,'formal nine business fact')`)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('catalog-owner','catalog.owner','catalog@example.test','Retained owner','opaque',17,4,'2026-01-01')`)
	deliveryExec(t, db, `INSERT INTO rcc_table_policies(table_name,query_policy_code,mutation_policy_code,enabled,creator,modifier) VALUES('catalog_business_marker','query','mutation',1,'original-admin','original-admin')`)
	deliveryExec(t, db, `INSERT INTO rcc_approval_notifications(account_id,order_id,sequence,pending,pending_sequence,result_sequence,read_sequence) VALUES('catalog-owner','retained-notification',11,1,9,11,7)`)
	before := preTemplateDataSnapshot(t, db)
	ledger := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	deliveryExec(t, db, `CREATE USER 'catalog_migrator'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT,CREATE,ALTER,INDEX,REFERENCES ON rcc_test.* TO 'catalog_migrator'@'%'`)
	deliveryExec(t, db, `GRANT INSERT,UPDATE ON rcc_test.rcc_goose_db_version TO 'catalog_migrator'@'%'`)
	deliveryExec(t, db, `GRANT INSERT,UPDATE ON rcc_test.rcc_schema_migration_attempts TO 'catalog_migrator'@'%'`)
	limited := *driver
	limited.User = "catalog_migrator"
	if output, err := schemaMigrationCommand(current, &limited, "up").CombinedOutput(); err == nil || !strings.Contains(string(output), "migration_failed") {
		t.Fatalf("catalog seed without INSERT must fail after DDL: %v %s", err, output)
	}
	var templates, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_release_templates`).Scan(&templates); err != nil || templates != 0 {
		t.Fatalf("catalog CREATE must commit before failed seed: %d %v", templates, err)
	}
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 9 {
		t.Fatalf("failed catalog advanced formal version: %d %v", version, err)
	}
	if preTemplateDataSnapshot(t, db) != before || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`) != ledger {
		t.Fatal("partial catalog migration changed formal account, policy, notification or business facts")
	}
	requireSchemaMigrationState(t, current, &limited, "recovery_required", "status")
	if _, err := schemaMigrationCommand(current, &limited, "up").CombinedOutput(); err == nil {
		t.Fatal("ordinary up bypassed explicit catalog recovery")
	}
	deliveryExec(t, db, `GRANT INSERT ON rcc_test.rcc_release_templates TO 'catalog_migrator'@'%'`)
	requireSchemaMigrationState(t, current, &limited, "pending", "recover")
	if err := db.QueryRow(`SELECT MAX(version_id) FROM rcc_goose_db_version`).Scan(&version); err != nil || version != 10 {
		t.Fatalf("recovery must confirm only catalog v10: %d %v", version, err)
	}
	if preTemplateDataSnapshot(t, db) != before {
		t.Fatal("catalog recovery changed formal account, policy, notification or business facts")
	}
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if preTemplateDataSnapshot(t, db) != before || baselineRows(t, db, `SELECT * FROM rcc_goose_db_version WHERE version_id<=9 ORDER BY id`) != ledger {
		t.Fatal("association increment changed formal facts or published version prefix")
	}
	fresh := createSchemaComparisonDatabase(t, &owner)
	requireSchemaMigrationState(t, current, fresh, "current", "up")
	assertBaselinePhysicalSchemaEqual(t, db, deliveryDB(t, fresh))
	final := baselineDataSnapshot(t, db)
	requireSchemaMigrationState(t, current, driver, "current", "up")
	if baselineDataSnapshot(t, db) != final {
		t.Fatal("repeated current upgrade changed recovered catalog or association choices")
	}
}
