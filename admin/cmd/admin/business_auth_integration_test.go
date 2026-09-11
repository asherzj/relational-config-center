//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

type droppedResponseTransport struct {
	base http.RoundTripper
	drop atomic.Bool
}

func (transport *droppedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || !transport.drop.CompareAndSwap(true, false) {
		return response, err
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	return nil, errors.New("injected response loss after server completed request")
}

func TestCommittedWritesRemainSingleWhenHTTPResponsesAreLost(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/008-mutation-policy-snapshot-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	assignRelationalMutationPolicy(t, app, "response_loss_mutation_v1", true, true, true, true)
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	transport := &droppedResponseTransport{base: http.DefaultTransport}
	client := &http.Client{Jar: jar, Transport: transport}

	do := func(method, path, body, csrf string, keys ...string) (*http.Response, error) {
		request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", "http://127.0.0.1:5173")
		request.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		if len(keys) > 0 {
			request.Header.Set("Idempotency-Key", keys[0])
		}
		return client.Do(request)
	}
	decode := func(response *http.Response, target any) {
		t.Helper()
		defer response.Body.Close()
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
	}
	prepare := func() string {
		response, err := do(http.MethodGet, "/api/v1/auth/csrf", "", "")
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			CSRF string `json:"csrf_token"`
		}
		decode(response, &payload)
		return payload.CSRF
	}
	login := func(password string, expected int) string {
		body := fmt.Sprintf(`{"username":"response.loss","password":%q}`, password)
		response, err := do(http.MethodPost, "/api/v1/auth/login", body, prepare())
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != expected {
			payload, _ := io.ReadAll(response.Body)
			t.Fatalf("login status: got %d want %d: %s", response.StatusCode, expected, payload)
		}
		if expected != http.StatusOK {
			return ""
		}
		var identity struct {
			CSRF string `json:"csrf_token"`
		}
		if err := json.NewDecoder(response.Body).Decode(&identity); err != nil {
			t.Fatal(err)
		}
		return identity.CSRF
	}

	registration := `{"username":"response.loss","email":"response.loss@example.com","password":"correct horse battery staple"}`
	registrationCSRF := prepare()
	transport.drop.Store(true)
	if response, err := do(http.MethodPost, "/api/v1/auth/register", registration, registrationCSRF); err == nil || response != nil {
		t.Fatalf("registration response was not lost: response=%v err=%v", response, err)
	}
	csrf := login("correct horse battery staple", http.StatusOK)
	if _, err := application.NewAccountMaintenance(app.mysql, passwordadapter.NewArgon2id()).GrantAdmin(t.Context(), application.AccountSelector{Username: "response.loss"}); err != nil {
		t.Fatal(err)
	}

	reviewer := publicationFixtureReviewer(t, app)
	configurePublicationReviewer(t, app, reviewer, "mutation_snapshot_items")
	// Prepare and independently approve through the public release workflow.
	created, err := do(http.MethodPost, "/api/v1/release-orders", `{"items":[{"content":{"code":"response-loss","label":"committed once"},"operation":"ADD","table_name":"mutation_snapshot_items"}],"title":"集成测试发布单"}`, csrf, "response-loss-create")
	if err != nil {
		t.Fatal(err)
	}
	if created.StatusCode != 201 {
		payload, _ := io.ReadAll(created.Body)
		created.Body.Close()
		t.Fatalf("create: %d %s", created.StatusCode, payload)
	}
	var draft domain.ReleaseOrder
	decode(created, &draft)
	path := "/api/v1/release-orders/" + draft.ID
	submitted, err := do(http.MethodPost, path+"/submit", `{"expected_version":"1"}`, csrf, "response-loss-submit")
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := io.ReadAll(submitted.Body)
	submitted.Body.Close()
	if submitted.StatusCode != 200 {
		t.Fatalf("submit: %d %s", submitted.StatusCode, payload)
	}
	approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, reviewer, path, "independently checked"), "response-loss-approve")
	if approved.Code != 200 {
		t.Fatalf("approve: %d %s", approved.Code, approved.Body)
	}
	transport.drop.Store(true)
	if response, err := do(http.MethodPost, path+"/execute", `{"expected_version":"3"}`, csrf, "response-loss-execute"); err == nil || response != nil {
		t.Fatalf("publication response was not lost: response=%v err=%v", response, err)
	}
	retried, err := do(http.MethodPost, path+"/execute", `{"expected_version":"3"}`, csrf, "response-loss-execute")
	if err != nil {
		t.Fatal(err)
	}
	if retried.StatusCode != 200 {
		payload, _ := io.ReadAll(retried.Body)
		retried.Body.Close()
		t.Fatalf("retry: %d %s", retried.StatusCode, payload)
	}
	var published domain.ReleaseOrder
	decode(retried, &published)
	if published.State != "SUCCEEDED" || published.Version != "4" || len(published.Executions) < 1 || len(executionCommands(published, "PUBLICATION")) != 1 || singleExecutionTableVersion(published.Executions[0]) != "1" || published.Executions[0].ActorID != draft.ApplicantID || published.Executions[0].ID == "" {
		t.Fatalf("lost response retry did not recover one publication: %+v", published)
	}
	executions := 0
	for _, event := range published.History {
		if event.Action == "EXECUTE" {
			executions++
			if event.ActorID != draft.ApplicantID {
				t.Fatalf("execution actor: %+v", event)
			}
		}
	}
	if executions != 1 {
		t.Fatalf("execution history count: %d", executions)
	}
	var rows, commands, notifications int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mutation_snapshot_items WHERE code = 'response-loss' AND label = 'committed once'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_publication_commands WHERE order_id = ?`, draft.ID).Scan(&commands); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM rcc_refresh_notifications WHERE order_id = ?`, draft.ID).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || commands != 1 || notifications != 1 {
		t.Fatalf("duplicate publication effects: rows=%d commands=%d notifications=%d", rows, commands, notifications)
	}

	transport.drop.Store(true)
	passwordBody := `{"current_password":"correct horse battery staple","new_password":"new correct horse battery staple"}`
	if response, err := do(http.MethodPost, "/api/v1/auth/password", passwordBody, csrf); err == nil || response != nil {
		t.Fatalf("password response was not lost: response=%v err=%v", response, err)
	}
	current, err := do(http.MethodGet, "/api/v1/auth/session", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = current.Body.Close()
	if current.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old session after password commit: got %d want 401", current.StatusCode)
	}
	login("correct horse battery staple", http.StatusUnauthorized)
	login("new correct horse battery staple", http.StatusOK)
}

