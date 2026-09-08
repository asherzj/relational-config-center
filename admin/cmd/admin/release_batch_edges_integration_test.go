//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

func batchEdgeApplication(t *testing.T, setup ...string) (*adminApplication, *sql.DB) {
	t.Helper()
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/006-mutation-fixture.sql", "testdata/010-record-identity-fixture.sql")
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	for _, statement := range setup {
		deliveryExec(t, db, statement)
	}
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	return app, db
}

func batchEdgeOrder(t *testing.T, response *httptest.ResponseRecorder, status int) domain.ReleaseOrder {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP %d, want %d: %s", response.Code, status, response.Body)
	}
	var order domain.ReleaseOrder
	if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil || order.ID == "" {
		t.Fatalf("invalid order: %v %s", err, response.Body)
	}
	return order
}

func batchEdgeIndex(t *testing.T, response *httptest.ResponseRecorder, index int) {
	t.Helper()
	var issue struct {
		Error struct {
			ItemIndex *int `json:"item_index"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &issue); err != nil || issue.Error.ItemIndex == nil || *issue.Error.ItemIndex != index {
		t.Fatalf("want zero-based item_index %d: %s", index, response.Body)
	}
}

func batchEdgeCounts(t *testing.T, db *sql.DB, expected map[string]int) {
	t.Helper()
	for query, want := range expected {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d, want %d, error %v", query, got, want, err)
		}
	}
}

// AC-038: validation rejects the complete replacement, including an invalid
// later member and all three kinds of MySQL identity equivalence.
func TestReleaseBatchEdgeDraftValidation(t *testing.T) {
	app, db := batchEdgeApplication(t,
		`INSERT INTO record_identity_ci VALUES('Résumé','original')`,
		`INSERT INTO record_identity_pad VALUES('key','original')`)
	for _, table := range []string{"mutation_delete_parents", "mutation_add_items", "record_identity_ci", "record_identity_pad"} {
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	}
	for _, tc := range []struct{ name, table, original, replacement, code string }{
		{"invalid-later-field", "mutation_delete_parents", `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"kept"}}`, `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"discarded"}},{"operation":"ADD","content":{"not_a_column":"bad"}}`, "invalid_mutation_content"},
		{"numeric-existing", "mutation_delete_parents", `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"kept"}}`, `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"discarded"}},{"operation":"DELETE","id":"01","expected_record_version":"0","content":{}}`, "release_duplicate_target"},
		{"numeric-missing-add", "mutation_add_items", `{"operation":"ADD","content":{"code":"kept","label":"kept"}}`, `{"operation":"ADD","expected_record_version":"0","content":{"id":"99","code":"first","label":"first"}},{"operation":"ADD","expected_record_version":"0","content":{"id":"099","code":"second","label":"second"}}`, "release_duplicate_target"},
		{"collation-existing", "record_identity_ci", `{"operation":"MODIFY","id":"Résumé","expected_record_version":"0","content":{"label":"kept"}}`, `{"operation":"MODIFY","id":"Résumé","expected_record_version":"0","content":{"label":"discarded"}},{"operation":"DELETE","id":"RESUME","expected_record_version":"0","content":{}}`, "release_duplicate_target"},
		{"collation-missing-add", "record_identity_ci", `{"operation":"ADD","content":{"id":"other","label":"kept"}}`, `{"operation":"ADD","expected_record_version":"0","content":{"id":"Café","label":"first"}},{"operation":"ADD","expected_record_version":"0","content":{"id":"CAFE","label":"second"}}`, "release_duplicate_target"},
		{"pad-space-existing", "record_identity_pad", `{"operation":"MODIFY","id":"key","expected_record_version":"0","content":{"label":"kept"}}`, `{"operation":"DELETE","id":"key ","expected_record_version":"0","content":{}},{"operation":"MODIFY","id":"key","expected_record_version":"0","content":{"label":"discarded"}}`, "release_duplicate_target"},
		{"pad-space-missing-add", "record_identity_pad", `{"operation":"ADD","content":{"id":"other","label":"kept"}}`, `{"operation":"ADD","expected_record_version":"0","content":{"id":"missing","label":"first"}},{"operation":"ADD","expected_record_version":"0","content":{"id":"missing ","label":"second"}}`, "release_duplicate_target"},
		{"cross-table", "mutation_delete_parents", `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"kept"}}`, `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"discarded"}},{"table_name":"mutation_add_items","operation":"ADD","content":{"code":"cross","label":"cross"}}`, "release_cross_table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := releaseRequest(t, app, "POST", "/api/v1/release-orders", fmt.Sprintf(`{"table_name":%q,"items":[%s]}`, tc.table, tc.original), tc.name+"-create")
			order := batchEdgeOrder(t, created, 201)
			path := "/api/v1/release-orders/" + order.ID
			response := releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"table_name":%q,"expected_version":"1","items":[%s]}`, tc.table, tc.replacement), tc.name+"-replace")
			assertIntegrationErrorCode(t, response, 422, tc.code)
			batchEdgeIndex(t, response, 1)
			current := releaseRequest(t, app, "GET", path, "", "")
			if current.Code != 200 || current.Body.String() != created.Body.String() {
				t.Fatalf("invalid replacement altered draft: %s", current.Body)
			}
		})
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM mutation_add_items`:                                 0,
		`SELECT COUNT(*) FROM record_identity_ci WHERE label='original'`:          1,
		`SELECT COUNT(*) FROM record_identity_pad WHERE label='original'`:         1,
		`SELECT COUNT(*) FROM rcc_release_targets`:                                0,
		`SELECT COUNT(*) FROM rcc_record_versions`:                                0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'edit:%'`: 0,
	})
}

