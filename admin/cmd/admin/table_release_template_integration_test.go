//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func createAssociationTable(t *testing.T, app *adminApplication, table string) {
	t.Helper()
	body := tablePolicyCodePayload(table, "association_query_v1", "association_mutation_v1")
	r := templateRequest(t, app, http.MethodPost, "/api/v1/table-policies", body, "create-"+table)
	if r.Code != 201 {
		t.Fatalf("create table: %d %s", r.Code, r.Body)
	}
}
func associationCatalog(t *testing.T, app *adminApplication, table string) []map[string]any {
	t.Helper()
	r := templateRequest(t, app, http.MethodGet, "/api/v1/table-policies/"+table+"/release-templates", "", "")
	if r.Code != 200 {
		t.Fatalf("read associations: %d %s", r.Code, r.Body)
	}
	var body struct {
		Associations []map[string]any `json:"associations"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Associations
}
func associationBody(code, version string, enabled bool) string {
	return fmt.Sprintf(`{"template_code":%q,"enabled":%t,"expected_version":%q}`, code, enabled, version)
}

func TestTableReleaseTemplateHTTPAC006AndAC008SelectShareProtectAndDisable(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	for _, table := range []string{"policy_alpha", "policy_beta"} {
		createAssociationTable(t, app, table)
	}
	for i, body := range []string{standardReleaseTemplateBody, emergencyReleaseTemplateBody} {
		r := templateRequest(t, app, "POST", "/api/v1/release-templates", body, fmt.Sprintf("association-template-%d", i))
		if r.Code != 201 {
			t.Fatalf("template: %d %s", r.Code, r.Body)
		}
	}
	for _, table := range []string{"policy_alpha", "policy_beta"} {
		rows := associationCatalog(t, app, table)
		if len(rows) != 1 || rows[0]["type"] != "EMERGENCY" || rows[0]["template_code"] != "default_emergency_v1" || rows[0]["enabled"] != true {
			t.Fatalf("created table lacks emergency: %#v", rows)
		}
		for _, kind := range []string{"STANDARD", "EMERGENCY"} {
			code, version := "ordinary_release_v1", "0"
			if kind == "EMERGENCY" {
				code, version = "urgent_release_v1", "1"
			}
			r := templateRequest(t, app, "PUT", "/api/v1/table-policies/"+table+"/release-templates/"+kind, associationBody(code, version, true), table+kind)
			if r.Code != 200 || !strings.Contains(r.Body.String(), `"modifier":"`+integrationAccountID(t, app)+`"`) {
				t.Fatalf("association save: %d %s", r.Code, r.Body)
			}
		}
		rows = associationCatalog(t, app, table)
		if len(rows) != 2 {
			t.Fatalf("one per type: %#v", rows)
		}
	}
	path := "/api/v1/table-policies/policy_alpha/release-templates/EMERGENCY"
	wrong := templateRequest(t, app, "PUT", path, associationBody("ordinary_release_v1", "2", true), "wrong-association-type")
	assertIntegrationErrorCode(t, wrong, 422, "invalid_table_release_template")
	off := templateRequest(t, app, "PUT", path, associationBody("urgent_release_v1", "2", false), "disable-emergency-association")
	assertIntegrationErrorCode(t, off, 409, "emergency_association_protected")
	for _, method := range []string{"DELETE", "POST"} {
		r := templateRequest(t, app, method, path, `{"expected_version":"2"}`, "unbind-"+method)
		if r.Code < 400 {
			t.Fatalf("emergency removal accepted: %d %s", r.Code, r.Body)
		}
	}
	referenced := templateRequest(t, app, "DELETE", "/api/v1/release-templates/ordinary_release_v1", `{"expected_version":"1"}`, "delete-referenced-standard")
	assertIntegrationErrorCode(t, referenced, 409, "release_template_in_use")
	standard := templateRequest(t, app, "PUT", "/api/v1/table-policies/policy_alpha/release-templates/STANDARD", associationBody("ordinary_release_v1", "1", false), "disable-standard-association")
	if standard.Code != 200 || !strings.Contains(standard.Body.String(), `"enabled":false`) {
		t.Fatalf("standard disable: %d %s", standard.Code, standard.Body)
	}
	enabled := templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/enable", `{"expected_version":"1"}`, "enable-alpha")
	if enabled.Code != 200 {
		t.Fatalf("enable: %d %s", enabled.Code, enabled.Body)
	}
	disabled := templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/disable", `{"expected_version":"2"}`, "disable-alpha")
	if disabled.Code != 200 {
		t.Fatalf("disable entire table: %d %s", disabled.Code, disabled.Body)
	}
	rows := associationCatalog(t, app, "policy_alpha")
	if len(rows) != 2 || rows[0]["enabled"] != true || rows[0]["template_code"] != "urgent_release_v1" {
		t.Fatalf("invalid operation changed emergency: %#v", rows)
	}
}

func TestTableReleaseTemplateHTTPConcurrentSwitchAndOriginalReplay(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	createAssociationTable(t, app, "policy_alpha")
	for i, body := range []string{emergencyReleaseTemplateBody, strings.ReplaceAll(emergencyReleaseTemplateBody, "urgent_release_v1", "other_urgent_v1")} {
		r := templateRequest(t, app, "POST", "/api/v1/release-templates", body, fmt.Sprintf("parallel-template-%d", i))
		if r.Code != 201 {
			t.Fatalf("template: %d %s", r.Code, r.Body)
		}
	}
	session := integrationAdminSession(t, app)
	second := registerAccount(t, app, "association.second", "association.second@example.com", "correct horse battery staple")
	grant := templateRequest(t, app, "PUT", "/api/v1/account-roles/"+accountID(t, second), `{"roles":["ADMIN"],"expected_version":"1"}`, "association-second-admin")
	if grant.Code != 200 {
		t.Fatalf("grant second administrator: %d %s", grant.Code, grant.Body)
	}
	actors := []*httptest.ResponseRecorder{session, second}
	actorRequest := func(index int, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, "PUT", "/api/v1/table-policies/policy_alpha/release-templates/EMERGENCY", body, actors[index].Result().Cookies(), sessionCSRF(t, actors[index]), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})
	}
	path := "/api/v1/table-policies/policy_alpha/release-templates/EMERGENCY"
	codes := []string{"urgent_release_v1", "other_urgent_v1"}
	responses := make(chan struct {
		index int
		r     *httptest.ResponseRecorder
	}, 2)
	start := make(chan struct{})
	for i, code := range codes {
		go func(i int, code string) {
			<-start
			responses <- struct {
				index int
				r     *httptest.ResponseRecorder
			}{i, actorRequest(i, associationBody(code, "1", true), fmt.Sprintf("switch-%d", i))}
		}(i, code)
	}
	close(start)
	winner := -1
	var saved string
	for range 2 {
		result := <-responses
		if result.r.Code == 200 {
			if winner != -1 {
				t.Fatal("both stale editors won")
			}
			winner = result.index
			saved = result.r.Body.String()
		} else {
			assertIntegrationErrorCode(t, result.r, 409, "table_release_template_conflict")
		}
	}
	if winner < 0 {
		t.Fatal("no switch committed")
	}
	replay := actorRequest(winner, associationBody(codes[winner], "1", true), fmt.Sprintf("switch-%d", winner))
	if replay.Code != 200 || replay.Body.String() != saved {
		t.Fatalf("original response lost: %d %s", replay.Code, replay.Body)
	}
	conflict := actorRequest(winner, associationBody("default_emergency_v1", "2", true), fmt.Sprintf("switch-%d", winner))
	assertIntegrationErrorCode(t, conflict, 409, "idempotency_conflict")
	later := templateRequest(t, app, "PUT", path, associationBody("default_emergency_v1", "2", true), "switch-later")
	if later.Code != 200 {
		t.Fatalf("later: %d %s", later.Code, later.Body)
	}
	replay = actorRequest(winner, associationBody(codes[winner], "1", true), fmt.Sprintf("switch-%d", winner))
	if replay.Body.String() != saved {
		t.Fatalf("later choice changed original result: %s", replay.Body)
	}
	rows := associationCatalog(t, app, "policy_alpha")
	if len(rows) != 1 || rows[0]["template_code"] != "default_emergency_v1" || rows[0]["version"] != "3" {
		t.Fatalf("replay overwrote later choice: %#v", rows)
	}
}

func TestTableReleaseTemplateHTTPAC007AtomicCreateEnableAndRequestResultFailure(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	owner := *driver
	owner.User = "root"
	db := deliveryDB(t, &owner)
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	// The dependency rejects the emergency row after creation of the table policy.
	deliveryExec(t, db, `CREATE TRIGGER reject_emergency_insert BEFORE INSERT ON rcc_table_release_templates FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected emergency failure'`)
	body := tablePolicyCodePayload("policy_alpha", "association_query_v1", "association_mutation_v1")
	failed := templateRequest(t, app, "POST", "/api/v1/table-policies", body, "create-alpha-failure")
	if failed.Code != 503 {
		t.Fatalf("create storage failure: %d %s", failed.Code, failed.Body)
	}
	missing := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", "")
	assertIntegrationErrorCode(t, missing, 404, "table_policy_not_found")
	deliveryExec(t, db, `DROP TRIGGER reject_emergency_insert`)
	created := templateRequest(t, app, "POST", "/api/v1/table-policies", body, "create-alpha-failure")
	if created.Code != 201 {
		t.Fatalf("create retry: %d %s", created.Code, created.Body)
	}
	// Missing association on a disabled rule is repaired only inside enabling.
	deliveryExec(t, db, `DELETE FROM rcc_table_release_templates WHERE table_policy_id=(SELECT id FROM rcc_table_policies WHERE table_name='policy_alpha')`)
	deliveryExec(t, db, `CREATE TRIGGER reject_management_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'table-%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected request result failure'; END IF; END`)
	enableBody := `{"expected_version":"1"}`
	failed = templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/enable", enableBody, "enable-alpha-failure")
	if failed.Code != 503 {
		t.Fatalf("enable result failure: %d %s", failed.Code, failed.Body)
	}
	after := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", "")
	if after.Body.String() != created.Body.String() {
		t.Fatalf("partial enable: %s", after.Body)
	}
	if rows := associationCatalog(t, app, "policy_alpha"); len(rows) != 0 {
		t.Fatalf("failed enable left association: %#v", rows)
	}
	deliveryExec(t, db, `DROP TRIGGER reject_management_result`)
	enabled := templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/enable", enableBody, "enable-alpha-failure")
	if enabled.Code != 200 {
		t.Fatalf("enable retry: %d %s", enabled.Code, enabled.Body)
	}
	if rows := associationCatalog(t, app, "policy_alpha"); len(rows) != 1 || rows[0]["enabled"] != true {
		t.Fatalf("enabled table missing emergency: %#v", rows)
	}
	// All following writes must roll back their business fields, versions and audit.
	cases := []struct{ action, method, path, body string }{
		{"association", "PUT", "/api/v1/table-policies/policy_alpha/release-templates/EMERGENCY", associationBody("default_emergency_v1", "1", true)},
		{"replace", "PUT", "/api/v1/table-policies/policy_alpha", strings.TrimSuffix(body, "}") + `,"expected_version":"2"}`},
		{"disable", "POST", "/api/v1/table-policies/policy_alpha/disable", `{"expected_version":"2"}`},
	}
	for _, write := range cases {
		beforePolicy := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", "").Body.String()
		beforeAssociation := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha/release-templates", "", "").Body.String()
		deliveryExec(t, db, `CREATE TRIGGER reject_management_result BEFORE UPDATE ON rcc_release_requests FOR EACH ROW BEGIN IF NEW.operation LIKE 'table-%' AND NEW.result IS NOT NULL THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected request result failure'; END IF; END`)
		r := templateRequest(t, app, write.method, write.path, write.body, "result-failure-"+write.action)
		if r.Code != 503 {
			t.Fatalf("%s expected dependency failure: %d %s", write.action, r.Code, r.Body)
		}
		if r := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", ""); r.Body.String() != beforePolicy {
			t.Fatalf("%s partial policy: %s", write.action, r.Body)
		}
		if r := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha/release-templates", "", ""); r.Body.String() != beforeAssociation {
			t.Fatalf("%s partial association: %s", write.action, r.Body)
		}
		deliveryExec(t, db, `DROP TRIGGER reject_management_result`)
	}
	disabled := templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/disable", `{"expected_version":"2"}`, "disabled-later")
	if disabled.Code != 200 {
		t.Fatalf("disable: %d %s", disabled.Code, disabled.Body)
	}
	replay := templateRequest(t, app, "POST", "/api/v1/table-policies/policy_alpha/enable", enableBody, "enable-alpha-failure")
	if replay.Code != 200 || replay.Body.String() != enabled.Body.String() {
		t.Fatalf("replay changed original enable: %s", replay.Body)
	}
	current := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", "")
	if current.Body.String() != disabled.Body.String() {
		t.Fatal("replay re-enabled a subsequently disabled table")
	}
	replay = templateRequest(t, app, "POST", "/api/v1/table-policies", body, "create-alpha-failure")
	if replay.Body.String() != created.Body.String() {
		t.Fatal("create replay did not retain original result")
	}
	conflict := templateRequest(t, app, "POST", "/api/v1/table-policies", strings.Replace(body, "policy_alpha", "policy_beta", 1), "create-alpha-failure")
	assertIntegrationErrorCode(t, conflict, 409, "idempotency_conflict")
}

