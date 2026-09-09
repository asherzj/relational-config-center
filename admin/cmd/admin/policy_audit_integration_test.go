//go:build integration

package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
)

var policyAuditTables = []string{"rcc_query_policies", "rcc_mutation_policies", "rcc_table_policies"}

func TestPolicyAuditMigrationPreservesDataAndIsRestartable(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	deliveryExec(t, db, `INSERT INTO rcc_query_policies
		(code, name, type_code, default_order_field, default_order_direction, default_page_size, max_page_size, creator, modifier, created_at, updated_at)
		VALUES ('audit_query_v1', 'Audit query', 'page_query', 'id', 'DESC', 20, 200, 'original', 'editor', '2024-02-29 01:02:03', '2025-12-31 23:59:58')`)
	deliveryExec(t, db, `INSERT INTO rcc_mutation_policies
		(code, name, type_code, create_time_field, allow_add, creator, modifier, created_at, updated_at)
		VALUES ('audit_mutation_v1', 'Audit mutation', 'single_table_mutation', 'gmt_created', 1, 'original', 'editor', '2024-02-29 01:02:03', '2025-12-31 23:59:58')`)
	deliveryExec(t, db, `INSERT INTO rcc_table_policies
		(table_name, query_policy_code, mutation_policy_code, creator, modifier, created_at, updated_at)
		VALUES ('audit_items', 'audit_query_v1', 'audit_mutation_v1', 'original', 'editor', '2024-02-29 01:02:03', '2025-12-31 23:59:58')`)
	wantSchema := policyCatalogSchemaSignature(t, ctx, db)
	wantRows := policyAuditRows(t, db)
	if err := applyPolicyAuditMigration(t, db); err != nil {
		t.Fatalf("migration on fresh schema: %v", err)
	}
	for _, state := range []string{"legacy", "partial"} {
		t.Run(state, func(t *testing.T) {
			for index, table := range policyAuditTables {
				// Cover both legacy columns, one already-renamed column, and a
				// fully migrated table, as may remain after an interrupted rollout.
				if state == "legacy" || index == 0 {
					deliveryExec(t, db, "ALTER TABLE "+table+" RENAME COLUMN created_at TO gmt_created, RENAME COLUMN updated_at TO gmt_modified")
				} else if index == 1 {
					deliveryExec(t, db, "ALTER TABLE "+table+" RENAME COLUMN updated_at TO gmt_modified")
				}
			}
			if err := app.mysql.Ready(ctx); err == nil || !strings.Contains(err.Error(), "013") {
				t.Fatalf("readiness must reject old audit columns with migration guidance: %v", err)
			}
			for run := 0; run < 2; run++ {
				if err := applyPolicyAuditMigration(t, db); err != nil {
					t.Fatalf("migration run %d: %v", run, err)
				}
				if got := policyAuditRows(t, db); got != wantRows {
					t.Fatalf("migration changed policy values or timestamps:\n%s\nwant:\n%s", got, wantRows)
				}
				if got := policyCatalogSchemaSignature(t, ctx, db); got != wantSchema {
					expected := strings.Split(wantSchema, "\n")
					for index, line := range strings.Split(got, "\n") {
						if index >= len(expected) {
							t.Fatalf("unexpected schema line: %s", line)
						}
						if line != expected[index] {
							t.Fatalf("schema differs at line %d: got %s; want %s", index, line, expected[index])
						}
					}
					t.Fatal("schema signature length differs")
				}
				if err := app.mysql.Ready(ctx); err != nil {
					t.Fatalf("readiness after migration: %v", err)
				}
			}
		})
	}
}

func TestPolicyAuditMigrationRejectsAmbiguousOrMissingColumnsBeforeDDL(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	for _, table := range policyAuditTables {
		deliveryExec(t, db, "ALTER TABLE "+table+" RENAME COLUMN created_at TO gmt_created, RENAME COLUMN updated_at TO gmt_modified")
	}
	for _, test := range []struct{ name, setup, cleanup string }{
		{"duplicate", "ADD COLUMN created_at datetime NULL", "DROP COLUMN created_at"},
		{"missing", "RENAME COLUMN gmt_modified TO unexpected_audit", "RENAME COLUMN unexpected_audit TO gmt_modified"},
	} {
		t.Run(test.name, func(t *testing.T) {
			deliveryExec(t, db, "ALTER TABLE rcc_table_policies "+test.setup)
			before := policyCatalogSchemaSignature(t, ctx, db)
			err := applyPolicyAuditMigration(t, db)
			if err == nil || !strings.Contains(err.Error(), "013 blocked: rcc_table_policies") {
				t.Fatalf("expected table-scoped preflight rejection: %v", err)
			}
			if got := policyCatalogSchemaSignature(t, ctx, db); got != before {
				t.Fatal("failed preflight changed a catalog table")
			}
			deliveryExec(t, db, "ALTER TABLE rcc_table_policies "+test.cleanup)
		})
	}
	if err := applyPolicyAuditMigration(t, db); err != nil {
		t.Fatalf("corrected migration must be retryable: %v", err)
	}
}

func policyAuditRows(t *testing.T, db *sql.DB) string {
	t.Helper()
	var result []string
	for _, table := range policyAuditTables {
		rows, err := db.QueryContext(t.Context(), "SELECT * FROM "+table+" ORDER BY id")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]sql.RawBytes, len(columns))
			targets := make([]any, len(columns))
			for index := range values {
				targets[index] = &values[index]
			}
			if err := rows.Scan(targets...); err != nil {
				t.Fatal(err)
			}
			result = append(result, fmt.Sprintf("%s: %#v", table, values))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		_ = rows.Close()
	}
	return strings.Join(result, "\n")
}

func applyPolicyAuditMigration(t *testing.T, db *sql.DB) error {
	t.Helper()
	data, err := os.ReadFile("../../../deploy/mysql/migrations/013-policy-audit-timestamps.sql")
	if err != nil {
		t.Fatal(err)
	}
	// DELIMITER is a mysql-client directive. Send the procedure body as one
	// statement on a single connection, followed by CALL and DROP separately.
	preamble, body, found := strings.Cut(string(data), "DELIMITER $$\n")
	if !found {
		t.Fatal("missing migration procedure delimiter")
	}
	procedure, tail, found := strings.Cut(body, "DELIMITER ;\n")
	if !found {
		t.Fatal("missing migration call delimiter")
	}
	conn, err := db.Conn(t.Context())
	if err != nil {
		return err
	}
	defer conn.Close()
	statements := append([]string{preamble}, strings.Split(procedure, "$$")...)
	statements = append(statements, strings.Split(tail, ";")...)
	for _, statement := range statements {
		if strings.TrimSpace(statement) != "" {
			if _, err := conn.ExecContext(t.Context(), statement); err != nil {
				return err
			}
		}
	}
	return nil
}