func TestReleaseBatchEdgeRequestLimitsPreserveDraft(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	item := `{"operation":"ADD","content":{"code":"kept","label":"kept"}}`
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"table_name":"mutation_add_items","items":[`+item+`]}`, "limits-create")
	order := batchEdgeOrder(t, created, 201)
	path := "/api/v1/release-orders/" + order.ID
	for _, tc := range []struct {
		name, items, code string
		status            int
	}{
		{"empty", "", "release_item_limit", 422},
		{"1001", strings.Repeat(item+",", 1000) + item, "release_item_limit", 422},
		{"body", `{"operation":"ADD","content":{"code":"large","label":"` + strings.Repeat("x", 1<<20) + `"}}`, "request_body_too_large", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"table_name":"mutation_add_items","expected_version":"1","items":[` + tc.items + `]}`
			response := releaseRequest(t, app, "PUT", path, body, "limits-"+tc.name)
			assertIntegrationErrorCode(t, response, tc.status, tc.code)
			current := releaseRequest(t, app, "GET", path, "", "")
			if current.Code != 200 || current.Body.String() != created.Body.String() {
				t.Fatalf("limit failure altered draft: %s", current.Body)
			}
		})
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'edit:%'`: 0, `SELECT COUNT(*) FROM mutation_add_items`: 0})
}

// AC-039: each competing batch has an exclusive target plus a shared target.
// The loser must leave no reservation, including targets encountered earlier.
func TestReleaseBatchEdgeOverlappingSubmissions(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	paths := make([]string, 2)
	for i, ids := range [][]int{{10, 20}, {30, 20}} {
		body := fmt.Sprintf(`{"table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"id":"%d","code":"exclusive-%d","label":"draft"}},{"operation":"ADD","content":{"id":"%d","code":"shared","label":"draft"}}]}`, ids[0], i, ids[1])
		order := batchEdgeOrder(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, fmt.Sprintf("overlap-create-%d", i)), 201)
		paths[i] = "/api/v1/release-orders/" + order.ID
	}
	session := integrationAdminSession(t, app)
	cookies, csrf := session.Result().Cookies(), sessionCSRF(t, session)
	type outcome struct {
		index    int
		response *httptest.ResponseRecorder
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for i := range paths {
		go func(i int) {
			<-start
			results <- outcome{i, accountRequestFrom(app, "POST", paths[i]+"/submit", `{"expected_version":"1"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("overlap-submit-%d", i)})}
		}(i)
	}
	close(start)
	winner, loser := -1, -1
	for range 2 {
		result := <-results
		if result.response.Code == 200 {
			if winner != -1 {
				t.Fatal("both overlapping batches submitted")
			}
			winner = result.index
		} else {
			assertIntegrationErrorCode(t, result.response, 409, "release_target_conflict")
			batchEdgeIndex(t, result.response, 1)
			loser = result.index
		}
	}
	if winner == -1 || loser == -1 {
		t.Fatalf("winner %d loser %d", winner, loser)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_targets`: 2,
		fmt.Sprintf(`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='%s'`, strings.TrimPrefix(paths[loser], "/api/v1/release-orders/")): 0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'submit:%'`:                                                              1,
		`SELECT COUNT(*) FROM mutation_add_items`:                                                                                                0,
		`SELECT COUNT(*) FROM rcc_record_versions`:                                                                                               0,
	})
	losingOrder := batchEdgeOrder(t, releaseRequest(t, app, "GET", paths[loser], "", ""), 200)
	if losingOrder.State != "DRAFT" || losingOrder.Version != "1" {
		t.Fatal("failed submit changed order")
	}
	batchEdgeOrder(t, releaseRequest(t, app, "POST", paths[winner]+"/cancel", `{"expected_version":"2","reason":"release complete target set"}`, "overlap-cancel-winner"), 200)
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 0})
	// Reuse the failed original request key to prove no durable failed request remains.
	batchEdgeOrder(t, releaseRequest(t, app, "POST", paths[loser]+"/submit", `{"expected_version":"1"}`, fmt.Sprintf("overlap-submit-%d", loser)), 200)
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 2})
	batchEdgeOrder(t, releaseRequest(t, app, "POST", paths[loser]+"/cancel", `{"expected_version":"2","reason":"done"}`, "overlap-cancel-loser"), 200)
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_targets`: 0})
}

// AC-039: the stale final member invalidates earlier MODIFY/DELETE/ADD work.
func TestReleaseBatchEdgeStaleMemberRollsBack(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'ten','old'),(30,'thirty','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	publishedFixtureCommand(t, publicationFixtureRequest(t, app, "ADD", "mutation_add_items", "", `{"content":{"id":"20","code":"twenty","label":"old"}}`))
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"must-rollback"}},{"operation":"DELETE","id":"30","expected_record_version":"0","content":{}},{"operation":"ADD","content":{"code":"must-rollback","label":"new"}},{"operation":"MODIFY","id":"20","expected_record_version":"1","content":{"label":"stale"}}]}`, "stale-batch")
	before := releaseRequest(t, app, "GET", path, "", "")
	// Simulate a supported external maintenance writer that advances the known
	// version together with its business change; only id 20 has a version row.
	deliveryExec(t, db, `UPDATE mutation_add_items SET label='newer' WHERE id=20`)
	deliveryExec(t, db, `UPDATE rcc_record_versions SET lock_version=2 WHERE table_name='mutation_add_items'`)
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "stale-batch-execute")
	assertIntegrationErrorCode(t, response, 409, "record_version_conflict")
	batchEdgeIndex(t, response, 3)
	current := releaseRequest(t, app, "GET", path, "", "")
	if current.Body.String() != before.Body.String() {
		t.Fatalf("stale execute changed approval/history: %s", current.Body)
	}
	for _, tc := range []struct{ id, label, version string }{{"10", "old", "0"}, {"20", "newer", "2"}, {"30", "old", "0"}} {
		row, version := recordVersionRow(t, app, "mutation_add_items", tc.id)
		if row["label"] == nil || *row["label"] != tc.label || version != tc.version {
			t.Fatalf("id %s row %v version %s", tc.id, row, version)
		}
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM mutation_add_items`:                                                3,
		`SELECT COUNT(*) FROM rcc_release_targets`:                                               3,
		`SELECT COUNT(*) FROM rcc_publication_commands`:                                          1,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`:                                         1,
		`SELECT COUNT(*) FROM rcc_record_versions`:                                               1,
		`SELECT table_version FROM rcc_table_publications WHERE table_name='mutation_add_items'`: 1,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE request_key='stale-batch-execute'`:      0,
	})
}

