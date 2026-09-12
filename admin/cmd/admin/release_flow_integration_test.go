//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type flowAcceptanceNode struct {
	Code, Type, Name, State string
	RequiredRole            string `json:"required_role"`
	ActorID                 string `json:"actor_id"`
	At                      string
}
type flowAcceptanceInstance struct {
	InstanceID         string               `json:"instance_id"`
	TableName          string               `json:"table_name"`
	ReleaseType        string               `json:"release_type"`
	TemplateCode       string               `json:"template_code"`
	TemplateName       string               `json:"template_name"`
	TemplateVersion    string               `json:"template_version"`
	AssociationVersion string               `json:"association_version"`
	InstantiatedAt     string               `json:"instantiated_at"`
	Nodes              []flowAcceptanceNode `json:"node_list"`
}
type flowAcceptanceOrder struct {
	ID, Title, State, Version string
	ReleaseType               string                   `json:"release_type"`
	TableFlows                []flowAcceptanceInstance `json:"table_flows"`
	MissingFlowTables         []string                 `json:"missing_flow_tables"`
	AllowedActions            []string                 `json:"allowed_actions"`
}

func flowResponse(t *testing.T, r *httptest.ResponseRecorder, status int) flowAcceptanceOrder {
	t.Helper()
	var order flowAcceptanceOrder
	if r.Code != status || json.Unmarshal(r.Body.Bytes(), &order) != nil {
		t.Fatalf("flow response: %d %s", r.Code, r.Body)
	}
	return order
}
func bindFlowTemplate(t *testing.T, app *adminApplication, table, code string, enabled bool) {
	t.Helper()
	version := "0"
	for _, row := range associationCatalog(t, app, table) {
		if row["type"] == "STANDARD" {
			version = row["version"].(string)
		}
	}
	r := templateRequest(t, app, "PUT", "/api/v1/table-policies/"+table+"/release-templates/STANDARD", associationBody(code, version, enabled), fmt.Sprintf("flow-bind-%d", publicationFixtureSequence.Add(1)))
	if r.Code != 200 {
		t.Fatalf("bind flow template: %d %s", r.Code, r.Body)
	}
}
func flowFixture(t *testing.T, app *adminApplication) {
	t.Helper()
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	for _, table := range []string{"policy_alpha", "policy_beta"} {
		createAssociationTable(t, app, table)
		r := templateRequest(t, app, "POST", "/api/v1/table-policies/"+table+"/enable", `{"expected_version":"1"}`, "flow-enable-"+table)
		if r.Code != 200 {
			t.Fatalf("enable flow table: %d %s", r.Code, r.Body)
		}
	}
	r := templateRequest(t, app, "POST", "/api/v1/release-templates", standardReleaseTemplateBody, "flow-template-create")
	if r.Code != 201 {
		t.Fatalf("create flow template: %d %s", r.Code, r.Body)
	}
}

const flowDraftBody = `{"title":"各表保存流程","items":[{"table_name":"policy_alpha","operation":"ADD","content":{"value":"alpha"}},{"table_name":"policy_beta","operation":"ADD","content":{"value":"beta"}}]}`

