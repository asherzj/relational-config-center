//go:build integration

package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// Current HTTP authentication can finish while a maintenance change has yet to
// commit. The existing release authorization lock must order the business write
// after that change, including a replay of an earlier successful original key.
func TestReleaseFlowCurrentAuthorizationAfterWaitingForMaintenance(t *testing.T) {
	f := newMaintenanceFixture(t)
	app := f.app
	db := f.databaseOwner
	deliveryExec(t, db, `CREATE TABLE policy_alpha(id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,value VARCHAR(64) NOT NULL)`)
	deliveryExec(t, db, `CREATE TABLE policy_beta(id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,value VARCHAR(64))`)
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	source := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-auth-source"), 201)
	source = rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+source.ID+"/cancel", `{"expected_version":"1","reason":"copy source"}`, "flow-auth-cancel"), 200)
	for _, mode := range []string{"create", "copy", "replay"} {
		t.Run(mode, func(t *testing.T) {
			actor := registerAccount(t, app, "flow.auth."+mode, "flow.auth."+mode+"@example.com", "correct horse battery staple")
			actorID := accountID(t, actor)
			grantReleaseRole(t, app, actor, `["EDITOR"]`, "1", "flow-auth-role-"+mode)
			path, body, key := "/api/v1/release-orders", flowDraftBody, "flow-auth-"+mode
			if mode == "copy" {
				path += "/" + source.ID + "/copy"
				body = derivedDraftBody(t, source)
			}
			if mode == "replay" {
				flowResponse(t, releaseActorRequest(t, app, actor, "POST", path, body, key), 201)
			}
			cookies, csrf := actor.Result().Cookies(), sessionCSRF(t, actor)
			gate, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			var held int
			if err = gate.QueryRowContext(context.Background(), `SELECT GET_LOCK('flow_auth_gate',5)`).Scan(&held); err != nil || held != 1 {
				t.Fatal("authorization gate", err)
			}
			defer gate.ExecContext(context.Background(), `SELECT RELEASE_LOCK('flow_auth_gate')`)
			deliveryExec(t, db, fmt.Sprintf(`CREATE TRIGGER pause_flow_account_disable BEFORE UPDATE ON rcc_accounts FOR EACH ROW BEGIN IF OLD.id='%s' AND OLD.enabled=1 AND NEW.enabled=0 THEN SET @flow_auth_entered=GET_LOCK('flow_auth_entered',0); SET @flow_auth_wait=GET_LOCK('flow_auth_gate',12); SET @flow_auth_done=RELEASE_LOCK('flow_auth_entered'); SET @flow_auth_open=RELEASE_LOCK('flow_auth_gate'); END IF; END`, actorID))
			defer deliveryExec(t, db, `DROP TRIGGER IF EXISTS pause_flow_account_disable`)
			disabled := make(chan error, 1)
			go func() {
				output, err := f.command("", "disable", "--id", actorID).CombinedOutput()
				if err != nil {
					disabled <- fmt.Errorf("maintenance: %v %s", err, output)
				} else {
					disabled <- nil
				}
			}()
			awaitFlowDatabaseCondition(t, db, `SELECT IF(IS_USED_LOCK('flow_auth_entered') IS NULL,0,1)`)
			response := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				response <- accountRequestFrom(app, "POST", path, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
			}()
			awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_auth_control_lock")
			if err = gate.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK('flow_auth_gate')`).Scan(&held); err != nil || held != 1 {
				t.Fatal("open authorization gate", err)
			}
			if err = <-disabled; err != nil {
				t.Fatal(err)
			}
			assertIntegrationErrorCode(t, receiveFlowResponse(t, response), 403, "permission_denied")
			next := releaseActorRequest(t, app, actor, "POST", path, body, key)
			if next.Code != 401 {
				t.Fatalf("new disabled-account request was accepted: %d %s", next.Code, next.Body)
			}
			orders := releaseRequest(t, app, "GET", "/api/v1/release-orders?applicant_id="+actorID, "", "")
			var expected string
			if mode == "replay" {
				expected = `"orders":[{`
			} else {
				expected = `"orders":[]`
			}
			if orders.Code != 200 || !strings.Contains(orders.Body.String(), expected) {
				t.Fatalf("authorization failure created or lost an order: %d %s", orders.Code, orders.Body)
			}
		})
	}
}
