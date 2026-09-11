//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
)

// AC-020: fail the real notification UPDATE after state/results/data writes.
// The original retry can commit once; unrelated-body retries cannot replace it.
func TestReleaseNotificationsPersistenceFailuresPreserveAllFacts(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	for _, action := range []string{"execute", "complete", "quick-rollback"} {
		t.Run(action, func(t *testing.T) {
			path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "atomic-"+action)
			body := `{"expected_version":"3"}`
			if action != "execute" {
				rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", body, "atomic-publish-"+action), 200)
				body = `{"expected_version":"4"}`
			}
			if action == "quick-rollback" {
				preview := readQuickPreview(t, app, publisher, path, "4")
				body = quickRollbackBody(preview.ExpectedVersion, preview.Digest, "原子恢复")
			}
			before := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, applicant, "GET", path, "", ""), 200)
			facts := releaseNotificationFacts(t, db, before.ID)
			applicantBefore := readApprovalProgress(t, app, applicant, path)
			reviewerBefore := readApprovalProgress(t, app, reviewer, path)
			// Fail after one intended recipient may already have been updated, so the
			// transaction must restore both notification aggregates as well as business.
			deliveryExec(t, db, fmt.Sprintf(`CREATE TRIGGER fail_result_notice BEFORE UPDATE ON rcc_approval_notifications FOR EACH ROW BEGIN IF NEW.account_id='%s' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='result recipient write failure'; END IF; END`, accountID(t, reviewer)))
			failed := releaseActorRequest(t, app, publisher, "POST", path+"/"+action, body, "atomic-action-"+action)
			deliveryExec(t, db, `DROP TRIGGER fail_result_notice`)
			assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
			current := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, applicant, "GET", path, "", ""), 200)
			if current.State != before.State || current.Version != before.Version || !reflect.DeepEqual(current.Executions, before.Executions) || !reflect.DeepEqual(current.Items, before.Items) {
				t.Fatal("notification failure partially committed state or actual results")
			}
			if !reflect.DeepEqual(facts, releaseNotificationFacts(t, db, before.ID)) || readApprovalProgress(t, app, applicant, path) != applicantBefore || readApprovalProgress(t, app, reviewer, path) != reviewerBefore {
				t.Fatal("notification failure partially committed values/versions/notification progress")
			}
			response := releaseActorRequest(t, app, publisher, "POST", path+"/"+action, body, "atomic-action-"+action)
			result := rollbackOrderResponse(t, response, 200)
			applicantAfter := assertReleaseNotificationAdvance(t, app, applicant, path, applicantBefore)
			reviewerAfter := assertReleaseNotificationAdvance(t, app, reviewer, path, reviewerBefore)
			savedFacts := releaseNotificationFacts(t, db, result.ID)
			for range 2 {
				replay := releaseActorRequest(t, app, publisher, "POST", path+"/"+action, body, "atomic-action-"+action)
				if replay.Code != 200 || replay.Body.String() != response.Body.String() {
					t.Fatal("original result changed")
				}
			}
			different := strings.Replace(body, `"expected_version":"3"`, `"expected_version":"999"`, 1)
			different = strings.Replace(different, `"expected_version":"5"`, `"expected_version":"999"`, 1)
			different = strings.Replace(different, `"expected_version":"4"`, `"expected_version":"999"`, 1)
			assertIntegrationErrorCode(t, releaseActorRequest(t, app, publisher, "POST", path+"/"+action, different, "atomic-action-"+action), 409, "idempotency_conflict")
			if !reflect.DeepEqual(savedFacts, releaseNotificationFacts(t, db, result.ID)) || readApprovalProgress(t, app, applicant, path) != applicantAfter || readApprovalProgress(t, app, reviewer, path) != reviewerAfter {
				t.Fatal("retries duplicated durable facts")
			}
		})
	}
}

