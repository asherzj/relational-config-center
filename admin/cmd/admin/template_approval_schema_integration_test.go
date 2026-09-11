//go:build integration

package main

import "testing"

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
