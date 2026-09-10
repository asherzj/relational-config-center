//go:build integration

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

// AC-021 uses a new disposable MySQL and the shipped maintenance process.
func TestReleaseResetPreservesRecordsAndContinuesPublication(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	db := deliveryDB(t, driver)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	deliveryExec(t, db, `INSERT INTO rcc_record_versions VALUES ('mutation_supplied_id_items', X'', 40)`)
	first := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "mutation_supplied_id_items", "", `{"content":{"id":"kept","label":"published value"}}`))
	if first.RecordVersion != "41" || first.Sequence != "1" || first.TableVersion != "1" {
		t.Fatalf("unexpected initial publication: %+v", first)
	}
	_ = approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"pending cleanup","table_name":"mutation_supplied_id_items","items":[{"operation":"MODIFY","id":"kept","expected_record_version":"41","content":{"label":"never applied"}}]}`, "reset-approved")
	before := resetCounts(t, driver)
	for table, count := range before {
		if count == 0 {
			t.Fatalf("missing fixture for %s", table)
		}
	}
	var uuid string
	if err := db.QueryRow(`SELECT @@server_uuid`).Scan(&uuid); err != nil {
		t.Fatal(err)
	}
	binary := buildReleaseReset(t)
	args := []string{"--environment=test", "--target-address=" + driver.Addr, "--target-database=" + driver.DBName, "--target-server-uuid=" + uuid, "--writers-stopped"}
	output, err := resetCommand(binary, driver, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("reset: %v %s", err, output)
	}
	var report struct {
		Committed     bool `json:"committed"`
		Before, After map[string]uint64
	}
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("report: %v %s", err, output)
	}
	if !report.Committed || !reflect.DeepEqual(before, report.Before) {
		t.Fatalf("wrong report: %s", output)
	}
	for table, count := range resetCounts(t, driver) {
		if count != 0 || report.After[table] != 0 {
			t.Fatalf("retained history: %s %d", table, count)
		}
	}
	row, version := recordVersionRow(t, app, "mutation_supplied_id_items", "kept")
	if *row["label"] != "published value" || version != "41" {
		t.Fatal("reset changed business value/version", row, version)
	}
	var floor, tableVersion, cursor uint64
	if err := db.QueryRow(`SELECT lock_version FROM rcc_record_versions WHERE table_name='mutation_supplied_id_items' AND record_key=X''`).Scan(&floor); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT table_version,command_cursor FROM rcc_table_publications WHERE table_name='mutation_supplied_id_items'`).Scan(&tableVersion, &cursor); err != nil {
		t.Fatal(err)
	}
	if floor != 40 || tableVersion != 1 || cursor != 1 {
		t.Fatalf("lost progress: %d %d %d", floor, tableVersion, cursor)
	}
	output, err = resetCommand(binary, driver, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("repeat reset: %v %s", err, output)
	}
	next := publishedFixtureCommand(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_supplied_id_items", "kept", `{"expected_version":"41","content":{"label":"next publication"}}`))
	if next.RecordVersion != "42" || next.Sequence != "2" || next.TableVersion != "2" {
		t.Fatalf("reused progress after reset: %+v", next)
	}
	t.Logf("AC-021 before=%v after=%v; business value kept; generation=40; actual publication record 41→42, table 1→2, cursor 1→2; repeat succeeded", before, report.After)
}

var resetTables = []string{"rcc_release_orders", "rcc_release_requests", "rcc_release_targets", "rcc_publication_commands", "rcc_refresh_notifications"}

func resetCounts(t *testing.T, driver *mysqldriver.Config) map[string]uint64 {
	t.Helper()
	db := deliveryDB(t, driver)
	result := map[string]uint64{}
	for _, table := range resetTables {
		var count uint64
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		result[table] = count
	}
	return result
}
func buildReleaseReset(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "release-reset")
	command := exec.Command("go", "build", "-o", binary, "../release-reset")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build release-reset: %v %s", err, output)
	}
	return binary
}
func resetCommand(binary string, driver *mysqldriver.Config, args ...string) *exec.Cmd {
	command := exec.Command(binary, args...)
	command.Env = []string{"PATH=" + os.Getenv("PATH")}
	for _, value := range integrationEnvironment(driver, "127.0.0.1:0") {
		if strings.HasPrefix(value, "MYSQL_") {
			command.Env = append(command.Env, value)
		}
	}
	return command
}