func TestBusinessAPIsRequireSessionAndCSRF(t *testing.T) {
	app := startIntegrationApplication(t)
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/database-tables"}, {"GET", "/api/v1/database-tables/example"},
		{"GET", "/api/v1/query-policy-types"}, {"GET", "/api/v1/mutation-policy-types"},
		{"GET", "/api/v1/query-policies"}, {"POST", "/api/v1/query-policies"}, {"GET", "/api/v1/query-policies/example"}, {"PUT", "/api/v1/query-policies/example"}, {"PATCH", "/api/v1/query-policies/example/metadata"}, {"POST", "/api/v1/query-policies/example/activate"}, {"POST", "/api/v1/query-policies/example/deprecate"}, {"DELETE", "/api/v1/query-policies/example"},
		{"GET", "/api/v1/mutation-policies"}, {"POST", "/api/v1/mutation-policies"}, {"GET", "/api/v1/mutation-policies/example"}, {"PUT", "/api/v1/mutation-policies/example"}, {"PATCH", "/api/v1/mutation-policies/example/metadata"}, {"POST", "/api/v1/mutation-policies/example/activate"}, {"POST", "/api/v1/mutation-policies/example/deprecate"}, {"DELETE", "/api/v1/mutation-policies/example"},
		{"GET", "/api/v1/table-policies"}, {"POST", "/api/v1/table-policies"}, {"GET", "/api/v1/table-policies/example"}, {"PUT", "/api/v1/table-policies/example"}, {"POST", "/api/v1/table-policies/example/enable"}, {"POST", "/api/v1/table-policies/example/disable"},
		{"POST", "/api/v1/tables/example/query"},
		{"GET", "/api/v1/release-orders"}, {"POST", "/api/v1/release-orders"}, {"GET", "/api/v1/release-orders/example"}, {"PUT", "/api/v1/release-orders/example"},
		{"POST", "/api/v1/release-orders/example/submit"}, {"POST", "/api/v1/release-orders/example/approve"}, {"POST", "/api/v1/release-orders/example/reject"}, {"POST", "/api/v1/release-orders/example/cancel"}, {"POST", "/api/v1/release-orders/example/copy"}, {"POST", "/api/v1/release-orders/example/reprepare"}, {"POST", "/api/v1/release-orders/example/execute"},
	}
	for _, path := range []string{"/health/live", "/health/ready"} {
		if response := accountRequest(app, "GET", path, "", nil, ""); response.Code != 200 {
			t.Fatalf("empty account health %s: %d", path, response.Code)
		}
	}
	for _, route := range routes {
		for _, token := range []string{"", "Bearer integration-token"} {
			response := accountRequestFrom(app, route.method, route.path, `{}`, nil, "", "192.0.2.1:1000", map[string]string{"Authorization": token})
			if response.Code != 401 || !strings.Contains(response.Body.String(), `"session_invalid"`) {
				t.Fatalf("unauthenticated %s %s: %d %s", route.method, route.path, response.Code, response.Body.String())
			}
		}
	}
	session := registerAccount(t, app, "business.user", "business@example.com", "correct horse battery staple")
	for _, route := range routes {
		if route.method == http.MethodGet {
			continue
		}
		response := accountRequest(app, route.method, route.path, `{}`, session.Result().Cookies(), "")
		if response.Code != 403 || !strings.Contains(response.Body.String(), `"csrf_invalid"`) {
			t.Fatalf("missing CSRF %s %s: %d %s", route.method, route.path, response.Code, response.Body.String())
		}
	}
	csrf := sessionCSRF(t, session)
	for _, headers := range []map[string]string{{"Origin": "https://evil.example"}, {"Origin": "", "Referer": "https://evil.example/x"}} {
		response := accountRequestFrom(app, "POST", "/api/v1/query-policies", `{}`, session.Result().Cookies(), csrf, "192.0.2.1:1000", headers)
		if response.Code != 403 {
			t.Fatalf("origin admitted: %d %s", response.Code, response.Body.String())
		}
	}
	if response := accountRequest(app, "GET", "/api/v1/query-policies", "", session.Result().Cookies(), ""); response.Code != 200 {
		t.Fatalf("authenticated read: %d %s", response.Code, response.Body.String())
	} else if got, want := response.Header().Get("X-RCC-Account-ID"), accountID(t, session); got != want {
		t.Fatalf("authenticated response account: got %q want %q", got, want)
	}
}

