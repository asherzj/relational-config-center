//go:build integration

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const standardReleaseTemplateBody = `{"code":"ordinary_release_v1","name":"常规发布","description":"按表审批后发布并完结","type":"STANDARD","node_list":[{"code":"approval","type":"APPROVAL","name":"按表审批","required_role":"TABLE_APPROVER"},{"code":"publication","type":"PUBLICATION","name":"发布","required_role":"PUBLISHER"},{"code":"completion","type":"COMPLETION","name":"完结","required_role":"PUBLISHER"}]}`
const emergencyReleaseTemplateBody = `{"code":"urgent_release_v1","name":"应急发布","description":"免审批发布并完结","type":"EMERGENCY","node_list":[{"code":"publication","type":"PUBLICATION","name":"应急发布","required_role":"PUBLISHER"},{"code":"completion","type":"COMPLETION","name":"完结","required_role":"PUBLISHER"}]}`

func templateRequest(t *testing.T, app *adminApplication, method, path, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	session := integrationAdminSession(t, app)
	headers := map[string]string{}
	if key != "" {
		headers["Idempotency-Key"] = key
	}
	return accountRequestFrom(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", headers)
}

func TestReleaseTemplateHTTPMaintainsMultipleTypesWithIdempotencyAndVersions(t *testing.T) {
	app := startIntegrationApplication(t)
	created := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", emergencyReleaseTemplateBody, "template-create-urgent-001")
	if created.Code != http.StatusCreated {
		t.Fatalf("create emergency template: %d %s", created.Code, created.Body)
	}
	var createdTemplate struct {
		Creator     string          `json:"creator"`
		MonitorList []string        `json:"monitor_list"`
		Nodes       json.RawMessage `json:"node_list"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdTemplate); err != nil || createdTemplate.Creator != integrationAccountID(t, app) || createdTemplate.MonitorList == nil || len(createdTemplate.Nodes) == 0 {
		t.Fatalf("creation audit or node contract invalid: %v: %#v %s", err, createdTemplate, created.Body)
	}
	replayed := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", emergencyReleaseTemplateBody, "template-create-urgent-001")
	if replayed.Code != http.StatusCreated || replayed.Body.String() != created.Body.String() {
		t.Fatalf("idempotent create changed result: %d %s", replayed.Code, replayed.Body)
	}
	if response := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "template-create-standard-001"); response.Code != http.StatusCreated {
		t.Fatalf("create standard template: %d %s", response.Code, response.Body)
	}
	if response := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "template-create-urgent-001"); response.Code != http.StatusConflict {
		t.Fatalf("same request key with different content: %d %s", response.Code, response.Body)
	}
	list := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates", "", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list templates: %d %s", list.Code, list.Body)
	}
	var catalog struct {
		Templates []json.RawMessage `json:"templates"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &catalog); err != nil || len(catalog.Templates) < 4 {
		t.Fatalf("expected seeded and custom templates: %v %s", err, list.Body)
	}
	updated := `{"code":"ordinary_release_v1","name":"常规发布（新版）","description":"保留输入","type":"STANDARD","node_list":[{"code":"approval","type":"APPROVAL","name":"业务审批","required_role":"TABLE_APPROVER"},{"code":"publication","type":"PUBLICATION","name":"人工发布","required_role":"PUBLISHER"},{"code":"completion","type":"COMPLETION","name":"人工完结","required_role":"PUBLISHER"}],"expected_version":"1"}`
	if response := templateRequest(t, app, http.MethodPut, "/api/v1/release-templates/ordinary_release_v1", updated, "template-replace-first"); response.Code != http.StatusOK {
		t.Fatalf("replace current template: %d %s", response.Code, response.Body)
	}
	if response := templateRequest(t, app, http.MethodPut, "/api/v1/release-templates/ordinary_release_v1", updated, "template-replace-stale"); response.Code != http.StatusConflict {
		t.Fatalf("stale replacement: %d %s", response.Code, response.Body)
	}
	if response := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates/default_standard_v1/disable", `{"expected_version":"1"}`, "template-disable-default"); response.Code != http.StatusOK {
		t.Fatalf("disable standard template: %d %s", response.Code, response.Body)
	}
	assertHealth(t, app, "/health/ready", http.StatusOK, `{"status":"ready"}`)
}