// #100 AC-009: the first visible response already acknowledges two persistent,
// independent instances. The first detail read occurs after the source changes.
func TestReleaseFlowAC009PersistsIndependentTablesBeforeFirstRead(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	saved := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-two-create"), 201)
	if saved.ReleaseType != "STANDARD" || len(saved.TableFlows) != 2 || len(saved.MissingFlowTables) != 0 {
		t.Fatalf("first response lacks saved per-table STANDARD flows: %+v", saved)
	}
	alpha, beta := saved.TableFlows[0], saved.TableFlows[1]
	if alpha.TableName != "policy_alpha" || alpha.TemplateCode != "ordinary_release_v1" || alpha.TemplateName != "常规发布" || beta.TableName != "policy_beta" || beta.TemplateCode != "default_standard_v1" || alpha.InstanceID == "" || alpha.InstanceID == beta.InstanceID || alpha.InstantiatedAt == "" || alpha.TemplateVersion != "1" || alpha.AssociationVersion != "1" || len(alpha.Nodes) != 3 || alpha.Nodes[0].Name != "按表审批" || alpha.Nodes[0].State != "PENDING" {
		t.Fatalf("wrong independent flow definition: %+v", saved.TableFlows)
	}
	updated := strings.TrimSuffix(strings.ReplaceAll(standardReleaseTemplateBody, "按表审批", "新版审批"), "}") + `,"expected_version":"1"}`
	if r := templateRequest(t, app, "PUT", "/api/v1/release-templates/ordinary_release_v1", updated, "flow-template-before-first-read"); r.Code != 200 {
		t.Fatal(r.Body)
	}
	bindFlowTemplate(t, app, "policy_beta", "ordinary_release_v1", true)
	for range 2 {
		read := flowResponse(t, releaseRequest(t, app, "GET", "/api/v1/release-orders/"+saved.ID, "", ""), 200)
		if !reflect.DeepEqual(read.TableFlows, saved.TableFlows) || read.Version != "1" {
			t.Fatalf("detail read replaced persisted instances: %+v", read)
		}
	}
}

