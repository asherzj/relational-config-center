//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// AC-019: opening restoration is an explicit, versioned save on the original
// order. Only the first preview chooses templates; GET never instantiates.
func TestRollbackFlowAC019PersistsCurrentEmergencyInstancesBeforeDisplay(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	mutation := mutationPolicyPayload("rollback_mutation_v1", "single_table_mutation", true, true, true, "null", "null", "null", "null")
	if r := policyIntegrationRequest(t, app, "POST", "/api/v1/mutation-policies", mutation); r.Code != 201 {
		t.Fatal(r.Body)
	}
	if r := policyIntegrationRequest(t, app, "POST", "/api/v1/mutation-policies/rollback_mutation_v1/activate", ""); r.Code != 200 {
		t.Fatal(r.Body)
	}
	for _, table := range []string{"policy_alpha", "policy_beta"} {
		body := strings.TrimSuffix(tablePolicyCodePayload(table, "association_query_v1", "rollback_mutation_v1"), "}") + `,"expected_version":"2"}`
		if r := templateRequest(t, app, "PUT", "/api/v1/table-policies/"+table, body, "rollback-policy-"+table); r.Code != 200 {
			t.Fatal(r.Body)
		}
	}
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	path := approvePublication(t, app, publicationFixtureReviewer(t, app), flowDraftBody, "rollback-flow")
	published := flowResponse(t, releaseRequest(t, app, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-flow-publish"), 200)
	if response := templateRequest(t, app, "POST", "/api/v1/release-templates", emergencyReleaseTemplateBody, "rollback-template"); response.Code != 201 {
		t.Fatal(response.Body)
	}
	bindRollbackTemplate(t, app, "policy_alpha", "urgent_release_v1")
	before := readRollbackFlows(t, app, path)
	if before.Version != "4" || before.Flows == nil || len(before.Flows) != 0 {
		t.Fatal("ordinary detail read instantiated restoration", before)
	}
	response := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-save")
	preview := decodeRollbackFlowPreview(t, response.Code, response.Body.Bytes())
	if preview.Version != "5" || preview.ReleaseType != "EMERGENCY" || len(preview.Flows) != 2 {
		t.Fatalf("first visible restoration lacks persisted emergency instances/new order version: %+v", preview)
	}
	alpha, beta := preview.Flows[0], preview.Flows[1]
	if alpha.TableName != "policy_alpha" || alpha.TemplateCode != "urgent_release_v1" || beta.TableName != "policy_beta" || beta.TemplateCode != "default_emergency_v1" || alpha.InstanceID == "" || alpha.InstanceID == beta.InstanceID || len(alpha.Nodes) != 2 || alpha.Nodes[0].State != "ACTIVE" || alpha.Nodes[1].State != "PENDING" {
		t.Fatalf("incorrect per-table restoration definitions: %+v", preview.Flows)
	}
	updated := strings.TrimSuffix(strings.ReplaceAll(emergencyReleaseTemplateBody, "应急发布", "后来修改的应急发布"), "}") + `,"expected_version":"1"}`
	if r := templateRequest(t, app, "PUT", "/api/v1/release-templates/urgent_release_v1", updated, "rollback-template-update"); r.Code != 200 {
		t.Fatal(r.Body)
	}
	bindRollbackTemplate(t, app, "policy_beta", "urgent_release_v1")
	read := readRollbackFlows(t, app, path)
	if read.Version != "5" || !reflect.DeepEqual(read.Flows, preview.Flows) || !reflect.DeepEqual(flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200).TableFlows, published.TableFlows) {
		t.Fatal("detail changed the saved restoration or original forward flows", read)
	}
	for _, endpoint := range []string{"/api/v1/release-orders?id=" + published.ID, "/api/v1/release-orders?view=all&id=" + published.ID} {
		var page struct {
			Orders []struct {
				Flows []flowAcceptanceInstance `json:"rollback_table_flows"`
			} `json:"orders"`
		}
		r := releaseRequest(t, app, "GET", endpoint, "", "")
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &page) != nil || len(page.Orders) != 1 || !reflect.DeepEqual(page.Orders[0].Flows, preview.Flows) {
			t.Fatalf("saved restoration lost from list %s: %d %s", endpoint, r.Code, r.Body)
		}
	}
	reopened := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"5"}`, "rollback-preview-reopen")
	second := decodeRollbackFlowPreview(t, reopened.Code, reopened.Body.Bytes())
	if second.Version != "5" || !reflect.DeepEqual(second.Flows, preview.Flows) || second.Digest != preview.Digest {
		t.Fatal("reopening replaced instances or advanced version", second)
	}
	replay := releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-save")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("original preview request did not replay exact saved outcome", replay.Code, replay.Body)
	}
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"5"}`, "rollback-preview-save"), 409, "idempotency_conflict")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-preview-stale"), 409, "release_version_conflict")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/quick-rollback/preview", `{"expected_version":"5"}`, ""), 422, "release_invalid")
}