func TestReleaseTemplateHTTPProtectsEmergencyAndRejectsInvalidNodes(t *testing.T) {
	app := startIntegrationApplication(t)
	invalid := `{"code":"invalid_release_v1","name":"非法流程","description":"","type":"EMERGENCY","node_list":[{"code":"approval","type":"APPROVAL","name":"审批","required_role":"APPROVER"},{"code":"publication","type":"PUBLICATION","name":"发布","required_role":"PUBLISHER"}]}`
	if response := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", invalid, "template-create-invalid-001"); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid nodes: %d %s", response.Code, response.Body)
	}
	if response := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates/default_emergency_v1/disable", `{"expected_version":"1"}`, "template-protect-disable"); response.Code != http.StatusConflict {
		t.Fatalf("disable emergency template: %d %s", response.Code, response.Body)
	}
	if response := templateRequest(t, app, http.MethodDelete, "/api/v1/release-templates/default_emergency_v1", `{"expected_version":"1"}`, "template-protect-delete"); response.Code != http.StatusConflict {
		t.Fatalf("delete emergency template: %d %s", response.Code, response.Body)
	}
	invalidEdit := `{"code":"default_emergency_v1","name":"不能保存","description":"","type":"EMERGENCY","node_list":[{"code":"approval","type":"APPROVAL","name":"审批","required_role":"TABLE_APPROVER"},{"code":"completion","type":"COMPLETION","name":"完结","required_role":"PUBLISHER"}],"expected_version":"1"}`
	if response := templateRequest(t, app, http.MethodPut, "/api/v1/release-templates/default_emergency_v1", invalidEdit, "template-invalid-edit"); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid emergency edit: %d %s", response.Code, response.Body)
	}
	changedType := strings.Replace(strings.TrimSuffix(standardReleaseTemplateBody, "}"), "ordinary_release_v1", "default_emergency_v1", 1) + `,"expected_version":"1"}`
	response := templateRequest(t, app, http.MethodPut, "/api/v1/release-templates/default_emergency_v1", changedType, "template-immutable-type")
	assertIntegrationErrorCode(t, response, http.StatusUnprocessableEntity, "invalid_release_template")
	changedCode := strings.TrimSuffix(emergencyReleaseTemplateBody, "}") + `,"expected_version":"1"}`
	response = templateRequest(t, app, http.MethodPut, "/api/v1/release-templates/default_emergency_v1", changedCode, "template-immutable-code")
	assertIntegrationErrorCode(t, response, http.StatusUnprocessableEntity, "invalid_release_template")
	current := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates/default_emergency_v1", "", "")
	if current.Code != http.StatusOK || !strings.Contains(current.Body.String(), `"name":"默认应急发布"`) {
		t.Fatalf("invalid edit changed emergency template: %d %s", current.Code, current.Body)
	}
}

func TestReleaseTemplateHTTPRejectsNonAdministratorManagement(t *testing.T) {
	app := startIntegrationApplication(t)
	account := registerAccount(t, app, "template.viewer", "template.viewer@example.com", "correct horse battery staple")
	response := accountRequest(app, http.MethodGet, "/api/v1/release-templates", "", account.Result().Cookies(), "")
	assertIntegrationErrorCode(t, response, http.StatusForbidden, "permission_denied")
}

