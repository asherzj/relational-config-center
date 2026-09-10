//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
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
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
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
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	missing := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"missing"}}`)
	assertIntegrationErrorCode(t, missing, http.StatusUnprocessableEntity, "record_version_required")
	for _, value := range []string{`"01"`, `"-1"`, `"18446744073709551616"`} {
		invalid := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"invalid"},"expected_version":`+value+`}`)
		assertIntegrationErrorCode(t, invalid, 422, "record_version_invalid")
	}
	missingDelete := publicationFixtureRequest(t, app, "DELETE", "mutation_delete_parents", "1", "")
	assertIntegrationErrorCode(t, missingDelete, 422, "record_version_required")
	wrongType := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"numeric"},"expected_version":0}`)
	assertIntegrationErrorCode(t, wrongType, 400, "invalid_request")
	updated := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "01", `{"content":{"code":"winner"},"expected_version":"0"}`)
	assertMutationAffected(t, updated)
	stale := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"loser"},"expected_version":"0"}`)
	assertIntegrationErrorCode(t, stale, http.StatusConflict, "record_version_conflict")
	deleted := publicationFixtureRequest(t, app, "DELETE", "mutation_delete_parents", "1", `{"expected_version":"0"}`)
	assertIntegrationErrorCode(t, deleted, http.StatusConflict, "record_version_conflict")
	row, version := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if version != "1" || *row["code"] != "winner" {
		t.Fatalf("CAS failed: %v %s", row, version)
	}
}

func TestRecordVersionAddDeleteRecreateAndRollback(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/006-mutation-fixture.sql")
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	add := func() {
		t.Helper()
		response := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"7","code":"aba","label":"original"}}`)
		if response.Code != http.StatusOK {
			t.Fatalf("add: %d %s", response.Code, response.Body.String())
		}
	}
	add()
	_, version := recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "1" {
		t.Fatalf("new record version = %s", version)
	}
	failed := publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", "7", `{"content":{"label":"rollback"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "mutation_unavailable")
	row, version := recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "1" || *row["label"] != "original" {
		t.Fatalf("failed write advanced state: %v %s", row, version)
	}
	assertMutationAffected(t, publicationFixtureRequest(t, app, "DELETE", "mutation_add_items", "7", `{"expected_version":"1"}`))
	missing := publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", "7", `{"content":{"label":"gone"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "mutation_row_not_found")
	add()
	_, version = recordVersionRow(t, app, "mutation_add_items", "7")
	if version != "3" {
		t.Fatalf("recreated record version = %s", version)
	}
	stale := publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", "7", `{"content":{"label":"stale"},"expected_version":"1"}`)
	assertIntegrationErrorCode(t, stale, http.StatusConflict, "record_version_conflict")
}

func TestRecordVersionMySQLPrimaryKeyEquivalence(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/010-record-identity-fixture.sql")
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
				r := publicationFixtureRequest(t, app, "ADD", test.table, "", string(body))
				if r.Code != 200 {
					t.Fatalf("add: %d %s", r.Code, r.Body.String())
				}
			}
			add(test.first)
			_, version := recordVersionRow(t, app, test.table, test.equivalent)
			if version != "1" {
				t.Fatalf("equivalent version %s", version)
			}
			assertMutationAffected(t, publicationFixtureRequest(t, app, "DELETE", test.table, test.equivalent, `{"expected_version":"1"}`))
			add(test.recreated)
			_, version = recordVersionRow(t, app, test.table, test.equivalent)
			if version != "3" {
				t.Fatalf("identity reset: %s", version)
			}
			stale := publicationFixtureRequest(t, app, "MODIFY", test.table, test.first, `{"content":{"label":"stale"},"expected_version":"1"}`)
			assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
		})
	}
	enableMutationPolicy(t, app, "record_identity_bin", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	for _, id := range []string{"A", "a", "a "} {
		body, _ := json.Marshal(map[string]any{"content": map[string]string{"id": id, "label": "distinct"}})
		r := publicationFixtureRequest(t, app, "ADD", "record_identity_bin", "", string(body))
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
		_, v := recordVersionRow(t, app, "record_identity_bin", id)
		if v != "1" {
			t.Fatalf("distinct identity version %s", v)
		}
	}
}