// AC-038/039: real CHECK and UNIQUE failures after earlier business DML roll
// back every row and publication artifact while retaining approval and targets.
func TestReleaseBatchEdgeMidwayConstraintRollback(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'ten','old'),(20,'twenty','old'),(30,'thirty','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	for _, tc := range []struct {
		name, code, label, errorCode string
		status                       int
	}{{"check", "bad-check", "rollback", "mutation_unavailable", 503}, {"unique", "thirty", "valid", "duplicate_key", 409}} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"table_name":"mutation_add_items","items":[{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"must-rollback"}},{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}},{"operation":"ADD","content":{"code":"early-add","label":"valid"}},{"operation":"ADD","content":{"code":%q,"label":%q}}]}`, tc.code, tc.label)
			path := approvePublication(t, app, reviewer, body, "constraint-"+tc.name)
			before := releaseRequest(t, app, "GET", path, "", "")
			response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "constraint-execute-"+tc.name)
			assertIntegrationErrorCode(t, response, tc.status, tc.errorCode)
			batchEdgeIndex(t, response, 3)
			current := releaseRequest(t, app, "GET", path, "", "")
			if current.Body.String() != before.Body.String() {
				t.Fatalf("failed execute changed approval/history: %s", current.Body)
			}
			batchEdgeCounts(t, db, map[string]int{
				`SELECT COUNT(*) FROM mutation_add_items`:                                    3,
				`SELECT COUNT(*) FROM mutation_add_items WHERE label='old'`:                  3,
				`SELECT COUNT(*) FROM rcc_release_targets`:                                   2,
				`SELECT COUNT(*) FROM rcc_record_versions`:                                   0,
				`SELECT COUNT(*) FROM rcc_publication_commands`:                              0,
				`SELECT COUNT(*) FROM rcc_table_publications`:                                0,
				`SELECT COUNT(*) FROM rcc_refresh_notifications`:                             0,
				`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'execute:%'`: 0,
			})
			batchEdgeOrder(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"3","reason":"constraint failure reviewed"}`, "constraint-cancel-"+tc.name), 200)
		})
	}
}