func TestReleaseTemplateHTTPReplaysCommittedReplacementAfterLaterChanges(t *testing.T) {
	app := startIntegrationApplication(t)
	path := "/api/v1/release-templates/ordinary_release_v1"
	created := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "replace-replay-create")
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	body := strings.TrimSuffix(standardReleaseTemplateBody, "}") + `,"expected_version":"1"}`
	body = strings.Replace(body, "常规发布", "首次保存", 1)
	first := templateRequest(t, app, http.MethodPut, path, body, "replace-replay-first")
	if first.Code != http.StatusOK {
		t.Fatalf("replace: %d %s", first.Code, first.Body)
	}
	// A client that lost the successful response resends the captured request.
	replay := templateRequest(t, app, http.MethodPut, path, body, "replace-replay-first")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("committed replacement must replay its original response: first=%d %s replay=%d %s", first.Code, first.Body, replay.Code, replay.Body)
	}
	laterBody := strings.Replace(strings.Replace(body, "首次保存", "后续保存", 1), `"expected_version":"1"`, `"expected_version":"2"`, 1)
	later := templateRequest(t, app, http.MethodPut, path, laterBody, "replace-replay-later")
	if later.Code != http.StatusOK {
		t.Fatalf("later replacement: %d %s", later.Code, later.Body)
	}
	replay = templateRequest(t, app, http.MethodPut, path, body, "replace-replay-first")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("later version changed original result: %d %s", replay.Code, replay.Body)
	}
	current := templateRequest(t, app, http.MethodGet, path, "", "")
	if current.Body.String() != later.Body.String() {
		t.Fatalf("replay changed current template: %s", current.Body)
	}
	conflict := templateRequest(t, app, http.MethodPut, path, laterBody, "replace-replay-first")
	assertIntegrationErrorCode(t, conflict, http.StatusConflict, "idempotency_conflict")
	deleted := templateRequest(t, app, http.MethodDelete, path, `{"expected_version":"3"}`, "replace-replay-delete")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete replacement target: %d %s", deleted.Code, deleted.Body)
	}
	replay = templateRequest(t, app, http.MethodPut, path, body, "replace-replay-first")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("replacement replay after deletion: %d %s", replay.Code, replay.Body)
	}
	recreated := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "replace-replay-recreate")
	if recreated.Code != http.StatusCreated {
		t.Fatalf("recreate replacement target: %d %s", recreated.Code, recreated.Body)
	}
	replay = templateRequest(t, app, http.MethodPut, path, body, "replace-replay-first")
	if replay.Code != first.Code || replay.Body.String() != first.Body.String() {
		t.Fatalf("replacement replay after recreation: %d %s", replay.Code, replay.Body)
	}
	current = templateRequest(t, app, http.MethodGet, path, "", "")
	if current.Body.String() != recreated.Body.String() {
		t.Fatalf("old replacement changed new template: %s", current.Body)
	}

}

