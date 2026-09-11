//go:build integration

package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// Original preview outcomes belong to the account that saved them; sharing the
// request bytes or key never imports another account's acknowledgement.
func TestRollbackFlowPreviewReplayUsesCurrentAccountAndRoles(t *testing.T) {
	app, _ := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "rollback-preview-identity")
	rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-preview-identity-publish"), 200)
	target, body, key := path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-identity-save"
	saved := releaseActorRequest(t, app, publisher, "POST", target, body, key)
	preview := decodeRollbackFlowPreview(t, saved.Code, saved.Body.Bytes())
	grantReleaseRole(t, app, applicant, `["EDITOR"]`, "2", "rollback-preview-applicant-role")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, applicant, "POST", target, body, key), 403, "permission_denied")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", target, body, key), 409, "release_version_conflict")
	grantReleaseRole(t, app, publisher, `["VIEWER"]`, "2", "rollback-preview-revoke")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, publisher, "POST", target, body, key), 403, "permission_denied")
	grantReleaseRole(t, app, publisher, `["PUBLISHER"]`, "3", "rollback-preview-restore")
	replay := releaseActorRequest(t, app, publisher, "POST", target, body, key)
	if replay.Code != 200 || replay.Body.String() != saved.Body.String() || readRollbackFlows(t, app, path).Version != preview.Version {
		t.Fatal("current identity failed to recover its original preview without rewriting the order", replay.Code, replay.Body)
	}
}

// Current HTTP authentication can finish while a maintenance change has yet to
// commit. The existing release authorization lock must order the business write
// after that change, including a replay of an earlier successful original key.
func TestRollbackFlowCurrentAuthorizationAfterWaitingForMaintenance(t *testing.T) {
	f := newMaintenanceFixture(t)
	app := f.app
	db := f.databaseOwner
	deliveryExec(t, db, `CREATE TABLE rollback_auth_rows(id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,value VARCHAR(64))`)
	enableMutationPolicy(t, app, "rollback_auth_rows", mutationPolicyFixture{AllowAdd: true, AllowDelete: true})
	reviewer := publicationFixtureReviewer(t, app)
	for _, mode := range []string{"preview", "preview-replay", "rollback", "rollback-replay"} {
		t.Run(mode, func(t *testing.T) {
			actor := registerAccount(t, app, "rollback.auth."+mode, "rollback.auth."+mode+"@example.com", "correct horse battery staple")
			actorID := accountID(t, actor)
			grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "rollback-auth-role-"+mode)
			path := approvePublication(t, app, reviewer, `{"title":"恢复权限竞争","items":[{"table_name":"rollback_auth_rows","operation":"ADD","content":{"value":"new"}}]}`, "rollback-auth-create-"+mode)
			rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-auth-publish-"+mode), 200)
			target, body, key := path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-auth-"+mode
			if mode != "preview" {
				r := releaseActorRequest(t, app, actor, "POST", target, body, key)
				preview := decodeRollbackFlowPreview(t, r.Code, r.Body.Bytes())
				if strings.HasPrefix(mode, "rollback") {
					target, body = path+"/quick-rollback", quickRollbackBody(preview.Version, preview.Digest, "")
					if mode == "rollback-replay" {
						rollbackOrderResponse(t, releaseActorRequest(t, app, actor, "POST", target, body, key), 200)
					}
				}
			}
			before := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
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
				response <- accountRequestFrom(app, "POST", target, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
			}()
			awaitFlowDatabaseCondition(t, db, flowLockWaitSQL, "rcc_auth_control_lock")
			if err = gate.QueryRowContext(context.Background(), `SELECT RELEASE_LOCK('flow_auth_gate')`).Scan(&held); err != nil || held != 1 {
				t.Fatal("open authorization gate", err)
			}
			if err = <-disabled; err != nil {
				t.Fatal(err)
			}
			assertIntegrationErrorCode(t, receiveFlowResponse(t, response), 403, "permission_denied")
			next := releaseActorRequest(t, app, actor, "POST", target, body, key)
			if next.Code != 401 {
				t.Fatalf("new disabled-account request was accepted: %d %s", next.Code, next.Body)
			}
			after := rollbackOrderResponse(t, releaseReadAllDetails(t, app, "GET", path, "", ""), 200)
			if after.Version != before.Version || after.State != before.State {
				t.Fatal("revoked actor changed original order", before, after)
			}
		})
	}
}