// AC-040: IDs come from actual MySQL allocation even with increment/offset;
// byte-identical independent requests create distinct orders and business rows.
func TestReleaseBatchEdgeAutoIncrementAndIndependentRequests(t *testing.T) {
	app, db := batchEdgeApplication(t,
		`CREATE TABLE batch_auto_items(id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,label VARCHAR(64) NOT NULL) ENGINE=InnoDB`,
		`SET GLOBAL auto_increment_increment=3`, `SET GLOBAL auto_increment_offset=2`)
	enableMutationPolicy(t, app, "batch_auto_items", mutationPolicyFixture{AllowAdd: true})
	body := `{"table_name":"batch_auto_items","items":[{"operation":"ADD","content":{"label":"first"}},{"operation":"ADD","content":{"label":"second"}},{"operation":"ADD","content":{"label":"third"}}]}`
	reviewer := publicationFixtureReviewer(t, app)
	paths := []string{approvePublication(t, app, reviewer, body, "auto-one"), approvePublication(t, app, reviewer, body, "auto-two")}
	if paths[0] == paths[1] {
		t.Fatal("independent content-equivalent requests merged")
	}
	seen := map[string]bool{}
	var previous uint64
	for i, path := range paths {
		key := fmt.Sprintf("auto-execute-%d", i)
		response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, key)
		order := batchEdgeOrder(t, response, 200)
		if order.State != "SUCCEEDED" || order.Publication == nil || len(order.Publication.Commands) != 3 {
			t.Fatal(response.Body)
		}
		for j, command := range order.Publication.Commands {
			id, err := strconv.ParseUint(command.ID, 10, 64)
			if err != nil || id%3 != 2 || (previous != 0 && id-previous != 3) || seen[command.ID] || command.RecordVersion != "1" {
				t.Fatalf("wrong actual ID/version: %+v", command)
			}
			previous, seen[command.ID] = id, true
			var label string
			if err := db.QueryRow(`SELECT label FROM batch_auto_items WHERE id=?`, command.ID).Scan(&label); err != nil || label != []string{"first", "second", "third"}[j] {
				t.Fatalf("item %d actual row %s label %q error %v", j, command.ID, label, err)
			}
			finalID, err := command.Final.RecordID()
			if err != nil || finalID != command.ID {
				t.Fatalf("result identity %s final %s error %v", command.ID, finalID, err)
			}
		}
		// Treat the initial response as lost; original-key recovery is exact.
		replay := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, key)
		if replay.Code != 200 || replay.Body.String() != response.Body.String() {
			t.Fatalf("recovery differs: %d %s", replay.Code, replay.Body)
		}
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM batch_auto_items`:                                                6,
		`SELECT COUNT(*) FROM batch_auto_items WHERE label='first'`:                            2,
		`SELECT COUNT(*) FROM rcc_release_orders`:                                              2,
		`SELECT COUNT(*) FROM rcc_publication_commands`:                                        6,
		`SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=1`:                        6,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`:                                       2,
		`SELECT COUNT(*) FROM rcc_release_targets`:                                             6,
		`SELECT table_version FROM rcc_table_publications WHERE table_name='batch_auto_items'`: 2,
	})
}

