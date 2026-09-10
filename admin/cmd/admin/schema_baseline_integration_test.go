//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// The released historical 014–017 adoption contract is fixed at versions 1–5.
const historicalTestSchemaVersion int64 = 5
const historicalTestMigrationCount int64 = 5

// MySQL image init scripts use the client's default charset. Apply historical
// 014 through an explicit UTF-8 connection so its Chinese comments stay exact.
func startHistoricalBaselineMySQL(t *testing.T, scripts ...string) (context.Context, *mysqldriver.Config) {
	t.Helper()
	ctx, driver := startIntegrationMySQL(t, append([]string{"testdata/pre-goose-8b5cd859.sql"}, scripts...)...)
	fixture := *driver
	fixture.Params = map[string]string{"charset": "utf8mb4"}
	fixture.MultiStatements = true
	db := deliveryDB(t, &fixture)
	applyUnmanagedCurrentMigrations(t, db)
	return ctx, driver
}

// Replay the documented, unpublished manual stages only for unmanaged installs.
// Published 001–014 and the frozen historical input remain immutable.
func applyUnmanagedCurrentMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, name := range []string{"014-table-field-policies.sql", "015-original-order-executions.sql", "016-draft-target-reservations.sql", "017-release-main-order.sql"} {
		source, err := os.ReadFile("../../../deploy/mysql/migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		deliveryExec(t, db, string(source))
	}
}