func TestReleaseTemplateHTTPReplaysEnableAndDisableWithoutChangingLaterState(t *testing.T) {
	app := startIntegrationApplication(t)
	path := "/api/v1/release-templates/ordinary_release_v1"
	created := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "state-replay-create")
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	disabled := templateRequest(t, app, http.MethodPost, path+"/disable", `{"expected_version":"1"}`, "state-replay-disable")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable: %d %s", disabled.Code, disabled.Body)
	}
	replay := templateRequest(t, app, http.MethodPost, path+"/disable", `{"expected_version":"1"}`, "state-replay-disable")
	if replay.Code != disabled.Code || replay.Body.String() != disabled.Body.String() {
		t.Fatalf("disable replay changed original result: %d %s", replay.Code, replay.Body)
	}
	enabled := templateRequest(t, app, http.MethodPost, path+"/enable", `{"expected_version":"2"}`, "state-replay-enable")
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable: %d %s", enabled.Code, enabled.Body)
	}
	for _, operation := range []struct {
		action, version string
		original        *httptest.ResponseRecorder
	}{{"disable", "1", disabled}, {"enable", "2", enabled}} {
		body := `{"expected_version":"` + operation.version + `"}`
		replay = templateRequest(t, app, http.MethodPost, path+"/"+operation.action, body, "state-replay-"+operation.action)
		if replay.Code != operation.original.Code || replay.Body.String() != operation.original.Body.String() {
			t.Fatalf("%s replay changed original result: %d %s", operation.action, replay.Code, replay.Body)
		}
		conflict := templateRequest(t, app, http.MethodPost, path+"/"+operation.action, `{"expected_version":"3"}`, "state-replay-"+operation.action)
		assertIntegrationErrorCode(t, conflict, http.StatusConflict, "idempotency_conflict")
	}
	current := templateRequest(t, app, http.MethodGet, path, "", "")
	if current.Body.String() != enabled.Body.String() {
		t.Fatalf("replays changed current state or audit: %s", current.Body)
	}
	deleted := templateRequest(t, app, http.MethodDelete, path, `{"expected_version":"3"}`, "state-replay-delete")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete state target: %d %s", deleted.Code, deleted.Body)
	}
	for _, stage := range []string{"deleted", "recreated"} {
		var recreated *httptest.ResponseRecorder
		if stage == "recreated" {
			recreated = templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "state-replay-recreate")
			if recreated.Code != http.StatusCreated {
				t.Fatalf("recreate state target: %d %s", recreated.Code, recreated.Body)
			}
		}
		for _, operation := range []struct {
			action, version string
			original        *httptest.ResponseRecorder
		}{{"disable", "1", disabled}, {"enable", "2", enabled}} {
			replay = templateRequest(t, app, http.MethodPost, path+"/"+operation.action, `{"expected_version":"`+operation.version+`"}`, "state-replay-"+operation.action)
			if replay.Code != operation.original.Code || replay.Body.String() != operation.original.Body.String() {
				t.Fatalf("%s replay after %s: %d %s", operation.action, stage, replay.Code, replay.Body)
			}
		}
		if recreated != nil {
			current = templateRequest(t, app, http.MethodGet, path, "", "")
			if current.Body.String() != recreated.Body.String() {
				t.Fatalf("old state requests changed new template: %s", current.Body)
			}
		}
	}

}

func TestReleaseTemplateHTTPReplaysDeletionWithoutDeletingRecreatedTemplate(t *testing.T) {
	app := startIntegrationApplication(t)
	path := "/api/v1/release-templates/ordinary_release_v1"
	created := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "delete-replay-create")
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	body := `{"expected_version":"1"}`
	deleted := templateRequest(t, app, http.MethodDelete, path, body, "delete-replay-delete")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", deleted.Code, deleted.Body)
	}
	replay := templateRequest(t, app, http.MethodDelete, path, body, "delete-replay-delete")
	if replay.Code != deleted.Code || replay.Body.String() != deleted.Body.String() {
		t.Fatalf("deleted template must replay successful removal: %d %s", replay.Code, replay.Body)
	}
	missing := templateRequest(t, app, http.MethodGet, path, "", "")
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "release_template_not_found")
	recreated := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", strings.Replace(standardReleaseTemplateBody, "常规发布", "同编码的新模板", 1), "delete-replay-new-create")
	if recreated.Code != http.StatusCreated {
		t.Fatalf("recreate: %d %s", recreated.Code, recreated.Body)
	}
	conflict := templateRequest(t, app, http.MethodDelete, path, `{"expected_version":"2"}`, "delete-replay-delete")
	assertIntegrationErrorCode(t, conflict, http.StatusConflict, "idempotency_conflict")
	replay = templateRequest(t, app, http.MethodDelete, path, body, "delete-replay-delete")
	if replay.Code != http.StatusNoContent {
		t.Fatalf("delete replay after recreation: %d %s", replay.Code, replay.Body)
	}
	createReplay := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "delete-replay-create")
	if createReplay.Code != created.Code || createReplay.Body.String() != created.Body.String() {
		t.Fatalf("old creation replay changed original result: %d %s", createReplay.Code, createReplay.Body)
	}
	current := templateRequest(t, app, http.MethodGet, path, "", "")
	if current.Code != http.StatusOK || current.Body.String() != recreated.Body.String() {
		t.Fatalf("old requests changed recreated template: %d %s", current.Code, current.Body)
	}
}