func TestReleaseResetRefusesUnverifiedTargetsAndSchema(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	seedReleaseResetHistory(t, driver)
	before := resetCounts(t, driver)
	binary := buildReleaseReset(t)
	var uuid string
	if err := db.QueryRow(`SELECT @@server_uuid`).Scan(&uuid); err != nil {
		t.Fatal(err)
	}
	valid := []string{"--environment=test", "--target-address=" + driver.Addr, "--target-database=" + driver.DBName, "--target-server-uuid=" + uuid, "--writers-stopped"}
	for _, test := range []struct {
		name string
		args []string
	}{
		{"no target", nil},
		{"production", append(append([]string{}, valid...), "--environment=production")},
		{"writers not stopped", valid[:4]},
		{"wrong address", append(append([]string{}, valid...), "--target-address=wrong:3306")},
		{"wrong database", append(append([]string{}, valid...), "--target-database=wrong")},
		{"wrong instance", append(append([]string{}, valid...), "--target-server-uuid=wrong")},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := resetCommand(binary, driver, test.args...).CombinedOutput()
			if err == nil {
				t.Fatalf("unsafe target accepted: %s", output)
			}
			if strings.Contains(string(output), driver.Passwd) {
				t.Fatal("credential leaked")
			}
			if !reflect.DeepEqual(before, resetCounts(t, driver)) {
				t.Fatal("rejected target deleted history")
			}
		})
	}
	root := *driver
	root.User = "root"
	owner := deliveryDB(t, &root)
	for _, test := range []struct{ name, change, restore string }{
		{"missing table", "RENAME TABLE rcc_refresh_notifications TO reset_saved_notifications", "RENAME TABLE reset_saved_notifications TO rcc_refresh_notifications"},
		{"nontransactional table", "ALTER TABLE rcc_release_requests ENGINE=MyISAM", "ALTER TABLE rcc_release_requests ENGINE=InnoDB"},
		{"unexpected column", "ALTER TABLE rcc_release_orders ADD unexpected INT", "ALTER TABLE rcc_release_orders DROP COLUMN unexpected"},
		{"side effect trigger", "CREATE TRIGGER reset_side_effect AFTER DELETE ON rcc_release_orders FOR EACH ROW DELETE FROM rcc_record_versions", "DROP TRIGGER reset_side_effect"},
		{"hidden inbound foreign key", "CREATE TABLE reset_hidden.child(id INT PRIMARY KEY,order_id VARBINARY(32),FOREIGN KEY(order_id) REFERENCES rcc_test.rcc_release_orders(id) ON DELETE CASCADE) ENGINE=InnoDB", "DROP TABLE reset_hidden.child"},
		{"no metadata privilege", "REVOKE PROCESS ON *.* FROM 'rcc_admin'@'%'", "GRANT PROCESS ON *.* TO 'rcc_admin'@'%'"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "hidden inbound foreign key" {
				deliveryExec(t, owner, "CREATE DATABASE reset_hidden")
			}
			deliveryExec(t, owner, test.change)
			output, err := resetCommand(binary, driver, valid...).CombinedOutput()
			deliveryExec(t, owner, test.restore)
			if err == nil {
				t.Fatalf("unsafe schema accepted: %s", output)
			}
			if strings.Contains(string(output), driver.Passwd) {
				t.Fatal("credential leaked")
			}
			if !reflect.DeepEqual(before, resetCounts(t, driver)) {
				t.Fatal("schema rejection partially deleted history")
			}
		})
	}
}

func TestReleaseResetInterruptedTransactionRollsBackAndRetries(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	seedReleaseResetHistory(t, driver)
	before := resetCounts(t, driver)
	binary := buildReleaseReset(t)
	var uuid string
	if err := db.QueryRow(`SELECT @@server_uuid`).Scan(&uuid); err != nil {
		t.Fatal(err)
	}
	args := []string{"--environment=test", "--target-address=" + driver.Addr, "--target-database=" + driver.DBName, "--target-server-uuid=" + uuid, "--writers-stopped"}
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	var id string
	if err := blocker.QueryRow(`SELECT id FROM rcc_release_orders LIMIT 1 FOR UPDATE`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	command := resetCommand(binary, driver, args...)
	var output strings.Builder
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	deadline := time.Now().Add(4 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE INFO='DELETE FROM ` + "`rcc_release_orders`" + `'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			waiting = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("reset did not reach last DELETE with previous four deletes uncommitted")
	}
	if !reflect.DeepEqual(before, resetCounts(t, driver)) {
		t.Fatal("partial reset visible before commit")
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("interrupted reset claimed success")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("interruption did not cancel reset")
	}
	if !strings.Contains(output.String(), "keep writers stopped") {
		t.Fatalf("missing recovery instruction: %s", output.String())
	}
	if err := blocker.Rollback(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, resetCounts(t, driver)) {
		t.Fatal("interruption left partial deletion")
	}
	retried, err := resetCommand(binary, driver, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("retry: %v %s", err, retried)
	}
	for table, n := range resetCounts(t, driver) {
		if n != 0 {
			t.Fatalf("retry left %s=%d", table, n)
		}
	}
	t.Log("AC-021: signal interrupted final DELETE after four transaction-local deletes; no partial changes visible; rollback preserved all five counts; same target retry cleared all five")
}

func seedReleaseResetHistory(t *testing.T, driver *mysqldriver.Config) {
	t.Helper()
	db := deliveryDB(t, driver)
	for _, statement := range []string{
		`INSERT INTO rcc_release_orders VALUES('old','example','actor','APPROVED',3,JSON_OBJECT())`,
		`INSERT INTO rcc_release_requests VALUES('actor','create','historic',UNHEX(REPEAT('00',32)),JSON_OBJECT('id','old','state','DRAFT'))`,
		`INSERT INTO rcc_release_requests VALUES('actor','execute:old','unfinished',UNHEX(REPEAT('00',32)),NULL)`,
		`INSERT INTO rcc_release_targets VALUES('example',UNHEX(REPEAT('01',32)),'old')`,
		`INSERT INTO rcc_publication_commands VALUES('example',7,'old',JSON_OBJECT())`,
		`INSERT INTO rcc_refresh_notifications VALUES('old','example',6,JSON_OBJECT())`,
		`INSERT INTO rcc_table_publications VALUES('example',6,7)`,
		`INSERT INTO rcc_record_versions VALUES('example',X'',40),('example',UNHEX(REPEAT('01',32)),41)`,
	} {
		deliveryExec(t, db, statement)
	}
}