// Snapshot observable persisted facts at the real dependency boundary. Global
// version totals catch a write outside this order even when its history stayed.
func releaseNotificationFacts(t *testing.T, db *sql.DB, id string) map[string]string {
	t.Helper()
	queries := []string{
		`SELECT COALESCE(GROUP_CONCAT(CONCAT(code,':',label) ORDER BY code),'') FROM mutation_add_items`,
		`SELECT COALESCE(GROUP_CONCAT(CONCAT(id,':',label) ORDER BY id),'') FROM mutation_supplied_id_items`,
		`SELECT COALESCE(SUM(lock_version),0) FROM rcc_record_versions`,
		`SELECT COALESCE(SUM(table_version),0) FROM rcc_table_publications`,
		`SELECT COUNT(*) FROM rcc_publication_commands`,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`,
		`SELECT COUNT(*) FROM rcc_release_executions`,
		`SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + id + `'`,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE operation IN ('execute:` + id + `','complete:` + id + `','quick-rollback:` + id + `') AND result IS NOT NULL`,
		`SELECT COALESCE(GROUP_CONCAT(CONCAT(account_id,':',sequence,':',pending_sequence,':',result_sequence,':',read_sequence) ORDER BY account_id),'') FROM rcc_approval_notifications WHERE order_id='` + id + `'`,
	}
	result := map[string]string{}
	for _, query := range queries {
		var value string
		if err := db.QueryRow(query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		result[query] = value
	}
	return result
}

// The client consumes and loses a successful HTTP response, then explicitly
// resends the exact original key and bytes. Four public operations are covered.
func TestReleaseNotificationsLostResponsesRecoverOriginalFacts(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	admin := integrationAdminSession(t, app)
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	transport := &droppedResponseTransport{base: http.DefaultTransport}
	client := &http.Client{Transport: transport}
	for _, action := range []string{"execute", "complete", "quick-rollback", "reprepare"} {
		t.Run(action, func(t *testing.T) {
			path, body, status := prepareReleaseNotificationAction(t, app, applicant, reviewer, publisher, action, "lost-"+action)
			actor := publisher
			if action == "reprepare" {
				actor = admin
			}
			observed := readApprovalProgress(t, app, applicant, path)
			request, err := http.NewRequest("POST", server.URL+path+"/"+action, strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			for _, cookie := range actor.Result().Cookies() {
				request.AddCookie(cookie)
			}
			request.Header.Set("Origin", "http://127.0.0.1:5173")
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-CSRF-Token", sessionCSRF(t, actor))
			request.Header.Set("Idempotency-Key", "lost-result-"+action)
			transport.drop.Store(true)
			response, err := client.Do(request)
			if err == nil || response != nil {
				t.Fatalf("expected response loss: %v %v", response, err)
			}
			before := readTableApprovalOrder(t, app, applicant, path)
			applicantAfter := assertReleaseNotificationAdvance(t, app, applicant, path, observed)
			reviewerAfter := readApprovalProgress(t, app, reviewer, path)
			facts := releaseNotificationFacts(t, db, before.ID)
			retry := releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "lost-result-"+action)
			recovered := rollbackOrderResponse(t, retry, status)
			again := releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "lost-result-"+action)
			if again.Code != status || again.Body.String() != retry.Body.String() || recovered.ID == "" {
				t.Fatal("recovered original response diverged")
			}
			after := readTableApprovalOrder(t, app, applicant, path)
			if after.State != before.State || after.Version != before.Version || len(after.History) != len(before.History) || !reflect.DeepEqual(facts, releaseNotificationFacts(t, db, before.ID)) || readApprovalProgress(t, app, applicant, path) != applicantAfter || readApprovalProgress(t, app, reviewer, path) != reviewerAfter {
				t.Fatal("lost response repeated business or reminder")
			}
			// Write/replay bodies must not persist or expose one recipient's read receipt.
			var wire map[string]json.RawMessage
			if json.Unmarshal(retry.Body.Bytes(), &wire) != nil {
				t.Fatal("invalid result")
			}
			if _, ok := wire["notification"]; ok {
				t.Fatal("write result leaked personal receipt")
			}
		})
	}
}

// Unlike an HTTP loss, this fault consumes the actual MySQL COMMIT OK before
// the driver sees it. Complete/reprepare do not write a downstream refresh, so
// target the personal-result write inside the original transaction explicitly.
func TestReleaseNotificationsCommitUnknownRecoversEveryLifecycle(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/006-mutation-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close() })
	root := *driver
	root.User = "root"
	db := deliveryDB(t, &root)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	admin := integrationAdminSession(t, app)
	proxy := newPublicationWireProxy(t, driver.Addr, "UPDATE RCC_APPROVAL_NOTIFICATIONS")
	through := *driver
	through.Addr = proxy.listener.Addr().String()
	uncertain, err := newApplication(ctx, integrationConfig(&through))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { uncertain.Close() })
	for _, action := range []string{"execute", "complete", "quick-rollback", "reprepare"} {
		t.Run(action, func(t *testing.T) {
			path, body, status := prepareReleaseNotificationAction(t, app, applicant, reviewer, publisher, action, "commit-"+action)
			actor := publisher
			if action == "reprepare" {
				actor = admin
			}
			observed := readApprovalProgress(t, app, applicant, path)
			ack := proxy.ack.Load()
			proxy.mode.Store(3)
			failed := releaseActorRequest(t, uncertain, actor, "POST", path+"/"+action, body, "commit-result-"+action)
			assertIntegrationErrorCode(t, failed, 503, "release_result_unknown")
			if proxy.mode.Load() != 0 || proxy.ack.Load() != ack+1 {
				t.Fatal("did not consume exactly one real COMMIT OK")
			}
			before := readTableApprovalOrder(t, app, applicant, path)
			applicantAfter := assertReleaseNotificationAdvance(t, app, applicant, path, observed)
			facts := releaseNotificationFacts(t, db, before.ID)
			for _, event := range before.History {
				if strings.HasSuffix(event.Action, "_FAILED") {
					t.Fatal("COMMIT unknown incorrectly saved a failure history")
				}
			}
			recovered := releaseActorRequest(t, uncertain, actor, "POST", path+"/"+action, body, "commit-result-"+action)
			rollbackOrderResponse(t, recovered, status)
			replay := releaseActorRequest(t, app, actor, "POST", path+"/"+action, body, "commit-result-"+action)
			if replay.Code != status || replay.Body.String() != recovered.Body.String() || !reflect.DeepEqual(facts, releaseNotificationFacts(t, db, before.ID)) || readApprovalProgress(t, app, applicant, path) != applicantAfter {
				t.Fatal("COMMIT recovery changed committed facts")
			}
		})
	}
}

// Real simultaneous requests compete on the same observed multi-table version.
// Only the winner advances participants; a same-key race returns one saved fact.
func TestReleaseNotificationsTerminalCompetitionHasOneResult(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	for _, mode := range []string{"complete", "rollback-distinct", "rollback-same"} {
		t.Run(mode, func(t *testing.T) {
			path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "race-notice-"+mode)
			original := rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "race-notice-publish-"+mode), 200)
			applicantBefore := readApprovalProgress(t, app, applicant, path)
			reviewerBefore := readApprovalProgress(t, app, reviewer, path)
			preview := readQuickPreview(t, app, publisher, path, "4")
			body := quickRollbackBody(preview.ExpectedVersion, preview.Digest, "竞争恢复")
			type result struct {
				action, key, body string
				response          *httptest.ResponseRecorder
			}
			start := make(chan struct{})
			results := make(chan result, 2)
			cookies, csrf := publisher.Result().Cookies(), sessionCSRF(t, publisher)
			for n := 0; n < 2; n++ {
				action, key, request := "quick-rollback", fmt.Sprintf("race-result-%s-%d", mode, n), body
				if n == 1 && mode == "complete" {
					action = "complete"
					request = fmt.Sprintf(`{"expected_version":%q}`, preview.ExpectedVersion)
				}
				if n == 1 && mode == "rollback-same" {
					key = "race-result-" + mode + "-0"
				}
				go func(action, key, request string) {
					<-start
					results <- result{action, key, request, accountRequestFrom(app, "POST", path+"/"+action, request, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})}
				}(action, key, request)
			}
			close(start)
			successes := []result{}
			for range 2 {
				result := <-results
				if result.response.Code == 200 {
					successes = append(successes, result)
				} else {
					assertIntegrationErrorCode(t, result.response, 409, "release_version_conflict")
				}
			}
			expected := 1
			if mode == "rollback-same" {
				expected = 2
			}
			if len(successes) != expected {
				t.Fatalf("successful responses=%d want %d", len(successes), expected)
			}
			winner := successes[0]
			terminal := rollbackOrderResponse(t, winner.response, 200)
			applicantAfter := assertReleaseNotificationAdvance(t, app, applicant, path, applicantBefore)
			reviewerAfter := assertReleaseNotificationAdvance(t, app, reviewer, path, reviewerBefore)
			if terminal.Version != "6" || len(terminal.History) != 6 {
				t.Fatal("duplicate workflow result")
			}
			executions, rows := 2, 0
			if winner.action == "complete" {
				executions, rows = 1, 1
				if terminal.State != "COMPLETED" {
					t.Fatal("completion winner mismatch")
				}
			} else if terminal.State != "ROLLED_BACK" {
				t.Fatal("rollback winner mismatch")
			}
			batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM rcc_release_executions WHERE order_id='` + original.ID + `'`: executions, `SELECT COUNT(*) FROM rcc_publication_commands WHERE order_id='` + original.ID + `'`: executions * 2, `SELECT COUNT(*) FROM mutation_add_items WHERE code='race-notice-` + mode + `'`: rows, `SELECT COUNT(*) FROM mutation_supplied_id_items WHERE id='race-notice-` + mode + `'`: rows, `SELECT COUNT(*) FROM rcc_release_targets WHERE order_id='` + original.ID + `'`: 0})
			facts := releaseNotificationFacts(t, db, original.ID)
			for _, success := range successes {
				replay := releaseActorRequest(t, app, publisher, "POST", path+"/"+success.action, success.body, success.key)
				if replay.Code != 200 || replay.Body.String() != success.response.Body.String() {
					t.Fatal("winner original result did not replay")
				}
			}
			if !reflect.DeepEqual(facts, releaseNotificationFacts(t, db, original.ID)) || readApprovalProgress(t, app, applicant, path) != applicantAfter || readApprovalProgress(t, app, reviewer, path) != reviewerAfter {
				t.Fatal("racing replay duplicated result")
			}
		})
	}
}