func TestTableReleaseTemplateHTTPRejectsMissingTableAndNonAdmin(t *testing.T) {
	app := startIntegrationApplication(t)
	response := templateRequest(t, app, http.MethodGet, "/api/v1/table-policies/missing/release-templates", "", "")
	assertIntegrationErrorCode(t, response, http.StatusNotFound, "table_policy_not_found")
	account := registerAccount(t, app, "association.viewer", "association.viewer@example.com", "correct horse battery staple")
	response = accountRequest(app, http.MethodGet, "/api/v1/table-policies/missing/release-templates", "", account.Result().Cookies(), "")
	assertIntegrationErrorCode(t, response, http.StatusForbidden, "permission_denied")
	response = templateRequest(t, app, http.MethodPut, "/api/v1/table-policies/missing/release-templates/EMERGENCY", `{"template_code":"default_standard_v1","enabled":true,"expected_version":"0"}`, "association-missing")
	if response.Code == http.StatusOK || strings.Contains(response.Body.String(), `"version":"1"`) {
		t.Fatalf("missing table accepted: %d %s", response.Code, response.Body)
	}
}

func TestTableReleaseTemplateHTTPCurrentAuthorizationAndRequiredRequestIdentity(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	_ = integrationAdminSession(t, app)
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	actor := registerAccount(t, app, "association.actor", "association.actor@example.com", "correct horse battery staple")
	actorID := accountID(t, actor)
	request := func(method, path, body, key string) *httptest.ResponseRecorder {
		return accountRequestFrom(app, method, path, body, actor.Result().Cookies(), sessionCSRF(t, actor), "192.0.2.1:1234", map[string]string{"Idempotency-Key": key, "X-RCC-Roles": "ADMIN"})
	}
	body := tablePolicyCodePayload("policy_alpha", "association_query_v1", "association_mutation_v1")
	writes := []struct {
		method, path, body, key string
		status                  int
	}{
		{"POST", "/api/v1/table-policies", body, "actor-create", 201},
		{"PUT", "/api/v1/table-policies/policy_alpha", strings.TrimSuffix(body, "}") + `,"expected_version":"1"}`, "actor-replace", 200},
		{"POST", "/api/v1/table-policies/policy_alpha/enable", `{"expected_version":"2"}`, "actor-enable", 200},
		{"POST", "/api/v1/table-policies/policy_alpha/disable", `{"expected_version":"3"}`, "actor-disable", 200},
		{"PUT", "/api/v1/table-policies/policy_alpha/release-templates/STANDARD", associationBody("default_standard_v1", "0", true), "actor-association", 200},
	}
	for _, w := range writes {
		assertIntegrationErrorCode(t, request(w.method, w.path, w.body, w.key), 403, "permission_denied")
	}
	grant := templateRequest(t, app, "PUT", "/api/v1/account-roles/"+actorID, `{"roles":["ADMIN"],"expected_version":"1"}`, "association-grant-admin")
	if grant.Code != 200 {
		t.Fatalf("grant: %d %s", grant.Code, grant.Body)
	}
	for _, w := range writes {
		missing := request(w.method, w.path, w.body, "")
		if missing.Code != 422 {
			t.Fatalf("missing key accepted: %d %s", missing.Code, missing.Body)
		}
		response := request(w.method, w.path, w.body, w.key)
		if response.Code != w.status {
			t.Fatalf("authorized write: %d %s", response.Code, response.Body)
		}
		if !strings.Contains(response.Body.String(), `"modifier":"`+actorID+`"`) {
			t.Fatalf("forged actor attribution: %s", response.Body)
		}
	}
	for _, w := range writes {
		r := request(w.method, w.path, w.body, w.key)
		if r.Code != w.status {
			t.Fatalf("original result replay: %d %s", r.Code, r.Body)
		}
	}
	demote := templateRequest(t, app, "PUT", "/api/v1/account-roles/"+actorID, `{"roles":["VIEWER"],"expected_version":"2"}`, "association-revoke-admin")
	if demote.Code != 200 {
		t.Fatalf("demote: %d %s", demote.Code, demote.Body)
	}
	for _, w := range writes {
		assertIntegrationErrorCode(t, request(w.method, w.path, w.body, w.key), 403, "permission_denied")
	}
	for _, table := range []string{"rcc_table_release_templates", "RCC_TABLE_RELEASE_TEMPLATES"} {
		r := templateRequest(t, app, "POST", "/api/v1/table-policies", tablePolicyCodePayload(table, "association_query_v1", "association_mutation_v1"), "protected-"+table)
		assertIntegrationErrorCode(t, r, 403, "protected_table")
		r = templateRequest(t, app, "GET", "/api/v1/table-policies/"+table+"/release-templates", "", "")
		assertIntegrationErrorCode(t, r, 403, "protected_table")
	}
}

