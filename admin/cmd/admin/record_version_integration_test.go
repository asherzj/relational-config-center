//go:build integration

package main

import (
	"encoding/json"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func recordVersionRow(t *testing.T, app *adminApplication, table, id string) (map[string]*string, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"conditions": []any{map[string]string{"field": "id", "operator": "exact", "value": id}}})
	response := policyIntegrationRequest(t, app, http.MethodPost, "/api/v1/tables/"+table+"/query", string(body))
	if response.Code != http.StatusOK {
		t.Fatalf("query record: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		Rows     []map[string]*string `json:"rows"`
		Versions []string             `json:"record_versions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || len(result.Versions) != 1 {
		t.Fatalf("expected row and separate version metadata: %s", response.Body.String())
	}
	return result.Rows[0], result.Versions[0]
}

func TestRecordVersionLegacyBaseline(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "01")
	if version != "0" || *row["code"] != "delete-rollback" {
		t.Fatalf("legacy snapshot: row=%v version=%q", row, version)
	}
	if _, exists := row["lock_version"]; exists {
		t.Fatal("record version leaked into business fields")
	}
}

func TestRecordVersionCompareAndSwap(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	missing := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusUnprocessableEntity, "record_version_required")
	for _, value := range []string{`"01"`, `"-1"`, `"18446744073709551616"`} {
		invalid := policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"invalid"},"expected_version":`+value+`}`)
		assertIntegrationErrorCode(t, invalid, 422, "record_version_invalid")
	}
	missingDelete := policyIntegrationRequest(t, app, "DELETE", "/api/v1/tables/mutation_delete_parents/rows/1", "")
	assertIntegrationErrorCode(t, missingDelete, 422, "record_version_required")
	wrongType := policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"numeric"},"expected_version":0}`)
	assertIntegrationErrorCode(t, wrongType, 400, "invalid_request")
	updated := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_delete_parents/rows/01", `{"content":{"code":"winner"},"expected_version":"0"}`)
	assertMutationAffected(t, updated)
	stale := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"loser"},"expected_version":"0"}`)
	assertIntegrationErrorCode(t, stale, http.StatusConflict, "record_version_conflict")
	deleted := policyIntegrationRequest(t, app, http.MethodDelete, "/api/v1/tables/mutation_delete_parents/rows/1", `{"expected_version":"0"}`)
	assertIntegrationErrorCode(t, deleted, http.StatusConflict, "record_version_conflict")
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if version != "1" || *row["code"] != "winner" {
		t.Fatalf("CAS failed: %v %s", row, version)
	}
}

func TestRecordVersionAddDeleteRecreateAndRollback(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	add := func() {
		t.Helper()
		response := policyIntegrationRequest(t, app, http.MethodPost, "/api/v1/tables/mutation_add_items/rows", `{"content":{"id":"7","code":"aba","label":"original"}}`)
		if response.Code != http.StatusCreated {
			t.Fatalf("add: %d %s", response.Code, response.Body.String())
		}
	}
	add()
	_, version := recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "1" {
		t.Fatalf("new record version = %s", version)
	}
	failed := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/7", `{"content":{"label":"rollback"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row, version := recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "1" || *row["label"] != "original" {
		t.Fatalf("failed write advanced state: %v %s", row, version)
	}
	assertMutationAffected(t, policyIntegrationRequest(t, app, http.MethodDelete, "/api/v1/tables/mutation_add_items/rows/7", `{"expected_version":"1"}`))
	missing := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/7", `{"content":{"label":"gone"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
	add()
	_, version = recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "3" {
		t.Fatalf("recreated record version = %s", version)
	}
	stale := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/mutation_add_items/rows/7", `{"content":{"label":"stale"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, stale, http.StatusConflict, "record_version_conflict")
}