func TestConcurrentAccountsOwnTheirBusinessChanges(t *testing.T) {
	app, db := accountFixture(t, nil)
	for _, table := range []string{"actor_alpha", "actor_beta"} {
		if _, err := db.Exec("CREATE TABLE " + table + " (id BIGINT PRIMARY KEY AUTO_INCREMENT, value VARCHAR(100), creator VARCHAR(36), modifier VARCHAR(36))"); err != nil {
			t.Fatal(err)
		}
	}
	sessions := []*httptest.ResponseRecorder{
		registerAccount(t, app, "actor.alpha", "alpha@example.com", "correct horse battery staple"),
		registerAccount(t, app, "actor.beta", "beta@example.com", "correct horse battery staple"),
	}
	for _, session := range sessions {
		grantTestAdministrator(t, app, session)
	}
	for i, table := range []string{"actor_alpha", "actor_beta"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			session := sessions[i]
			actor := accountID(t, session)
			request := func(method, path, body string, status int) *httptest.ResponseRecorder {
				var response *httptest.ResponseRecorder
				if strings.HasPrefix(path, "/api/v1/table-policies") && method != "GET" {
					response = accountRequestFrom(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("actor-policy-%d", policyRequestSequence.Add(1))})
				} else {
					response = accountRequest(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session))
				}
				if response.Code != status {
					t.Fatalf("%s %s: %d %s", method, path, response.Code, response.Body.String())
				}
				return response
			}
			checkActor := func(response *httptest.ResponseRecorder) {
				t.Helper()
				var audit struct{ Creator, Modifier string }
				if err := json.Unmarshal(response.Body.Bytes(), &audit); err != nil {
					t.Fatal(err)
				}
				if audit.Creator != actor || audit.Modifier != actor {
					t.Fatalf("request identity lost: want %s, got %s", actor, response.Body.String())
				}
			}
			q, m := table+"_query_v1", table+"_mutation_v1"
			queryBody := fmt.Sprintf(`{"code":%q,"name":"Query","description":"","type_code":"page_query","default_order_field":"id","default_order_direction":"ASC","default_page_size":20,"max_page_size":100}`, q)
			mutationBody := mutationPolicyPayload(m, "single_table_mutation", true, true, true, `"creator"`, `null`, `"modifier"`, `null`)
			for _, item := range []struct{ path, body, code string }{{"query-policies", queryBody, q}, {"mutation-policies", mutationBody, m}} {
				path := "/api/v1/" + item.path
				checkActor(request("POST", path, item.body, 201))
				checkActor(request("PUT", path+"/"+item.code, item.body, 200))
				checkActor(request("POST", path+"/"+item.code+"/activate", "", 200))
				checkActor(request("PATCH", path+"/"+item.code+"/metadata", `{"name":"Changed","description":"Updated"}`, 200))
			}
			path := "/api/v1/table-policies"
			body := tablePolicyCodePayload(table, q, m)
			checkActor(request("POST", path, body, 201))
			checkActor(request("PUT", path+"/"+table, strings.TrimSuffix(body, "}")+`,"expected_version":"1"}`, 200))
			checkActor(request("POST", path+"/"+table+"/enable", `{"expected_version":"2"}`, 200))
			publish := func(input, key string) domain.ReleaseOrder {
				path := approveActorPublication(t, app, session, sessions[1-i], input, key)
				response := releaseActorRequest(t, app, session, "POST", path+"/execute", `{"expected_version":"3"}`, key+"-execute")
				if response.Code != 200 {
					t.Fatalf("publish: %d %s", response.Code, response.Body)
				}
				var order domain.ReleaseOrder
				if err := json.Unmarshal(response.Body.Bytes(), &order); err != nil {
					t.Fatal(err)
				}
				if len(order.Executions) < 1 || order.Executions[0].ActorID != actor {
					t.Fatalf("publisher identity: %s", response.Body)
				}
				// Finish this actor's publication before the next independent row change.
				rollbackOrderResponse(t, releaseActorRequest(t, app, session, "POST", path+"/complete", `{"expected_version":"4"}`, key+"-complete"), 200)
				return order
			}
			added := publish(fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":%q,"operation":"ADD","content":{"value":"created"}}]}`, table), table+"-add")
			id := executionCommands(added, "PUBLICATION")[0].ID
			publish(fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":%q,"operation":"MODIFY","id":%q,"expected_record_version":"1","content":{"value":"modified"}}]}`, table, id), table+"-modify")
			result := request("POST", "/api/v1/tables/"+table+"/query", `{}`, 200)
			if !strings.Contains(result.Body.String(), actor) || strings.Contains(result.Body.String(), "integration-test") {
				t.Fatalf("row identity: %s", result.Body.String())
			}
			var data struct {
				Rows []map[string]*string `json:"rows"`
			}
			if err := json.Unmarshal(result.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if len(data.Rows) != 1 || data.Rows[0]["creator"] == nil || *data.Rows[0]["creator"] != actor || data.Rows[0]["modifier"] == nil || *data.Rows[0]["modifier"] != actor {
				t.Fatalf("row attribution: %s", result.Body.String())
			}
			checkActor(request("POST", path+"/"+table+"/disable", `{"expected_version":"3"}`, 200))
		})
	}
}

