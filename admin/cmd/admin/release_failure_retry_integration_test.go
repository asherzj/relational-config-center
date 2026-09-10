//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// #81 AC-011/012: a failed whole-order publication leaves only truthful
// operation history and permits the exact original request after repair.
func TestConfirmedReleaseFailureHistoryPreservesOriginalRetry(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(90,'occupied','external')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"失败后原请求重推","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"id":"10","code":"first","label":"first"}},{"operation":"ADD","content":{"id":"20","code":"occupied","label":"second"}}]}`, "failure-retry")
	body, key := `{"expected_version":"3"}`, "failure-original-request"
	failed := releaseRequest(t, app, "POST", path+"/execute", body, key)
	assertIntegrationErrorCode(t, failed, 409, "duplicate_key")
	var response struct {
		Error struct {
			ExecutionOutcome string `json:"execution_outcome"`
			FailureHistory   string `json:"failure_history"`
		}
	}
	if json.Unmarshal(failed.Body.Bytes(), &response) != nil || response.Error.ExecutionOutcome != "not_committed" || response.Error.FailureHistory != "saved" {
		t.Fatalf("missing truthful failure history receipt: %s", failed.Body)
	}
	current := batchEdgeOrder(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "APPROVED" || current.Version != "3" || current.Publication != nil || len(current.Executions) != 0 || len(current.History) != 4 || current.History[3].Action != "EXECUTE_FAILED" || current.History[3].Version != "3" {
		t.Fatalf("failure changed execution/CAS or missed audit: %#v", current)
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items WHERE id IN (10,20)`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 0, `SELECT COUNT(*) FROM rcc_publication_commands`: 0, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 0, `SELECT COUNT(*) FROM rcc_record_versions`: 0})
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"4"}`, key), 409, "idempotency_conflict")
	deliveryExec(t, db, `DELETE FROM mutation_add_items WHERE id=90`)
	success := releaseRequest(t, app, "POST", path+"/execute", body, key)
	published := batchEdgeOrder(t, success, 200)
	if published.Version != "4" || len(published.Executions) != 1 || len(published.History) != 5 {
		t.Fatal("original retry failed to commit once", success.Body)
	}
	replay := releaseRequest(t, app, "POST", path+"/execute", body, key)
	if replay.Code != 200 || replay.Body.String() != success.Body.String() {
		t.Fatal("original retry result changed", replay.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"4"}`, key), 409, "idempotency_conflict")
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items`: 2, `SELECT COUNT(*) FROM rcc_release_executions`: 1, `SELECT COUNT(*) FROM rcc_publication_commands`: 2, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 1, `SELECT COUNT(*) FROM rcc_record_versions`: 2})
}

func TestReleaseFailureHistoryUnavailableDoesNotLeaveBusinessWrites(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"审计存储故障","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"id":"10","code":"audit-failure","label":"retained"}}]}`, "audit-failure")
	// A real database dependency rejects the final workflow save and the later
	// audit append. No owned Store or Session implementation is replaced.
	deliveryExec(t, db, `CREATE TRIGGER reject_release_update BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='acceptance storage failure'`)
	response := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "audit-original")
	assertIntegrationErrorCode(t, response, 503, "release_unavailable")
	var receipt struct {
		Error struct {
			Outcome string `json:"execution_outcome"`
			History string `json:"failure_history"`
		}
	}
	if json.Unmarshal(response.Body.Bytes(), &receipt) != nil || receipt.Error.Outcome != "not_committed" || receipt.Error.History != "unavailable" {
		t.Fatal("unavailable history misrepresented", response.Body)
	}
	current := batchEdgeOrder(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.Version != "3" || current.State != "APPROVED" || len(current.History) != 3 || len(current.Executions) != 0 {
		t.Fatal("fabricated history or result", current)
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 0, `SELECT COUNT(*) FROM rcc_publication_commands`: 0, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 0, `SELECT COUNT(*) FROM rcc_record_versions`: 0})
	deliveryExec(t, db, `DROP TRIGGER reject_release_update`)
	batchEdgeOrder(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "audit-original"), 200)
}