func TestSchemaBaselineAdoptsCurrentDatabaseWithoutReplayingHistory(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	ctx, driver := startIntegrationMySQL(t, "testdata/pre-goose-8b5cd859.sql", "testdata/006-mutation-fixture.sql")
	fixture := *driver
	fixture.MultiStatements = true
	fixture.Params = map[string]string{"charset": "utf8mb4"}
	db := deliveryDB(t, &fixture)
	deliveryExec(t, db, `CREATE TABLE business_marker(id int PRIMARY KEY,note text)`)
	cookies, published := loadHistoricalBaselineData(t, db)
	legacyDocuments := baselineRows(t, db, `SELECT id,document FROM rcc_release_orders ORDER BY id`)
	applyUnmanagedCurrentMigrations(t, db)
	if baselineRows(t, db, `SELECT id,document FROM rcc_release_orders ORDER BY id`) != legacyDocuments {
		t.Fatal("manual structural upgrade rewrote old business release documents")
	}
	deliveryExec(t, db, `INSERT INTO rcc_table_field_policies(table_name,field_name,display_name,creator,modifier) VALUES('mutation_add_items','label','保留字段名称','historical-owner','historical-owner')`)
	var publication struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(published, &publication); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/release-orders/" + publication.ID
	dataBefore := baselineDataSnapshot(t, db)
	account := baselineRows(t, db, `SELECT * FROM rcc_accounts ORDER BY id`)
	requireSchemaMigrationState(t, binary, driver, "unmanaged", "status")
	requireSchemaMigrationState(t, binary, driver, "pending", "baseline")
	history := baselineRows(t, db, `SELECT * FROM rcc_goose_db_version ORDER BY id`)
	attempts := baselineRows(t, db, `SELECT * FROM rcc_schema_migration_attempts ORDER BY id`)
	requireSchemaMigrationState(t, binary, driver, "pending", "baseline")
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
	var versions string
	if err := db.QueryRow(`SELECT GROUP_CONCAT(version_id ORDER BY id) FROM rcc_goose_db_version`).Scan(&versions); err != nil || versions != "0,1,2,3,4,5" {
		t.Fatalf("historical baseline must register only the released prefix: %s %v", versions, err)
	}
	var templateTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_release_templates'`).Scan(&templateTables); err != nil || templateTables != 0 {
		t.Fatalf("baseline executed a later migration: %d %v", templateTables, err)
	}
	requireSchemaMigrationState(t, binary, driver, "pending", "status")
	requireSchemaMigrationState(t, binary, driver, "current", "up")
	if got := baselineDataSnapshot(t, db, "rcc_release_templates"); got != dataBefore {
		t.Fatal("explicit upgrade changed historical control or business data")
	}
	if err := db.QueryRow(`SELECT GROUP_CONCAT(version_id ORDER BY id) FROM rcc_goose_db_version`).Scan(&versions); err != nil || versions != "0,1,2,3,4,5,8" {
		t.Fatalf("explicit upgrade must append candidate migration 8 once: %s %v", versions, err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if session := accountRequest(app, "GET", "/api/v1/auth/session", "", cookies, ""); session.Code != 200 {
		t.Fatalf("existing session unavailable: %d %s", session.Code, session.Body)
	}
	// Old release business documents remain untouched and unsupported by the
	// new main-order interface; account/session adoption is independently valid.
	read := accountRequest(app, "GET", path, "", cookies, "")
	assertIntegrationErrorCode(t, read, 503, "release_unavailable")
	if baselineRows(t, db, `SELECT id,document FROM rcc_release_orders ORDER BY id`) != legacyDocuments {
		t.Fatal("read or baseline changed old release business data")
	}

}

// Frozen output of the pre-Goose T2 public account/approval/publication workflow.
// Restoring old data must not require starting current Admin before adoption.
func loadHistoricalBaselineData(t *testing.T, db *sql.DB) ([]*http.Cookie, json.RawMessage) {
	t.Helper()
	var saved struct {
		Tables []struct {
			Name    string
			Columns []string
			Rows    [][]*string
		}
		Cookies   []*http.Cookie
		Published json.RawMessage
	}
	data, err := os.ReadFile("testdata/pre-goose-8b5cd859-data.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(t.Context(), "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(t.Context(), "SET FOREIGN_KEY_CHECKS=1")
	for _, table := range saved.Tables {
		if _, err := conn.ExecContext(t.Context(), "DELETE FROM `"+table.Name+"`"); err != nil {
			t.Fatal(err)
		}
		for _, row := range table.Rows {
			values := make([]any, len(row))
			for i, value := range row {
				if value != nil {
					decoded, err := base64.StdEncoding.DecodeString(*value)
					if err != nil {
						t.Fatal(err)
					}
					values[i] = string(decoded)
				}
			}
			query := "INSERT INTO `" + table.Name + "` (`" + strings.Join(table.Columns, "`,`") + "`) VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + ")"
			if _, err := conn.ExecContext(t.Context(), query, values...); err != nil {
				t.Fatalf("restore frozen %s: %v", table.Name, err)
			}
		}
	}
	// Anchor both session expiry and its idle window before preservation snapshots.
	if _, err := conn.ExecContext(t.Context(), "UPDATE rcc_login_sessions SET expires_at=UTC_TIMESTAMP(6)+INTERVAL 8 HOUR,last_active_at=UTC_TIMESTAMP(6)"); err != nil {
		t.Fatal(err)
	}
	return saved.Cookies, saved.Published
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

func baselineDataSnapshot(t *testing.T, db *sql.DB, excludedTables ...string) string {
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
		if slices.Contains(excludedTables, table) {
			continue
		}
		fmt.Fprintf(&snapshot, "%s:%s\n", table, baselineRows(t, db, "SELECT * FROM `"+strings.ReplaceAll(table, "`", "``")+"`"))
	}
	return snapshot.String()
}

func TestSchemaBaselineRejectsIncompatibleControlStructureWithoutWrites(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startHistoricalBaselineMySQL(t)
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('reject-account','rejected.account','rejected@example.test','Retained','opaque-hash',31,4,7,'2025-01-02')`)
	deliveryExec(t, db, `INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,creator,modifier,created_at,updated_at) VALUES('retained_v1','Retained','page_query','id','ASC',20,100,'owner','editor','2025-01-02','2025-03-04')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_orders VALUES('retained-order','reject-account','SUCCEEDED',7,'{"retained":true}')`)
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
		{"empty_roles", `UPDATE rcc_accounts SET roles=0`, `UPDATE rcc_accounts SET roles=31`},
		{"unknown_role", `UPDATE rcc_accounts SET roles=32`, `UPDATE rcc_accounts SET roles=31`},
		{"zero_role_version", `UPDATE rcc_accounts SET role_version=0`, `UPDATE rcc_accounts SET role_version=4`},
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
	requireSchemaMigrationState(t, binary, driver, "pending", "baseline")
}

func TestSchemaBaselineRecoversUnconfirmedRegistration(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startHistoricalBaselineMySQL(t)
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('recover-account','recover','recover@example.test','Retained','opaque-hash',31,4,7,'2025-01-02')`)
	deliveryExec(t, db, `INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES('retained-order','recover-account','SUCCEEDED',7,'{"retained":true}')`)
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version`).Scan(&count); err != nil || int64(count) != historicalTestMigrationCount+1 {
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
	previous, current := buildPreviousSchemaMigrationRelease(t), buildSchemaMigrationReleaseAt(t, historicalTestSchemaVersion)
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
	requireSchemaMigrationState(t, current, driver, "pending", "recover")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	var retained int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_query_policies WHERE code='retained_v1' AND creator='owner' AND modifier='editor' AND created_at='2025-01-02' AND updated_at='2025-03-04'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("rename changed audit data: %d %v", retained, err)
	}
	freshDriver := createSchemaComparisonDatabase(t, driver)
	freshDriver.MultiStatements = true
	fresh := deliveryDB(t, freshDriver)
	snapshot, err := os.ReadFile("testdata/pre-goose-8b5cd859.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, fresh, string(snapshot))
	applyUnmanagedCurrentMigrations(t, fresh)
	assertBaselinePhysicalSchemaEqual(t, db, fresh)
}

func TestSchemaBaselineRecoversBeforeAttemptWasRecorded(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startHistoricalBaselineMySQL(t)
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
	requireSchemaMigrationState(t, binary, driver, "pending", "recover")
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
	ctx, driver := startHistoricalBaselineMySQL(t)
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version`).Scan(&versions); err != nil || int64(versions) != historicalTestMigrationCount+1 {
		t.Fatalf("concurrent adoption duplicated prefix: %d %v", versions, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_schema_migration_attempts`).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatalf("concurrent adoption duplicated journal: %d %v", attempts, err)
	}
}

func TestSchemaBaselineProcessInterruptionRetainsUnconfirmedState(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startHistoricalBaselineMySQL(t)
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
	requireSchemaMigrationState(t, binary, driver, "pending", "recover")
	if got := baselineDataSnapshot(t, db); got != before {
		t.Fatal("interrupted baseline and recovery changed existing data")
	}
}

func TestSchemaBaselineUsesTransactionalVersionLedger(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startHistoricalBaselineMySQL(t)
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	deliveryExec(t, db, `SET GLOBAL default_storage_engine='MyISAM'`)
	requireSchemaMigrationState(t, binary, driver, "pending", "baseline")
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
	_, driver := startHistoricalBaselineMySQL(t)
	db := deliveryDB(t, driver)
	requireSchemaMigrationState(t, binary, driver, "pending", "baseline")
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

func TestBaselineGooseInstallationMatchesFrozenAdoptionStructure(t *testing.T) {
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, buildSchemaMigrationReleaseAt(t, historicalTestSchemaVersion), driver, "current", "up")
	installed := deliveryDB(t, driver)
	owner := *driver
	owner.User = "root"
	owner.MultiStatements = true
	root := deliveryDB(t, &owner)
	deliveryExec(t, root, `CREATE DATABASE frozen_pre_goose CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	owner.DBName = "frozen_pre_goose"
	historical := deliveryDB(t, &owner)
	snapshot, err := os.ReadFile("testdata/pre-goose-8b5cd859.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, historical, string(snapshot))
	applyUnmanagedCurrentMigrations(t, historical)
	assertBaselinePhysicalSchemaEqual(t, installed, historical)
	tables := `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND LEFT(table_name,4)='rcc_' AND table_name NOT IN ('rcc_schema_migration_attempts','rcc_goose_db_version') ORDER BY table_name`
	if baselineRows(t, installed, tables) != baselineRows(t, historical, tables) {
		t.Fatal("current initialization differs from the frozen baseline plus historical 014 through 017")
	}
}