func accountID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var identity struct{ Account struct{ ID string } }
	if err := json.Unmarshal(response.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	return identity.Account.ID
}

func TestOperatorColumnsRejectIncompatibleWritesAndPreserveHistory(t *testing.T) {
	app, db := accountFixture(t, nil)
	if _, err := db.Exec(`CREATE TABLE actor_history (id BIGINT PRIMARY KEY AUTO_INCREMENT, value VARCHAR(100), creator VARCHAR(64))`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO actor_history (value,creator) VALUES ('historical','legacy-admin')`); err != nil {
		t.Fatal(err)
	}
	session := registerAccount(t, app, "history.user", "history@example.com", "correct horse battery staple")
	grantTestAdministrator(t, app, session)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		if strings.HasPrefix(path, "/api/v1/table-policies") && method != "GET" {
			if strings.HasSuffix(path, "/enable") || strings.HasSuffix(path, "/disable") {
				body = `{"expected_version":"1"}`
			}
			return accountRequestFrom(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("business-policy-%d", policyRequestSequence.Add(1))})
		}
		return accountRequest(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session))
	}
	query := `{"code":"history_query_v1","name":"History","type_code":"page_query","default_order_field":"id","default_order_direction":"ASC","default_page_size":20,"max_page_size":100}`
	mutation := mutationPolicyPayload("history_mutation_v1", "single_table_mutation", true, true, true, `"creator"`, `null`, `null`, `null`)
	for _, step := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/api/v1/query-policies", query, 201}, {"POST", "/api/v1/query-policies/history_query_v1/activate", "", 200},
		{"POST", "/api/v1/mutation-policies", mutation, 201}, {"POST", "/api/v1/mutation-policies/history_mutation_v1/activate", "", 200},
		{"POST", "/api/v1/table-policies", tablePolicyCodePayload("actor_history", "history_query_v1", "history_mutation_v1"), 201}, {"POST", "/api/v1/table-policies/actor_history/enable", "", 200},
	} {
		if response := request(step.method, step.path, step.body); response.Code != step.status {
			t.Fatalf("setup %s: %d %s", step.path, response.Code, response.Body.String())
		}
	}
	input := `{"items":[{"content":{"value":"must not commit"},"operation":"ADD","table_name":"actor_history"}],"title":"集成测试发布单"}`
	reviewer := publicationFixtureReviewer(t, app)
	for i, definition := range []string{"VARCHAR(12)", "CHAR(35)", "ENUM('legacy-admin','aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa')"} {
		key := fmt.Sprintf("operator-incompatible-%d", i)
		// Capture compatible drafts and approvals before an external schema change.
		draft := releaseActorRequest(t, app, session, "POST", "/api/v1/release-orders", input, key+"-draft")
		if draft.Code != 201 {
			t.Fatalf("compatible draft: %d %s", draft.Code, draft.Body)
		}
		var order domain.ReleaseOrder
		if err := json.Unmarshal(draft.Body.Bytes(), &order); err != nil {
			t.Fatal(err)
		}
		approvedPath := approveActorPublication(t, app, session, reviewer, input, key+"-approved")
		if _, err := db.Exec("ALTER TABLE actor_history MODIFY creator " + definition); err != nil {
			t.Fatal(err)
		}
		created := releaseActorRequest(t, app, session, "POST", "/api/v1/release-orders", input, key+"-invalid-draft")
		assertIntegrationErrorCode(t, created, 422, "operator_field_incompatible")
		submitted := releaseActorRequest(t, app, session, "POST", "/api/v1/release-orders/"+order.ID+"/submit", `{"expected_version":"1"}`, key+"-invalid-submit")
		assertIntegrationErrorCode(t, submitted, 422, "operator_field_incompatible")
		executed := releaseActorRequest(t, app, session, "POST", approvedPath+"/execute", `{"expected_version":"3"}`, key+"-invalid-execute")
		assertIntegrationErrorCode(t, executed, 409, "release_frozen_changed")
		stored := releaseActorReadAllDetails(t, app, session, "GET", approvedPath, "", "")
		var retained domain.ReleaseOrder
		if stored.Code != 200 || json.Unmarshal(stored.Body.Bytes(), &retained) != nil || retained.State != "APPROVED" || retained.Version != "3" || len(retained.Executions) >= 1 {
			t.Fatalf("invalid execution changed approval: %d %s", stored.Code, stored.Body)
		}
		history := request("POST", "/api/v1/tables/actor_history/query", `{}`)
		if history.Code != 200 || !strings.Contains(history.Body.String(), "legacy-admin") || strings.Contains(history.Body.String(), "must not commit") {
			t.Fatalf("history changed or hidden: %d %s", history.Code, history.Body)
		}
		if _, err := db.Exec("ALTER TABLE actor_history MODIFY creator VARCHAR(64)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("ALTER TABLE actor_history MODIFY creator CHAR(36)"); err != nil {
		t.Fatal(err)
	}
	path := approveActorPublication(t, app, session, reviewer, `{"items":[{"content":{"value":"compatible"},"operation":"ADD","table_name":"actor_history"}],"title":"集成测试发布单"}`, "operator-compatible")
	if response := releaseActorRequest(t, app, session, "POST", path+"/execute", `{"expected_version":"3"}`, "operator-compatible-execute"); response.Code != 200 {
		t.Fatalf("36 character column rejected: %d %s", response.Code, response.Body)
	}

}

// All pre-existing business regressions use a real public account and login.
// Credentials belong to the test client; no server handler or Application field
// changes identity or disables authentication.
// The test process retains clients for each app lifetime, including nested subtests.
// Each app owns and closes its real database pool through its existing cleanup.
var integrationClients sync.Map

type integrationClient struct {
	once    sync.Once
	session *httptest.ResponseRecorder
}

func integrationAdminSession(t *testing.T, app *adminApplication) *httptest.ResponseRecorder {
	t.Helper()
	value, _ := integrationClients.LoadOrStore(app, &integrationClient{})
	client := value.(*integrationClient)
	client.once.Do(func() {
		cookies, csrf := prepareAccount(t, app)
		registered := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":"integration.user","email":"integration@example.com","password":"correct horse battery staple"}`, cookies, csrf)
		if registered.Code != 201 && registered.Code != 409 {
			t.Fatalf("register business test account: %d %s", registered.Code, registered.Body.String())
		}
		client.session = loginAccount(t, app, "integration.user", "correct horse battery staple")
		// These fixtures exercise catalog and mutation contracts with an explicit grant.
		grantTestAdministrator(t, app, client.session)
	})
	return client.session
}