// Business facts remain byte-for-byte equivalent; only one independently
// persisted failure event is permitted between these two public reads.
func assertReleaseFailureOnly(t *testing.T, before, current domain.ReleaseOrder, action string) {
	t.Helper()
	if len(current.History) != len(before.History)+1 {
		t.Fatalf("expected one failure event, got %d -> %d", len(before.History), len(current.History))
	}
	event := current.History[len(current.History)-1]
	if event.Action != action || event.ActorID == "" || event.Version != before.Version || event.Reason == "" {
		t.Fatalf("invalid failure event: %#v", event)
	}
	if _, err := time.Parse(time.RFC3339Nano, event.At); err != nil {
		t.Fatal("failure timestamp", err)
	}
	current.History = current.History[:len(current.History)-1]
	if !reflect.DeepEqual(before, current) {
		t.Fatal("failure audit altered business facts or previous history")
	}
}

// A cancellation already waiting for the business transaction commits before
// failure auditing. The append must preserve that current main record exactly.
func TestReleaseFailureAuditPreservesConcurrentCancellation(t *testing.T) {
	app, db := batchEdgeApplication(t)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"失败留痕与取消竞争","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"id":"10","code":"audit-race","label":"intent"}}]}`, "audit-race")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	external, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer external.Rollback()
	// An external writer holds the destination unique key. The publication must
	// reach this real row lock before cancellation queues on the main record.
	if _, err := external.ExecContext(ctx, `INSERT INTO mutation_add_items(id,code,label) VALUES(90,'audit-race','external')`); err != nil {
		t.Fatal(err)
	}
	session := integrationAdminSession(t, app)
	cookies, csrf := session.Result().Cookies(), sessionCSRF(t, session)
	failedResponses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		failedResponses <- accountRequestFrom(app, "POST", path+"/execute", `{"expected_version":"3"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "audit-race-execute"})
	}()
	waitForLock := func(table string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			var waiting int
			if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks requested ON requested.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE requested.OBJECT_SCHEMA=DATABASE() AND requested.OBJECT_NAME=?`, table).Scan(&waiting); err != nil {
				t.Fatal(err)
			}
			if waiting > 0 {
				return
			}
			select {
			case early := <-failedResponses:
				t.Fatalf("publication escaped expected %s lock: %d %s", table, early.Code, early.Body)
			default:
			}
			if time.Now().After(deadline) {
				t.Fatalf("request did not reach real %s lock", table)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitForLock("mutation_add_items")
	cancelledResponses := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		cancelledResponses <- accountRequestFrom(app, "POST", path+"/cancel", `{"expected_version":"3","reason":"concurrent cancellation"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": "audit-race-cancel"})
	}()
	waitForLock("rcc_release_orders")
	if err := external.Commit(); err != nil {
		t.Fatal(err)
	}
	var failed, cancelled *httptest.ResponseRecorder
	select {
	case failed = <-failedResponses:
	case <-ctx.Done():
		t.Fatal("failure request did not finish")
	}
	select {
	case cancelled = <-cancelledResponses:
	case <-ctx.Done():
		t.Fatal("cancellation did not finish")
	}
	assertIntegrationErrorCode(t, failed, 409, "duplicate_key")
	cancelledOrder := batchEdgeOrder(t, cancelled, 200)
	current := batchEdgeOrder(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if current.State != "CANCELLED" || current.Version != "4" {
		t.Fatal("audit overwrote concurrent main state", current)
	}
	if len(current.History) != 5 || current.History[4].Action != "EXECUTE_FAILED" || current.History[4].Version != "3" {
		t.Fatal("failure audit did not follow cancellation", current.History)
	}
	current.History = current.History[:4]
	if !reflect.DeepEqual(current, cancelledOrder) {
		t.Fatal("audit overwrote concurrent cancellation facts")
	}
	batchEdgeCounts(t, db, map[string]int{`SELECT COUNT(*) FROM mutation_add_items WHERE id=10`: 0, `SELECT COUNT(*) FROM rcc_release_executions`: 0, `SELECT COUNT(*) FROM rcc_publication_commands`: 0, `SELECT COUNT(*) FROM rcc_refresh_notifications`: 0, `SELECT COUNT(*) FROM rcc_record_versions`: 0, `SELECT COUNT(*) FROM rcc_release_targets`: 0})
}
