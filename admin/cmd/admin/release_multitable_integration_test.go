//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// AC-001/007/009: interleaving tables is part of the approved intent. Grouping
// parent writes would delete the referenced old parent before its child moves.
func TestMultitablePublicationPreservesGlobalOrderAndOriginalResults(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	deliveryExec(t, db, `CREATE TABLE z_release_parents(id INT PRIMARY KEY, label VARCHAR(80))`)
	deliveryExec(t, db, `CREATE TABLE a_release_children(id INT PRIMARY KEY, parent_id INT NOT NULL, label VARCHAR(80), FOREIGN KEY(parent_id) REFERENCES z_release_parents(id))`)
	deliveryExec(t, db, `CREATE TABLE m_release_generated(id INT AUTO_INCREMENT PRIMARY KEY, label VARCHAR(80))`)
	deliveryExec(t, db, `INSERT INTO z_release_parents VALUES(1,'original parent')`)
	deliveryExec(t, db, `INSERT INTO a_release_children VALUES(1,1,'original child')`)
	for _, table := range []string{"z_release_parents", "a_release_children", "m_release_generated"} {
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	}
	body := `{"title":"有序多表迁移","items":[{"table_name":"z_release_parents","operation":"ADD","content":{"id":"2","label":"new parent"}},{"table_name":"a_release_children","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"parent_id":"2"}},{"table_name":"z_release_parents","operation":"DELETE","id":"1","expected_record_version":"0","content":{}},{"table_name":"m_release_generated","operation":"ADD","content":{"label":"generated"}}]}`
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), body, "multitable-order")
	deliveryExec(t, db, `UPDATE a_release_children SET label='external drift' WHERE id=1`)
	drift := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "multitable-publish")
	assertIntegrationErrorCode(t, drift, 409, "record_version_conflict")
	if !strings.Contains(drift.Body.String(), `"item_index":1`) {
		t.Fatalf("lost global failure position: %s", drift.Body.String())
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM z_release_parents WHERE id=2`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 0, `SELECT COUNT(*) FROM rcc_table_publications`: 0})
	deliveryExec(t, db, `UPDATE a_release_children SET label='original child' WHERE id=1`)
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "multitable-publish"), 200)
	wantTables := []string{"z_release_parents", "a_release_children", "z_release_parents", "m_release_generated"}
	for index, command := range published.Publication.Commands {
		if command.TableName != wantTables[index] || command.RecordVersion != "1" || command.TableVersion != "1" {
			t.Fatalf("command %d: %+v", index, command)
		}
	}
	if len(published.Executions) != 1 || len(published.Executions[0].TableVersions) != 3 {
		t.Fatalf("execution: %+v", published.Executions)
	}
	row, version := recordVersionRow(t, app, "a_release_children", "1")
	if *row["parent_id"] != "2" || version != "1" {
		t.Fatal(row, version)
	}
	conflict := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"actual generated identity","items":[{"table_name":"m_release_generated","operation":"MODIFY","id":"1","expected_record_version":"1","content":{"label":"blocked"}}]}`, "multitable-generated-conflict")
	assertIntegrationErrorCode(t, conflict, 409, "release_target_conflict")
	deliveryExec(t, db, `ALTER TABLE a_release_children MODIFY label VARCHAR(81)`)
	changedSchema := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "")
	assertIntegrationErrorCode(t, changedSchema, 409, "release_frozen_changed")
	if !strings.Contains(changedSchema.Body.String(), `"item_index":2`) {
		t.Fatalf("restoration failure must use reverse position: %s", changedSchema.Body.String())
	}
	deliveryExec(t, db, `ALTER TABLE a_release_children MODIFY label VARCHAR(80)`)
	var preview quickPreviewResponse
	previewResponse := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "")
	if previewResponse.Code != 200 || json.Unmarshal(previewResponse.Body.Bytes(), &preview) != nil {
		t.Fatalf("preview: %s", previewResponse.Body)
	}
	restored := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, ""), "multitable-rollback"), 200)
	if restored.ID != published.ID || restored.State != "ROLLED_BACK" || !reflect.DeepEqual(restored.Items, published.Items) || !reflect.DeepEqual(restored.Publication, published.Publication) {
		t.Fatal("original facts changed")
	}
	for index, command := range restored.Rollback.Commands {
		if command.TableName != wantTables[len(wantTables)-1-index] || command.RecordVersion != "2" || command.TableVersion != "2" {
			t.Fatalf("reverse command %d: %+v", index, command)
		}
	}
	row, version = recordVersionRow(t, app, "a_release_children", "1")
	if *row["parent_id"] != "1" || version != "2" {
		t.Fatal(row, version)
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_orders`: 1, `SELECT COUNT(*) FROM rcc_release_details`: 4, `SELECT COUNT(*) FROM rcc_release_executions`: 2, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 6, `SELECT COUNT(*) FROM rcc_release_targets`: 0, `SELECT COUNT(*) FROM rcc_release_table_references`: 0, `SELECT COUNT(*) FROM z_release_parents WHERE id=1 AND label='original parent'`: 1, `SELECT COUNT(*) FROM m_release_generated`: 0})
	for _, table := range wantTables {
		response := releaseRequest(t, app, "GET", "/api/v1/release-orders?table_name="+table, "", "")
		var list struct {
			Orders []domain.ReleaseOrderSummary `json:"orders"`
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &list) != nil || len(list.Orders) != 1 || list.Orders[0].ID != published.ID {
			t.Fatalf("table filter: %s", response.Body)
		}
	}
}

// AC-006/014: pagination is a save/view concern; the entire 1,000-detail order
// freezes and executes together, including equal IDs in different tables.
func TestMultitableThousandPagedDetailsExecuteAsOneOrder(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for _, table := range []string{"thousand_release_a", "thousand_release_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, label VARCHAR(80))")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	}
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"千条多表草稿","items":[]}`, "thousand-multi-empty")
	order := rollbackOrderResponse(t, created, 201)
	path := "/api/v1/release-orders/" + order.ID
	for page := 0; page < 10; page++ {
		items := make([]any, 100)
		for offset := range items {
			index := page*100 + offset
			table := "thousand_release_a"
			if index%2 == 1 {
				table = "thousand_release_b"
			}
			items[offset] = map[string]any{"table_name": table, "operation": "ADD", "content": map[string]string{"id": fmt.Sprint(index/2 + 1), "label": fmt.Sprintf("item-%d", index+1)}}
		}
		body, _ := json.Marshal(map[string]any{"title": order.Title, "expected_version": order.Version, "changes": map[string]any{"upserts": items}})
		order = rollbackOrderResponse(t, releaseRequest(t, app, "PUT", path, string(body), fmt.Sprintf("thousand-multi-page-%d", page)), 200)
	}
	if len(order.Items) != 1000 || len(order.TableNames) != 2 {
		t.Fatal("paged save shrank order")
	}
	tooMany := releaseRequest(t, app, "PUT", path, fmt.Sprintf(`{"title":"千条多表草稿","expected_version":%q,"changes":{"upserts":[{"table_name":"thousand_release_a","operation":"ADD","content":{"id":"501","label":"rejected"}}]}}`, order.Version), "thousand-multi-over-limit")
	assertIntegrationErrorCode(t, tooMany, 422, "release_item_limit")
	submitted := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/submit", fmt.Sprintf(`{"expected_version":%q}`, order.Version), "thousand-multi-submit"), 200)
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, publicationFixtureReviewer(t, app), "POST", path+"/approve", fmt.Sprintf(`{"expected_version":%q,"reason":"review all pages"}`, submitted.Version), "thousand-multi-approve"), 200)
	started := time.Now()
	published := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", fmt.Sprintf(`{"expected_version":%q}`, approved.Version), "thousand-multi-execute"), 200)
	t.Logf("1000 multitable publish %s", time.Since(started))
	if len(published.Publication.Commands) != 1000 || len(published.Executions) != 1 || published.Executions[0].ItemCount != 1000 {
		t.Fatal("partial whole-order publication")
	}
	for i, command := range published.Publication.Commands {
		want := "thousand_release_a"
		if i%2 == 1 {
			want = "thousand_release_b"
		}
		if command.TableName != want || command.ID != fmt.Sprint(i/2+1) || command.Sequence != fmt.Sprint(i/2+1) {
			t.Fatalf("global position %d: %+v", i, command)
		}
	}
	var preview quickPreviewResponse
	response := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", fmt.Sprintf(`{"expected_version":%q}`, published.Version), "")
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &preview) != nil {
		t.Fatalf("preview %d %.500s", response.Code, response.Body.String())
	}
	started = time.Now()
	restored := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/quick-rollback", quickRollbackBody(published.Version, preview.Digest, ""), "thousand-multi-rollback"), 200)
	t.Logf("1000 multitable rollback %s", time.Since(started))
	if len(restored.Rollback.Commands) != 1000 || !reflect.DeepEqual(restored.Items, published.Items) {
		t.Fatal("incomplete inverse")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM thousand_release_a`: 0, `SELECT COUNT(*) FROM thousand_release_b`: 0, `SELECT COUNT(*) FROM rcc_publication_commands`: 2000, `SELECT COUNT(*) FROM rcc_release_executions`: 2, `SELECT COUNT(*) FROM rcc_record_versions WHERE lock_version=2`: 1000, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 4, `SELECT COUNT(*) FROM rcc_release_targets`: 0})
}

// Waiting for a later table guard must not reuse a snapshot established by the
// earlier table. The external transaction models publication before target release.
func TestMultitableDraftWaitsForAllGuardsBeforeAnySnapshot(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	settings := integrationConfig(driver)
	settings.MySQL.MaxOpenConnections = 1
	settings.MySQL.MaxIdleConnections = 1
	app, err := newApplication(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for _, table := range []string{"snapshot_release_a", "snapshot_release_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY,label VARCHAR(80))")
		deliveryExec(t, db, "INSERT INTO "+table+" VALUES(1,'before')")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowModify: true})
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var locked string
	if err = tx.QueryRow(`SELECT table_name FROM rcc_table_policies WHERE table_name='snapshot_release_b' FOR UPDATE`).Scan(&locked); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE snapshot_release_b SET label='committed while waiting' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	responses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		responses <- releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"guard all tables","items":[{"table_name":"snapshot_release_a","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"A proposal"}},{"table_name":"snapshot_release_b","operation":"MODIFY","id":"1","expected_record_version":"0","content":{"label":"B proposal"}}]}`, "snapshot-multitable-save")
	}()
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner := deliveryDB(t, &ownerDriver)
	deadline := time.Now().Add(3 * time.Second)
	for {
		var waiting int
		if err := owner.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='rcc_table_policies'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case response := <-responses:
			t.Fatalf("draft crossed table guard: %d %.500s", response.Code, response.Body.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("draft did not reach the locked second table with a one-connection pool")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case response := <-responses:
		order := rollbackOrderResponse(t, response, 201)
		if *order.Items[1].Before["label"] != "committed while waiting" {
			t.Fatal("saved stale second-table snapshot")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("draft did not resume")
	}
	// mode 0 permits distinct physical tables. A CI catalog key must not lend
	// another table's policy to a table that has no exact assignment.
	deliveryExec(t, db, `CREATE TABLE SNAPSHOT_RELEASE_A(id INT PRIMARY KEY,label VARCHAR(80))`)
	otherCase := releaseRequest(t, app, "POST", "/api/v1/release-orders", `{"title":"no borrowed policy","items":[{"table_name":"SNAPSHOT_RELEASE_A","operation":"ADD","content":{"id":"2","label":"must reject"}}]}`, "snapshot-case-policy")
	if otherCase.Code < 400 {
		t.Fatalf("borrowed other physical table policy: %d", otherCase.Code)
	}

}

// AC-006: supported LONGTEXT values exceed both the former field threshold and
// the former whole-order JSON budget, through the real HTTP lifecycle.
func TestMultitableLargeValuesThroughHTTPLifecycle(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	db := deliveryDB(t, driver)
	for _, table := range []string{"large_release_a", "large_release_b"} {
		deliveryExec(t, db, "CREATE TABLE "+table+"(id INT PRIMARY KEY, payload LONGTEXT NOT NULL)")
		enableMutationPolicy(t, app, table, mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	}
	value := strings.Repeat("宽值ABC", 10000)
	items := make([]any, 130)
	for i := range items {
		table := "large_release_a"
		if i%2 == 1 {
			table = "large_release_b"
		}
		items[i] = map[string]any{"table_name": table, "operation": "ADD", "content": map[string]string{"id": fmt.Sprint(i/2 + 1), "payload": value}}
	}
	body, _ := json.Marshal(map[string]any{"title": "large multitable values", "items": items})
	if len(body) <= 8<<20 {
		t.Fatal("fixture must exceed former whole-order limit")
	}
	created := releaseRequest(t, app, "POST", "/api/v1/release-orders", string(body), "large-multitable-create")
	if created.Code != 201 {
		t.Fatalf("large save: status %d, bytes %d, %.500s", created.Code, created.Body.Len(), created.Body.String())
	}
	var order domain.ReleaseOrder
	if json.Unmarshal(created.Body.Bytes(), &order) != nil || len(order.Items) != 130 {
		t.Fatal("large saved details")
	}
	path := "/api/v1/release-orders/" + order.ID
	for _, action := range []struct{ name, version string }{{"submit", "1"}, {"approve", "2"}, {"execute", "3"}} {
		payload := fmt.Sprintf(`{"expected_version":%q}`, action.version)
		response := releaseRequest(t, app, "POST", path+"/"+action.name, payload, "large-multitable-"+action.name)
		if action.name == "approve" {
			response = releaseActorRequest(t, app, publicationFixtureReviewer(t, app), "POST", path+"/approve", `{"expected_version":"2","reason":"reviewed whole order"}`, "large-multitable-independent-approve")
		}
		if response.Code != 200 {
			t.Fatalf("large %s: status %d, bytes %d, %.500s", action.name, response.Code, response.Body.Len(), response.Body.String())
		}
	}
	read := releaseRequest(t, app, "GET", path, "", "")
	if read.Code != 200 || json.Unmarshal(read.Body.Bytes(), &order) != nil || len(order.Publication.Commands) != 130 {
		t.Fatal("read complete actual results")
	}
	for _, command := range order.Publication.Commands {
		found := false
		for _, field := range command.Final.Fields {
			if field.Name == "payload" {
				found = field.Value != nil && *field.Value == value
			}
		}
		if !found {
			t.Fatal("large actual result truncated")
		}
	}
	previewResponse := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "")
	var preview quickPreviewResponse
	if previewResponse.Code != 200 || json.Unmarshal(previewResponse.Body.Bytes(), &preview) != nil {
		t.Fatalf("large preview: status %d, %.500s", previewResponse.Code, previewResponse.Body.String())
	}
	restored := releaseRequest(t, app, "POST", path+"/quick-rollback", quickRollbackBody("4", preview.Digest, ""), "large-multitable-rollback")
	if restored.Code != 200 {
		t.Fatalf("large restore: status %d, %.500s", restored.Code, restored.Body.String())
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM large_release_a`: 0, `SELECT COUNT(*) FROM large_release_b`: 0, `SELECT COUNT(*) FROM rcc_release_targets`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 2})
}
