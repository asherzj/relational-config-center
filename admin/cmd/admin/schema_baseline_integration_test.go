//go:build integration

package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestSchemaBaselineAdoptsCurrentDatabaseWithoutReplayingHistory(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('retained-account','retained','retained@example.test','Retained','opaque-hash',31,4,7,'2025-01-02')`)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	deliveryExec(t, db, `INSERT INTO business_marker VALUES(17,'preserved business data')`)
	deliveryExec(t, db, `INSERT INTO rcc_record_versions VALUES('business_marker','',73)`)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := registerAccount(t, app, "baseline.reviewer", "baseline.reviewer@example.test", "correct horse battery staple")
	grantReleaseRole(t, app, reviewer, `["APPROVER","PUBLISHER"]`, "1", "baseline-roles")
	path := approvePublication(t, app, reviewer, `{"title":"Retained publication","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"published","label":"","metadata":"null"}}]}`, "baseline-publication")
	published := releaseActorRequest(t, app, reviewer, "POST", path+"/execute", `{"expected_version":"3"}`, "baseline-execute")
	if published.Code != 200 {
		t.Fatalf("seed publication: %d %s", published.Code, published.Body)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	dataBefore := baselineDataSnapshot(t, db)
	account := baselineRows(t, db, `SELECT * FROM rcc_accounts ORDER BY id`)
	requireSchemaMigrationState(t, binary, driver, "unmanaged", "status")
	requireSchemaMigrationState(t, binary, driver, "current", "baseline")
	history := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	attempts := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	requireSchemaMigrationState(t, binary, driver, "current", "baseline")
	requireSchemaMigrationState(t, binary, driver, "current", "up")
	if got := baselineDataSnapshot(t, db); got != dataBefore {
		t.Fatal("baseline changed control/business data, sessions, policies, record versions or publication history")
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_accounts ORDER BY id`); got != account {
		t.Fatal("baseline changed account, administrator grant or control versions")
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`); got != history {
		t.Fatal("repeated baseline changed migration history")
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`); got != attempts {
		t.Fatal("repeated baseline created another attempt")
	}
	var retained int
	if err := db.QueryRow(`SELECT COUNT(*) FROM business_marker WHERE id=17 AND note='preserved business data'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("business data changed: %d %v", retained, err)
	}
	app, err = newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if session := accountRequest(app, "GET", "/api/v1/auth/session", "", reviewer.Result().Cookies(), ""); session.Code != 200 {
		t.Fatalf("existing session unavailable: %d %s", session.Code, session.Body)
	}
	read := releaseActorRequest(t, app, reviewer, "GET", path, "", "")
	if read.Code != 200 || read.Body.String() != published.Body.String() {
		t.Fatalf("publication changed after adoption: %d %s", read.Code, read.Body)
	}
}

func baselineRows(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result []string
	for rows.Next() {
		values := make([]sql.RawBytes, len(cols))
		targets := make([]any, len(cols))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			t.Fatal(err)
		}
		result = append(result, fmt.Sprintf("%#v", values))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(result)
	return strings.Join(result, "\n")
}

func baselineDataSnapshot(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND table_type='BASE TABLE' AND table_name NOT IN ('rcc_schema_migration_attempts','rcc_goose_db_version') ORDER BY table_name`)
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
	var snapshot strings.Builder
	for _, table := range tables {
		fmt.Fprintf(&snapshot, "%s:%s\n", table, baselineRows(t, db, "SELECT * FROM `"+strings.ReplaceAll(table, "`", "``")+"`"))
	}
	return snapshot.String()
}

func TestSchemaBaselineRejectsIncompatibleControlStructureWithoutWrites(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('reject-account','rejected.account','rejected@example.test','Retained','opaque-hash',31,4,7,'2025-01-02')`)
	deliveryExec(t, db, `INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,creator,modifier,created_at,updated_at) VALUES('retained_v1','Retained','page_query','id','ASC',20,100,'owner','editor','2025-01-02','2025-03-04')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_orders VALUES('retained-order','business_marker','reject-account','SUCCEEDED',7,'{"retained":true}')`)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	deliveryExec(t, db, `INSERT INTO business_marker VALUES(17,'preserved')`)
	for _, test := range []struct{ name, setup, restore string }{
		{"missing_table", `RENAME TABLE rcc_refresh_notifications TO held_notifications`, `RENAME TABLE held_notifications TO rcc_refresh_notifications`},
		{"missing_column", `ALTER TABLE rcc_query_policies DROP COLUMN description`, `ALTER TABLE rcc_query_policies ADD COLUMN description varchar(500) NOT NULL DEFAULT '' AFTER name`},
		{"extra_column", `ALTER TABLE rcc_accounts ADD COLUMN required_extra int NOT NULL`, `ALTER TABLE rcc_accounts DROP COLUMN required_extra`},
		{"type", `ALTER TABLE rcc_query_policies MODIFY name varchar(99) NOT NULL`, `ALTER TABLE rcc_query_policies MODIFY name varchar(100) NOT NULL`},
		{"default", `ALTER TABLE rcc_accounts ALTER COLUMN enabled SET DEFAULT 0`, `ALTER TABLE rcc_accounts ALTER COLUMN enabled SET DEFAULT 1`},
		{"default_containing_table_option_text", `ALTER TABLE rcc_query_policies ALTER COLUMN description SET DEFAULT ' AUTO_INCREMENT=123'`, `ALTER TABLE rcc_query_policies ALTER COLUMN description SET DEFAULT ''`},
		{"index", `ALTER TABLE rcc_query_policies DROP INDEX idx_query_policy_status_type`, `ALTER TABLE rcc_query_policies ADD KEY idx_query_policy_status_type(status,type_code)`},
		{"check", `ALTER TABLE rcc_query_policies ALTER CHECK chk_query_policy_max_page_size NOT ENFORCED`, `ALTER TABLE rcc_query_policies ALTER CHECK chk_query_policy_max_page_size ENFORCED`},
		{"foreign_key", `ALTER TABLE rcc_login_sessions DROP FOREIGN KEY fk_rcc_sessions_account`, `ALTER TABLE rcc_login_sessions ADD CONSTRAINT fk_rcc_sessions_account FOREIGN KEY(account_id) REFERENCES rcc_accounts(id)`},
		{"engine", `ALTER TABLE rcc_auth_rate_limits ENGINE=MyISAM`, `ALTER TABLE rcc_auth_rate_limits ENGINE=InnoDB`},
		{"required_metadata", `DELETE FROM rcc_auth_control_lock WHERE id=1`, `INSERT INTO rcc_auth_control_lock VALUES(1)`},
	} {
		t.Run(test.name, func(t *testing.T) {
			deliveryExec(t, db, test.setup)
			t.Cleanup(func() { deliveryExec(t, db, test.restore) })
			before := baselineDataSnapshot(t, db)
			output, err := schemaMigrationCommand(binary, driver, "baseline").CombinedOutput()
			if err == nil || !strings.Contains(string(output), "schema_mismatch") || !strings.Contains(string(output), "deploy/mysql/migrations/README.md") {
				t.Fatalf("incompatible baseline accepted or missing guidance: %v %s", err, output)
			}
			requireSchemaMigrationState(t, binary, driver, "unmanaged", "status")
			var ledgers int
			if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('rcc_goose_db_version','rcc_schema_migration_attempts')`).Scan(&ledgers); err != nil || ledgers != 0 {
				t.Fatalf("rejected adoption wrote metadata: %d %v", ledgers, err)
			}
			if after := baselineDataSnapshot(t, db); after != before {
				t.Fatal("rejected adoption changed existing control or business data")
			}
		})
	}
	requireSchemaMigrationState(t, binary, driver, "current", "baseline")
}

