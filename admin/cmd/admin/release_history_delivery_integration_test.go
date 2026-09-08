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
	}{{editor, "EDITOR"}, {reviewer, "APPROVER"}, {publisher, "PUBLISHER"}} {
		request(admin, "PUT", "/api/v1/account-roles/"+grant.actor.id, fmt.Sprintf(`{"expected_version":"1","roles":[%q]}`, grant.role), fmt.Sprintf("history-role-%d", i), 200)
	}
	decode := func(data []byte) domain.ReleaseOrder {
		var order domain.ReleaseOrder
		if err := json.Unmarshal(data, &order); err != nil {
			t.Fatal(err)
		}
		return order
	}
	action := func(a actor, order domain.ReleaseOrder, name, reason string) domain.ReleaseOrder {
		body, _ := json.Marshal(map[string]string{"expected_version": order.Version, "reason": reason})
		// Submit and execute accept the expected version without an opinion.
		if name == "submit" || name == "execute" || name == "complete" {
			body, _ = json.Marshal(map[string]string{"expected_version": order.Version})
		}
		return decode(request(a, "POST", "/api/v1/release-orders/"+order.ID+"/"+name, string(body), order.ID+"-"+name, 200))
	}
	orders := map[string]domain.ReleaseOrder{}
	for _, state := range []string{"DRAFT", "PENDING_APPROVAL", "APPROVED", "REJECTED", "CANCELLED"} {
		payload := fmt.Sprintf(`{"table_name":"history_items","items":[{"operation":"ADD","content":{"id":%q,"value":"","metadata":"null"}}]}`, strings.ToLower(state))
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
	forward := decode(request(editor, "POST", "/api/v1/release-orders", `{"table_name":"history_items","items":[{"operation":"MODIFY","id":"forward","expected_record_version":"0","content":{"value":"发布后新值","metadata":"null"}}]}`, "history-create-forward", 201))
	forward = action(editor, forward, "submit", "")
	forward = action(reviewer, forward, "approve", "正向批准意见")
	forward = action(publisher, forward, "execute", "")
	forward = action(publisher, forward, "complete", "")
	inverse := decode(request(editor, "POST", "/api/v1/release-orders/"+forward.ID+"/rollback", fmt.Sprintf(`{"expected_version":%q,"reason":"反向申请理由"}`, forward.Version), "history-create-inverse", 201))
	inverse = action(editor, inverse, "submit", "")
	inverse = action(reviewer, inverse, "approve", "反向批准意见")
	inverse = action(publisher, inverse, "execute", "")
	forward = decode(request(viewer, "GET", "/api/v1/release-orders/"+forward.ID, "", "", 200))
	if forward.State != "ROLLED_BACK" || inverse.State != "COMPLETED" || forward.RollbackOrderID != inverse.ID || inverse.RollbackOfID != forward.ID {
		t.Fatal("rollback history missing")
	}
	orders[forward.ID], orders[inverse.ID] = forward, inverse
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
			if event.Action == "EXECUTE" || event.Action == "COMPLETE" || event.Action == "ROLLED_BACK" {
				want = publisher.id
			}
			if event.ActorID != want {
				t.Fatalf("%s actor: %s, want %s", event.Action, event.ActorID, want)
			}
		}
		if order.Publication != nil && (order.Publication.PublisherID != publisher.id || len(order.Publication.Commands) != 1) {
			t.Fatal("missing permanent publisher/command")
		}
	}
	if len(states) != 7 {
		t.Fatalf("missing historical state: %v", states)
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
	request(admin, "POST", "/api/v1/table-policies/history_items/disable", "", "", 200)
	deliveryExec(t, f.databaseOwner, `UPDATE rcc_mutation_policies SET name='维护后规则名称' WHERE code=?`, mutationCode)
	deliveryExec(t, f.databaseOwner, `DROP TABLE history_items`)
	// Rerun the documented restartable 008–012 migrations on populated controls.
	applyRoleMigration(t, f.databaseOwner)
	ownerSettings := settings.Clone()
	ownerSettings.User, ownerSettings.MultiStatements = "root", true
	migrationOwner := deliveryDB(t, ownerSettings)
	for _, file := range []string{"009-record-versions.sql", "010-release-drafts.sql", "011-release-targets.sql", "012-publication.sql"} {
		migration, err := os.ReadFile("../../../deploy/mysql/migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		deliveryExec(t, migrationOwner, string(migration))
	}
	process.stop(t)
	process = accountProcessCommand(t, binary, settings)
	process.ready(t)
	for id, before := range orders {
		after := decode(request(viewer, "GET", "/api/v1/release-orders/"+id, "", "", 200))
		if !reflect.DeepEqual(before, after) {
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
		unchanged := decode(request(viewer, "GET", "/api/v1/release-orders/"+id, "", "", 200))
		if !reflect.DeepEqual(before, unchanged) {
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
	request(viewer, "POST", "/api/v1/release-orders/"+inverse.ID+"/execute", `{"expected_version":"3"}`, inverse.ID+"-execute", 403)
	process.stop(t)
	for _, secret := range []string{"history password long enough", editor.cookies[0].Value, reviewer.csrf, "发布后新值"} {
		if strings.Contains(process.output.String(), secret) {
			t.Fatal("history process log disclosed sensitive content")
		}
	}
}