func TestRecordVersionMySQLPrimaryKeyEquivalence(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/010-record-identity-fixture.sql")
	for _, test := range []struct{ table, first, equivalent, recreated string }{
		{"record_identity_ci", "Résumé", "resume", "RESUME"},
		{"record_identity_pad", "Code ", "code", "CÓDE"},
		{"record_identity_decimal", "+001.00", "1.0000", "1"},
		{"record_identity_timestamp", "2026-09-07T00:00:00Z", "2026-09-07T00:00:00.000000Z", "2026-09-07T00:00:00Z"},
	} {
		t.Run(test.table, func(t *testing.T) {
			enableMutationPolicy(t, app, test.table, mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
			add := func(id string) {
				t.Helper()
				body, _ := json.Marshal(map[string]any{"content": map[string]string{"id": id, "label": "created"}})
				r := policyIntegrationRequest(t, app, http.MethodPost, "/api/v1/tables/"+test.table+"/rows", string(body))
				if r.Code != 201 {
					t.Fatalf("add: %d %s", r.Code, r.Body.String())
				}
			}
			add(test.first)
			_, version := recordVersionRow(t, app, test.table, test.equivalent)
			if version != "1" {
				t.Fatalf("equivalent version %s", version)
			}
			assertMutationAffected(t, policyIntegrationRequest(t, app, http.MethodDelete, "/api/v1/tables/"+test.table+"/rows/"+url.PathEscape(test.equivalent), `{"expected_version":"1"}`))
			add(test.recreated)
			_, version = recordVersionRow(t, app, test.table, test.equivalent)
			if version != "3" {
				t.Fatalf("identity reset: %s", version)
			}
			stale := policyIntegrationRequest(t, app, http.MethodPatch, "/api/v1/tables/"+test.table+"/rows/"+url.PathEscape(test.first), `{"content":{"label":"stale"},"expected_version":"1"}`)
			assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
		})
	}
	enableMutationPolicy(t, app, "record_identity_bin", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	for _, id := range []string{"A", "a", "a "} {
		body, _ := json.Marshal(map[string]any{"content": map[string]string{"id": id, "label": "distinct"}})
		r := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/record_identity_bin/rows", string(body))
		if r.Code != 201 {
			t.Fatal(r.Body.String())
		}
		_, v := recordVersionRow(t, app, "record_identity_bin", id)
		if v != "1" {
			t.Fatalf("distinct identity version %s", v)
		}
	}
}

func TestRecordVersionRealConcurrentWriters(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	_ = integrationAdminSession(t, app)
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, value := range []string{"first", "second"} {
		go func(value string) {
			<-start
			results <- policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"`+value+`"},"expected_version":"0"}`)
		}(value)
	}
	close(start)
	a, b := <-results, <-results
	if a.Code == 409 {
		a, b = b, a
	}
	assertMutationAffected(t, a)
	assertIntegrationErrorCode(t, b, 409, "record_version_conflict")
	_, v := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "1" {
		t.Fatalf("concurrent initialization advanced %s", v)
	}
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	id := mutationResponseID(t, policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/rows", `{"content":{"code":"race","label":"original"}}`))
	start = make(chan struct{})
	go func() {
		<-start
		results <- policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_add_items/rows/"+id, `{"content":{"label":"modified"},"expected_version":"1"}`)
	}()
	go func() {
		<-start
		results <- policyIntegrationRequest(t, app, "DELETE", "/api/v1/tables/mutation_add_items/rows/"+id, `{"expected_version":"1"}`)
	}()
	close(start)
	a, b = <-results, <-results
	if a.Code != 200 {
		a, b = b, a
	}
	assertMutationAffected(t, a)
	if b.Code != 409 && b.Code != 404 {
		t.Fatalf("competing mutation: %d %s", b.Code, b.Body.String())
	}
}

// Existing operation tests explicitly read a baseline before their ordinary
// writes. CAS/required-version tests use the raw request helper instead.
func versionedMutationRequest(t *testing.T, app *adminApplication, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	if (method == "PATCH" || method == "DELETE") && strings.Contains(path, "/rows/") {
		payload := map[string]any{}
		if body == "" || json.Unmarshal([]byte(body), &payload) == nil && payload != nil {
			if _, exists := payload["expected_version"]; !exists {
				parts := strings.SplitN(strings.TrimPrefix(path, "/api/v1/tables/"), "/rows/", 2)
				id, _ := url.PathUnescape(parts[1])
				query, _ := json.Marshal(map[string]any{"conditions": []any{map[string]string{"field": "id", "operator": "exact", "value": id}}})
				read := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/"+parts[0]+"/query", string(query))
				var result struct {
					Versions []string `json:"record_versions"`
				}
				_ = json.Unmarshal(read.Body.Bytes(), &result)
				version := "0"
				if len(result.Versions) == 1 {
					version = result.Versions[0]
				}
				payload["expected_version"] = version
				encoded, _ := json.Marshal(payload)
				body = string(encoded)
			}
		}
	}
	return policyIntegrationRequest(t, app, method, path, body)
}

func TestRecordVersionSchemaReadinessAndRestartableMigration(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	if _, err := db.Exec("DROP TABLE rcc_record_versions"); err != nil {
		t.Fatal(err)
	}
	if err := app.mysql.Ready(ctx); err == nil {
		t.Fatal("Admin became ready without record version storage")
	}
	migration, err := os.ReadFile("../../../deploy/mysql/migrations/009-record-versions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	assertMutationAffected(t, policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"preserved"},"expected_version":"0"}`))
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	_, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if version != "1" {
		t.Fatal("rerun reset versions")
	}
	if _, err := db.Exec("ALTER TABLE rcc_record_versions ENGINE=MyISAM"); err != nil {
		t.Fatal(err)
	}
	if err := app.mysql.Ready(ctx); err == nil {
		t.Fatal("Admin became ready with nontransactional version storage")
	}
}