// AC-040: distinct unknown-ID batches may both be approved; a common business
// unique key is arbitrated by MySQL during execution, with no partial loser.
func TestReleaseBatchEdgeIndependentUniqueConflict(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	reviewer := publicationFixtureReviewer(t, app)
	paths := []string{}
	for i := range 2 {
		body := fmt.Sprintf(`{"table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"exclusive-%d","label":"valid"}},{"operation":"ADD","content":{"code":"shared-business-key","label":"valid"}}]}`, i)
		paths = append(paths, approvePublication(t, app, reviewer, body, fmt.Sprintf("independent-%d", i)))
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders WHERE state='APPROVED'`: 2, `SELECT COUNT(*) FROM rcc_release_targets`: 0})
	session := integrationAdminSession(t, app)
	cookies, csrf := session.Result().Cookies(), sessionCSRF(t, session)
	type outcome struct {
		index    int
		response *httptest.ResponseRecorder
	}
	results := make(chan outcome, 2)
	start := make(chan struct{})
	for i := range paths {
		go func(i int) {
			<-start
			results <- outcome{i, accountRequestFrom(app, "POST", paths[i]+"/execute", `{"expected_version":"3"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("independent-execute-%d", i)})}
		}(i)
	}
	close(start)
	winner, loser := -1, -1
	for range 2 {
		result := <-results
		if result.response.Code == 200 {
			if winner != -1 {
				t.Fatal("both unique-conflicting batches succeeded")
			}
			winner = result.index
			order := batchEdgeOrder(t, result.response, 200)
			if order.State != "SUCCEEDED" || order.Publication == nil || len(order.Publication.Commands) != 2 {
				t.Fatal(result.response.Body)
			}
		} else {
			assertIntegrationErrorCode(t, result.response, 409, "duplicate_key")
			batchEdgeIndex(t, result.response, 1)
			loser = result.index
		}
	}
	if winner == -1 || loser == -1 {
		t.Fatalf("winner %d loser %d", winner, loser)
	}
	order := batchEdgeOrder(t, releaseRequest(t, app, "GET", paths[loser], "", ""), 200)
	if order.State != "APPROVED" || order.Version != "3" || order.Publication != nil {
		t.Fatal("unique loser lost approval")
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM mutation_add_items`:                                                2,
		fmt.Sprintf(`SELECT COUNT(*) FROM mutation_add_items WHERE code='exclusive-%d'`, winner): 1,
		fmt.Sprintf(`SELECT COUNT(*) FROM mutation_add_items WHERE code='exclusive-%d'`, loser):  0,
		`SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=1`:                          2,
		`SELECT COUNT(*) FROM rcc_publication_commands`:                                          2,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`:                                         1,
		`SELECT COUNT(*) FROM rcc_release_targets`:                                               2,
		`SELECT table_version FROM rcc_table_publications WHERE table_name='mutation_add_items'`: 1,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'execute:%'`:             1,
	})
}

// Existing ENUM keys were supported before batching. Preserve their MySQL
// identity for MODIFY/DELETE without broadening missing-key ADD support.
func TestReleaseBatchExistingEnumPrimaryKeys(t *testing.T) {
	app, db := batchEdgeApplication(t, `CREATE TABLE batch_enum_keys(id ENUM('alpha','beta','gamma') COLLATE utf8mb4_0900_ai_ci PRIMARY KEY,label VARCHAR(64) NOT NULL) ENGINE=InnoDB`, `INSERT INTO batch_enum_keys VALUES('alpha','original'),('beta','delete')`)
	enableMutationPolicy(t, app, "batch_enum_keys", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"table_name":"batch_enum_keys","items":[{"operation":"MODIFY","id":"ALPHA","expected_record_version":"0","content":{"label":"changed"}},{"operation":"DELETE","id":"beta","expected_record_version":"0","content":{}}]}`, "enum-batch")
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "enum-execute")
	order := batchEdgeOrder(t, response, 200)
	if order.Publication == nil || len(order.Publication.Commands) != 2 || order.Publication.Commands[0].ID != "alpha" || order.Publication.Commands[1].ID != "beta" || !order.Publication.Commands[1].Final.Deleted {
		t.Fatalf("wrong ENUM results: %s", response.Body)
	}
	replay := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "enum-execute")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("ENUM replay changed")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM batch_enum_keys WHERE id='alpha' AND label='changed'`: 1, `SELECT COUNT(*) FROM batch_enum_keys`: 1, `SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=1`: 2, `SELECT COUNT(*) FROM rcc_publication_commands`: 2, `SELECT COUNT(*) FROM rcc_release_targets`: 2})
	missing := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"table_name":"batch_enum_keys","items":[{"operation":"ADD","content":{"id":"gamma","label":"unsupported"}}]}`, "enum-missing")
	assertIntegrationErrorCode(t, missing, 422, "release_snapshot_unsupported")
}