type rollbackFlowPreview struct {
	Version     string                   `json:"expected_version"`
	ReleaseType string                   `json:"release_type"`
	Digest      string                   `json:"preview_digest"`
	Flows       []flowAcceptanceInstance `json:"table_flows"`
}

func decodeRollbackFlowPreview(t *testing.T, code int, body []byte) rollbackFlowPreview {
	t.Helper()
	var result rollbackFlowPreview
	if code != 200 || json.Unmarshal(body, &result) != nil {
		t.Fatalf("rollback preview: %d %s", code, body)
	}
	return result
}

func readRollbackFlows(t *testing.T, app *adminApplication, path string) struct {
	Version string
	Flows   []flowAcceptanceInstance `json:"rollback_table_flows"`
} {
	t.Helper()
	var result struct {
		Version string
		Flows   []flowAcceptanceInstance `json:"rollback_table_flows"`
	}
	r := releaseRequest(t, app, "GET", path, "", "")
	if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &result) != nil {
		t.Fatalf("rollback flow detail: %d %s", r.Code, r.Body)
	}
	return result
}

func bindRollbackTemplate(t *testing.T, app *adminApplication, table, code string) {
	t.Helper()
	for _, row := range associationCatalog(t, app, table) {
		if row["type"] == "EMERGENCY" {
			r := templateRequest(t, app, "PUT", "/api/v1/table-policies/"+table+"/release-templates/EMERGENCY", associationBody(code, row["version"].(string), true), fmt.Sprintf("rollback-bind-%d", publicationFixtureSequence.Add(1)))
			if r.Code != 200 {
				t.Fatalf("bind rollback template: %d %s", r.Code, r.Body)
			}
			return
		}
	}
	t.Fatal("managed table lacks emergency association")
}

// AC-020/021: the original order restores once, ends immediately and preserves
// the true reviewers as recipients, even when a reviewer performs the reversal.
func TestRollbackFlowAC020EndsOriginalAndRecordsOnlyTrueExecution(t *testing.T) {
	app, _ := batchEdgeApplication(t)
	applicant, reviewer, publisher := releaseNotificationActors(t, app)
	path := approvedReleaseNotificationOrder(t, app, applicant, reviewer, "rollback-flow-end")
	rollbackOrderResponse(t, releaseActorRequest(t, app, publisher, "POST", path+"/execute", `{"expected_version":"3"}`, "rollback-flow-end-publish"), 200)
	beforeApplicant := readApprovalProgress(t, app, applicant, path)
	beforeReviewer := readApprovalProgress(t, app, reviewer, path)
	response := releaseActorRequest(t, app, reviewer, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-flow-end-preview")
	preview := decodeRollbackFlowPreview(t, response.Code, response.Body.Bytes())
	body := quickRollbackBody(preview.Version, preview.Digest, "")
	rolledResponse := releaseActorRequest(t, app, reviewer, "POST", path+"/quick-rollback", body, "rollback-flow-end-execute")
	rolled := rollbackOrderResponse(t, rolledResponse, 200)
	if rolled.State != "ROLLED_BACK" || len(rolled.Executions) != 2 || rolled.Executions[1].ActorID != accountID(t, reviewer) {
		t.Fatal("restoration did not end the original", rolled)
	}
	flows := readRollbackFlows(t, app, path)
	for _, flow := range flows.Flows {
		if flow.Nodes[0].State != "COMPLETED" || flow.Nodes[0].ActorID != accountID(t, reviewer) || flow.Nodes[0].At == "" || flow.Nodes[1].State != "STOPPED" || flow.Nodes[1].ActorID != "" || flow.Nodes[1].At != "" {
			t.Fatalf("restoration fabricated completion or missed real execution: %+v", flow)
		}
	}
	if len(flows.Flows) != 2 {
		t.Fatal("missing saved restoration flows")
	}
	for _, event := range rolled.History {
		if event.Action == "COMPLETE" {
			t.Fatal("restoration appended manual completion")
		}
	}
	assertReleaseNotificationAdvance(t, app, applicant, path, beforeApplicant)
	if readApprovalProgress(t, app, reviewer, path) != beforeReviewer {
		t.Fatal("own action changed prior unread notification")
	}
	replay := releaseActorRequest(t, app, reviewer, "POST", path+"/quick-rollback/preview", `{"expected_version":"4"}`, "rollback-flow-end-preview")
	if replay.Code != 200 || replay.Body.String() != response.Body.String() {
		t.Fatal("preview replay reread current state instead of its original result", replay.Body)
	}
	if readRollbackFlows(t, app, path).Version != rolled.Version {
		t.Fatal("historical preview rolled back the current order version")
	}
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/quick-rollback/preview", fmt.Sprintf(`{"expected_version":%q}`, rolled.Version), "rollback-flow-end-new-preview"), 422, "release_state_invalid")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/quick-rollback", quickRollbackBody(rolled.Version, preview.Digest, ""), "rollback-flow-end-again"), 422, "release_state_invalid")
	assertIntegrationErrorCode(t, releaseActorRequest(t, app, reviewer, "POST", path+"/complete", fmt.Sprintf(`{"expected_version":%q}`, rolled.Version), "rollback-flow-end-complete"), 422, "release_state_invalid")
}