func TestRecordVersionLosslessMaintenanceFloor(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	if _, err := db.Exec(`INSERT INTO rcc_record_versions VALUES('mutation_delete_parents',X'',9007199254740993)`); err != nil {
		t.Fatal(err)
	}
	_, v := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "9007199254740993" {
		t.Fatalf("rounded version %s", v)
	}
	stale := policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"stale"},"expected_version":"9007199254740992"}`)
	assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
	assertMutationAffected(t, policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"current"},"expected_version":"9007199254740993"}`))
	_, v = recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "9007199254740994" {
		t.Fatalf("advance %s", v)
	}
	// A maintenance window raises the entire table above every issued token.
	if _, err := db.Exec(`UPDATE rcc_record_versions SET lock_version=9007199254740995 WHERE table_name='mutation_delete_parents' AND record_key=X''`); err != nil {
		t.Fatal(err)
	}
	stale = policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_delete_parents/rows/1", `{"content":{"code":"stale"},"expected_version":"9007199254740994"}`)
	assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
	_, v = recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "9007199254740995" {
		t.Fatalf("maintenance floor %s", v)
	}
}

func TestRecordVersionSnapshotAndIndependentResources(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	for _, id := range []string{"1", "2"} {
		r := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/rows", `{"content":{"id":"`+id+`","code":"record-`+id+`","label":"before"}}`)
		if r.Code != 201 {
			t.Fatal(r.Body.String())
		}
	}
	schema, err := app.mysql.GetTableSchema(t.Context(), "mutation_add_items")
	if err != nil {
		t.Fatal(err)
	}
	query := domain.PageQuery{TableName: schema.Name, Columns: schema.Columns, Order: domain.QueryOrder{Field: "id", Direction: "ASC"}, PageNumber: 1, PageSize: 20}
	_, err = app.mysql.ExecuteQuerySnapshot(t.Context(), func(session application.QuerySnapshotSession) (domain.QueryResult, error) {
		first, err := session.ExecutePageQuery(t.Context(), query)
		if err != nil {
			return first, err
		}
		assertMutationAffected(t, policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_add_items/rows/1", `{"content":{"label":"after"},"expected_version":"1"}`))
		second, err := session.ExecutePageQuery(t.Context(), query)
		if err != nil {
			return second, err
		}
		if *second.Rows[0]["label"] != "before" || second.RecordVersions[0] != "1" || !reflect.DeepEqual(first, second) {
			t.Fatalf("mixed record snapshot: %+v", second)
		}
		return second, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	row, v := recordVersionRow(t, app, "mutation_add_items", "1")
	if *row["label"] != "after" || v != "2" {
		t.Fatal("fresh query missed committed pair")
	}
	locked, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	go func() {
		finished <- app.mysql.ExecuteMutationSnapshot(t.Context(), func(session application.MutationSnapshotSession) error {
			idColumn, _ := schema.Column("id")
			labelColumn, _ := schema.Column("label")
			if _, err := session.UpdateRow(t.Context(), domain.RowUpdate{TableName: schema.Name, IDColumn: idColumn, ID: uint64(1), ExpectedVersion: "2", Values: []domain.MutationValue{{Column: labelColumn, Value: "held"}}}); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-finished:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("row transaction did not start")
	}
	secondDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		secondDone <- policyIntegrationRequest(t, app, "PATCH", "/api/v1/tables/mutation_add_items/rows/2", `{"content":{"label":"independent"},"expected_version":"1"}`)
	}()
	select {
	case r := <-secondDone:
		assertMutationAffected(t, r)
	case <-time.After(3 * time.Second):
		t.Fatal("a different record waited for unrelated row transaction")
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestRecordVersionRejectsNonTransactionalBusinessWrite(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	if _, err := db.Exec(`CREATE TABLE nontransactional_items(id INT PRIMARY KEY,label VARCHAR(64)) ENGINE=MyISAM`); err != nil {
		t.Fatal(err)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "nontransactional_items", mutationPolicyFixture{AllowAdd: true})
	r := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/nontransactional_items/rows", `{"content":{"id":"1","label":"must not persist"}}`)
	if r.Code == 201 {
		t.Fatal("accepted a nontransactional write")
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM nontransactional_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("rejected ADD left a business side effect")
	}
}

func TestRecordVersionControlStorageFailureRollsBackBusinessWrites(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	// Fault injection alone uses the disposable database owner; production writes use the application account.
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	if _, err := owner.Exec(`CREATE TRIGGER reject_record_version BEFORE UPDATE ON rcc_record_versions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected storage fault'`); err != nil {
		t.Fatal(err)
	}
	r := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/rows", `{"content":{"id":"7","code":"failure","label":"must rollback"}}`)
	assertIntegrationErrorCode(t, r, 503, "mutation_unavailable")
	read := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/query", `{}`)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"rows":[]`) {
		t.Fatalf("failed ADD persisted: %s", read.Body.String())
	}
	if _, err := owner.Exec(`DROP TRIGGER reject_record_version`); err != nil {
		t.Fatal(err)
	}
	r = policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/rows", `{"content":{"id":"7","code":"failure","label":"recovered"}}`)
	if r.Code != 201 {
		t.Fatal(r.Body.String())
	}
	_, v := recordVersionRow(t, app, "mutation_add_items", "7")
	if v != "1" {
		t.Fatalf("failed ADD left version %s", v)
	}
	for _, method := range []string{"PATCH", "DELETE"} {
		body := `{"expected_version":"0"}`
		if method == "PATCH" {
			body = `{"content":{},"expected_version":"0"}`
		}
		r = policyIntegrationRequest(t, app, method, "/api/v1/tables/rcc_record_versions/rows/key", body)
		if r.Code != 403 {
			t.Fatalf("control table write allowed: %d", r.Code)
		}
	}
	r = policyIntegrationRequest(t, app, "POST", "/api/v1/tables/rcc_record_versions/query", `{}`)
	assertIntegrationErrorCode(t, r, 403, "protected_table")
}