func TestReleaseTemplateHTTPStandardDeletionKeepsReadOnlyReadiness(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	deleted := templateRequest(t, app, http.MethodDelete, "/api/v1/release-templates/default_standard_v1", `{"expected_version":"1"}`, "readiness-standard-delete")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete default standard template: %d %s", deleted.Code, deleted.Body)
	}
	assertHealth(t, app, "/health/ready", http.StatusOK, `{"status":"ready"}`)
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	deliveryExec(t, db, `CREATE USER 'template_reader'@'%' IDENTIFIED BY 'rcc_password'`)
	deliveryExec(t, db, `GRANT SELECT ON rcc_test.* TO 'template_reader'@'%'`)
	reader := *driver
	reader.User = "template_reader"
	readOnlyApp, err := newApplication(ctx, integrationConfig(&reader))
	if err != nil {
		t.Fatalf("read-only startup after legitimate deletion: %v", err)
	}
	t.Cleanup(func() { _ = readOnlyApp.Close() })
	assertHealth(t, readOnlyApp, "/health/ready", http.StatusOK, `{"status":"ready"}`)
	if _, err := deliveryDB(t, &reader).Exec(`UPDATE rcc_release_templates SET version=version+1`); err == nil {
		t.Fatal("readiness account unexpectedly has write access")
	}
	missing := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates/default_standard_v1", "", "")
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "release_template_not_found")
	// Emergency availability remains a required invariant of the read-only check.
	deliveryExec(t, db, `DELETE FROM rcc_release_templates WHERE code='default_emergency_v1'`)
	assertHealth(t, readOnlyApp, "/health/ready", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
}

func TestReleaseTemplateHTTPChecksCurrentAuthorizationBeforeReplayingAllWrites(t *testing.T) {
	app := startIntegrationApplication(t)
	root := integrationAdminSession(t, app)
	actor := registerAccount(t, app, "template.actor", "template.actor@example.com", "correct horse battery staple")
	actorID := accountID(t, actor)
	path := "/api/v1/release-templates/ordinary_release_v1"
	request := func(method, path, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, method, path, body, actor.Result().Cookies(), sessionCSRF(t, actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key, "X-RCC-Account-ID": accountID(t, root), "X-RCC-Roles": "ADMIN"})
	}
	writes := []struct {
		method, path, body, key string
		status                  int
	}{
		{http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "authorized-create", http.StatusCreated},
		{http.MethodPut, path, strings.TrimSuffix(standardReleaseTemplateBody, "}") + `,"expected_version":"1"}`, "authorized-replace", http.StatusOK},
		{http.MethodPost, path + "/disable", `{"expected_version":"2"}`, "authorized-disable", http.StatusOK},
		{http.MethodPost, path + "/enable", `{"expected_version":"3"}`, "authorized-enable", http.StatusOK},
		{http.MethodDelete, path, `{"expected_version":"4"}`, "authorized-delete", http.StatusNoContent},
	}
	for _, write := range writes {
		assertIntegrationErrorCode(t, request(write.method, write.path, write.body, write.key), http.StatusForbidden, "permission_denied")
	}
	grant := templateRequest(t, app, http.MethodPut, "/api/v1/account-roles/"+actorID, `{"roles":["ADMIN"],"expected_version":"1"}`, "template-grant-admin")
	if grant.Code != http.StatusOK {
		t.Fatalf("grant administrator: %d %s", grant.Code, grant.Body)
	}
	for _, write := range writes {
		response := request(write.method, write.path, write.body, write.key)
		if response.Code != write.status {
			t.Fatalf("authorized %s: %d %s", write.key, response.Code, response.Body)
		}
		if write.status != http.StatusNoContent {
			var audit struct{ Creator, Modifier string }
			if err := json.Unmarshal(response.Body.Bytes(), &audit); err != nil || audit.Creator != actorID || audit.Modifier != actorID {
				t.Fatalf("forged identity affected audit: %v %s", err, response.Body)
			}
		}
	}
	demote := templateRequest(t, app, http.MethodPut, "/api/v1/account-roles/"+actorID, `{"roles":["VIEWER"],"expected_version":"2"}`, "template-revoke-admin")
	if demote.Code != http.StatusOK {
		t.Fatalf("revoke administrator: %d %s", demote.Code, demote.Body)
	}
	for _, write := range writes {
		assertIntegrationErrorCode(t, request(write.method, write.path, write.body, write.key), http.StatusForbidden, "permission_denied")
	}
}