func TestTablePolicyHTTPConcurrentCreateAndEnablePreserveEmergency(t *testing.T) {
	app := startIntegrationApplication(t, "testdata/003-policy-fixture.sql")
	createAndActivateQueryDefinition(t, app, "association_query_v1", "id")
	createAndActivateMutationDefinition(t, app, "association_mutation_v1", nil)
	session := integrationAdminSession(t, app)
	cookies := session.Result().Cookies()
	csrf := sessionCSRF(t, session)
	parallel := func(method, path, body string, keys [2]string) [2]*httptest.ResponseRecorder {
		results := make(chan struct {
			index    int
			response *httptest.ResponseRecorder
		}, 2)
		start := make(chan struct{})
		for i, key := range keys {
			go func(i int, key string) {
				<-start
				results <- struct {
					index    int
					response *httptest.ResponseRecorder
				}{i, accountRequestFrom(app, method, path, body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Idempotency-Key": key})}
			}(i, key)
		}
		close(start)
		var responses [2]*httptest.ResponseRecorder
		for range 2 {
			r := <-results
			responses[r.index] = r.response
		}
		return responses
	}
	body := tablePolicyCodePayload("policy_alpha", "association_query_v1", "association_mutation_v1")
	creates := parallel("POST", "/api/v1/table-policies", body, [2]string{"same-create-key", "same-create-key"})
	if creates[0].Code != 201 || creates[1].Code != 201 || creates[0].Body.String() != creates[1].Body.String() {
		t.Fatalf("duplicate creates: %d %s / %d %s", creates[0].Code, creates[0].Body, creates[1].Code, creates[1].Body)
	}
	rows := associationCatalog(t, app, "policy_alpha")
	if len(rows) != 1 || rows[0]["version"] != "1" || rows[0]["type"] != "EMERGENCY" {
		t.Fatalf("duplicate emergency association: %#v", rows)
	}
	enables := parallel("POST", "/api/v1/table-policies/policy_alpha/enable", `{"expected_version":"1"}`, [2]string{"enable-editor-one", "enable-editor-two"})
	if enables[0].Code+enables[1].Code != 200+409 {
		t.Fatalf("enable editors: %d %s / %d %s", enables[0].Code, enables[0].Body, enables[1].Code, enables[1].Body)
	}
	current := templateRequest(t, app, "GET", "/api/v1/table-policies/policy_alpha", "", "")
	if !strings.Contains(current.Body.String(), `"version":"2"`) || !strings.Contains(current.Body.String(), `"enabled":true`) {
		t.Fatalf("current managed table: %d %s", current.Code, current.Body)
	}
	rows = associationCatalog(t, app, "policy_alpha")
	if len(rows) != 1 || rows[0]["enabled"] != true || rows[0]["version"] != "1" {
		t.Fatalf("enable changed emergency identity: %#v", rows)
	}
}
