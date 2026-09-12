//go:build integration

package main

import (
	"database/sql"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func awaitFlowDatabaseCondition(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		var ready int
		if err := db.QueryRow(query, args...).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if ready > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("database boundary not reached: %s", query)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func receiveFlowResponse(t *testing.T, responses <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case r := <-responses:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request did not finish after database gate opened")
	}
	return nil
}

const flowLockWaitSQL = `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME=?`

// #100 AC-022: real database gates arrange overlap, rather than assuming that
// simultaneous goroutines reached the configuration read at the same time.
func TestReleaseFlowAC022ConcurrentTemplateAndAssociationSnapshots(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "ordinary_release_v1", true)
	admin := integrationAdminSession(t, app)
	cookies, csrf := admin.Result().Cookies(), sessionCSRF(t, admin)
	request := func(method, path, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, method, path, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	}

	t.Run("template_update_commits_while_draft_retains_complete_old_snapshot", func(t *testing.T) {
		gate, err := db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer gate.Close()
		var acquired int
		if err = gate.QueryRowContext(ctx, `SELECT GET_LOCK('flow_snapshot_gate',5)`).Scan(&acquired); err != nil || acquired != 1 {
			t.Fatal("gate lock", err)
		}
		defer gate.ExecContext(ctx, `SELECT RELEASE_LOCK('flow_snapshot_gate')`)
		deliveryExec(t, db, `CREATE TRIGGER pause_flow_insert BEFORE INSERT ON rcc_release_orders FOR EACH ROW BEGIN SET @flow_entered=GET_LOCK('flow_snapshot_entered',0); SET @flow_wait=GET_LOCK('flow_snapshot_gate',12); SET @flow_done=RELEASE_LOCK('flow_snapshot_entered'); SET @flow_open=RELEASE_LOCK('flow_snapshot_gate'); END`)
		defer deliveryExec(t, db, `DROP TRIGGER IF EXISTS pause_flow_insert`)
		held, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Rollback()
		var id uint64
		if err = held.QueryRowContext(ctx, `SELECT id FROM rcc_release_templates WHERE code='ordinary_release_v1' FOR UPDATE`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		updateBody := strings.TrimSuffix(strings.ReplaceAll(standardReleaseTemplateBody, "按表审批", "并发新版审批"), "}") + `,"expected_version":"1"}`
		changed := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			changed <- request("PUT", "/api/v1/release-templates/ordinary_release_v1", updateBody, "flow-concurrent-template")
		}()
		awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_release_templates")
		created := make(chan *httptest.ResponseRecorder, 1)
		go func() { created <- request("POST", "/api/v1/release-orders", flowDraftBody, "flow-concurrent-create") }()
		awaitFlowDatabaseCondition(t, db, `SELECT IF(IS_USED_LOCK('flow_snapshot_entered') IS NULL,0,1)`)
		if err = held.Commit(); err != nil {
			t.Fatal(err)
		}
		if r := receiveFlowResponse(t, changed); r.Code != 200 {
			t.Fatalf("concurrent template edit: %d %s", r.Code, r.Body)
		}
		if err = gate.QueryRowContext(ctx, `SELECT RELEASE_LOCK('flow_snapshot_gate')`).Scan(&acquired); err != nil || acquired != 1 {
			t.Fatal("release gate", err)
		}
		saved := flowResponse(t, receiveFlowResponse(t, created), 201)
		if len(saved.TableFlows) != 2 {
			t.Fatal("missing old snapshot", saved)
		}
		for _, flow := range saved.TableFlows {
			if flow.TemplateVersion != "1" || flow.Nodes[0].Name != "按表审批" || flow.AssociationVersion != "1" {
				t.Fatal("mixed old/new template snapshot", flow)
			}
		}
		replay := flowResponse(t, request("POST", "/api/v1/release-orders", flowDraftBody, "flow-concurrent-create"), 201)
		if replay.ID != saved.ID || !reflect.DeepEqual(replay.TableFlows, saved.TableFlows) {
			t.Fatal("retry regenerated changed template", replay)
		}
		assertIntegrationErrorCode(t, request("POST", "/api/v1/release-orders", strings.Replace(flowDraftBody, "各表保存流程", "different intent", 1), "flow-concurrent-create"), 409, "idempotency_conflict")
		current := flowResponse(t, request("GET", "/api/v1/release-orders/"+saved.ID, "", ""), 200)
		if !reflect.DeepEqual(current.TableFlows, saved.TableFlows) {
			t.Fatal("read replaced acknowledged snapshot", current)
		}
		fresh := flowResponse(t, request("POST", "/api/v1/release-orders", flowDraftBody, "flow-current-template-create"), 201)
		for _, flow := range fresh.TableFlows {
			if flow.TemplateVersion != "2" || flow.Nodes[0].Name != "并发新版审批" {
				t.Fatal("new instance ignored committed template", flow)
			}
		}
	})

	t.Run("association_switch_precedes_waiting_draft_as_one_snapshot", func(t *testing.T) {
		held, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Rollback()
		var id uint64
		if err = held.QueryRowContext(ctx, `SELECT id FROM rcc_release_templates WHERE code='default_standard_v1' FOR UPDATE`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		changed := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			changed <- request("PUT", "/api/v1/table-policies/policy_alpha/release-templates/STANDARD", associationBody("default_standard_v1", "1", true), "flow-concurrent-association")
		}()
		awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_release_templates")
		created := make(chan *httptest.ResponseRecorder, 1)
		go func() { created <- request("POST", "/api/v1/release-orders", flowDraftBody, "flow-association-create") }()
		awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_table_policies")
		if err = held.Commit(); err != nil {
			t.Fatal(err)
		}
		if r := receiveFlowResponse(t, changed); r.Code != 200 {
			t.Fatalf("association switch: %d %s", r.Code, r.Body)
		}
		saved := flowResponse(t, receiveFlowResponse(t, created), 201)
		if len(saved.TableFlows) != 2 {
			t.Fatal("missing switched flow", saved)
		}
		alpha, beta := saved.TableFlows[0], saved.TableFlows[1]
		if alpha.TemplateCode != "default_standard_v1" || alpha.TemplateVersion != "1" || alpha.AssociationVersion != "2" || alpha.Nodes[0].Name != "按表审批" || beta.TemplateCode != "ordinary_release_v1" || beta.TemplateVersion != "2" || beta.Nodes[0].Name != "并发新版审批" {
			t.Fatalf("mixed association/template snapshot: %+v", saved.TableFlows)
		}
		bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
		replay := flowResponse(t, request("POST", "/api/v1/release-orders", flowDraftBody, "flow-association-create"), 201)
		if !reflect.DeepEqual(replay.TableFlows, saved.TableFlows) {
			t.Fatal("association replay generated another instance", replay)
		}
		t.Logf("complete snapshot retained with alpha %s@%s / association %s", alpha.TemplateCode, alpha.TemplateVersion, alpha.AssociationVersion)
	})
}
