//go:build integration

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSchemaReadinessRejectsUnmanagedStartup(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "testdata/pre-goose-8b5cd859.sql")
	p := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	select {
	case <-p.done:
		if p.waitErr == nil || !strings.Contains(p.output.String(), "schema_not_ready") || !strings.Contains(p.output.String(), "schema-migrate") {
			t.Fatalf("unmanaged startup needs migration guidance: %v %s", p.waitErr, p.output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Admin started against an unmanaged database")
	}
}

// The already-running process must observe changes and recovery through the
// existing readiness route, while its database identity cannot write any table.
func TestSchemaReadinessContinuouslyChecksStateAndCompleteStructureReadOnly(t *testing.T) {
	_, driver := startCurrentIntegrationMySQL(t)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	deliveryExec(t, db, `INSERT INTO business_marker VALUES(1,'retained')`)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,created_at) VALUES('readiness-account','readiness.account','readiness@example.test','Retained','fixture-hash',31,1,'2025-01-02')`)
	deliveryExec(t, db, `CREATE USER 'readiness_reader'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT ON rcc_test.* TO 'readiness_reader'@'%'`)
	reader := *driver
	reader.User = "readiness_reader"
	restricted := deliveryDB(t, &reader)
	if _, err := restricted.Exec(`CREATE TABLE readiness_ddl_probe(id int)`); err == nil {
		t.Fatal("business identity unexpectedly has DDL")
	}
	if _, err := restricted.Exec(`UPDATE rcc_goose_db_version SET is_applied=1`); err == nil {
		t.Fatal("business identity can write version ledger")
	}
	binary, migration := buildIntegrationAdmin(t), buildSchemaMigrationCommand(t)
	p := accountProcessCommand(t, binary, &reader)
	p.ready(t)
	snapshot := func() string {
		return baselineDataSnapshot(t, db) + baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`) + baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	}
	assertReadiness := func(t *testing.T, want int) {
		t.Helper()
		before := snapshot()
		status, _, body := p.request(t, "GET", "/health/ready", "", nil, "")
		if status != want {
			t.Fatalf("readiness: want %d got %d %s", want, status, body)
		}
		if got := snapshot(); got != before {
			t.Fatal("readiness wrote control or business data")
		}
	}
	assertReadiness(t, 200)
	for _, fault := range []struct{ name, apply, restore string }{
		{"unmanaged", `RENAME TABLE rcc_goose_db_version TO held_versions, rcc_schema_migration_attempts TO held_attempts`, `RENAME TABLE held_versions TO rcc_goose_db_version, held_attempts TO rcc_schema_migration_attempts`},
		{"ahead", fmt.Sprintf(`UPDATE rcc_goose_db_version SET version_id=99 WHERE version_id=%d`, currentTestSchemaVersion), fmt.Sprintf(`UPDATE rcc_goose_db_version SET version_id=%d WHERE version_id=99`, currentTestSchemaVersion)},
		{"unknown", `UPDATE rcc_goose_db_version SET version_id=77 WHERE version_id=1`, `UPDATE rcc_goose_db_version SET version_id=1 WHERE version_id=77`},
		{"release_digest", fmt.Sprintf(`UPDATE rcc_schema_migration_attempts SET release_digest=REPEAT('0',64) WHERE target_version=%d`, currentTestSchemaVersion), ""},
		{"missing_column", `ALTER TABLE rcc_query_policies RENAME COLUMN description TO missing_description`, `ALTER TABLE rcc_query_policies RENAME COLUMN missing_description TO description`},
		{"wrong_default", `ALTER TABLE rcc_accounts ALTER COLUMN enabled SET DEFAULT 0`, `ALTER TABLE rcc_accounts ALTER COLUMN enabled SET DEFAULT 1`},
		{"missing_index", `ALTER TABLE rcc_query_policies DROP INDEX idx_query_policy_status_type`, `ALTER TABLE rcc_query_policies ADD KEY idx_query_policy_status_type(status,type_code)`},
		{"missing_field_policy", `RENAME TABLE rcc_table_field_policies TO held_field_policies`, `RENAME TABLE held_field_policies TO rcc_table_field_policies`},
		{"missing_release_templates", `RENAME TABLE rcc_release_templates TO held_release_templates`, `RENAME TABLE held_release_templates TO rcc_release_templates`},
		{"release_template_unique_key", `ALTER TABLE rcc_release_templates DROP INDEX uk_release_template_code`, `ALTER TABLE rcc_release_templates DROP INDEX idx_release_template_type_enabled, ADD UNIQUE KEY uk_release_template_code(code), ADD KEY idx_release_template_type_enabled(release_type,enabled)`},
		{"release_template_check", `ALTER TABLE rcc_release_templates ALTER CHECK chk_release_template_monitors NOT ENFORCED`, `ALTER TABLE rcc_release_templates ALTER CHECK chk_release_template_monitors ENFORCED`},
		{"missing_default_emergency_template", `UPDATE rcc_release_templates SET code='held_emergency' WHERE code='default_emergency_v1'`, `UPDATE rcc_release_templates SET code='default_emergency_v1' WHERE code='held_emergency'`},
		{"field_policy_unique_key", `ALTER TABLE rcc_table_field_policies DROP INDEX uk_table_field`, `ALTER TABLE rcc_table_field_policies ADD UNIQUE KEY uk_table_field(table_name,field_name)`},
		{"field_policy_check", `ALTER TABLE rcc_table_field_policies ALTER CHECK chk_field_policy_flags NOT ENFORCED`, `ALTER TABLE rcc_table_field_policies ALTER CHECK chk_field_policy_flags ENFORCED`},
		{"unenforced_check", `ALTER TABLE rcc_query_policies ALTER CHECK chk_query_policy_max_page_size NOT ENFORCED`, `ALTER TABLE rcc_query_policies ALTER CHECK chk_query_policy_max_page_size ENFORCED`},
		{"wrong_engine", `ALTER TABLE rcc_auth_rate_limits ENGINE=MyISAM`, `ALTER TABLE rcc_auth_rate_limits ENGINE=InnoDB`},
		{"empty_roles", `UPDATE rcc_accounts SET roles=0`, `UPDATE rcc_accounts SET roles=31`},
		{"unknown_role", `UPDATE rcc_accounts SET roles=32`, `UPDATE rcc_accounts SET roles=31`},
		{"zero_role_version", `UPDATE rcc_accounts SET role_version=0`, `UPDATE rcc_accounts SET role_version=1`},
		{"journal_engine", `ALTER TABLE rcc_schema_migration_attempts ENGINE=MyISAM`, `ALTER TABLE rcc_schema_migration_attempts ENGINE=InnoDB`},
		{"journal_column", `ALTER TABLE rcc_schema_migration_attempts RENAME COLUMN recovery_count TO missing_recovery_count`, `ALTER TABLE rcc_schema_migration_attempts RENAME COLUMN missing_recovery_count TO recovery_count`},
		{"older_unconfirmed_attempt", `UPDATE rcc_schema_migration_attempts SET state='RUNNING' WHERE id=1`, `UPDATE rcc_schema_migration_attempts SET state='SUCCEEDED' WHERE id=1`},
		{"missing_required_metadata", `DELETE FROM rcc_auth_control_lock WHERE id=1`, `INSERT INTO rcc_auth_control_lock VALUES(1)`},
	} {
		t.Run(fault.name, func(t *testing.T) {
			var digest string
			if fault.name == "release_digest" {
				if err := db.QueryRow(`SELECT release_digest FROM rcc_schema_migration_attempts WHERE target_version=?`, currentTestSchemaVersion).Scan(&digest); err != nil {
					t.Fatal(err)
				}
			}
			var templateTable, templateDefinition string
			if fault.name == "release_template_unique_key" {
				if err := db.QueryRow("SHOW CREATE TABLE rcc_release_templates").Scan(&templateTable, &templateDefinition); err != nil {
					t.Fatal(err)
				}
			}
			deliveryExec(t, db, fault.apply)
			before := baselineDataSnapshot(t, db)
			rejected := accountProcessCommand(t, binary, &reader)
			requireSchemaStartupRejected(t, rejected)
			status, _, body := p.request(t, "GET", "/health/ready", "", nil, "")
			if status != 503 {
				t.Fatalf("live process accepted %s: %d %s", fault.name, status, body)
			}
			if before != baselineDataSnapshot(t, db) {
				t.Fatal("rejected startup/readiness changed rows")
			}
			if fault.restore != "" {
				deliveryExec(t, db, fault.restore)
			} else {
				deliveryExec(t, db, `UPDATE rcc_schema_migration_attempts SET release_digest=? WHERE target_version=?`, digest, currentTestSchemaVersion)
			}
			// MySQL reserializes ASCII-column CHECK literals while rebuilding an
			// index; restore the original UTF-8 expressions as well as the index.
			if fault.name == "release_template_unique_key" || fault.name == "release_template_check" {
				deliveryExec(t, db, `ALTER TABLE rcc_release_templates DROP CHECK chk_emergency_template_enabled, DROP CHECK chk_release_template_type, ADD CONSTRAINT chk_emergency_template_enabled CHECK (release_type <> _utf8mb4'EMERGENCY' OR enabled=1), ADD CONSTRAINT chk_release_template_type CHECK (release_type IN (_utf8mb4'STANDARD',_utf8mb4'EMERGENCY'))`)
			}
			if fault.name == "release_template_unique_key" {
				var restoredDefinition string
				if err := db.QueryRow("SHOW CREATE TABLE rcc_release_templates").Scan(&templateTable, &restoredDefinition); err != nil {
					t.Fatal(err)
				}
				if restoredDefinition != templateDefinition {
					t.Fatalf("fault restoration changed template structure:\nbefore:\n%s\nafter:\n%s", templateDefinition, restoredDefinition)
				}
			}
			assertReadiness(t, 200)
		})
	}
	deliveryExec(t, db, `UPDATE rcc_schema_migration_attempts SET state='RUNNING',finished_at=NULL WHERE target_version=?`, currentTestSchemaVersion)
	assertReadiness(t, 503)
	requireSchemaStartupRejected(t, accountProcessCommand(t, binary, &reader))
	if output, err := schemaMigrationCommand(migration, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("deployment retried unconfirmed migration: %s", output)
	}
	requireSchemaMigrationState(t, migration, driver, "current", "recover")
	assertReadiness(t, 200)
	p.stop(t)
}