func TestReleaseTemplateHTTPResultStorageFailureRollsBackEveryWrite(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t)
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	path := "/api/v1/release-templates/ordinary_release_v1"
	for _, write := range []struct {
		name, method, path, body, version string
		status                            int
	}{
		{"create", http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "1", http.StatusCreated},
		{"replace", http.MethodPut, path, strings.TrimSuffix(strings.Replace(standardReleaseTemplateBody, "常规发布", "故障后保存", 1), "}") + `,"expected_version":"1"}`, "2", http.StatusOK},
		{"disable", http.MethodPost, path + "/disable", `{"expected_version":"2"}`, "3", http.StatusOK},
		{"enable", http.MethodPost, path + "/enable", `{"expected_version":"3"}`, "4", http.StatusOK},
		{"delete", http.MethodDelete, path, `{"expected_version":"4"}`, "", http.StatusNoContent},
	} {
		t.Run(write.name, func(t *testing.T) {
			before := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates", "", "")
			if before.Code != http.StatusOK {
				t.Fatalf("read before failure: %d %s", before.Code, before.Body)
			}
			// This real dependency failure occurs after the template was changed,
			// when the transaction tries to persist its successful request result.
			deliveryExec(t, db, `CREATE TRIGGER reject_template_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'release-template:%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected template result failure'; END IF; END`)
			failed := templateRequest(t, app, write.method, write.path, write.body, "storage-failure-"+write.name)
			assertIntegrationErrorCode(t, failed, http.StatusServiceUnavailable, "release_template_unavailable")
			after := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates", "", "")
			if after.Code != http.StatusOK || after.Body.String() != before.Body.String() {
				t.Fatalf("failed %s partially changed catalog: before=%s after=%d %s", write.name, before.Body, after.Code, after.Body)
			}
			deliveryExec(t, db, `DROP TRIGGER reject_template_result`)
			retried := templateRequest(t, app, write.method, write.path, write.body, "storage-failure-"+write.name)
			if retried.Code != write.status {
				t.Fatalf("manual retry after dependency recovery: %d %s", retried.Code, retried.Body)
			}
			if write.version != "" && !strings.Contains(retried.Body.String(), `"version":"`+write.version+`"`) {
				t.Fatalf("failed attempt advanced version: %s", retried.Body)
			}
			replayed := templateRequest(t, app, write.method, write.path, write.body, "storage-failure-"+write.name)
			if replayed.Code != retried.Code || replayed.Body.String() != retried.Body.String() {
				t.Fatalf("retry did not retain its original result: %d %s", replayed.Code, replayed.Body)
			}
		})
	}
}