func TestRecordVersionRealConcurrentWriters(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	assertRace := func(table, baseline string, winner, loser *httptest.ResponseRecorder, committed int) domain.PublicationCommand {
		t.Helper()
		command := publishedFixtureCommand(t, winner)
		var failure struct{ Error struct{ Code string } }
		if json.Unmarshal(loser.Body.Bytes(), &failure) != nil {
			t.Fatal(loser.Body)
		}
		// Another submitted order can reserve the target before the winner
		// commits. After commit, the same fixed baseline is stale (or deleted).
		valid := loser.Code == 409 && (failure.Error.Code == "release_target_conflict" || failure.Error.Code == "record_version_conflict") || command.Operation == "DELETE" && loser.Code == 404 && failure.Error.Code == "mutation_row_not_found"
		if !valid {
			t.Fatalf("competing publication: %d %s", loser.Code, loser.Body)
		}
		rows, err := db.Query("SELECT document FROM rcc_release_orders WHERE table_name=? AND state<>'COMPLETED'", table)
		if err != nil {
			t.Fatal(err)
		}
		var drafts []domain.ReleaseOrder
		losers := 0
		for rows.Next() {
			var document []byte
			var draft domain.ReleaseOrder
			if err := rows.Scan(&document); err != nil || json.Unmarshal(document, &draft) != nil {
				t.Fatalf("read losing draft: %v", err)
			}
			losers++
			if len(draft.Items) != 1 || draft.Items[0].ExpectedRecordVersion != baseline || draft.Publication != nil {
				t.Fatalf("loser changed its baseline or published: %+v", draft)
			}
			switch draft.State {
			case "DRAFT":
				if draft.Version != "1" || len(draft.History) != 1 || draft.History[0].Action != "CREATE" {
					t.Fatalf("submission refusal changed draft: %+v", draft)
				}
				drafts = append(drafts, draft)
			case "CANCELLED":
				// The real fixture verifies APPROVED v3 after an execute failure,
				// then explicitly cancels it. Preserve that distinct history.
				if failure.Error.Code == "release_target_conflict" || draft.Version != "4" || draft.Frozen == nil || len(draft.History) != 4 {
					t.Fatalf("invalid execute-failure cleanup: %+v", draft)
				}
				for i, action := range []string{"CREATE", "SUBMIT", "APPROVE", "CANCEL"} {
					if draft.History[i].Action != action {
						t.Fatalf("loser history action: %+v", draft.History)
					}
				}
			default:
				t.Fatalf("race left an in-flight loser: %+v", draft)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if losers > 1 || failure.Error.Code == "release_target_conflict" && len(drafts) != 1 {
			t.Fatalf("target refusal must retain exactly its unsubmitted draft: %d", len(drafts))
		}
		for _, draft := range drafts {
			cancelled := releaseRequest(t, app, "POST", "/api/v1/release-orders/"+draft.ID+"/cancel", `{"expected_version":"1","reason":"concurrent loser cleanup"}`, "race-cleanup-"+draft.ID)
			var result domain.ReleaseOrder
			if cancelled.Code != 200 || json.Unmarshal(cancelled.Body.Bytes(), &result) != nil || result.State != "CANCELLED" || result.Version != "2" || result.Publication != nil {
				t.Fatalf("loser cleanup: %d %s", cancelled.Code, cancelled.Body)
			}
		}
		for _, query := range []string{"SELECT COUNT(*) FROM rcc_publication_commands WHERE table_name=?", "SELECT COUNT(*) FROM rcc_refresh_notifications WHERE table_name=?", "SELECT COUNT(*) FROM rcc_release_orders WHERE table_name=? AND state='COMPLETED'", "SELECT table_version FROM rcc_table_publications WHERE table_name=?", "SELECT command_cursor FROM rcc_table_publications WHERE table_name=?", "SELECT lock_version FROM rcc_record_versions WHERE table_name=? AND LENGTH(record_key)=32"} {
			var n int
			if err := db.QueryRow(query, table).Scan(&n); err != nil || n != committed {
				t.Fatalf("race must commit once, got %d %v: %s", n, err, query)
			}
		}
		var targets int
		if err := db.QueryRow("SELECT COUNT(*) FROM rcc_release_targets WHERE table_name=?", table).Scan(&targets); err != nil || targets != 0 {
			t.Fatalf("race left targets: %d %v", targets, err)
		}
		return command
	}
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true, AllowDelete: true})
	_ = integrationAdminSession(t, app)
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, value := range []string{"first", "second"} {
		go func(value string) {
			<-start
			results <- publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"`+value+`"},"expected_version":"0"}`)
		}(value)
	}
	close(start)
	a, b := <-results, <-results
	if a.Code == 409 {
		a, b = b, a
	}
	assertRace("mutation_delete_parents", "0", a, b, 1)
	_, v := recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "1" {
		t.Fatalf("concurrent initialization advanced %s", v)
	}
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	id := mutationResponseID(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"code":"race","label":"original"}}`))
	start = make(chan struct{})
	go func() {
		<-start
		results <- publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", id, `{"content":{"label":"modified"},"expected_version":"1"}`)
	}()
	go func() {
		<-start
		results <- publicationFixtureRequest(t, app, "DELETE", "mutation_add_items", id, `{"expected_version":"1"}`)
	}()
	close(start)
	a, b = <-results, <-results
	if a.Code != 200 {
		a, b = b, a
	}
	command := assertRace("mutation_add_items", "1", a, b, 2)
	var remaining int
	if err := db.QueryRow("SELECT COUNT(*) FROM mutation_add_items WHERE id=?", id).Scan(&remaining); err != nil || remaining != map[bool]int{true: 0, false: 1}[command.Final.Deleted] {
		t.Fatalf("winner business state disagrees with command: %d %v", remaining, err)
	}
}