func TestSchemaBaselineRecoversUnconfirmedRegistration(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('recover-account','recover','recover@example.test','Retained','opaque-hash',31,4,7,'2025-01-02')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_orders VALUES('retained-order','business_marker','recover-account','SUCCEEDED',7,'{"retained":true}')`)
	deliveryExec(t, db, `INSERT INTO rcc_record_versions VALUES('business_marker','',73)`)
	before := baselineDataSnapshot(t, db)
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE,INSERT,UPDATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(binary, driver, "baseline").CombinedOutput(); err == nil {
		t.Fatalf("registration without ledger insert succeeded: %s", output)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	for _, operation := range []string{"up", "baseline"} {
		if output, err := schemaMigrationCommand(binary, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s retried unconfirmed adoption: %s", operation, output)
		}
	}
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `CREATE TRIGGER block_baseline_confirmation BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.state='BASELINED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='private_confirmation_detail'; END IF; END`)
	if output, err := schemaMigrationCommand(binary, driver, "recover").CombinedOutput(); err == nil || strings.Contains(string(output), "private_confirmation_detail") {
		t.Fatalf("confirmation failure must remain unconfirmed and safe: %v %s", err, output)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("adopted version prefix: %d %v", count, err)
	}
	deliveryExec(t, db, `DROP TRIGGER block_baseline_confirmation`)
	deliveryExec(t, db, `ALTER TABLE rcc_accounts MODIFY display_name varchar(63) NOT NULL`)
	if output, err := schemaMigrationCommand(binary, driver, "recover").CombinedOutput(); err == nil || !strings.Contains(string(output), "schema_mismatch") {
		t.Fatalf("recovery accepted damaged current structure: %v %s", err, output)
	}
	deliveryExec(t, db, `ALTER TABLE rcc_accounts MODIFY display_name varchar(64) NOT NULL`)
	deliveryExec(t, db, `CREATE TRIGGER block_baseline_recovery BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.recovery_count>OLD.recovery_count THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='private_recovery_detail'; END IF; END`)
	if output, err := schemaMigrationCommand(binary, driver, "recover").CombinedOutput(); err == nil || strings.Contains(string(output), "private_recovery_detail") {
		t.Fatalf("recovery record failure must not confirm adoption: %v %s", err, output)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	deliveryExec(t, db, `DROP TRIGGER block_baseline_recovery`)
	altered := buildSchemaMigrationVariant(t, func(directory string) {
		file := filepath.Join(directory, "00002_policy_audit_timestamps.sql")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, append(data, []byte("\n-- altered release\n")...), 0644); err != nil {
			t.Fatal(err)
		}
	})
	if output, err := schemaMigrationCommand(altered, driver, "recover").CombinedOutput(); err == nil || !strings.Contains(string(output), "recovery_release_mismatch") {
		t.Fatalf("changed release accepted baseline recovery: %v %s", err, output)
	}
	next := buildNextSchemaMigrationRelease(t)
	requireSchemaMigrationState(t, next, driver, "pending", "recover")
	if got := baselineDataSnapshot(t, db); got != before {
		t.Fatal("baseline recovery changed account, release history or record-version floor")
	}
	history := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	requireSchemaMigrationState(t, next, driver, "pending", "baseline")
	if got := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`); got != history {
		t.Fatal("repeat baseline changed prior release versions")
	}
	requireSchemaMigrationState(t, next, driver, "current", "up")
}

func TestSchemaBaselineReleaseUpgradesT1AndRecoversPartialAuditRename(t *testing.T) {
	previous, current := buildPreviousSchemaMigrationRelease(t), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,creator,modifier,gmt_created,gmt_modified) VALUES('retained_v1','Retained','page_query','id','ASC',20,100,'owner','editor','2025-01-02','2025-03-04')`)
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,INSERT,UPDATE,CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT ALTER ON rcc_test.rcc_query_policies TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(current, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("partial rename should fail: %s", output)
	}
	var renamed int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_query_policies' AND column_name='created_at'`).Scan(&renamed); err != nil || renamed != 1 {
		t.Fatalf("first rename not committed: %d %v", renamed, err)
	}
	requireSchemaMigrationState(t, current, driver, "recovery_required", "status")
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, current, driver, "current", "recover")
	var retained int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_query_policies WHERE code='retained_v1' AND creator='owner' AND modifier='editor' AND created_at='2025-01-02' AND updated_at='2025-03-04'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("rename changed audit data: %d %v", retained, err)
	}
	_, freshDriver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	fresh := deliveryDB(t, freshDriver)
	assertBaselinePhysicalSchemaEqual(t, db, fresh)
}