// Formal account maintenance may disable a past decision maker. Their permanent
// participation still receives release outcomes and is readable upon re-enable.
func TestReleaseNotificationsDisabledParticipantsRetainOutcomes(t *testing.T) {
	f := newMaintenanceFixture(t)
	fixture, err := os.ReadFile("testdata/006-mutation-fixture.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, f.databaseOwner, statement)
		}
	}
	app := f.app
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	completePath := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "disabled-complete")
	rollbackPath := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "disabled-rollback")
	f.run(t, "", "disable", "--id", accountID(t, reviewer))
	for _, path := range []string{completePath, rollbackPath} {
		rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "disabled-execute-"+path[len(path)-32:]), 200)
	}
	rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", completePath+"/complete", `{"expected_version":"4"}`, "disabled-complete"), 200)
	preview := readQuickPreview(t, app, publisher, rollbackPath, "4")
	rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", rollbackPath+"/quick-rollback", quickRollbackBody(preview.ExpectedVersion, preview.Digest, "停用参与人仍有历史"), "disabled-rollback"), 200)
	f.run(t, "", "enable", "--id", accountID(t, reviewer))
	reviewer = loginAccount(t, app, "lifecycle.reviewer", "correct horse battery staple")
	for _, path := range []string{completePath, rollbackPath} {
		progress := readApprovalProgress(t, app, reviewer, path)
		if progress.Sequence != "3" || !progress.Unread || progress.Pending {
			t.Fatalf("disabled participant lost release outcomes: %+v", progress)
		}
	}
	assertNotificationOrders(t, readNotificationPage(t, app, reviewer, "view=handled&unread=true"), completePath, rollbackPath)
}

// Preparation is shared; the two transports and their failure assertions remain
// independent so a HTTP response loss cannot stand in for a COMMIT ACK loss.
func prepareReleaseNotificationAction(t *testing.T, app *adminApplication, applicant, reviewer, publisher *httptest.ResponseRecorder, action, key string) (path, body string, status int) {
	t.Helper()
	path = approvedReleaseNotificationOrder(t, app, applicant, reviewer, key)
	body, status = `{"expected_version":"3"}`, 200
	if action == "complete" || action == "quick-rollback" {
		rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", body, key+"-publish"), 200)
		body = `{"expected_version":"4"}`
	}
	if action == "quick-rollback" {
		preview := readQuickPreview(t, app, publisher, path, "4")
		body = quickRollbackBody(preview.ExpectedVersion, preview.Digest, "transport recovery")
	}
	if action == "reprepare" {
		source := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, applicant, "GET", path, "", ""), 200)
		body, status = derivedDraftBody(t, source), 201
	}
	return path, body, status
}
