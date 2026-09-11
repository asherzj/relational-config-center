//go:build integration

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// The synthetic next-release fixture follows the current embedded migration set.
// Historical release tests keep their explicit version arguments unchanged.
func currentTestSchemaVersion(t *testing.T) int64 {
	t.Helper()
	entries, err := os.ReadDir("../../internal/infrastructure/mysql/migrations")
	if err != nil {
		t.Fatal(err)
	}
	var latest int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, _ := strings.Cut(entry.Name(), "_")
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		t.Fatal("no schema migration source found")
	}
	return latest
}

// A second isolated release uses the unchanged public command and adds an
// embedded migration. No production test hook or arbitrary-SQL CLI is needed.
func buildSchemaMigrationVariant(t *testing.T, prepare func(string)) string {
	t.Helper()
	root := t.TempDir()
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	copyFile := func(relative string) {
		data, err := os.ReadFile(filepath.Join(source, relative))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum", "cmd/schema-migrate/main.go"} {
		copyFile(name)
	}
	if err := filepath.WalkDir(filepath.Join(source, "internal"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql") || strings.HasSuffix(path, ".json") {
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			copyFile(rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "internal/infrastructure/mysql/migrations")
	prepare(directory)
	binary := filepath.Join(root, "schema-migrate")
	build := exec.Command("go", "build", "-o", binary, "./cmd/schema-migrate")
	build.Dir, build.Env = root, append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build next release: %v %s", err, output)
	}
	return binary
}

func buildNextSchemaMigrationRelease(t *testing.T, slow ...bool) string {
	t.Helper()
	return buildSchemaMigrationVariant(t, func(directory string) {
		fixture := "-- +goose NO TRANSACTION\n-- +goose Up\nCREATE TABLE IF NOT EXISTS rcc_migration_fixture (id bigint NOT NULL PRIMARY KEY, note varchar(20) NOT NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;\nINSERT INTO rcc_migration_fixture VALUES (1,'upgraded') ON DUPLICATE KEY UPDATE id=id;\n"
		if len(slow) > 0 && slow[0] {
			fixture = strings.Replace(fixture, "\nINSERT", "\nSELECT SLEEP(3);\nINSERT", 1)
		}
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("%05d_fixture.sql", currentTestSchemaVersion(t)+1)), []byte(fixture), 0644); err != nil {
			t.Fatal(err)
		}
		var manifest map[string]string
		data, err := os.ReadFile(filepath.Join(directory, fmt.Sprintf("%05d_schema.json", currentTestSchemaVersion(t))))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest["rcc_migration_fixture"] = "CREATE TABLE `rcc_migration_fixture` (\n  `id` bigint NOT NULL,\n  `note` varchar(20) NOT NULL,\n  PRIMARY KEY (`id`)\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci"
		data, err = json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, fmt.Sprintf("%05d_schema.json", currentTestSchemaVersion(t)+1)), data, 0644); err != nil {
			t.Fatal(err)
		}
	})
}
func buildPreviousSchemaMigrationRelease(t *testing.T) string {
	t.Helper()
	return buildSchemaMigrationReleaseAt(t, 1)
}