func TestReleaseBatchCopyInvalidItemIsLocated(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_delete_parents(id,code) VALUES(2,'copy-second')`)
	enableMutationPolicy(t, app, "mutation_delete_parents", mutationPolicyFixture{AllowModify: true})
	first := `{"operation":"MODIFY","id":"1","expected_record_version":"0","content":{"code":"first"}}`
	second := `{"operation":"MODIFY","id":"2","expected_record_version":"0","content":{"code":"second"}}`
	body := `{"table_name":"mutation_delete_parents","items":[` + first + `,` + second + `]}`
	created := batchEdgeOrder(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "copy-batch-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	cancelled := releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"rework"}`, "copy-batch-cancel")
	batchEdgeOrder(t, cancelled, 200)
	for i, invalid := range []string{strings.Replace(second, `"second"`, `"changed"`, 1), strings.Replace(second, `"expected_record_version":"0"`, `"expected_record_version":"invalid"`, 1)} {
		response := releaseRequest(t, app, "POST", path+"/copy", `{"expected_version":"2","confirmed":true,"items":[`+first+`,`+invalid+`]}`, fmt.Sprintf("copy-batch-invalid-%d", i))
		assertIntegrationErrorCode(t, response, 422, "release_invalid")
		batchEdgeIndex(t, response, 1)
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 1, `SELECT COUNT(*) FROM rcc_release_requests WHERE operation LIKE 'copy:%'`: 0})
	if current := releaseRequest(t, app, "GET", path, "", ""); current.Body.String() != cancelled.Body.String() {
		t.Fatal("invalid copy changed source")
	}
	copied := batchEdgeOrder(t, releaseRequest(t, app, "POST", path+"/copy", `{"expected_version":"2","confirmed":true,"items":[`+first+`,`+second+`]}`, "copy-batch-valid"), 201)
	if len(copied.Items) != 2 || copied.CopiedFromID != created.ID {
		t.Fatal("valid batch copy incomplete")
	}
}