func TestReleaseTemplateHTTPConcurrentRequestsRetainOneResultAndRejectStaleAdministrator(t *testing.T) {
	app := startIntegrationApplication(t)
	admin := integrationAdminSession(t, app)
	second := registerAccount(t, app, "template.second", "template.second@example.com", "correct horse battery staple")
	grant := templateRequest(t, app, http.MethodPut, "/api/v1/account-roles/"+accountID(t, second), `{"roles":["ADMIN"],"expected_version":"1"}`, "concurrent-grant-admin")
	if grant.Code != http.StatusOK {
		t.Fatalf("grant second administrator: %d %s", grant.Code, grant.Body)
	}
	type write struct {
		actor                   *httptest.ResponseRecorder
		method, path, body, key string
	}
	concurrent := func(requests [2]write) [2]*httptest.ResponseRecorder {
		start := make(chan struct{})
		responses := make(chan struct {
			index    int
			response *httptest.ResponseRecorder
		}, 2)
		for i, request := range requests {
			go func() {
				<-start
				response := accountRequestFrom(app, request.method, request.path, request.body, request.actor.Result().Cookies(), sessionCSRF(t, request.actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": request.key})
				responses <- struct {
					index    int
					response *httptest.ResponseRecorder
				}{i, response}
			}()
		}
		close(start)
		var result [2]*httptest.ResponseRecorder
		for range requests {
			response := <-responses
			result[response.index] = response.response
		}
		return result
	}
	assertSameResult := func(request write, status int, version string) {
		t.Helper()
		responses := concurrent([2]write{request, request})
		if responses[0].Code != status || responses[1].Code != status || responses[0].Body.String() != responses[1].Body.String() {
			t.Fatalf("concurrent original requests did not share result: %d %s / %d %s", responses[0].Code, responses[0].Body, responses[1].Code, responses[1].Body)
		}
		if version != "" && !strings.Contains(responses[0].Body.String(), `"version":"`+version+`"`) {
			t.Fatalf("duplicate request advanced version: %s", responses[0].Body)
		}
	}
	path := "/api/v1/release-templates/ordinary_release_v1"
	assertSameResult(write{admin, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "parallel-create"}, http.StatusCreated, "1")
	firstBody := strings.TrimSuffix(strings.Replace(standardReleaseTemplateBody, "常规发布", "管理员甲", 1), "}") + `,"expected_version":"1"}`
	secondBody := strings.Replace(firstBody, "管理员甲", "管理员乙", 1)
	responses := concurrent([2]write{{admin, http.MethodPut, path, firstBody, "parallel-admin-one"}, {second, http.MethodPut, path, secondBody, "parallel-admin-two"}})
	winner := responses[0]
	winnerID := accountID(t, admin)
	if responses[0].Code == http.StatusConflict {
		winner = responses[1]
		winnerID = accountID(t, second)
	}
	if winner.Code != http.StatusOK || responses[0].Code+responses[1].Code != http.StatusOK+http.StatusConflict {
		t.Fatalf("two administrators did not produce one winner and one conflict: %d %s / %d %s", responses[0].Code, responses[0].Body, responses[1].Code, responses[1].Body)
	}
	var audit struct{ Creator, Modifier, Version string }
	if err := json.Unmarshal(winner.Body.Bytes(), &audit); err != nil || audit.Creator != accountID(t, admin) || audit.Modifier != winnerID || audit.Version != "2" {
		t.Fatalf("concurrent audit or version invalid: %v %s", err, winner.Body)
	}
	current := templateRequest(t, app, http.MethodGet, path, "", "")
	if current.Code != http.StatusOK || current.Body.String() != winner.Body.String() {
		t.Fatalf("current template differs from winning result: %d %s", current.Code, current.Body)
	}
	assertSameResult(write{admin, http.MethodPut, path, strings.Replace(firstBody, `"expected_version":"1"`, `"expected_version":"2"`, 1), "parallel-replace"}, http.StatusOK, "3")
	assertSameResult(write{admin, http.MethodPost, path + "/disable", `{"expected_version":"3"}`, "parallel-disable"}, http.StatusOK, "4")
	assertSameResult(write{admin, http.MethodPost, path + "/enable", `{"expected_version":"4"}`, "parallel-enable"}, http.StatusOK, "5")
	assertSameResult(write{admin, http.MethodDelete, path, `{"expected_version":"5"}`, "parallel-delete"}, http.StatusNoContent, "")
	missing := templateRequest(t, app, http.MethodGet, path, "", "")
	assertIntegrationErrorCode(t, missing, http.StatusNotFound, "release_template_not_found")
}

func TestReleaseTemplateHTTPRequiresRequestIdentityAndRejectsChangedTargets(t *testing.T) {
	app := startIntegrationApplication(t)
	path := "/api/v1/release-templates/default_standard_v1"
	body := strings.Replace(standardReleaseTemplateBody, "ordinary_release_v1", "default_standard_v1", 1)
	before := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates", "", "")
	for _, request := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody},
		{http.MethodPut, path, strings.TrimSuffix(body, "}") + `,"expected_version":"1"}`},
		{http.MethodPost, path + "/enable", `{"expected_version":"1"}`},
		{http.MethodPost, path + "/disable", `{"expected_version":"1"}`},
		{http.MethodDelete, path, `{"expected_version":"1"}`},
	} {
		for _, key := range []string{"", "short", "invalid/key"} {
			response := templateRequest(t, app, request.method, request.path, request.body, key)
			assertIntegrationErrorCode(t, response, http.StatusUnprocessableEntity, "invalid_release_template")
		}
	}
	after := templateRequest(t, app, http.MethodGet, "/api/v1/release-templates", "", "")
	if before.Code != http.StatusOK || after.Code != http.StatusOK || before.Body.String() != after.Body.String() {
		t.Fatalf("unidentified writes changed catalog: %s / %s", before.Body, after.Body)
	}
	created := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "bound-target-create")
	if created.Code != http.StatusCreated {
		t.Fatalf("create target: %d %s", created.Code, created.Body)
	}
	conflict := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", body, "bound-target-create")
	assertIntegrationErrorCode(t, conflict, http.StatusConflict, "idempotency_conflict")
	duplicate := templateRequest(t, app, http.MethodPost, "/api/v1/release-templates", standardReleaseTemplateBody, "bound-duplicate-code")
	assertIntegrationErrorCode(t, duplicate, http.StatusConflict, "release_template_exists")
	for _, request := range []struct {
		action, method, suffix, body, alternate string
		status                                  int
	}{
		{"replace", http.MethodPut, "", strings.TrimSuffix(standardReleaseTemplateBody, "}") + `,"expected_version":"1"}`, strings.TrimSuffix(body, "}") + `,"expected_version":"1"}`, http.StatusOK},
		{"disable", http.MethodPost, "/disable", `{"expected_version":"2"}`, `{"expected_version":"1"}`, http.StatusOK},
		{"enable", http.MethodPost, "/enable", `{"expected_version":"3"}`, `{"expected_version":"1"}`, http.StatusOK},
		{"delete", http.MethodDelete, "", `{"expected_version":"4"}`, `{"expected_version":"1"}`, http.StatusNoContent},
	} {
		first := templateRequest(t, app, request.method, "/api/v1/release-templates/ordinary_release_v1"+request.suffix, request.body, "bound-target-"+request.action)
		if first.Code != request.status {
			t.Fatalf("initial %s: %d %s", request.action, first.Code, first.Body)
		}
		changed := templateRequest(t, app, request.method, path+request.suffix, request.alternate, "bound-target-"+request.action)
		assertIntegrationErrorCode(t, changed, http.StatusConflict, "idempotency_conflict")
	}
}

func TestReleaseTemplateHTTPControlTableRejectsGenericManagement(t *testing.T) {
	app := startIntegrationApplication(t)
	for _, request := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "/api/v1/database-tables/rcc_release_templates", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/table-policies", tablePolicyCodePayload("rcc_release_templates", "unused_query_v1", "unused_mutation_v1"), http.StatusForbidden},
		{http.MethodPut, "/api/v1/table-policies/rcc_release_templates", tablePolicyCodePayload("rcc_release_templates", "unused_query_v1", "unused_mutation_v1"), http.StatusForbidden},
		{http.MethodPost, "/api/v1/tables/rcc_release_templates/query", `{}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/release-orders", `{"title":"不得修改控制表","items":[{"table_name":"rcc_release_templates","operation":"DELETE","id":"1","expected_record_version":"0","content":{}}]}`, http.StatusForbidden},
	} {
		response := templateRequest(t, app, request.method, request.path, request.body, "template-protected-table")
		if response.Code != request.status {
			t.Fatalf("generic management of template catalog: %s %s: %d %s", request.method, request.path, response.Code, response.Body)
		}
		if request.status == http.StatusForbidden {
			assertIntegrationErrorCode(t, response, http.StatusForbidden, "protected_table")
		}
	}
}
