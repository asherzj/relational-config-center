//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
	driver "github.com/go-sql-driver/mysql"
)

// AC-048: persisted history is independent of current accounts, policy metadata
// and live business tables. All seven histories below come from actual HTTP
// workflow actions; no release document is fabricated or rewritten by the fixture.
func TestReleaseHistorySurvivesExecutableRestartAndExternalChanges(t *testing.T) {
	f := newMaintenanceFixture(t)
	deliveryExec(t, f.databaseOwner, `CREATE TABLE history_items(id VARCHAR(32) PRIMARY KEY,value TEXT,metadata JSON) ENGINE=InnoDB`)
	deliveryExec(t, f.databaseOwner, `INSERT INTO history_items VALUES('forward','旧值\r\n原样',NULL)`)
	enableMutationPolicy(t, f.app, "history_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	mysql := f.settings.MySQL
	settings := &driver.Config{User: mysql.User, Passwd: mysql.Password, Net: mysql.Network, Addr: mysql.Address, DBName: mysql.Database, ParseTime: true}
	binary := buildIntegrationAdmin(t)
	process := accountProcessCommand(t, binary, settings)
	process.ready(t)
	type actor struct {
		id, csrf string
		cookies  []*http.Cookie
	}
	register := func(name string) actor {
		cookies, csrf, data := processCredentials(t, process, "/api/v1/auth/register", fmt.Sprintf(`{"username":%q,"email":%q,"password":"history password long enough"}`, name, name+"@example.com"))
		var result struct {
			Account struct {
				ID string `json:"id"`
			} `json:"account"`
		}
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		return actor{result.Account.ID, csrf, cookies}
	}
	admin := register("history.admin")
	f.run(t, "", "grant-admin", "--id", admin.id)
	editor, reviewer, publisher, viewer := register("history.editor"), register("history.reviewer"), register("history.publisher"), register("history.viewer")
	request := func(a actor, method, path, body, key string, want int) []byte {
		status, _, data := process.requestWithKey(t, method, path, body, a.cookies, a.csrf, key)
		if status != want {
			t.Fatalf("%s %s: %d %s, want %d", method, path, status, data, want)
		}
		return data
	}
	for i, grant := range []struct {
		actor actor
		role  string
	}{{editor, "EDITOR"}, {publisher, "PUBLISHER"}} {
		request(admin, "PUT", "/api/v1/account-roles/"+grant.actor.id, fmt.Sprintf(`{"expected_version":"1","roles":[%q]}`, grant.role), fmt.Sprintf("history-role-%d", i), 200)
	}
	roleData := request(admin, "POST", "/api/v1/approval-roles", fmt.Sprintf(`{"name":"History reviewers","description":"","enabled":true,"member_ids":[%q]}`, reviewer.id), "history-reviewer-role", 201)
	var role approvalRoleResult
	if err := json.Unmarshal(roleData, &role); err != nil {
		t.Fatal(err)
	}
	request(admin, "PUT", "/api/v1/table-policies/history_items/approval-roles", fmt.Sprintf(`{"expected_version":"0","role_ids":[%q]}`, role.ID), "history-reviewer-assignment", 200)
	decode := func(data []byte) domain.ReleaseOrder {
		var order domain.ReleaseOrder
		if err := json.Unmarshal(data, &order); err != nil {
			t.Fatal(err)
		}
		return order
	}
	readOrderDetails := func(a actor, id string) domain.ReleaseOrder {
		path := "/api/v1/release-orders/" + id
		data := request(a, "GET", path, "", "", 200)
		var header domain.ReleaseHeader
		if err := json.Unmarshal(data, &header); err != nil {
			t.Fatal(err)
		}
		order := header.Workflow()
		order.Items = []domain.ReleaseItem{}
		for offset := 0; offset < header.ItemCount; {
			data = request(a, "GET", fmt.Sprintf("%s/details?expected_version=%s&offset=%d&limit=100", path, header.Version, offset), "", "", 200)
			var page domain.ReleaseDetailPage
			if err := json.Unmarshal(data, &page); err != nil {
				t.Fatal(err)
			}
			if page.OrderID != id || page.Version != header.Version || page.Offset != offset || len(page.Items) == 0 {
				t.Fatal("mixed history detail pages", string(data))
			}
			order.Items = append(order.Items, page.Items...)
			offset += len(page.Items)
		}
		return order
	}
	action := func(a actor, order domain.ReleaseOrder, name, reason string) domain.ReleaseOrder {
		body, _ := json.Marshal(map[string]string{"expected_version": order.Version, "reason": reason})
		// Submit and execute accept the expected version without an opinion.
		if name == "submit" || name == "execute" || name == "complete" {
			body, _ = json.Marshal(map[string]string{"expected_version": order.Version})
		}
		if name == "approve" || name == "reject" {
			data := request(a, "GET", "/api/v1/release-orders/"+order.ID, "", "", 200)
			var header domain.ReleaseHeader
			if err := json.Unmarshal(data, &header); err != nil {
				t.Fatal(err)
			}
			body, _ = json.Marshal(map[string]any{"expected_version": header.Version, "reason": reason, "confirmed_tables": header.ApprovalContext.ApprovableTables, "expected_approval_revision": header.ApprovalContext.Revision})
		}
		return decode(request(a, "POST", "/api/v1/release-orders/"+order.ID+"/"+name, string(body), order.ID+"-"+name, 200))
	}
	orders := map[string]domain.ReleaseOrder{}
	for _, state := range []string{"DRAFT", "PENDING_APPROVAL", "APPROVED", "REJECTED", "CANCELLED"} {
		payload := fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":"history_items","operation":"ADD","content":{"id":%q,"value":"","metadata":"null"}}]}`, strings.ToLower(state))
		order := decode(request(editor, "POST", "/api/v1/release-orders", payload, "history-create-"+state, 201))
		switch state {
		case "PENDING_APPROVAL":
			order = action(editor, order, "submit", "")
		case "APPROVED":
			order = action(editor, order, "submit", "")
			order = action(reviewer, order, "approve", "批准意见原样")
		case "REJECTED":
			order = action(editor, order, "submit", "")
			order = action(reviewer, order, "reject", "拒绝原因原样")
		case "CANCELLED":
			order = action(editor, order, "cancel", "取消原因原样")
		}
		orders[order.ID] = order
	}
	forward := decode(request(editor, "POST", "/api/v1/release-orders", `{"items":[{"content":{"metadata":"null","value":"发布后新值"},"expected_record_version":"0","id":"forward","operation":"MODIFY","table_name":"history_items"}],"title":"集成测试发布单"}`, "history-create-forward", 201))
	forward = action(editor, forward, "submit", "")
	forward = action(reviewer, forward, "approve", "正向批准意见")
	forward = action(publisher, forward, "execute", "")
	previewBytes := request(publisher, "POST", "/api/v1/release-orders/"+forward.ID+"/quick-rollback/preview", `{"expected_version":"4"}`, "", 200)
	var preview quickPreviewResponse
	if json.Unmarshal(previewBytes, &preview) != nil {
		t.Fatal("preview decode")
	}
	forward = decode(request(publisher, "POST", "/api/v1/release-orders/"+forward.ID+"/quick-rollback", quickRollbackBody("4", preview.Digest, "事后可选原因"), "history-restore", 200))
	if forward.State != "ROLLED_BACK" || len(forward.Executions) < 2 || len(forward.Executions) != 2 {
		t.Fatal("original rollback history missing")
	}
	orders[forward.ID] = forward
	completed := decode(request(editor, "POST", "/api/v1/release-orders", `{"items":[{"content":{"id":"completed","value":"done"},"operation":"ADD","table_name":"history_items"}],"title":"完结历史"}`, "history-completed", 201))
	completed = action(editor, completed, "submit", "")
	completed = action(reviewer, completed, "approve", "completed approval")
	completed = action(publisher, completed, "execute", "")
	completed = action(publisher, completed, "complete", "")
	orders[completed.ID] = completed

	states := map[string]bool{}
	for _, order := range orders {
		states[order.State] = true
		if order.ApplicantID != editor.id || len(order.Items) != 1 || len(order.Items[0].Fields) != 3 {
			t.Fatal("incomplete authored intent")
		}
		for _, event := range order.History {
			want := editor.id
			if event.Action == "APPROVE" || event.Action == "REJECT" {
				want = reviewer.id
			}
			if event.Action == "EXECUTE" || event.Action == "COMPLETE" || event.Action == "ROLLED_BACK" || event.Action == "QUICK_ROLLBACK" {
				want = publisher.id
			}
			if event.ActorID != want {
				t.Fatalf("%s actor: %s, want %s", event.Action, event.ActorID, want)
			}
		}
		if len(order.Executions) >= 1 && (order.Executions[0].ActorID != publisher.id || len(executionCommands(order, "PUBLICATION")) != 1) {
			t.Fatal("missing permanent publisher/command")
		}
	}
	if len(states) != 7 {
		t.Fatalf("missing historical state: %v", states)
	}
	savedFacts := map[string]string{}
	for _, table := range []string{"rcc_release_orders", "rcc_release_details", "rcc_release_executions", "rcc_release_requests", "rcc_release_targets", "rcc_release_table_references", "rcc_publication_commands", "rcc_refresh_notifications", "rcc_record_versions", "rcc_table_publications"} {
		savedFacts[table] = baselineRows(t, f.databaseOwner, "SELECT * FROM "+table)
	}
	assertSavedFacts := func() {
		t.Helper()
		for table, original := range savedFacts {
			if baselineRows(t, f.databaseOwner, "SELECT * FROM "+table) != original {
				t.Fatalf("immutable stored facts changed in %s", table)
			}
		}
	}
	for id, order := range orders {
		if order.State == "PENDING_APPROVAL" {
			qualified := readOrderDetails(reviewer, id).ApprovalContext
			if len(qualified.Tables) != 1 || qualified.Tables[0].Mode != "ROLE" || !qualified.Tables[0].CanApprove || !reflect.DeepEqual(qualified.ApprovableTables, []string{"history_items"}) {
				t.Fatalf("independent member did not hold current qualification: %+v", qualified)
			}
		}
	}
	for i, a := range []actor{editor, reviewer, publisher} {
		data := request(a, "PATCH", "/api/v1/auth/profile", fmt.Sprintf(`{"display_name":"改名用户%d"}`, i), "", 200)
		if !strings.Contains(string(data), a.id) {
			t.Fatal("profile changed permanent identity")
		}
		f.run(t, fmt.Sprintf("corrected-%d@example.com", i), "set-email", "--id", a.id, "--email-stdin")
		f.run(t, "", "disable", "--id", a.id)
	}
	// Catalog operations remain separate from publication approval. Metadata/lifecycle
	// edits and removal of the live table cannot erase saved intent or final rows.
	var mutationCode string
	if err := f.databaseOwner.QueryRow(`SELECT mutation_policy_code FROM rcc_table_policies WHERE table_name='history_items'`).Scan(&mutationCode); err != nil {
		t.Fatal(err)
	}
	request(admin, "POST", "/api/v1/mutation-policies/"+mutationCode+"/deprecate", "", "", 200)
	var currentPolicy struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(request(admin, "GET", "/api/v1/table-policies/history_items", "", "", 200), &currentPolicy); err != nil {
		t.Fatal(err)
	}
	disabledPolicy := request(admin, "POST", "/api/v1/table-policies/history_items/disable", `{"expected_version":"`+currentPolicy.Version+`"}`, "history-table-disable", 200)
	var requestActor, requestOperation, requestKey, requestDigest string
	var requestResult []byte
	if err := f.databaseOwner.QueryRow(`SELECT actor_id,operation,request_key,HEX(digest),result FROM rcc_release_requests WHERE actor_id=? AND operation='table-policy:disable' AND request_key='history-table-disable'`, admin.id).Scan(&requestActor, &requestOperation, &requestKey, &requestDigest, &requestResult); err != nil {
		t.Fatal(err)
	}
	priorRequestRows := baselineRows(t, f.databaseOwner, fmt.Sprintf(`SELECT * FROM rcc_release_requests WHERE NOT(actor_id=0x%x AND operation='table-policy:disable' AND request_key='history-table-disable')`, []byte(admin.id)))
	t.Logf("catalog request SQL delta: original_rows_unchanged=%t actor=%s operation=%s key=%s digest=%s stored_result=%s HTTP_response=%s", priorRequestRows == savedFacts["rcc_release_requests"], requestActor, requestOperation, requestKey, requestDigest, requestResult, disabledPolicy)
	if priorRequestRows != savedFacts["rcc_release_requests"] {
		t.Fatal("table-policy disable changed an existing request row or added another request")
	}
	var persistedPolicy domain.TablePolicy
	var disableResponse struct {
		TableName string `json:"table_name"`
		Version   string `json:"version"`
		Enabled   bool   `json:"enabled"`
		Modifier  string `json:"modifier"`
	}
	if err := json.Unmarshal(requestResult, &persistedPolicy); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(disabledPolicy, &disableResponse); err != nil {
		t.Fatal(err)
	}
	if requestActor != admin.id || requestOperation != "table-policy:disable" || requestKey != "history-table-disable" || persistedPolicy.TableName != "history_items" || persistedPolicy.Enabled || persistedPolicy.Version == 0 || persistedPolicy.Modifier != admin.id || disableResponse.TableName != persistedPolicy.TableName || disableResponse.Enabled != persistedPolicy.Enabled || disableResponse.Version != fmt.Sprint(persistedPolicy.Version) || disableResponse.Modifier != persistedPolicy.Modifier {
		t.Fatal("the new catalog request does not belong to the successful disable response")
	}
	// This explicit catalog write appends its durable result to the shared request
	// table. Every pre-existing row was compared above; restarts and reads must
	// now preserve the complete set, including this one verified new result.
	savedFacts["rcc_release_requests"] = baselineRows(t, f.databaseOwner, "SELECT * FROM rcc_release_requests")
	deliveryExec(t, f.databaseOwner, `UPDATE rcc_mutation_policies SET name='维护后规则名称' WHERE code=?`, mutationCode)
	deliveryExec(t, f.databaseOwner, `DROP TABLE history_items`)
	// Explicit account maintenance may legitimately reconcile pending recipients.
	// Subsequent migrations, restarts and reads must preserve every resulting column.
	savedNotifications := baselineRows(t, f.databaseOwner, "SELECT * FROM rcc_approval_notifications")
	// Rerun the documented restartable 008–012 migrations on populated controls.
	applyRoleMigration(t, f.databaseOwner)
	ownerSettings := settings.Clone()
	ownerSettings.User, ownerSettings.MultiStatements = "root", true
	migrationOwner := deliveryDB(t, ownerSettings)
	for _, file := range []string{"009-record-versions.sql", "010-release-drafts.sql", "011-release-targets.sql", "012-publication.sql", "015-original-order-executions.sql"} {
		migration, err := os.ReadFile("../../../deploy/mysql/migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		deliveryExec(t, migrationOwner, string(migration))
	}
	process.stop(t)
	process = accountProcessCommand(t, binary, settings)
	process.ready(t)
	assertSavedFacts()
	for id, before := range orders {
		after := readOrderDetails(viewer, id)
		context := after.ApprovalContext
		if len(context.Revision) != 64 || context.Revision == before.ApprovalContext.Revision || len(context.ApprovableTables) != 0 {
			t.Fatalf("viewer qualification was not recomputed independently of saved history: %+v", context)
		}
		if before.State == "CANCELLED" {
			if len(context.Tables) != 0 {
				t.Fatal("cancelled draft fabricated submitted responsibility", context)
			}
		} else {
			mode := "COMPLETED"
			if before.State == "PENDING_APPROVAL" || before.State == "DRAFT" {
				mode = "ADMIN"
			}
			if len(context.Tables) != 1 || context.Tables[0].TableName != "history_items" || context.Tables[0].Mode != mode || context.Tables[0].CanApprove {
				t.Fatalf("current %s qualification: %+v", before.State, context)
			}
		}
		if before.State == "PENDING_APPROVAL" {
			fallback := readOrderDetails(admin, id).ApprovalContext
			if len(fallback.Tables) != 1 || fallback.Tables[0].Mode != "ADMIN" || !fallback.Tables[0].CanApprove || !reflect.DeepEqual(fallback.ApprovableTables, []string{"history_items"}) || fallback.Revision == context.Revision {
				t.Fatalf("disabled member did not yield independent ADMIN fallback: %+v", fallback)
			}
		}
		// Only live eligibility differs; every saved business field remains compared.
		expected := before
		expected.ApprovalContext = context
		if !reflect.DeepEqual(expected, after) {
			t.Fatalf("history changed after account/schema/migration/restart: %s", id)
		}
		remove := request(admin, "DELETE", "/api/v1/release-orders/"+id, "", "", 400)
		if !strings.Contains(string(remove), `"code":"method_not_allowed"`) {
			t.Fatalf("unexpected deletion contract: %s", remove)
		}
		forged := request(admin, "PUT", "/api/v1/release-orders/"+id, fmt.Sprintf(`{"expected_version":%q,"history":[],"applicant_id":%q}`, after.Version, admin.id), "history-rewrite-"+id, 400)
		if !strings.Contains(string(forged), `"code":"invalid_request"`) {
			t.Fatalf("unexpected history rewrite contract: %s", forged)
		}
		unchanged := readOrderDetails(viewer, id)
		if !reflect.DeepEqual(after, unchanged) {
			t.Fatalf("negative history operations changed persisted order: %s", id)
		}
	}
	var listing struct {
		Orders []domain.ReleaseOrderSummary `json:"orders"`
	}
	data := request(viewer, "GET", "/api/v1/release-orders?table_name=history_items&limit=100", "", "", 200)
	if json.Unmarshal(data, &listing) != nil || len(listing.Orders) != len(orders) {
		t.Fatalf("history list lost orders: %s", data)
	}
	// Authorization still applies to replay: the surviving viewer reads history,
	// and cannot assume the disabled publisher's old successful request identity.
	request(viewer, "POST", "/api/v1/release-orders/"+forward.ID+"/execute", `{"expected_version":"3"}`, forward.ID+"-execute", 403)
	assertSavedFacts()
	if baselineRows(t, f.databaseOwner, "SELECT * FROM rcc_approval_notifications") != savedNotifications {
		t.Fatal("migration, restart or read/rejected write changed notification progress")
	}
	process.stop(t)
	for _, secret := range []string{"history password long enough", editor.cookies[0].Value, reviewer.csrf, "发布后新值"} {
		if strings.Contains(process.output.String(), secret) {
			t.Fatal("history process log disclosed sensitive content")
		}
	}
}