func buildSchemaMigrationReleaseAt(t *testing.T, version int64) string {
	t.Helper()
	return buildSchemaMigrationVariant(t, func(directory string) {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			number, _, _ := strings.Cut(entry.Name(), "_")
			candidate, err := strconv.ParseInt(number, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			if candidate <= version {
				continue
			}
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func buildSchemaMigrationCommand(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "schema-migrate")
	build := exec.Command("go", "build", "-o", binary, "../schema-migrate")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build migration command: %v %s", err, output)
	}
	return binary
}

func schemaMigrationCommand(binary string, driver *mysqldriver.Config, args ...string) *exec.Cmd {
	command := exec.Command(binary, args...)
	command.Env = []string{"PATH=" + os.Getenv("PATH")}
	for _, value := range integrationEnvironment(driver, "127.0.0.1:0") {
		if strings.HasPrefix(value, "MYSQL_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "MYSQL_MAX_OPEN_CONNS=1", "MYSQL_MAX_IDLE_CONNS=1")
	return command
}

func requireSchemaMigrationState(t *testing.T, binary string, driver *mysqldriver.Config, want string, args ...string) {
	t.Helper()
	output, err := schemaMigrationCommand(binary, driver, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("migration %v: %v %s", args, err, output)
	}
	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil || result.State != want {
		t.Fatalf("migration %v: want %s, got %s (%v)", args, want, output, err)
	}
}

func TestSchemaMigrationInitializesEmptyDatabase(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireSchemaMigrationState(t, binary, driver, "uninitialized", "status")
	var tables int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("status must not create tables: %d %v", tables, err)
	}
	if _, err := db.Exec("ALTER DATABASE rcc_test CHARACTER SET latin1 COLLATE latin1_swedish_ci"); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, binary, driver, "current", "up")
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('rcc_accounts','rcc_query_policies','rcc_release_orders','rcc_refresh_notifications')").Scan(&tables); err != nil || tables != 4 {
		t.Fatalf("required control tables: %d %v", tables, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='notification_templates'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("production migration must not create development fixture: %d %v", tables, err)
	}
	requireSchemaMigrationState(t, binary, driver, "current", "up")
	requireSchemaMigrationState(t, binary, driver, "current", "status")
}

func TestSchemaMigrationConcurrentInitialization(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	results := make(chan string, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			output, err := schemaMigrationCommand(binary, driver, "up").CombinedOutput()
			if err != nil {
				results <- string(output)
				return
			}
			results <- ""
		}()
	}
	close(start)
	for range 2 {
		if failure := <-results; failure != "" {
			t.Errorf("concurrent initialization failed: %s", failure)
		}
	}
	requireSchemaMigrationState(t, binary, driver, "current", "status")
}

func TestSchemaMigrationWaitsForMaintenanceLockWithDeadline(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	ctx, driver := startIntegrationMySQL(t)
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)),0)`).Scan(&locked); err != nil || locked != 1 {
		t.Fatalf("maintenance lock: %d %v", locked, err)
	}
	output, err := schemaMigrationCommand(binary, driver, "up", "--lock-timeout=100ms").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "migration_busy") {
		t.Fatalf("expected bounded lock conflict: %v %s", err, output)
	}
	var tables int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("lock loser wrote database: %d %v", tables, err)
	}
	if _, err := conn.ExecContext(ctx, `DO RELEASE_LOCK(CONCAT('rcc.schema:',LEFT(SHA2(DATABASE(),256),48)))`); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, binary, driver, "current", "up")
}

func TestSchemaMigrationPartialDDLRequiresExplicitRecovery(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	rootConfig := *driver
	rootConfig.User = "root"
	db, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'",
		"GRANT SELECT, INSERT, UPDATE, DELETE, ALTER, INDEX, DROP, REFERENCES, TRIGGER ON rcc_test.* TO 'rcc_admin'@'%'",
		"GRANT CREATE ON rcc_test.rcc_goose_db_version TO 'rcc_admin'@'%'",
		"GRANT CREATE ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'",
		"GRANT CREATE ON rcc_test.rcc_query_policies TO 'rcc_admin'@'%'",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	output, err := schemaMigrationCommand(binary, driver, "up").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "migration_failed") {
		t.Fatalf("expected partial DDL failure: %v %s", err, output)
	}
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_query_policies'").Scan(&exists); err != nil || exists != 1 {
		t.Fatalf("first DDL was not persisted: %d %v", exists, err)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	output, err = schemaMigrationCommand(binary, driver, "up").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "recovery_required") {
		t.Fatalf("ordinary retry must refuse: %v %s", err, output)
	}
	if _, err := db.Exec("GRANT CREATE ON rcc_test.* TO 'rcc_admin'@'%'"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE rcc_query_policies MODIFY name varchar(99) NOT NULL"); err != nil {
		t.Fatal(err)
	}
	if output, err := schemaMigrationCommand(binary, driver, "recover").CombinedOutput(); err == nil || !strings.Contains(string(output), "schema_mismatch") {
		t.Fatalf("recovery accepted an incompatible surviving table: %v %s", err, output)
	}
	if _, err := db.Exec("ALTER TABLE rcc_query_policies MODIFY name varchar(100) NOT NULL"); err != nil {
		t.Fatal(err)
	}
	next := buildNextSchemaMigrationRelease(t)
	requireSchemaMigrationState(t, next, driver, "pending", "recover")
	requireSchemaMigrationState(t, binary, driver, "current", "up")
	requireSchemaMigrationState(t, next, driver, "current", "up")
}

func TestSchemaMigrationUpgradesToNextRelease(t *testing.T) {
	first, next := buildSchemaMigrationCommand(t), buildNextSchemaMigrationRelease(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, first, driver, "current", "up")
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE business_marker(id int PRIMARY KEY, value varchar(20));"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO business_marker VALUES(1,'preserved')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,created_at) VALUES('fixture-account','fixture','fixture@example.test','Retained account','opaque-password-hash','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES('fixture-order','fixture-account','PUBLISHED',7,'{"retained":true}')`); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, next, driver, "pending", "status")
	requireSchemaMigrationState(t, next, driver, "current", "up")
	var value string
	if err := db.QueryRow("SELECT value FROM business_marker WHERE id=1").Scan(&value); err != nil || value != "preserved" {
		t.Fatalf("business marker changed: %q %v", value, err)
	}
	if err := db.QueryRow(`SELECT password_hash FROM rcc_accounts WHERE id='fixture-account'`).Scan(&value); err != nil || value != "opaque-password-hash" {
		t.Fatalf("account changed: %q %v", value, err)
	}
	var orderVersion int
	if err := db.QueryRow(`SELECT version FROM rcc_release_orders WHERE id='fixture-order' AND document->'$.retained'=true`).Scan(&orderVersion); err != nil || orderVersion != 7 {
		t.Fatalf("publication history changed: %d %v", orderVersion, err)
	}
	if err := db.QueryRow("SELECT note FROM rcc_migration_fixture WHERE id=1").Scan(&value); err != nil || value != "upgraded" {
		t.Fatalf("next migration missing: %q %v", value, err)
	}
	if _, err := db.Exec("UPDATE rcc_migration_fixture SET note='retained' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, next, driver, "current", "up")
	if err := db.QueryRow("SELECT note FROM rcc_migration_fixture WHERE id=1").Scan(&value); err != nil || value != "retained" {
		t.Fatalf("duplicate migration replayed: %q %v", value, err)
	}
	requireSchemaMigrationState(t, first, driver, "incompatible", "status")
	freshDriver := createSchemaComparisonDatabase(t, driver)
	requireSchemaMigrationState(t, next, freshDriver, "current", "up")
	freshDB, err := sql.Open("mysql", freshDriver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer freshDB.Close()
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND LEFT(table_name,4)='rcc_' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, table, upgraded, fresh string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &upgraded); err != nil {
			t.Fatal(err)
		}
		if err := freshDB.QueryRow("SHOW CREATE TABLE `"+name+"`").Scan(&table, &fresh); err != nil || upgraded != fresh {
			t.Fatalf("fresh and upgraded control structure differ for %s: %v", name, err)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Compare independent schemas within one disposable MySQL server so migration
// integration tests never need two concurrent containers.
func createSchemaComparisonDatabase(t *testing.T, driver *mysqldriver.Config) *mysqldriver.Config {
	t.Helper()
	owner := *driver
	owner.User = "root"
	root := deliveryDB(t, &owner)
	deliveryExec(t, root, `CREATE DATABASE schema_comparison CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci`)
	owner.DBName = "schema_comparison"
	owner.Params = map[string]string{"charset": "utf8mb4"}
	return &owner
}

func TestReleaseTemplateSchemaMigrationUpgradesVersionFive(t *testing.T) {
	previous, current := buildSchemaMigrationReleaseAt(t, 5), buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, previous, driver, "current", "up")
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE business_marker(id int PRIMARY KEY,note varchar(20) NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO business_marker VALUES(1,'preserved')`); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, current, driver, "pending", "status")
	requireSchemaMigrationState(t, current, driver, "current", "up")
	var marker string
	if err := db.QueryRow(`SELECT note FROM business_marker WHERE id=1`).Scan(&marker); err != nil || marker != "preserved" {
		t.Fatalf("upgrade changed business data: %q %v", marker, err)
	}
	var templates, emergency int
	if err := db.QueryRow(`SELECT COUNT(*),SUM(release_type='EMERGENCY' AND enabled=1) FROM rcc_release_templates`).Scan(&templates, &emergency); err != nil || templates != 2 || emergency != 1 {
		t.Fatalf("release template defaults: templates=%d emergency=%d err=%v", templates, emergency, err)
	}
	var templateVersion int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version WHERE version_id=9 AND is_applied=1`).Scan(&templateVersion); err != nil || templateVersion != 1 {
		t.Fatalf("candidate release template migration missing: %d %v", templateVersion, err)
	}
	rootConfig := *driver
	rootConfig.User = "root"
	root, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := root.Exec(`CREATE USER 'release_template_reader'@'%' IDENTIFIED BY 'rcc_password'`); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Exec(`GRANT SELECT ON rcc_test.* TO 'release_template_reader'@'%'`); err != nil {
		t.Fatal(err)
	}
	reader := *driver
	reader.User = "release_template_reader"
	process := accountProcessCommand(t, buildIntegrationAdmin(t), &reader)
	process.ready(t)
	process.stop(t)
}

func TestSchemaMigrationCommittedVersionNeedsConfirmedRecovery(t *testing.T) {
	first, next := buildSchemaMigrationCommand(t), buildNextSchemaMigrationRelease(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, first, driver, "current", "up")
	rootConfig := *driver
	rootConfig.User = "root"
	db, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Fail at the persistence boundary after Goose has committed the next version.
	for _, statement := range []string{
		`INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,created_at) VALUES('fixture-account','fixture','fixture@example.test','Retained account','opaque-password-hash','2026-01-01')`,
		`INSERT INTO rcc_query_policies(code,name,type_code,default_order_field,default_order_direction,default_page_size,max_page_size,creator,modifier,created_at,updated_at) VALUES('retained_v1','Retained policy','business_marker','id','ASC',20,100,'fixture-account','fixture-account','2026-01-01','2026-01-02')`,
		`INSERT INTO rcc_release_orders(id,applicant_id,state,version,document) VALUES('fixture-order','fixture-account','PUBLISHED',7,'{"retained":true}')`,
		`CREATE TABLE business_marker(id int PRIMARY KEY,note varchar(20) NOT NULL)`,
		`INSERT INTO business_marker VALUES(1,'retained')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`CREATE TRIGGER block_migration_confirmation BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.state='SUCCEEDED' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='private_database_detail'; END IF; END`); err != nil {
		t.Fatal(err)
	}
	output, err := schemaMigrationCommand(next, driver, "up").CombinedOutput()
	if err == nil || strings.Contains(string(output), "private_database_detail") {
		t.Fatalf("confirmation failure must be safe: %v %s", err, output)
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version WHERE version_id=? AND is_applied=1`, currentTestSchemaVersion(t)+1).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("version was not committed: %d %v", versions, err)
	}
	requireSchemaMigrationState(t, next, driver, "recovery_required", "status")
	output, err = schemaMigrationCommand(next, driver, "recover").CombinedOutput()
	if err == nil {
		t.Fatalf("unresolved confirmation failure must not report successful recovery: %s", output)
	}
	if _, err := db.Exec(`DROP TRIGGER block_migration_confirmation`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER block_recovery_record BEFORE UPDATE ON rcc_schema_migration_attempts FOR EACH ROW BEGIN IF NEW.recovery_count>OLD.recovery_count THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='private_recovery_detail'; END IF; END`); err != nil {
		t.Fatal(err)
	}
	if output, err := schemaMigrationCommand(next, driver, "recover").CombinedOutput(); err == nil || strings.Contains(string(output), "private_recovery_detail") {
		t.Fatalf("recovery record failure must be safe: %v %s", err, output)
	}
	requireSchemaMigrationState(t, next, driver, "recovery_required", "status")
	if _, err := db.Exec(`DROP TRIGGER block_recovery_record`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE rcc_migration_fixture SET note='retained' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, next, driver, "current", "recover")
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version WHERE version_id=?`, currentTestSchemaVersion(t)+1).Scan(&versions); err != nil || versions != 1 {
		t.Fatalf("committed version duplicated: %d %v", versions, err)
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM rcc_accounts WHERE id='fixture-account' AND username='fixture' AND password_hash='opaque-password-hash' AND roles=1 AND role_version=1 AND session_version=1 AND created_at='2026-01-01'`,
		`SELECT COUNT(*) FROM rcc_query_policies WHERE code='retained_v1' AND name='Retained policy' AND modifier='fixture-account' AND created_at='2026-01-01' AND updated_at='2026-01-02'`,
		`SELECT COUNT(*) FROM rcc_release_orders WHERE id='fixture-order' AND applicant_id='fixture-account' AND state='PUBLISHED' AND version=7 AND document->'$.retained'=true`,
		`SELECT COUNT(*) FROM business_marker WHERE id=1 AND note='retained'`,
		`SELECT COUNT(*) FROM rcc_migration_fixture WHERE id=1 AND note='retained'`,
	} {
		var preserved int
		if err := db.QueryRow(query).Scan(&preserved); err != nil || preserved != 1 {
			t.Fatalf("recovery changed retained data: %d %v (%s)", preserved, err, query)
		}
	}
}

func TestSchemaMigrationRecoversInterruptedBootstrap(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	rootConfig := *driver
	rootConfig.User = "root"
	db, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'",
		"GRANT SELECT, CREATE ON rcc_test.* TO 'rcc_admin'@'%'",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := schemaMigrationCommand(binary, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("bootstrap without INSERT succeeded: %s", output)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	if _, err := db.Exec("GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'"); err != nil {
		t.Fatal(err)
	}
	if output, err := schemaMigrationCommand(binary, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("bootstrap was implicitly retried: %s", output)
	}
	requireSchemaMigrationState(t, binary, driver, "pending", "recover")
	requireSchemaMigrationState(t, binary, driver, "current", "up")
}

func TestSchemaMigrationProcessInterruptionRequiresSameReleaseRecovery(t *testing.T) {
	first := buildSchemaMigrationCommand(t)
	next, altered := buildNextSchemaMigrationRelease(t, true), buildNextSchemaMigrationRelease(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, first, driver, "current", "up")
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	command := schemaMigrationCommand(next, driver, "up")
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		var present int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_migration_fixture'`).Scan(&present); err != nil {
			t.Fatal(err)
		}
		if present == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("migration did not reach partial DDL boundary")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatalf("killed migration reported success: %s", output.String())
	}
	requireSchemaMigrationState(t, next, driver, "recovery_required", "status")
	if output, err := schemaMigrationCommand(next, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("interrupted migration automatically retried: %s", output)
	}
	if output, err := schemaMigrationCommand(altered, driver, "recover").CombinedOutput(); err == nil {
		t.Fatalf("different release recovered the interrupted migration: %s", output)
	}
	requireSchemaMigrationState(t, next, driver, "current", "recover")
}

