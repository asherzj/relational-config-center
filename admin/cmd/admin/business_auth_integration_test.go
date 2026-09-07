//go:build integration

package main

import (
	"encoding/json"
	"fmt"
	"github.com/asherzj/relational-config-center/admin/internal/application"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	httpinterface "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBusinessAPIsRequireSessionAndCSRF(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	routes := []struct{ method, path string }{
		{"GET", "/api/v1/database-tables"}, {"GET", "/api/v1/database-tables/example"},
		{"GET", "/api/v1/query-policy-types"}, {"GET", "/api/v1/mutation-policy-types"},
		{"GET", "/api/v1/query-policies"}, {"POST", "/api/v1/query-policies"}, {"GET", "/api/v1/query-policies/example"}, {"PUT", "/api/v1/query-policies/example"}, {"PATCH", "/api/v1/query-policies/example/metadata"}, {"POST", "/api/v1/query-policies/example/activate"}, {"POST", "/api/v1/query-policies/example/deprecate"}, {"DELETE", "/api/v1/query-policies/example"},
		{"GET", "/api/v1/mutation-policies"}, {"POST", "/api/v1/mutation-policies"}, {"GET", "/api/v1/mutation-policies/example"}, {"PUT", "/api/v1/mutation-policies/example"}, {"PATCH", "/api/v1/mutation-policies/example/metadata"}, {"POST", "/api/v1/mutation-policies/example/activate"}, {"POST", "/api/v1/mutation-policies/example/deprecate"}, {"DELETE", "/api/v1/mutation-policies/example"},
		{"GET", "/api/v1/table-policies"}, {"POST", "/api/v1/table-policies"}, {"GET", "/api/v1/table-policies/example"}, {"PUT", "/api/v1/table-policies/example"}, {"POST", "/api/v1/table-policies/example/enable"}, {"POST", "/api/v1/table-policies/example/disable"},
		{"POST", "/api/v1/tables/example/query"}, {"POST", "/api/v1/tables/example/rows"}, {"PATCH", "/api/v1/tables/example/rows/1"}, {"DELETE", "/api/v1/tables/example/rows/1"},
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
	for i, table := range []string{"actor_alpha", "actor_beta"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			session := sessions[i]
			actor := accountID(t, session)
			request := func(method, path, body string, status int) *httptest.ResponseRecorder {
				response := accountRequest(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session))
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
			checkActor(request("PUT", path+"/"+table, body, 200))
			checkActor(request("POST", path+"/"+table+"/enable", "", 200))
			added := request("POST", "/api/v1/tables/"+table+"/rows", `{"content":{"value":"created"}}`, 201)
			id := mutationResponseID(t, added)
			request("PATCH", "/api/v1/tables/"+table+"/rows/"+id, `{"content":{"value":"modified"}}`, 200)
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
			checkActor(request("POST", path+"/"+table+"/disable", "", 200))
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
	request := func(method, path, body string) *httptest.ResponseRecorder {
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
	for _, definition := range []string{"VARCHAR(12)", "CHAR(35)", "ENUM('legacy-admin','aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa')"} {
		if _, err := db.Exec("ALTER TABLE actor_history MODIFY creator " + definition); err != nil {
			t.Fatal(err)
		}
		response := request("POST", "/api/v1/tables/actor_history/rows", `{"content":{"value":"must not commit"}}`)
		assertIntegrationErrorCode(t, response, 422, "operator_field_incompatible")
		history := request("POST", "/api/v1/tables/actor_history/query", `{}`)
		if history.Code != 200 || !strings.Contains(history.Body.String(), "legacy-admin") || strings.Contains(history.Body.String(), "must not commit") {
			t.Fatalf("history changed or hidden: %d %s", history.Code, history.Body.String())
		}
	}
	if _, err := db.Exec("ALTER TABLE actor_history MODIFY creator CHAR(36)"); err != nil {
		t.Fatal(err)
	}
	if response := request("POST", "/api/v1/tables/actor_history/rows", `{"content":{"value":"compatible"}}`); response.Code != 201 {
		t.Fatalf("36 character column rejected: %d %s", response.Code, response.Body.String())
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

func integrationSession(t *testing.T, app *adminApplication) *httptest.ResponseRecorder {
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
	})
	return client.session
}

func integrationAccountID(t *testing.T, app *adminApplication) string {
	t.Helper()
	return accountID(t, integrationSession(t, app))
}

func integrationRouterOptions(app *adminApplication) httpinterface.RouterOptions {
	return httpinterface.RouterOptions{Authentication: application.NewAuthentication(app.mysql, passwordadapter.NewArgon2id(), nil, app.mysql, application.AuthenticationLimits{}), AccountHTTP: httpinterface.AccountHTTPOptions{PublicOrigin: "http://127.0.0.1:5173", InsecureLocalHTTP: true}, AccessLog: io.Discard}
}

func TestRevocationRejectsNewRequestsButAllowsAuthenticatedWriteToFinish(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql", "testdata/008-mutation-policy-snapshot-fixture.sql")
	assignRelationalMutationPolicy(t, app, "inflight_mutation_v1", true, true, true, true)
	session := registerAccount(t, app, "inflight.user", "inflight@example.com", "correct horse battery staple")
	actor := accountID(t, session)
	csrf := sessionCSRF(t, session)
	resume := make(chan struct{})
	barrier := &mutationSnapshotBarrier{delegate: app.mysql, tableRead: make(chan struct{}), resumeTable: resume}
	installMutationSnapshotExecutor(t, app, barrier)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- accountRequest(app, "POST", "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"inflight","label":"already authenticated"}}`, session.Result().Cookies(), csrf)
	}()
	select {
	case <-barrier.tableRead:
	case <-time.After(10 * time.Second):
		close(resume)
		t.Fatal("write did not enter its authenticated transaction")
	}
	// The real logout-all transaction commits while the business request is held.
	revoked := accountRequest(app, "POST", "/api/v1/auth/logout-all", "", session.Result().Cookies(), csrf)
	if revoked.Code != 204 {
		close(resume)
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body.String())
	}
	rejected := accountRequest(app, "POST", "/api/v1/tables/mutation_snapshot_items/rows", `{"content":{"code":"after-revoke","label":"must reject"}}`, session.Result().Cookies(), csrf)
	if rejected.Code != 401 {
		close(resume)
		t.Fatalf("new request after revocation: %d %s", rejected.Code, rejected.Body.String())
	}
	close(resume)
	select {
	case completed := <-finished:
		if completed.Code != 201 {
			t.Fatalf("inflight write: %d %s", completed.Code, completed.Body.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("inflight write did not finish")
	}
	renewed := loginAccount(t, app, "inflight.user", "correct horse battery staple")
	result := accountRequest(app, "POST", "/api/v1/tables/mutation_snapshot_items/query", `{}`, renewed.Result().Cookies(), sessionCSRF(t, renewed))
	if result.Code != 200 || !strings.Contains(result.Body.String(), actor) || !strings.Contains(result.Body.String(), "already authenticated") || strings.Contains(result.Body.String(), "after-revoke") {
		t.Fatalf("revocation changed attribution/outcome: %d %s", result.Code, result.Body.String())
	}
}

func TestAccountControlTablesCannotBeDiscoveredOrManaged(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	session := registerAccount(t, app, "control.user", "control@example.com", "correct horse battery staple")
	request := func(method, path, body string) *httptest.ResponseRecorder {
		return accountRequest(app, method, path, body, session.Result().Cookies(), sessionCSRF(t, session))
	}
	discovered := request("GET", "/api/v1/database-tables", "")
	if discovered.Code != 200 || strings.Contains(discovered.Body.String(), "rcc_") {
		t.Fatalf("control table discovered: %d %s", discovered.Code, discovered.Body.String())
	}
	for _, table := range []string{"rcc_accounts", "rcc_login_sessions", "rcc_preauth_credentials", "rcc_auth_rate_limits", "rcc_auth_control_lock", "rcc_future_control", "RCC_ACCOUNTS"} {
		if response := request("GET", "/api/v1/database-tables/"+table, ""); response.Code != 404 {
			t.Fatalf("control detail %s: %d %s", table, response.Code, response.Body.String())
		}
		for _, route := range []struct{ method, path, body string }{
			{"POST", "/api/v1/table-policies", tablePolicyCodePayload(table, "unused_query_v1", "unused_mutation_v1")},
			{"GET", "/api/v1/table-policies/" + table, ""},
			{"PUT", "/api/v1/table-policies/" + table, tablePolicyCodePayload(table, "unused_query_v1", "unused_mutation_v1")},
			{"POST", "/api/v1/table-policies/" + table + "/enable", ""}, {"POST", "/api/v1/table-policies/" + table + "/disable", ""},
			{"POST", "/api/v1/tables/" + table + "/query", `{}`},
			{"POST", "/api/v1/tables/" + table + "/rows", `{"content":{}}`},
			{"PATCH", "/api/v1/tables/" + table + "/rows/1", `{"content":{}}`},
			{"DELETE", "/api/v1/tables/" + table + "/rows/1", ""},
		} {
			response := request(route.method, route.path, route.body)
			assertIntegrationErrorCode(t, response, 403, "protected_table")
		}
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("generic requests changed session: %d %s", current.Code, current.Body.String())
	}
}