// AC-021: a submitted emergency order has a real application but no approvals.
// Cancelling it must notify its applicant without creating imaginary review work.
func TestRollbackFlowAC021EmergencyCancellationUsesRealSubmission(t *testing.T) {
	app, _ := batchEdgeApplication(t)
	applicant, reviewer, _ := releaseNotificationActors(t, app)
	admin := integrationAdminSession(t, app)
	for _, submitted := range []bool{false, true} {
		key := fmt.Sprintf("emergency-cancel-%t", submitted)
		body := fmt.Sprintf(`{"title":"取消应急申请","release_type":"EMERGENCY","items":[{"table_name":"mutation_add_items","operation":"ADD","content":{"code":%q,"label":"new"}}]}`, key)
		order := rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", body, key), 201)
		if order.RollbackTableFlows == nil {
			t.Fatal("create response must expose empty restoration array")
		}
		path := "/api/v1/release-orders/" + order.ID
		if submitted {
			order = rollbackOrderResponse(t, releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1","emergency_reason":"配置应急修正"}`, key+"-submit"), 200)
		}
		if order.RollbackTableFlows == nil {
			t.Fatal("submitted response must expose empty restoration array")
		}
		for _, endpoint := range []string{path, "/api/v1/release-orders?id=" + order.ID, "/api/v1/release-orders?view=all&id=" + order.ID} {
			r := releaseRequest(t, app, "GET", endpoint, "", "")
			if r.Code != 200 || (submitted || endpoint != "/api/v1/release-orders?view=all&id="+order.ID) && !strings.Contains(r.Body.String(), `"rollback_table_flows":[]`) {
				t.Fatalf("empty restoration collection missing from %s: %s", endpoint, r.Body)
			}
		}
		beforeApplicant := readApprovalProgress(t, app, applicant, path)
		beforeReviewer := readApprovalProgress(t, app, reviewer, path)
		response := releaseActorRequest(t, app, admin, "POST", path+"/cancel", fmt.Sprintf(`{"expected_version":%q,"reason":"不再发布"}`, order.Version), key+"-cancel")
		cancelled := rollbackOrderResponse(t, response, 200)
		if cancelled.State != "CANCELLED" || len(cancelled.Approvals) != 0 || cancelled.RollbackTableFlows == nil {
			t.Fatal("cancel introduced approvals", cancelled)
		}
		if submitted {
			assertReleaseNotificationAdvance(t, app, applicant, path, beforeApplicant)
		} else if readApprovalProgress(t, app, applicant, path) != beforeApplicant {
			t.Fatal("unsubmitted draft generated result reminder")
		}
		if readApprovalProgress(t, app, reviewer, path) != beforeReviewer {
			t.Fatal("emergency invented reviewer participation")
		}
		for _, view := range []string{"pending", "handled"} {
			assertNotificationOrders(t, readNotificationPage(t, app, reviewer, "view="+view))
		}
	}
}
