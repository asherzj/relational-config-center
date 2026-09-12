//go:build integration

package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/domain"
)

// #81 AC-010 / #82: the reviewed reversal is a second execution of the original
// order, without a second application, approval, or mandatory reason.
func TestOriginalOrderRollbackPreservesApplicationAndBothExecutions(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'original-modify','old'),(20,'original-delete','retained')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowAdd: true, AllowModify: true, AllowDelete: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"items":[{"content":{"code":"original-add","label":"added"},"operation":"ADD","table_name":"mutation_add_items"},{"content":{"label":"published"},"expected_record_version":"0","id":"10","operation":"MODIFY","table_name":"mutation_add_items"},{"content":{},"expected_record_version":"0","id":"20","operation":"DELETE","table_name":"mutation_add_items"}],"title":"原单混合恢复"}`, "original-execution")
	publicationResponse := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "original-publish")
	published := rollbackOrderResponse(t, publicationResponse, 200)
	var originalRequest string
	if err := db.QueryRow(`SELECT result FROM rcc_release_requests WHERE operation=? AND request_key=?`, "execute:"+published.ID, "original-publish").Scan(&originalRequest); err != nil {
		t.Fatal(err)
	}

	actor := registerAccount(t, app, "original.publisher", "original.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "original-publisher")
	preview := readQuickPreview(t, app, actor, path, "4")
	body := quickRollbackBody(preview.ExpectedVersion, preview.Digest, "")
	response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "original-rollback")
	restored := rollbackOrderResponse(t, response, 200)
	if restored.ID != published.ID || restored.State != "ROLLED_BACK" || restored.Version != "6" || restored.ApplicantID != published.ApplicantID || restored.Title != published.Title || !reflect.DeepEqual(applicationItems(restored), applicationItems(published)) || !reflect.DeepEqual(executionCommands(restored, "PUBLICATION"), executionCommands(published, "PUBLICATION")) {
		t.Fatal("rollback replaced the application or created another order", restored)
	}
	if len(restored.History) != len(published.History)+2 || !reflect.DeepEqual(restored.History[:len(published.History)], published.History) {
		t.Fatal("application or approval history overwritten")
	}
	var result domain.ReleaseOrder
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Executions) < 2 || singleExecutionTableVersion(result.Executions[1]) != "2" || result.Executions[1].ActorID != accountID(t, actor) || len(result.Executions) != 2 || result.Executions[0].ID == result.Executions[1].ID {
		t.Fatal("missing separate successful executions", response.Body)
	}
	if executionCommands(result, "ROLLBACK")[0].Operation != "ADD" || executionCommands(result, "ROLLBACK")[2].Operation != "DELETE" || result.Executions[1].ID == published.Executions[0].ID {
		t.Fatal("rollback order or execution notification identity incorrect")
	}
	for _, id := range []string{"10", "20"} {
		row, version := recordVersionRow(t, app, "mutation_add_items", id)
		want := "old"
		if id == "20" {
			want = "retained"
		}
		if *row["label"] != want || version != "2" {
			t.Fatal("business restoration failed", row, version)
		}
	}
	replay := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "original-rollback")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("replay changed actual result", replay.Body)
	}
	publicationReplay := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "original-publish")
	// Business result is immutable; the response deliberately carries the
	// current approval qualification after rollback, not an obsolete revision.
	latest := readTableApprovalOrder(t, app, integrationAdminSession(t, app), path)
	expectedReplay := published
	expectedReplay.ApprovalContext = latest.ApprovalContext
	if publicationReplay.Code != 200 || !reflect.DeepEqual(rollbackOrderResponse(t, publicationReplay, 200), expectedReplay) {
		t.Fatal("original publication replay changed after rollback", publicationReplay.Body)
	}
	var replayedRequest string
	if err := db.QueryRow(`SELECT result FROM rcc_release_requests WHERE operation=? AND request_key=?`, "execute:"+published.ID, "original-publish").Scan(&replayedRequest); err != nil {
		t.Fatal(err)
	}
	if replayedRequest != originalRequest {
		t.Fatal("replay rewrote durable original publication result")
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                                         1,
		`SELECT COUNT(*) FROM rcc_release_details`:                                                        3,
		`SELECT COUNT(*) FROM rcc_release_executions`:                                                     2,
		`SELECT COUNT(*) FROM rcc_release_details WHERE publication IS NOT NULL AND rollback IS NOT NULL`: 3,
		`SELECT COUNT(*) FROM rcc_release_orders WHERE JSON_LENGTH(document,'$.items')>0 OR JSON_LENGTH(document,'$.publication.commands')>0`:                                                                                                                                                      0,
		`SELECT COUNT(*) FROM rcc_release_executions WHERE JSON_CONTAINS_PATH(document,'one','$.commands')`:                                                                                                                                                                                        0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE (JSON_TYPE(JSON_EXTRACT(result,'$.publication.commands'))='ARRAY' AND JSON_LENGTH(result,'$.publication.commands')>0) OR (JSON_TYPE(JSON_EXTRACT(result,'$.rollback.commands'))='ARRAY' AND JSON_LENGTH(result,'$.rollback.commands')>0)`: 0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE JSON_CONTAINS_PATH(result,'one','$.items[*].publication','$.items[*].rollback')`:                                                                                                                                                          0,
		`SELECT COUNT(*) FROM rcc_publication_commands`:  6,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`: 2,
		`SELECT COUNT(*) FROM rcc_release_targets`:       0,
	})
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("6", preview.Digest, ""), "original-repeat"), 422, "release_state_invalid")
}