func integrationAccountID(t *testing.T, app *adminApplication) string {
	t.Helper()
	return accountID(t, integrationAdminSession(t, app))
}

func integrationRouterOptions(app *adminApplication) httpinterface.RouterOptions {
	return httpinterface.RouterOptions{Authentication: application.NewAuthentication(app.mysql, passwordadapter.NewArgon2id(), nil, app.mysql, application.AuthenticationLimits{}), AccountHTTP: httpinterface.AccountHTTPOptions{PublicOrigin: "http://127.0.0.1:5173", InsecureLocalHTTP: true}, AccessLog: io.Discard}
}

func TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish(t *testing.T) {
	ctx, driver := startCurrentIntegrationMySQL(t, "testdata/008-mutation-policy-snapshot-fixture.sql")
	app, err := newApplication(ctx, integrationConfig(driver))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	ownerDriver := *driver
	ownerDriver.User = "root"
	owner, err := sql.Open("mysql", ownerDriver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	assignRelationalMutationPolicy(t, app, "inflight_mutation_v1", true, true, true, true)
	bindFlowTemplate(t, app, "mutation_snapshot_items", "default_standard_v1", true)
	session := registerAccount(t, app, "inflight.user", "inflight@example.com", "correct horse battery staple")
	grantTestAdministrator(t, app, session)
	actor := accountID(t, session)
	csrf := sessionCSRF(t, session)
	path := approveActorPublication(t, app, session, publicationFixtureReviewer(t, app), `{"items":[{"content":{"code":"inflight","label":"already authenticated"},"operation":"ADD","table_name":"mutation_snapshot_items"}],"title":"集成测试发布单"}`, "inflight-publication")
	// A real external InnoDB row lock holds the request after authentication.
	lock, err := owner.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	orderID := strings.TrimPrefix(path, "/api/v1/release-orders/")
	var lockedID string
	if err := lock.QueryRowContext(t.Context(), "SELECT id FROM rcc_release_orders WHERE id=? FOR UPDATE", orderID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- releaseActorRequest(t, app, session, "POST", path+"/execute", `{"expected_version":"3"}`, "inflight-execute")
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := owner.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='rcc_release_orders'`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case response := <-finished:
			t.Fatalf("publication did not wait for database lock: %d %s", response.Code, response.Body)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("publication did not enter its authenticated transaction")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Revocation serializes behind the authenticated publication's authorization
	// lock. Observe that real wait before allowing its business transaction to finish.
	revocationFinished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		revocationFinished <- accountRequest(app, "POST", "/api/v1/auth/logout-all", "", session.Result().Cookies(), csrf)
	}()
	deadline = time.Now().Add(3 * time.Second)
	for {
		var waiting int
		err := owner.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks l ON l.ENGINE_LOCK_ID=w.BLOCKING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA=DATABASE() AND l.OBJECT_NAME='rcc_auth_control_lock'`).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case response := <-revocationFinished:
			t.Fatalf("revocation did not wait for authorization lock: %d %s", response.Code, response.Body)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("revocation did not enter its authorization transaction")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := lock.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case completed := <-finished:
		if completed.Code != 200 {
			t.Fatalf("inflight publication: %d %s", completed.Code, completed.Body)
		}
		var order domain.ReleaseOrder
		if err := json.Unmarshal(completed.Body.Bytes(), &order); err != nil {
			t.Fatal(err)
		}
		if order.State != "SUCCEEDED" || len(order.Executions) < 1 || order.Executions[0].ActorID != actor {
			t.Fatalf("inflight publisher changed: %s", completed.Body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("inflight publication did not finish")
	}
	select {
	case revoked := <-revocationFinished:
		if revoked.Code != 204 {
			t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("revocation did not finish after publication")
	}
	rejected := releaseActorRequest(t, app, session, "POST", path+"/execute", `{"expected_version":"3"}`, "after-revoke-execute")
	if rejected.Code != 401 {
		t.Fatalf("new request after revocation: %d %s", rejected.Code, rejected.Body)
	}
	renewed := loginAccount(t, app, "inflight.user", "correct horse battery staple")
	result := accountRequest(app, "POST", "/api/v1/tables/mutation_snapshot_items/query", `{}`, renewed.Result().Cookies(), sessionCSRF(t, renewed))
	if result.Code != 200 || !strings.Contains(result.Body.String(), actor) || !strings.Contains(result.Body.String(), "already authenticated") {
		t.Fatalf("revocation changed attribution/outcome: %d %s", result.Code, result.Body)
	}
}

func TestAccountControlTablesCannotBeDiscoveredOrManaged(t *testing.T) {
	app := startIntegrationApplication(t)
	session := registerAccount(t, app, "control.user", "control@example.com", "correct horse battery staple")
	grantTestAdministrator(t, app, session)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		if strings.HasPrefix(path, "/api/v1/table-policies") && method != "GET" {
			if strings.HasSuffix(path, "/enable") || strings.HasSuffix(path, "/disable") {
				body = `{"expected_version":"1"}`
			}
			return accountRequestFrom(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session), "192.0.2.1:1234", map[string]string{"Idempotency-Key": fmt.Sprintf("business-policy-%d", policyRequestSequence.Add(1))})
		}
		return accountRequest(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session))
	}
	discovered := request("GET", "/api/v1/database-tables", "")
	if discovered.Code != 200 || strings.Contains(discovered.Body.String(), "rcc_") {
		t.Fatalf("control table discovered: %d %s", discovered.Code, discovered.Body.String())
	}
	for _, table := range []string{"rcc_accounts", "rcc_login_sessions", "rcc_preauth_credentials", "rcc_auth_rate_limits", "rcc_auth_control_lock", "rcc_account_role_history", "rcc_record_versions", "rcc_release_orders", "rcc_release_details", "rcc_release_executions", "rcc_release_requests", "rcc_release_targets", "rcc_release_table_references", "rcc_table_publications", "rcc_publication_commands", "rcc_refresh_notifications", "rcc_goose_db_version", "rcc_schema_migration_attempts", "rcc_future_control", "rcc_release_templates", "rcc_table_release_templates", "RCC_ACCOUNTS", "rcc_approval_roles", "rcc_approval_role_members", "rcc_approval_role_requests", "rcc_approval_role_references", "rcc_table_approval_assignments", "rcc_table_approval_requests"} {
		if response := request("GET", "/api/v1/database-tables/"+table, ""); response.Code != 404 {
			t.Fatalf("control detail %s: %d %s", table, response.Code, response.Body.String())
		}
		for _, route := range []struct{ method, path, body string }{
			{"POST", "/api/v1/table-policies", tablePolicyCodePayload(table, "unused_query_v1", "unused_mutation_v1")},
			{"GET", "/api/v1/table-policies/" + table, ""},
			{"PUT", "/api/v1/table-policies/" + table, tablePolicyCodePayload(table, "unused_query_v1", "unused_mutation_v1")},
			{"POST", "/api/v1/table-policies/" + table + "/enable", `{"expected_version":"1"}`}, {"POST", "/api/v1/table-policies/" + table + "/disable", `{"expected_version":"1"}`},
			{"POST", "/api/v1/tables/" + table + "/query", `{}`},
			{"POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":%q,"operation":"ADD","content":{}}]}`, table)},
			{"POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":%q,"operation":"MODIFY","id":"1","expected_record_version":"0","content":{}}]}`, table)},
			{"POST", "/api/v1/release-orders", fmt.Sprintf(`{"title":"集成测试发布单","items":[{"table_name":%q,"operation":"DELETE","id":"1","expected_record_version":"0","content":{}}]}`, table)},
		} {
			response := releaseActorRequest(t, app, session, route.method, route.path, route.body, fmt.Sprintf("protected-%d", publicationFixtureSequence.Add(1)))
			assertIntegrationErrorCode(t, response, 403, "protected_table")
		}
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("generic requests changed session: %d %s", current.Code, current.Body.String())
	}
}

