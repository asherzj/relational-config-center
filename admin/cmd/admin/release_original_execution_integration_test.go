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
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"原单混合恢复","table_name":"mutation_add_items","items":[{"operation":"ADD","content":{"code":"original-add","label":"added"}},{"operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}},{"operation":"DELETE","id":"20","expected_record_version":"0","content":{}}]}`, "original-execution")
	publicationResponse := releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "original-publish")
	published := rollbackOrderResponse(t, publicationResponse, 200)
	actor := registerAccount(t, app, "original.publisher", "original.publisher@example.com", "correct horse battery staple")
	grantReleaseRole(t, app, actor, `["PUBLISHER"]`, "1", "original-publisher")
	preview := readQuickPreview(t, app, actor, path, "4")
	body := quickRollbackBody("4", preview.Digest, "")
	response := releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", body, "original-rollback")
	restored := rollbackOrderResponse(t, response, 200)
	if restored.ID != published.ID || restored.State != "ROLLED_BACK" || restored.Version != "5" || restored.ApplicantID != published.ApplicantID || restored.Title != published.Title || !reflect.DeepEqual(restored.Items, published.Items) || !reflect.DeepEqual(restored.Publication, published.Publication) {
		t.Fatal("rollback replaced the application or created another order", restored)
	}
	if len(restored.History) != len(published.History)+1 || !reflect.DeepEqual(restored.History[:len(published.History)], published.History) {
		t.Fatal("application or approval history overwritten")
	}
	var result struct {
		Rollback   *domain.PublicationResult   `json:"rollback"`
		Executions []struct{ ID, Kind string } `json:"executions"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Rollback == nil || result.Rollback.TableVersion != "2" || result.Rollback.PublisherID != accountID(t, actor) || len(result.Executions) != 2 || result.Executions[0].ID == result.Executions[1].ID {
		t.Fatal("missing separate successful executions", response.Body)
	}
	if result.Rollback.Commands[0].Operation != "ADD" || result.Rollback.Commands[2].Operation != "DELETE" || result.Rollback.Notification.ID == published.Publication.Notification.ID {
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
	if publicationReplay.Code != 200 || !reflect.DeepEqual(rollbackOrderResponse(t, publicationReplay, 200), published) {
		t.Fatal("original publication replay changed after rollback", publicationReplay.Body)
	}
	batchEdgeCounts(t, db, map[string]int{
		`SELECT COUNT(*) FROM rcc_release_orders`:                                                         1,
		`SELECT COUNT(*) FROM rcc_release_details`:                                                        3,
		`SELECT COUNT(*) FROM rcc_release_executions`:                                                     2,
		`SELECT COUNT(*) FROM rcc_release_details WHERE publication IS NOT NULL AND rollback IS NOT NULL`: 3,
		`SELECT COUNT(*) FROM rcc_release_orders WHERE JSON_LENGTH(document,'$.items')>0 OR JSON_LENGTH(document,'$.publication.commands')>0`:                                                                                                                                                      0,
		`SELECT COUNT(*) FROM rcc_release_executions WHERE JSON_CONTAINS_PATH(document,'one','$.commands')`:                                                                                                                                                                                        0,
		`SELECT COUNT(*) FROM rcc_release_requests WHERE (JSON_TYPE(JSON_EXTRACT(result,'$.publication.commands'))='ARRAY' AND JSON_LENGTH(result,'$.publication.commands')>0) OR (JSON_TYPE(JSON_EXTRACT(result,'$.rollback.commands'))='ARRAY' AND JSON_LENGTH(result,'$.rollback.commands')>0)`: 0,
		`SELECT COUNT(*) FROM rcc_publication_commands`:  6,
		`SELECT COUNT(*) FROM rcc_refresh_notifications`: 2,
		`SELECT COUNT(*) FROM rcc_release_targets`:       0,
	})
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, actor, "POST", path+"/quick-rollback", quickRollbackBody("5", preview.Digest, ""), "original-repeat"), 422, "release_state_invalid")
}