func TestSchemaMigrationRejectsIncompleteVersionHistory(t *testing.T) {
	next := buildNextSchemaMigrationRelease(t)
	_, driver := startIntegrationMySQL(t)
	requireSchemaMigrationState(t, next, driver, "current", "up")
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM rcc_goose_db_version WHERE version_id=1`); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, next, driver, "incompatible", "status")
	for _, operation := range []string{"up", "recover"} {
		if output, err := schemaMigrationCommand(next, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s accepted incomplete history: %s", operation, output)
		}
	}
}

func TestSchemaMigrationRecoversMissingZeroVersion(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t)
	rootConfig := *driver
	rootConfig.User = "root"
	db, err := sql.Open("mysql", rootConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		"REVOKE ALL PRIVILEGES, GRANT OPTION FROM 'rcc_admin'@'%'",
		"GRANT SELECT, CREATE ON rcc_test.* TO 'rcc_admin'@'%'",
		"GRANT CREATE, INSERT ON rcc_test.rcc_schema_migration_attempts TO 'rcc_admin'@'%'",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := schemaMigrationCommand(binary, driver, "up").CombinedOutput(); err == nil {
		t.Fatalf("bootstrap without ledger INSERT succeeded: %s", output)
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_goose_db_version`).Scan(&versions); err != nil || versions != 0 {
		t.Fatalf("expected interrupted empty version table: %d %v", versions, err)
	}
	requireSchemaMigrationState(t, binary, driver, "recovery_required", "status")
	if _, err := db.Exec("GRANT ALL PRIVILEGES ON rcc_test.* TO 'rcc_admin'@'%'"); err != nil {
		t.Fatal(err)
	}
	requireSchemaMigrationState(t, binary, driver, "pending", "recover")
	requireSchemaMigrationState(t, binary, driver, "current", "up")
}