// Explicit setup for existing writer regressions. Registration helpers stay VIEWER.
func grantTestAdministrator(t *testing.T, app *adminApplication, session *httptest.ResponseRecorder) {
	t.Helper()
	if err := app.mysql.GrantAccountAdmin(t.Context(), accountID(t, session), time.Now()); err != nil {
		t.Fatal(err)
	}
}

// Each actor keeps their own session throughout draft creation, submission and
// execution. The separate reviewer exercises the real independent approval rule.
func approveActorPublication(t *testing.T, app *adminApplication, applicant, reviewer *httptest.ResponseRecorder, input, key string) string {
	t.Helper()
	created := releaseActorRequest(t, app, applicant, "POST", "/api/v1/release-orders", input, key+"-create")
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var order domain.ReleaseOrder
	if err := json.Unmarshal(created.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	configurePublicationReviewer(t, app, reviewer, order.TableNames...)
	path := "/api/v1/release-orders/" + order.ID
	submitted := releaseActorRequest(t, app, applicant, "POST", path+"/submit", `{"expected_version":"1"}`, key+"-submit")
	if submitted.Code != 200 {
		t.Fatalf("submit: %d %s", submitted.Code, submitted.Body)
	}
	approved := releaseActorRequest(t, app, reviewer, "POST", path+"/approve", confirmedApprovalBody(t, app, reviewer, path, "independently checked"), key+"-approve")
	if approved.Code != 200 {
		t.Fatalf("approve: %d %s", approved.Code, approved.Body)
	}
	return path
}