// #100 AC-010: unavailable standard config permits content but blocks submission.
// Repair is an explicit save, and never refreshes the other acknowledged table.
func TestReleaseFlowAC010MissingConfigurationRequiresExplicitSave(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	reviewer := publicationFixtureReviewer(t, app)
	configurePublicationReviewer(t, app, reviewer, "policy_alpha", "policy_beta")
	saved := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-missing-create"), 201)
	path := "/api/v1/release-orders/" + saved.ID
	if len(saved.TableFlows) != 1 || !reflect.DeepEqual(saved.MissingFlowTables, []string{"policy_beta"}) {
		t.Fatalf("missing standard is not explicit: %+v", saved)
	}
	blocked := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "flow-missing-submit")
	assertIntegrationErrorCode(t, blocked, 409, "release_flow_incomplete")
	if strings.Contains(strings.Join(saved.AllowedActions, ","), "submit") {
		t.Fatal("incomplete workflow offered submission")
	}
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	reread := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(reread.TableFlows, saved.TableFlows) || !reflect.DeepEqual(reread.MissingFlowTables, saved.MissingFlowTables) || reread.Version != "1" {
		t.Fatal("GET instantiated newly available config", reread)
	}
	repaired := flowResponse(t, releaseRequest(t, app, "PUT", path, `{"title":"各表保存流程","expected_version":"1","changes":{"upserts":[],"delete_detail_ids":[]}}`, "flow-repair-save"), 200)
	if repaired.Version != "2" || len(repaired.TableFlows) != 2 || len(repaired.MissingFlowTables) != 0 || !reflect.DeepEqual(repaired.TableFlows[0], saved.TableFlows[0]) {
		t.Fatalf("explicit save failed to fill only missing table: %+v", repaired)
	}
	if r := templateRequest(t, app, "POST", "/api/v1/release-templates/ordinary_release_v1/disable", `{"expected_version":"1"}`, "flow-disable-old-template"); r.Code != 200 {
		t.Fatal(r.Body)
	}
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", false)
	submitted := flowResponse(t, releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"2"}`, "flow-submit-old-instances"), 200)
	if submitted.State != "PENDING_APPROVAL" || submitted.TableFlows[0].InstanceID != repaired.TableFlows[0].InstanceID || submitted.TableFlows[1].InstanceID != repaired.TableFlows[1].InstanceID {
		t.Fatalf("old instances did not submit after configuration stopped: %+v", submitted)
	}
	fresh := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-fresh-disabled"), 201)
	if len(fresh.TableFlows) != 0 || !reflect.DeepEqual(fresh.MissingFlowTables, []string{"policy_alpha", "policy_beta"}) {
		t.Fatalf("new draft silently reused disabled config: %+v", fresh)
	}
}

// #100 AC-013: a derived draft reads the current associations; copying saved
// content never carries workflow identity or approval across orders.
func TestReleaseFlowAC013DerivedDraftUsesCurrentConfiguration(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	reviewer := publicationFixtureReviewer(t, app)
	configurePublicationReviewer(t, app, reviewer, "policy_alpha", "policy_beta")
	created := rollbackOrderResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-derived-create"), 201)
	path := "/api/v1/release-orders/" + created.ID
	original := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	source := rollbackOrderResponse(t, releaseRequest(t, app, "POST", path+"/cancel", `{"expected_version":"1","reason":"copy current templates"}`, "flow-derived-cancel"), 200)
	bindFlowTemplate(t, app, "policy_alpha", "default_standard_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "ordinary_release_v1", true)
	copiedResponse := releaseRequest(t, app, "POST", path+"/copy", derivedDraftBody(t, source), "flow-derived-copy")
	copied := flowResponse(t, copiedResponse, 201)
	if copied.ReleaseType != "STANDARD" || len(copied.TableFlows) != 2 || copied.TableFlows[0].TemplateCode != "default_standard_v1" || copied.TableFlows[1].TemplateCode != "ordinary_release_v1" || copied.TableFlows[0].InstanceID == original.TableFlows[0].InstanceID || copied.TableFlows[1].InstanceID == original.TableFlows[1].InstanceID {
		t.Fatalf("copy did not create current independent flow instances: %+v", copied)
	}
	copyPath := "/api/v1/release-orders/" + copied.ID
	rollbackOrderResponse(t, releaseRequest(t, app, "POST", copyPath+"/submit", `{"expected_version":"1"}`, "flow-derived-submit"), 200)
	approved := rollbackOrderResponse(t, releaseActorRequest(t, app, reviewer, "POST", copyPath+"/approve", confirmedApprovalBody(t, app, reviewer, copyPath, "new approval"), "flow-derived-approve"), 200)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	preparedResponse := releaseRequest(t, app, "POST", copyPath+"/reprepare", derivedDraftBody(t, approved), "flow-derived-reprepare")
	prepared := flowResponse(t, preparedResponse, 201)
	domainPrepared := rollbackOrderResponse(t, preparedResponse, 201)
	if prepared.ReleaseType != "STANDARD" || len(prepared.TableFlows) != 2 || prepared.TableFlows[0].TemplateCode != "ordinary_release_v1" || prepared.TableFlows[0].InstanceID == copied.TableFlows[0].InstanceID || prepared.TableFlows[1].InstanceID == copied.TableFlows[1].InstanceID || domainPrepared.ApplicantID != integrationAccountID(t, app) || domainPrepared.State != "DRAFT" {
		t.Fatalf("reprepare inherited workflow or approval: %+v", prepared)
	}
	for _, approval := range domainPrepared.Approvals {
		if approval.Decision != nil || approval.State != "PENDING" {
			t.Fatal("reprepare inherited a completed approval", approval)
		}
	}
	retired := flowResponse(t, releaseRequest(t, app, "GET", copyPath, "", ""), 200)
	if retired.State != "CANCELLED" || retired.TableFlows[0].InstanceID != copied.TableFlows[0].InstanceID {
		t.Fatal("reprepare rewrote source instance", retired)
	}
	replay := releaseRequest(t, app, "POST", copyPath+"/reprepare", derivedDraftBody(t, approved), "flow-derived-reprepare")
	if replay.Code != 201 || !reflect.DeepEqual(flowResponse(t, replay, 201).TableFlows, prepared.TableFlows) {
		t.Fatal("derived replay instantiated again", replay.Body)
	}
}

// #100 AC-014 / B-004: node definitions belong to the saved draft; role
// responsibility belongs to submission and member eligibility remains current.
func TestReleaseFlowAC014SeparatesInstancesFromSubmissionApprovals(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	oldReviewer := registerAccount(t, app, "flow.old", "flow.old@example.com", "correct horse battery staple")
	alphaReviewer := registerAccount(t, app, "flow.alpha", "flow.alpha@example.com", "correct horse battery staple")
	betaReviewer := registerAccount(t, app, "flow.beta", "flow.beta@example.com", "correct horse battery staple")
	oldRole := tableApprovalRole(t, app, "草稿时角色", oldReviewer)
	alphaRole := tableApprovalRole(t, app, "提交时甲角色", alphaReviewer)
	betaRole := tableApprovalRole(t, app, "提交时乙角色", betaReviewer)
	assignTableApproval(t, app, "policy_alpha", oldRole)
	assignTableApproval(t, app, "policy_beta", betaRole)
	saved := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", flowDraftBody, "flow-approval-create"), 201)
	path := "/api/v1/release-orders/" + saved.ID
	assignTableApproval(t, app, "policy_alpha", alphaRole)
	updated := strings.TrimSuffix(strings.ReplaceAll(standardReleaseTemplateBody, "按表审批", "后续新审批节点"), "}") + `,"expected_version":"1"}`
	if r := templateRequest(t, app, "PUT", "/api/v1/release-templates/ordinary_release_v1", updated, "flow-approval-template-edit"); r.Code != 200 {
		t.Fatal(r.Body)
	}
	submittedResponse := releaseRequest(t, app, "POST", path+"/submit", `{"expected_version":"1"}`, "flow-approval-submit")
	submitted := flowResponse(t, submittedResponse, 200)
	domainSubmitted := rollbackOrderResponse(t, submittedResponse, 200)
	if submitted.TableFlows[0].Nodes[0].Name != "按表审批" || submitted.TableFlows[0].Nodes[0].State != "ACTIVE" || submitted.TableFlows[1].Nodes[0].State != "ACTIVE" || submitted.TableFlows[0].InstanceID != saved.TableFlows[0].InstanceID || domainSubmitted.Approvals[0].Roles[0].ID != alphaRole.ID {
		t.Fatalf("submission changed nodes or failed to activate saved instances: %+v", submitted)
	}
	assignTableApproval(t, app, "policy_alpha", oldRole)
	denied := releaseActorRequest(t, app, oldReviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, oldReviewer, path, "old assignment"), "flow-old-reviewer")
	assertIntegrationErrorCode(t, denied, 403, "permission_denied")
	assertIntegrationErrorCode(t, releaseRequest(t, app, "POST", path+"/approve", confirmedApprovalBody(t, app, integrationAdminSession(t, app), path, "no self review"), "flow-self-reviewer"), 403, "permission_denied")
	partial := flowResponse(t, releaseActorRequest(t, app, alphaReviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, alphaReviewer, path, "甲表独立批准"), "flow-alpha-approve"), 200)
	if partial.State != "PENDING_APPROVAL" || partial.TableFlows[0].Nodes[0].State != "COMPLETED" || partial.TableFlows[0].Nodes[0].ActorID != accountID(t, alphaReviewer) || partial.TableFlows[0].Nodes[0].At == "" || partial.TableFlows[0].Nodes[1].State != "PENDING" || partial.TableFlows[1].Nodes[0].State != "ACTIVE" {
		t.Fatal("partial table approval did not advance only its real node", partial)
	}
	approved := flowResponse(t, releaseActorRequest(t, app, betaReviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, betaReviewer, path, "乙表独立批准"), "flow-beta-approve"), 200)
	if approved.State != "APPROVED" || approved.TableFlows[0].Nodes[1].State != "ACTIVE" || approved.TableFlows[1].Nodes[1].State != "ACTIVE" {
		t.Fatal("all-table gate did not activate publication", approved)
	}
	published := flowResponse(t, releaseRequest(t, app, "POST", path+"/execute", fmt.Sprintf(`{"expected_version":%q}`, approved.Version), "flow-publication"), 200)
	for _, flow := range published.TableFlows {
		if flow.Nodes[1].State != "COMPLETED" || flow.Nodes[1].ActorID != integrationAccountID(t, app) || flow.Nodes[1].At == "" || flow.Nodes[2].State != "ACTIVE" {
			t.Fatal("publication node lacks actual execution", flow)
		}
	}
	completed := flowResponse(t, releaseRequest(t, app, "POST", path+"/complete", fmt.Sprintf(`{"expected_version":%q}`, published.Version), "flow-complete"), 200)
	for _, flow := range completed.TableFlows {
		if flow.Nodes[2].State != "COMPLETED" || flow.Nodes[2].ActorID != integrationAccountID(t, app) || flow.Nodes[2].At == "" {
			t.Fatal("completion node lacks real event", flow)
		}
	}
	reread := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(reread.TableFlows, completed.TableFlows) {
		t.Fatal("node progress was not persisted", reread)
	}
}

// #100 AC-011: normal edits retain unaffected flow identity. Removing the last
// detail removes its flow and target; adding the table again reads current config.
func TestReleaseFlowAC011DraftEditsPreserveAndReplaceOnlyParticipatingTables(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	flowFixture(t, app)
	bindFlowTemplate(t, app, "policy_alpha", "ordinary_release_v1", true)
	bindFlowTemplate(t, app, "policy_beta", "default_standard_v1", true)
	body := `{"title":"草稿增删表","items":[{"table_name":"policy_alpha","operation":"ADD","content":{"id":"91","value":"alpha"}}]}`
	createdResponse := releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "flow-edit-create")
	created := flowResponse(t, createdResponse, 201)
	detail := rollbackOrderResponse(t, createdResponse, 201).Items[0]
	path := "/api/v1/release-orders/" + created.ID
	bindFlowTemplate(t, app, "policy_alpha", "default_standard_v1", true)
	editBody := fmt.Sprintf(`{"title":"草稿增删表","expected_version":"1","changes":{"upserts":[{"detail_id":%q,"table_name":"policy_alpha","operation":"ADD","expected_record_version":"0","content":{"id":"91","value":"edited-alpha"}},{"table_name":"policy_beta","operation":"ADD","content":{"id":"92","value":"beta"}}],"delete_detail_ids":[]}}`, detail.DetailID)
	editedResponse := releaseRequest(t, app, "PUT", path, editBody, "flow-edit-append-beta")
	edited := flowResponse(t, editedResponse, 200)
	if edited.Version != "2" || len(edited.TableFlows) != 2 || !reflect.DeepEqual(edited.TableFlows[0], created.TableFlows[0]) || edited.TableFlows[1].TemplateCode != "default_standard_v1" {
		t.Fatalf("edit replaced unaffected flow: %+v", edited)
	}
	stale := releaseRequest(t, app, "PUT", path, editBody, "flow-edit-stale")
	assertIntegrationErrorCode(t, stale, 409, "release_version_conflict")
	removal := fmt.Sprintf(`{"title":"草稿增删表","expected_version":"2","changes":{"upserts":[],"delete_detail_ids":[%q]}}`, detail.DetailID)
	removed := flowResponse(t, releaseRequest(t, app, "PUT", path, removal, "flow-edit-remove-alpha"), 200)
	if removed.Version != "3" || len(removed.TableFlows) != 1 || !reflect.DeepEqual(removed.TableFlows[0], edited.TableFlows[1]) {
		t.Fatal("removed table remained in flow", removed)
	}
	contender := flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders", body, "flow-edit-target-released"), 201)
	readd := `{"title":"草稿增删表","expected_version":"3","changes":{"upserts":[{"table_name":"policy_alpha","operation":"ADD","content":{"id":"91","value":"readded"}}],"delete_detail_ids":[]}}`
	assertIntegrationErrorCode(t, releaseRequest(t, app, "PUT", path, readd, "flow-edit-readd"), 409, "release_target_conflict")
	afterFailure := flowResponse(t, releaseRequest(t, app, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(afterFailure.TableFlows, removed.TableFlows) || afterFailure.Version != "3" {
		t.Fatal("target failure partially instantiated table", afterFailure)
	}
	flowResponse(t, releaseRequest(t, app, "POST", "/api/v1/release-orders/"+contender.ID+"/cancel", `{"expected_version":"1","reason":"释放目标"}`, "flow-edit-free-contender"), 200)
	readded := flowResponse(t, releaseRequest(t, app, "PUT", path, readd, "flow-edit-readd"), 200)
	if readded.Version != "4" || len(readded.TableFlows) != 2 || readded.TableFlows[0].TemplateCode != "default_standard_v1" || readded.TableFlows[0].InstanceID == created.TableFlows[0].InstanceID || !reflect.DeepEqual(readded.TableFlows[1], edited.TableFlows[1]) {
		t.Fatal("readded table did not get current independent flow", readded)
	}
}