func TestRecordVersionSchemaReadinessAndRestartableMigration(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
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
	assertMutationAffected(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"preserved"},"expected_version":"0"}`))
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
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
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
	stale := publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"stale"},"expected_version":"9007199254740992"}`)
	assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
	assertMutationAffected(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"current"},"expected_version":"9007199254740993"}`))
	_, v = recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "9007199254740994" {
		t.Fatalf("advance %s", v)
	}
	// A maintenance window raises the entire table above every issued token.
	if _, err := db.Exec(`UPDATE rcc_record_versions SET lock_version=9007199254740995 WHERE table_name='mutation_delete_parents' AND record_key=X''`); err != nil {
		t.Fatal(err)
	}
	stale = publicationFixtureRequest(t, app, "MODIFY", "mutation_delete_parents", "1", `{"content":{"code":"stale"},"expected_version":"9007199254740994"}`)
	assertIntegrationErrorCode(t, stale, 409, "record_version_conflict")
	_, v = recordVersionRow(t, app, "mutation_delete_parents", "1")
	if v != "9007199254740995" {
		t.Fatalf("maintenance floor %s", v)
	}
}

func TestRecordVersionSnapshotAndIndependentResources(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	rootDriver := *driver
	rootDriver.User = "root"
	owner := deliveryDB(t, &rootDriver)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true})
	for _, id := range []string{"1", "2"} {
		r := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"`+id+`","code":"record-`+id+`","label":"before"}}`)
		if r.Code != 200 {
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
		assertMutationAffected(t, publicationFixtureRequest(t, app, "MODIFY", "mutation_add_items", "1", `{"content":{"label":"after"},"expected_version":"1"}`))
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
	// Actual publication locks progress per table. Two different records of
	// that table serialize, while another table can finish independently.
	reviewer := publicationFixtureReviewer(t, app)
	firstPath := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"1","expected_record_version":"2","content":{"label":"held"}}]}`, "resource-first")
	secondPath := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"2","expected_record_version":"1","content":{"label":"ordered"}}]}`, "resource-second")
	enableMutationPolicy(t, app, "mutation_supplied_id_items", mutationPolicyFixture{AllowAdd: true})
	otherPath := approvePublication(t, app, reviewer, `{"title":"集成测试发布单","table_name":"mutation_supplied_id_items","items":[{"operation":"ADD","content":{"id":"other","label":"independent"}}]}`, "resource-other")
	holder, err := owner.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	var name string
	if err := holder.QueryRow(`SELECT table_name FROM rcc_table_publications WHERE table_name='mutation_add_items' FOR UPDATE`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	session := integrationAdminSession(t, app)
	cookies, csrf := session.Result().Cookies(), sessionCSRF(t, session)
	completed := make(chan *httptest.ResponseRecorder, 2)
	for i, path := range []string{firstPath, secondPath} {
		go func(i int, path string) {
			completed <- accountRequestFrom(app, "POST", path+"/execute", `{"expected_version":"3"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("resource-execute-%d", i)})
		}(i, path)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting int
		err := owner.QueryRow(`SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='rcc_table_publications'`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real publications did not reach per-table lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	independent := releaseRequest(t, app, "POST", otherPath+"/execute", `{"expected_version":"3"}`, "resource-independent")
	publishedFixtureCommand(t, independent)
	select {
	case response := <-completed:
		t.Fatalf("same-table publication passed held progress lock: %d", response.Code)
	default:
	}
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		publishedFixtureCommand(t, <-completed)
	}

}

func TestRecordVersionRejectsNonTransactionalBusinessWrite(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
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
	r := publicationFixtureRequest(t, app, "ADD", "nontransactional_items", "", `{"content":{"id":"1","label":"must not persist"}}`)
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
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
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
	r := publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"7","code":"failure","label":"must rollback"}}`)
	assertIntegrationErrorCode(t, r, 503, "mutation_unavailable")
	read := policyIntegrationRequest(t, app, "POST", "/api/v1/tables/mutation_add_items/query", `{}`)
	if read.Code != 200 || !strings.Contains(read.Body.String(), `"rows":[]`) {
		t.Fatalf("failed ADD persisted: %s", read.Body.String())
	}
	if _, err := owner.Exec(`DROP TRIGGER reject_record_version`); err != nil {
		t.Fatal(err)
	}
	r = publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"7","code":"failure","label":"recovered"}}`)
	if r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	_, v := recordVersionRow(t, app, "mutation_add_items", "7")
	if v != "1" {
		t.Fatalf("failed ADD left version %s", v)
	}
	for _, operation := range []string{"ADD", "MODIFY", "DELETE"} {
		r = publicationFixtureRequest(t, app, operation, "rcc_record_versions", "key", `{"content":{},"expected_version":"0"}`)
		assertIntegrationErrorCode(t, r, 403, "protected_table")
	}
	r = policyIntegrationRequest(t, app, "POST", "/api/v1/tables/rcc_record_versions/query", `{}`)
	assertIntegrationErrorCode(t, r, 403, "protected_table")
}
