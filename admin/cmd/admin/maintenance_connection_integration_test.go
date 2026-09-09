//go:build integration

package main

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyMigrationRunsWithoutAccountSchemaOrHTTPConfiguration(t *testing.T) {
	_, driver := startIntegrationMySQL(t,
		"testdata/001-legacy-policy-schema.sql",
		"../../../deploy/mysql/migrations/001-promote-mutation-capabilities.sql",
		"../../../deploy/mysql/migrations/002-rename-audit-timestamps.sql",
		"../../../deploy/mysql/migrations/003-create-query-policies.sql",
		"../../../deploy/mysql/migrations/004-create-mutation-policies.sql",
		"../../../deploy/mysql/migrations/005-expand-table-policy-code-references.sql",
		"../../../deploy/mysql/migrations/013-policy-audit-timestamps.sql",
	)
	binary := filepath.Join(t.TempDir(), "policy-migrate")
	build := exec.Command("go", "build", "-o", binary, "../policy-migrate")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build migration: %v %s", err, output)
	}
	command := exec.Command(binary)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "POLICY_MIGRATION_OPERATOR=historical-maintainer"}
	for _, value := range integrationEnvironment(driver, "127.0.0.1:0") {
		if strings.HasPrefix(value, "MYSQL_") {
			command.Env = append(command.Env, value)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("maintenance locked behind account/HTTP initialization: %v %s", err, output)
	}
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var legacyColumns, accountTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rcc_table_policies' AND column_name='query_policy_config'`).Scan(&legacyColumns); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='rcc_accounts'`).Scan(&accountTables); err != nil {
		t.Fatal(err)
	}
	if legacyColumns != 0 || accountTables != 0 {
		t.Fatalf("maintenance migration unexpected structures: legacy=%d accounts=%d", legacyColumns, accountTables)
	}
}