func TestSchemaMigrationConnectionDeadlineAndSafeDiagnostics(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			defer conn.Close()
			<-done // A reachable server that never finishes the MySQL handshake.
		}
	}()
	driver := mysqldriver.NewConfig()
	driver.Net, driver.Addr, driver.DBName = "tcp", listener.Addr().String(), "rcc_test"
	driver.User, driver.Passwd = "rcc_admin", "private_password_marker"
	start := time.Now()
	output, err := schemaMigrationCommand(binary, driver, "up", "--timeout=200ms", "--lock-timeout=100ms").CombinedOutput()
	if elapsed := time.Since(start); err == nil || elapsed > 2*time.Second || !strings.Contains(string(output), "database_unavailable") || strings.Contains(string(output), driver.Passwd) {
		t.Fatalf("expected bounded safe connection failure: %v in %s %s", err, elapsed, output)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	output, err = schemaMigrationCommand(binary, driver, "status").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "database_unavailable") || strings.Contains(string(output), driver.Passwd) {
		t.Fatalf("unreachable database must fail safely: %v %s", err, output)
	}
}

func TestSchemaMigrationLeavesUnmanagedDatabaseUntouched(t *testing.T) {
	binary := buildSchemaMigrationCommand(t)
	_, driver := startIntegrationMySQL(t, "testdata/pre-goose-8b5cd859.sql")
	requireSchemaMigrationState(t, binary, driver, "unmanaged", "status")
	for _, operation := range []string{"up", "recover", "down", "reset"} {
		if output, err := schemaMigrationCommand(binary, driver, operation).CombinedOutput(); err == nil {
			t.Fatalf("%s accepted an unmanaged database: %s", operation, output)
		}
	}
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('rcc_goose_db_version','rcc_schema_migration_attempts')`).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("unmanaged database was modified: %d %v", tables, err)
	}
}