func requireSchemaStartupRejected(t *testing.T, p *accountProcess) {
	t.Helper()
	select {
	case <-p.done:
		if p.waitErr == nil || !strings.Contains(p.output.String(), "schema_not_ready") || !strings.Contains(p.output.String(), "schema-migrate") {
			t.Fatalf("startup must reject with maintenance guidance: %v %s", p.waitErr, p.output.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Admin served an incompatible migration state")
	}
}

func TestSchemaReadinessRejectsKnownOldRelease(t *testing.T) {
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, buildPreviousSchemaMigrationRelease(t), driver, "current", "up")
	binary := buildIntegrationAdmin(t)
	requireSchemaStartupRejected(t, accountProcessCommand(t, binary, driver))
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	p := accountProcessCommand(t, binary, driver)
	p.ready(t)

	// Restore the known old test state while this process is still running.
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `DROP TABLE rcc_table_field_policies,rcc_release_details,rcc_release_executions,rcc_release_table_references,rcc_release_templates`)
	deliveryExec(t, db, `ALTER TABLE rcc_table_policies DROP COLUMN concurrency_key`)
	deliveryExec(t, db, "DROP TABLE rcc_publication_commands")
	deliveryExec(t, db, "CREATE TABLE `rcc_publication_commands` (\n  `table_name` varbinary(256) NOT NULL,\n  `sequence` bigint unsigned NOT NULL,\n  `order_id` varbinary(32) NOT NULL,\n  `document` json NOT NULL,\n  PRIMARY KEY (`table_name`,`sequence`),\n  KEY `publication_order` (`order_id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci")
	deliveryExec(t, db, "DROP TABLE rcc_refresh_notifications")
	deliveryExec(t, db, "CREATE TABLE `rcc_refresh_notifications` (\n  `order_id` varbinary(32) NOT NULL,\n  `table_name` varbinary(256) NOT NULL,\n  `table_version` bigint unsigned NOT NULL,\n  `document` json NOT NULL,\n  PRIMARY KEY (`order_id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci")
	deliveryExec(t, db, "DROP TABLE rcc_release_orders")
	deliveryExec(t, db, "CREATE TABLE `rcc_release_orders` (\n  `id` varbinary(32) NOT NULL,\n  `table_name` varbinary(256) NOT NULL,\n  `applicant_id` varbinary(36) NOT NULL,\n  `state` varchar(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,\n  `version` bigint unsigned NOT NULL,\n  `document` json NOT NULL,\n  PRIMARY KEY (`id`),\n  KEY `release_table` (`table_name`,`id`),\n  KEY `release_applicant` (`applicant_id`,`id`),\n  KEY `release_state` (`state`,`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci")

	for _, table := range []string{"rcc_query_policies", "rcc_mutation_policies", "rcc_table_policies"} {
		deliveryExec(t, db, "ALTER TABLE "+table+" RENAME COLUMN created_at TO gmt_created, RENAME COLUMN updated_at TO gmt_modified")
	}
	deliveryExec(t, db, `DELETE FROM rcc_goose_db_version WHERE version_id>1`)
	deliveryExec(t, db, `DELETE FROM rcc_schema_migration_attempts WHERE target_version>1`)
	status, _, body := p.request(t, "GET", "/health/ready", "", nil, "")
	if status != 503 {
		t.Fatalf("running process accepted old release: %d %s", status, body)
	}
	requireSchemaMigrationState(t, buildSchemaMigrationCommand(t), driver, "current", "up")
	p.ready(t)
	p.stop(t)
}