func TestSchemaBaselineRecoversBeforeAttemptWasRecorded(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(binary, driver, "baseline").CombinedOutput(); err == nil {
		t.Fatalf("attempt without insert privilege succeeded: %s", output)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	requireSchemaMigrationState(t, binary, driver, "current", "recover")
}

func assertBaselinePhysicalSchemaEqual(t *testing.T, left, right *sql.DB) {
	t.Helper()
	rows, err := right.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND LEFT(table_name,4)='rcc_' AND table_name NOT IN ('rcc_schema_migration_attempts','rcc_goose_db_version') ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, table, a, b string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if err := left.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &a); err != nil {
			t.Fatal(err)
		}
		if err := right.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &b); err != nil {
			t.Fatal(err)
		}
		// Physical AUTO_INCREMENT counters reflect retained rows, not structure.
		a = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`).ReplaceAllString(a, "")
		b = regexp.MustCompile(` AUTO_INCREMENT=[0-9]+`).ReplaceAllString(b, "")
		if a != b {
			t.Fatalf("physical schema differs for %s:\n%s\n%s", name, a, b)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaBaselineConcurrentAdoptionUsesOneVersionPrefix(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)),0)`).Scan(&locked); err != nil || locked != 1 {
		t.Fatal(err)
	}
	if output, err := schemaMigrationCommand(binary, driver, "baseline", "--lock-timeout=100ms").CombinedOutput(); err == nil || !strings.Contains(string(output), "migration_busy") {
		t.Fatalf("adoption bypassed migration lock: %v %s", err, output)
	}
	requireSchemaMigrationState(t, binary, driver, "unmanaged", "status")
	if _, err := conn.ExecContext(ctx, `DO RELEASE_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)))`); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan string, 2)
	for range 2 {
		go func() {
			<-start
			output, err := schemaMigrationCommand(binary, driver, "baseline").CombinedOutput()
			if err != nil {
				results <- fmt.Sprintf("%v %s", err, output)
			} else {
				results <- ""
			}
		}()
	}
	close(start)
	for range 2 {
		if failure := <-results; failure != "" {
			t.Fatal(failure)
		}
	}
	var versions, attempts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version`).Scan(&versions); err != nil || versions != 3 {
		t.Fatalf("concurrent adoption duplicated prefix: %d %v", versions, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_schema_migration_attempts`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("concurrent adoption duplicated journal: %d %v", attempts, err)
	}
}

