//go:build integration

package main

import (
	"fmt"
	"reflect"
	"testing"
)

// AC-019/024: source reads, persisted instances and the original request result
// share one transaction. None of these dependency faults can leak a preview.
func TestRollbackFlowPreviewStorageFailuresKeepOriginalAtomic(t *testing.T) {
	app, db := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "rollback-preview-fault")
	original := rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-preview-fault-publish"), 200)
	beforeNotice := readApprovalProgress(t, app, applicant, path)
	beforeFacts := releaseNotificationFacts(t, db, original.ID)
	for _, fault := range []struct{ name, install, remove string }{
		{"source", `RENAME TABLE rcc_release_templates TO unavailable_rollback_templates`, `RENAME TABLE unavailable_rollback_templates TO rcc_release_templates`},
		{"instances", `CREATE TRIGGER reject_rollback_instances BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='preview instances unavailable'`, `DROP TRIGGER reject_rollback_instances`},
		{"result", `CREATE TRIGGER reject_rollback_preview_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'quick-rollback-preview:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='preview result unavailable'; END IF; END`, `DROP TRIGGER reject_rollback_preview_result`},
	} {
		t.Run(fault.name, func(t *testing.T) {
			deliveryExec(t, db, fault.install)
			failed := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-fault-original")
			deliveryExec(t, db, fault.remove)
			assertIntegrationErrorCode(t, failed, 503, "release_unavailable")
			read := rollbackOrderResponse(t, releaseActorReadAllDetails(t, app, publisher, "GET", path, "", ""), 200)
			if !reflect.DeepEqual(read, original) || !reflect.DeepEqual(beforeFacts, releaseNotificationFacts(t, db, original.ID)) || readApprovalProgress(t, app, applicant, path) != beforeNotice {
				t.Fatal("failed preview leaked instances/version/history/business/notification")
			}
		})
	}
	response := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-fault-original")
	preview := decodeRollbackFlowPreview(t, response.Code, response.Body.Bytes())
	if preview.Version != "5" || len(preview.Flows) != 2 {
		t.Fatal("original failed request did not save exactly once", preview)
	}
	// Read-only GET and replay do not attempt to rewrite the saved workflow.
	deliveryExec(t, db, `CREATE TRIGGER reject_rollback_read_write BEFORE UPDATE ON rcc_release_orders FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='read cannot mutate'`)
	read := readRollbackFlows(t, app, path)
	replay := releaseActorRequest(t, app, publisher, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-fault-original")
	deliveryExec(t, db, `DROP TRIGGER reject_rollback_read_write`)
	if read.Version != "5" || !reflect.DeepEqual(read.Flows, preview.Flows) || replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("read or replay rewrote acknowledged instances")
	}
	if readApprovalProgress(t, app, applicant, path) != beforeNotice {
		t.Fatal("preview issued a result reminder")
	}
}

// A saved workflow gives no additional recovery period or authority over newer
// values. Completion after preview permanently closes the original order.
func TestRollbackFlowPreviewDoesNotExtendRecoveryOrFreezeBusinessValues(t *testing.T) {
	app, db := batchEdgeApplication(t, `INSERT INTO mutation_add_items(id,code,label) VALUES(10,'rollback-live','old')`)
	enableMutationPolicy(t, app, "mutation_add_items", mutationPolicyFixture{AllowModify: true})
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), `{"title":"恢复仍检查当前值","items":[{"table_name":"mutation_add_items","operation":"MODIFY","id":"10","expected_record_version":"0","content":{"label":"published"}}]}`, "rollback-live")
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-live-publish"), 200)
	response := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-live-preview")
	preview := decodeRollbackFlowPreview(t, response.Code, response.Body.Bytes())
	body := quickRollbackBody(preview.Version, preview.Digest, "")
	deliveryExec(t, db, `UPDATE mutation_add_items SET label='external-newer' WHERE id=10`)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback", body, "rollback-live-attempt"), 409, "record_version_conflict")
	row, version := recordVersionRow(t, app, "mutation_add_items", "10")
	if *row["label"] != "external-newer" || version != "1" {
		t.Fatal("saved preview overwrote newer business value", row, version)
	}
	completed := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/complete", fmt.Sprintf(`{"expected_version":%q}`, preview.Version), "rollback-live-complete"), 200)
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback", quickRollbackBody(completed.Version, preview.Digest, ""), "rollback-live-after-complete"), 422, "release_state_invalid")
	flows := readRollbackFlows(t, app, path)
	for _, flow := range flows.Flows {
		for _, node := range flow.Nodes {
			if node.State != "STOPPED" || node.ActorID != "" || node.At != "" {
				t.Fatal("completion fabricated a restoration fact", flow)
			}
		}
	}
}