func TestSchemaBaselineProcessInterruptionRetainsUnconfirmedState(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	before := baselineDataSnapshot(t, db)
	deliveryExec(t, db, `REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT SELECT,CREATE ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `GRANT CREATE,INSERT,UPDATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'`)
	if output, err := schemaMigrationCommand(binary, driver, "baseline").CombinedOutput(); err == nil {
		t.Fatalf("expected ledger bootstrap interruption: %s", output)
	}
	deliveryExec(t, db, `GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'`)
	deliveryExec(t, db, `CREATE TRIGGER pause_baseline_registration BEFORE INSERT ON rcc_goose_db_version FOR EACH ROW BEGIN IF NEW.version_id=1 THEN DO SLEEP(3); END IF; END`)
	command := schemaMigrationCommand(binary, driver, "recover")
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		var sleeping int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.processlist WHERE DB=DATABASE() AND STATE='User sleep'`).Scan(&sleeping); err != nil {
			t.Fatal(err)
		}
		if sleeping > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("baseline did not reach version registration boundary")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, operation := range []string{"up", "baseline", "recover"} {
		if output, err := schemaMigrationCommand(binary, driver, operation, "--lock-timeout=100ms").CombinedOutput(); err == nil || !strings.Contains(string(output), "migration_busy") {
			t.Fatalf("%s interfered with active baseline: %v %s", operation, err, output)
		}
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatalf("interrupted baseline reported success: %s", output.String())
	}
	deliveryExec(t, db, `DROP TRIGGER pause_baseline_registration`)
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	for _, operation := range []string{"up", "baseline"} {
		if output, err := schemaMigrationCommand(binary, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s retried interrupted baseline: %s", operation, output)
		}
	}
	requireSchemaMigrationState(t, binary, driver, "current", "recover")
	if got := baselineDataSnapshot(t, db); got != before {
		t.Fatal("interrupted baseline and recovery changed existing data")
	}
}

func TestSchemaBaselineUsesTransactionalVersionLedger(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `SET GLOBAL default_storage_engine='MyISAM'`)
	requireSchemaMigrationState(t, binary, driver, "current", "baseline")
	var engine string
	if err := db.QueryRow(`SELECT engine FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_goose_db_version'`).Scan(&engine); err != nil || engine != "InnoDB" {
		t.Fatalf("baseline registration requires a transactional ledger: %s %v", engine, err)
	}
	deliveryExec(t, db, `ALTER TABLE rcc_goose_db_version ENGINE=MyISAM`)
	requireSchemaMigrationState(t, binary, driver, "incompatible", "status")
	for _, operation := range []string{"up", "baseline", "recover"} {
		if output, err := schemaMigrationCommand(binary, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s accepted a nontransactional ledger: %s", operation, output)
		}
	}
}

func TestSchemaBaselineRefusesToRecreateLostConfirmedHistory(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	requireSchemaMigrationState(t, binary, driver, "current", "baseline")
	before := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	deliveryExec(t, db, `DROP TABLE rcc_goose_db_version`)
	requireSchemaMigrationState(t, binary, driver, "incompatible", "status")
	for _, operation := range []string{"up", "baseline", "recover"} {
		if output, err := schemaMigrationCommand(binary, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s recreated confirmed history after ledger loss: %s", operation, output)
		}
	}
	if got := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`); got != before {
		t.Fatal("lost confirmed history changed its original receipt")
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_goose_db_version'`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("lost confirmed history was recreated: %d %v", tables, err)
	}
}
