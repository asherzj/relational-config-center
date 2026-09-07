//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func TestLocalAccountRegistrationCreatesCurrentIdentity(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	prepared := accountRequest(app, "GET", "/api/v1/auth/csrf", "", nil, "")
	if prepared.Code != 200 {
		t.Fatalf("prepare: %d %s", prepared.Code, prepared.Body.String())
	}
	var challenge struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(prepared.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	registered := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":" Alice.One ","email":" Alice+tag@Example.com ","password":"correct horse battery staple","display_name":"小爱"}`, prepared.Result().Cookies(), challenge.CSRF)
	if registered.Code != 201 {
		t.Fatalf("register: %d %s", registered.Code, registered.Body.String())
	}
	current := accountRequest(app, "GET", "/api/v1/auth/session", "", registered.Result().Cookies(), "")
	if current.Code != 200 || !strings.Contains(current.Body.String(), `"username":"alice.one"`) || !strings.Contains(current.Body.String(), `"display_name":"小爱"`) || !strings.Contains(current.Body.String(), `"email_verified":false`) {
		t.Fatalf("identity: %d %s", current.Code, current.Body.String())
	}
}

func accountRequest(app *adminApplication, method, path, body string, cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
	return accountRequestFrom(app, method, path, body, cookies, csrf, "192.0.2.1:1234", nil)
}
func accountRequestFrom(app *adminApplication, method, path, body string, cookies []*http.Cookie, csrf, remote string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.RemoteAddr = remote
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	for _, cookie := range cookies {
		if cookie.MaxAge >= 0 {
			req.AddCookie(cookie)
		}
	}
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)
	return rec
}

func prepareAccount(t *testing.T, app *adminApplication) ([]*http.Cookie, string) {
	t.Helper()
	response := accountRequest(app, "GET", "/api/v1/auth/csrf", "", nil, "")
	if response.Code != 200 {
		t.Fatalf("prepare: %d %s", response.Code, response.Body.String())
	}
	var body struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return response.Result().Cookies(), body.CSRF
}
func registerAccount(t *testing.T, app *adminApplication, username, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	cookies, csrf := prepareAccount(t, app)
	body, _ := json.Marshal(map[string]string{"username": username, "email": email, "password": password})
	result := accountRequest(app, "POST", "/api/v1/auth/register", string(body), cookies, csrf)
	if result.Code != 201 {
		t.Fatalf("register: %d %s", result.Code, result.Body.String())
	}
	return result
}

func loginAccount(t *testing.T, app *adminApplication, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	cookies, csrf := prepareAccount(t, app)
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	result := accountRequest(app, "POST", "/api/v1/auth/login", string(body), cookies, csrf)
	if result.Code != 200 {
		t.Fatalf("login: %d %s", result.Code, result.Body.String())
	}
	return result
}

func sessionCSRF(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var identity struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	return identity.CSRF
}

func encodedTestPassword(password string, memory, iterations uint32) string {
	salt := []byte("0123456789abcdef")
	digest := argon2.IDKey([]byte(password), salt, iterations, memory, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=1$%s$%s", memory, iterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest))
}

func TestLocalAccountConcurrentSessionsActivityAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	app, _ := accountFixture(t, func() time.Time { return now })
	first := registerAccount(t, app, "sessions.user", "sessions@example.com", "correct horse battery staple")
	second := loginAccount(t, app, "sessions.user", "correct horse battery staple")
	for index, session := range []*httptest.ResponseRecorder{first, second} {
		if current := accountRequest(app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 200 {
			t.Fatalf("concurrent session %d: %d %s", index, current.Code, current.Body.String())
		}
	}

	// Reading identity is background work and must not renew idle time.
	now = now.Add(29 * time.Minute)
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", second.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("background read before idle boundary: %d", current.Code)
	}
	// A real foreground activity signal renews only the idle deadline, using server time.
	activity := accountRequest(app, "POST", "/api/v1/auth/activity", "", first.Result().Cookies(), sessionCSRF(t, first))
	if activity.Code != 200 || !strings.Contains(activity.Body.String(), `"idle_expires_at":"2026-09-07T00:59:00Z"`) {
		t.Fatalf("activity: %d %s", activity.Code, activity.Body.String())
	}
	now = now.Add(time.Minute)
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", second.Result().Cookies(), ""); current.Code != 401 {
		t.Fatalf("background read renewed session: %d", current.Code)
	}
	now = time.Date(2026, 9, 7, 0, 59, 0, 0, time.UTC)
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", first.Result().Cookies(), ""); current.Code != 401 {
		t.Fatalf("idle boundary after activity: %d", current.Code)
	}

	// Repeated foreground activity cannot move the fixed eight-hour boundary.
	long := loginAccount(t, app, "sessions.user", "correct horse battery staple")
	created := now
	for step := 1; step <= 16; step++ {
		now = created.Add(time.Duration(step) * 29 * time.Minute)
		if response := accountRequest(app, "POST", "/api/v1/auth/activity", "", long.Result().Cookies(), sessionCSRF(t, long)); response.Code != 200 {
			t.Fatalf("activity step %d: %d %s", step, response.Code, response.Body.String())
		}
	}
	now = created.Add(8 * time.Hour)
	if response := accountRequest(app, "POST", "/api/v1/auth/activity", "", long.Result().Cookies(), sessionCSRF(t, long)); response.Code != 401 {
		t.Fatalf("activity renewed absolute expiry: %d", response.Code)
	}
}

func TestLocalAccountProfileChangesAffectOnlyCurrentAccount(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	app, _ := accountFixture(t, func() time.Time { return now })
	first := registerAccount(t, app, "profile.user", "profile@example.com", "current password long enough")
	otherDevice := loginAccount(t, app, "profile.user", "current password long enough")
	registerAccount(t, app, "occupied.user", "occupied@example.com", "another password long enough")
	csrf := sessionCSRF(t, first)

	display := accountRequest(app, "PATCH", "/api/v1/auth/profile", `{"display_name":" 新名称 "}`, first.Result().Cookies(), csrf)
	if display.Code != 200 || !strings.Contains(display.Body.String(), `"display_name":"新名称"`) {
		t.Fatalf("display name: %d %s", display.Code, display.Body.String())
	}
	wrong := accountRequest(app, "PATCH", "/api/v1/auth/email", `{"email":"new@example.com","current_password":"wrong password long enough"}`, first.Result().Cookies(), csrf)
	if wrong.Code != 400 || !strings.Contains(wrong.Body.String(), `"current_password_invalid"`) {
		t.Fatalf("wrong current password: %d %s", wrong.Code, wrong.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", first.Result().Cookies(), ""); current.Code != 200 || !strings.Contains(current.Body.String(), `"email":"profile@example.com"`) {
		t.Fatalf("wrong password changed profile or invalidated session: %d %s", current.Code, current.Body.String())
	}
	occupied := accountRequest(app, "PATCH", "/api/v1/auth/email", `{"email":" OCCUPIED@EXAMPLE.COM ","current_password":"current password long enough"}`, first.Result().Cookies(), csrf)
	if occupied.Code != 409 {
		t.Fatalf("occupied email: %d %s", occupied.Code, occupied.Body.String())
	}
	changed := accountRequest(app, "PATCH", "/api/v1/auth/email", `{"email":" New+tag@Example.com ","current_password":"current password long enough"}`, first.Result().Cookies(), csrf)
	if changed.Code != 200 || !strings.Contains(changed.Body.String(), `"username":"profile.user"`) || !strings.Contains(changed.Body.String(), `"email":"new+tag@example.com"`) || !strings.Contains(changed.Body.String(), `"email_verified":false`) {
		t.Fatalf("email update: %d %s", changed.Code, changed.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", otherDevice.Result().Cookies(), ""); current.Code != 200 || !strings.Contains(current.Body.String(), `"display_name":"新名称"`) || !strings.Contains(current.Body.String(), `"email":"new+tag@example.com"`) {
		t.Fatalf("other device profile visibility: %d %s", current.Code, current.Body.String())
	}
}

func TestLocalAccountMaintenanceAuthorizesBeforeInputs(t *testing.T) {
	app, _ := accountFixture(t, nil)
	current := registerAccount(t, app, "authorize.user", "authorize@example.com", "current password long enough")
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"profile", "PATCH", "/api/v1/auth/profile", `{"display_name":""}`},
		{"email", "PATCH", "/api/v1/auth/email", `{"email":"invalid","current_password":"wrong password long enough"}`},
		{"password", "POST", "/api/v1/auth/password", `{"current_password":"wrong password long enough","new_password":"short"}`},
	}
	for _, test := range cases {
		for bodyName, body := range map[string]string{"invalid fields": test.body, "malformed json": "{"} {
			t.Run(test.name+" requires session before "+bodyName, func(t *testing.T) {
				response := accountRequest(app, test.method, test.path, body, nil, "forged")
				if response.Code != 401 || !strings.Contains(response.Body.String(), `"session_invalid"`) {
					t.Fatalf("authorization order: %d %s", response.Code, response.Body.String())
				}
			})
			t.Run(test.name+" requires csrf before "+bodyName, func(t *testing.T) {
				response := accountRequest(app, test.method, test.path, body, current.Result().Cookies(), "forged")
				if response.Code != 403 || !strings.Contains(response.Body.String(), `"csrf_invalid"`) {
					t.Fatalf("csrf order: %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestLocalAccountCurrentAndAllSessionRevocation(t *testing.T) {
	app, _ := accountFixture(t, nil)
	first := registerAccount(t, app, "revoke.user", "revoke@example.com", "current password long enough")
	second := loginAccount(t, app, "revoke.user", "current password long enough")
	logout := accountRequest(app, "POST", "/api/v1/auth/logout", "", first.Result().Cookies(), sessionCSRF(t, first))
	if logout.Code != 204 {
		t.Fatalf("current logout: %d %s", logout.Code, logout.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", second.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("current logout revoked other device: %d", current.Code)
	}

	third := loginAccount(t, app, "revoke.user", "current password long enough")
	all := accountRequest(app, "POST", "/api/v1/auth/logout-all", "", second.Result().Cookies(), sessionCSRF(t, second))
	if all.Code != 204 {
		t.Fatalf("logout all: %d %s", all.Code, all.Body.String())
	}
	for index, session := range []*httptest.ResponseRecorder{second, third} {
		if current := accountRequest(app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 401 {
			t.Fatalf("logout all retained session %d: %d", index, current.Code)
		}
	}
}

func TestLocalAccountPasswordChangeRevokesAllSessions(t *testing.T) {
	app, _ := accountFixture(t, nil)
	first := registerAccount(t, app, "password.user", "password@example.com", "current password long enough")
	second := loginAccount(t, app, "password.user", "current password long enough")
	wrong := accountRequest(app, "POST", "/api/v1/auth/password", `{"current_password":"wrong password long enough","new_password":"replacement password long enough"}`, first.Result().Cookies(), sessionCSRF(t, first))
	if wrong.Code != 400 || !strings.Contains(wrong.Body.String(), `"current_password_invalid"`) {
		t.Fatalf("wrong password change: %d %s", wrong.Code, wrong.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", first.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("wrong password invalidated session: %d", current.Code)
	}
	changed := accountRequest(app, "POST", "/api/v1/auth/password", `{"current_password":"current password long enough","new_password":"replacement password long enough"}`, first.Result().Cookies(), sessionCSRF(t, first))
	if changed.Code != 204 {
		t.Fatalf("password change: %d %s", changed.Code, changed.Body.String())
	}
	for index, session := range []*httptest.ResponseRecorder{first, second} {
		if current := accountRequest(app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 401 {
			t.Fatalf("password change retained session %d: %d", index, current.Code)
		}
	}
	oldCookies, oldCSRF := prepareAccount(t, app)
	oldLogin := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"password.user","password":"current password long enough"}`, oldCookies, oldCSRF)
	if oldLogin.Code != 401 {
		t.Fatalf("old password login: %d", oldLogin.Code)
	}
	loginAccount(t, app, "password.user", "replacement password long enough")
	if response := accountRequest(app, "GET", "/api/v1/auth/accounts", "", nil, ""); response.Code != 404 {
		t.Fatalf("account list exposed: %d", response.Code)
	}
}

func TestLocalAccountLoginAfterLogout(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	registration := registerAccount(t, app, "Login.User", "login@example.com", " correct horse battery staple ")
	var identity struct {
		CSRF string `json:"csrf_token"`
	}
	json.Unmarshal(registration.Body.Bytes(), &identity)
	logout := accountRequest(app, "POST", "/api/v1/auth/logout", "", registration.Result().Cookies(), identity.CSRF)
	if logout.Code != 204 {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", registration.Result().Cookies(), ""); current.Code != 401 {
		t.Fatalf("revoked session: %d", current.Code)
	}
	cookies, csrf := prepareAccount(t, app)
	login := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":" LOGIN.USER ","password":" correct horse battery staple "}`, cookies, csrf)
	if login.Code != 200 {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", login.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("current: %d", current.Code)
	}
}

func TestLocalAccountRegistrationRateLimit(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")
	cookies, csrf := prepareAccount(t, app)
	for i := 0; i < 10; i++ {
		result := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":"bad","email":"invalid","password":"too-short"}`, cookies, csrf)
		if result.Code != 400 {
			t.Fatalf("attempt %d: %d", i, result.Code)
		}
	}
	limited := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":"valid","email":"valid@example.com","password":"correct horse battery staple"}`, cookies, csrf)
	if limited.Code != 429 || limited.Header().Get("Retry-After") == "" {
		t.Fatalf("limited: %d %s", limited.Code, limited.Body.String())
	}
}

func TestLocalAccountNormalizedUniquenessAndAtomicRegistration(t *testing.T) {
	app, db := accountFixture(t, nil)
	for _, collision := range []string{"username", "email"} {
		cookies := make([][]*http.Cookie, 4)
		tokens := make([]string, 4)
		for i := range cookies {
			cookies[i], tokens[i] = prepareAccount(t, app)
		}
		results := make(chan *httptest.ResponseRecorder, 4)
		for i := 0; i < 4; i++ {
			go func(index int) {
				username := fmt.Sprintf("user.%s.%d", collision, index)
				email := fmt.Sprintf("%s.%d@example.com", collision, index)
				if collision == "username" {
					username = []string{" Same.User ", "SAME.USER", "same.user", "Same.User"}[index]
				} else {
					email = []string{" Same+tag@Example.com ", "SAME+TAG@EXAMPLE.COM", "same+tag@example.com", "Same+tag@example.com"}[index]
				}
				body, _ := json.Marshal(map[string]string{"username": username, "email": email, "password": "correct horse battery staple"})
				results <- accountRequest(app, "POST", "/api/v1/auth/register", string(body), cookies[index], tokens[index])
			}(i)
		}
		successes, conflicts := 0, 0
		for i := 0; i < 4; i++ {
			result := <-results
			switch result.Code {
			case 201:
				successes++
			case 409:
				conflicts++
			default:
				t.Fatalf("concurrent register: %d %s", result.Code, result.Body.String())
			}
		}
		if successes != 1 || conflicts != 3 {
			t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
		}
	}
	// Inject an actual database failure at the session write, then prove retry can
	// create the same account. No assertion depends on ORM calls or SQL generated.
	if _, err := db.Exec("ALTER TABLE rcc_login_sessions ADD COLUMN injected_failure INT NOT NULL"); err != nil {
		t.Fatal(err)
	}
	cookies, csrf := prepareAccount(t, app)
	body := `{"username":"atomic.user","email":"atomic@example.com","password":"correct horse battery staple"}`
	failed := accountRequest(app, "POST", "/api/v1/auth/register", body, cookies, csrf)
	if failed.Code != 503 {
		t.Fatalf("session failure: %d %s", failed.Code, failed.Body.String())
	}
	if _, err := db.Exec("ALTER TABLE rcc_login_sessions DROP COLUMN injected_failure"); err != nil {
		t.Fatal(err)
	}
	recovered := accountRequest(app, "POST", "/api/v1/auth/register", body, cookies, csrf)
	if recovered.Code != 201 {
		t.Fatalf("atomic retry: %d %s", recovered.Code, recovered.Body.String())
	}
}

func accountFixture(t *testing.T, now func() time.Time) (*adminApplication, *sql.DB) {
	t.Helper()
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	app, err := newApplicationWithClock(ctx, integrationConfig(driver), now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return app, db
}

func TestLocalAccountFailuresAndCredentialBoundaries(t *testing.T) {
	now := time.Now().UTC()
	app, db := accountFixture(t, func() time.Time { return now })
	registered := registerAccount(t, app, "credential.user", "credential@example.com", " exact password with spaces ")
	var identity struct {
		CSRF    string `json:"csrf_token"`
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	json.Unmarshal(registered.Body.Bytes(), &identity)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(identity.Account.ID) {
		t.Fatal("not UUID v4")
	}
	// Database state is used only to set up the disabled-account scenario.
	if _, err := db.Exec("UPDATE rcc_accounts SET enabled = FALSE, session_version = session_version + 1 WHERE id = ?", identity.Account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE rcc_login_sessions SET expires_at = ? WHERE account_id = ?", now.Add(-time.Minute), identity.Account.ID); err != nil {
		t.Fatal(err)
	}
	disabledSession := accountRequest(app, "GET", "/api/v1/auth/session", "", registered.Result().Cookies(), "")
	if disabledSession.Code != 401 || !strings.Contains(disabledSession.Body.String(), `"account_disabled"`) {
		t.Fatalf("disabled session classification: %d %s", disabledSession.Code, disabledSession.Body.String())
	}
	var failure string
	for _, attempt := range []string{`{"username":"unknown.user","password":"wrong password long enough"}`, `{"username":"credential.user","password":"wrong password long enough"}`, `{"username":"credential.user","password":" exact password with spaces "}`} {
		cookies, csrf := prepareAccount(t, app)
		response := accountRequest(app, "POST", "/api/v1/auth/login", attempt, cookies, csrf)
		if response.Code != 401 || len(response.Result().Cookies()) != 0 {
			t.Fatalf("login rejection: %d %s", response.Code, response.Body.String())
		}
		var decoded struct {
			Error struct{ Code, Message string } `json:"error"`
		}
		json.Unmarshal(response.Body.Bytes(), &decoded)
		safe := decoded.Error.Code + decoded.Error.Message
		if failure != "" && safe != failure {
			t.Fatal("login disclosed account state")
		}
		failure = safe
	}
	if _, err := db.Exec("UPDATE rcc_accounts SET enabled = TRUE WHERE id = ?", identity.Account.ID); err != nil {
		t.Fatal(err)
	}
	if restoredOldSession := accountRequest(app, "GET", "/api/v1/auth/session", "", registered.Result().Cookies(), ""); restoredOldSession.Code != 401 || !strings.Contains(restoredOldSession.Body.String(), `"session_invalid"`) {
		t.Fatalf("restored account revived old session: %d %s", restoredOldSession.Code, restoredOldSession.Body.String())
	}
	// Password spaces are significant.
	cookies, csrf := prepareAccount(t, app)
	trimmed := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"credential.user","password":"exact password with spaces"}`, cookies, csrf)
	if trimmed.Code != 401 {
		t.Fatalf("trimmed password: %d", trimmed.Code)
	}
	if response := accountRequest(app, "GET", "/api/v1/auth/session", "", cookies, ""); response.Code != 401 {
		t.Fatal("preauth granted identity")
	}
	for _, value := range []string{"forged", strings.Repeat("a", 43), cookies[0].Value} {
		response := accountRequest(app, "GET", "/api/v1/auth/session", "", []*http.Cookie{{Name: "rcc-session-dev", Value: value}}, "")
		if response.Code != 401 {
			t.Fatalf("forged identity: %d", response.Code)
		}
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	request.Header.Set("Authorization", "Bearer integration-token")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal("deployment token bypassed accounts")
	}
	staleCookies, staleCSRF := prepareAccount(t, app)
	login := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"credential.user","password":" exact password with spaces "}`, staleCookies, staleCSRF)
	if login.Code != 200 {
		t.Fatalf("valid login: %d %s", login.Code, login.Body.String())
	}
	replay := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"credential.user","password":" exact password with spaces "}`, staleCookies, staleCSRF)
	if replay.Code != 403 {
		t.Fatalf("consumed preauth: %d", replay.Code)
	}
	for _, cookie := range login.Result().Cookies() {
		if cookie.MaxAge > 0 && (!cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.Secure) {
			t.Fatalf("local cookie attributes: %#v", cookie)
		}
	}
	now = now.Add(30 * time.Minute)
	expired := accountRequest(app, "GET", "/api/v1/auth/session", "", login.Result().Cookies(), "")
	if expired.Code != 401 {
		t.Fatalf("idle boundary: %d", expired.Code)
	}
}

func TestLocalAccountRateLimitsAcrossIPsAndWindows(t *testing.T) {
	now := time.Now().UTC()
	app, _ := accountFixture(t, func() time.Time { return now })
	cookies, csrf := prepareAccount(t, app)
	results := make(chan *httptest.ResponseRecorder, 20)
	for index := 0; index < 20; index++ {
		go func(index int) {
			results <- accountRequestFrom(app, "POST", "/api/v1/auth/login", `{"username":" UNKNOWN.USER ","password":"wrong password long enough"}`, cookies, csrf, fmt.Sprintf("192.0.2.%d:8080", index+1), nil)
		}(index)
	}
	failures, limited := 0, 0
	for index := 0; index < 20; index++ {
		response := <-results
		switch response.Code {
		case 401:
			failures++
		case 429:
			limited++
			if response.Header().Get("Retry-After") == "" {
				t.Fatal("missing retry delay")
			}
		default:
			t.Fatalf("parallel login: %d %s", response.Code, response.Body.String())
		}
	}
	if failures != 10 || limited != 10 {
		t.Fatalf("per-username concurrent bound: failures=%d limited=%d", failures, limited)
	}
	now = now.Add(15 * time.Minute)
	cookies, csrf = prepareAccount(t, app)
	resumed := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"unknown.user","password":"wrong password long enough"}`, cookies, csrf)
	if resumed.Code != 401 {
		t.Fatalf("window did not recover: %d", resumed.Code)
	}
	for index := 0; index < 10; index++ {
		response := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":"valid.user","email":"invalid","password":"correct horse battery staple"}`, cookies, csrf)
		if response.Code != 400 {
			t.Fatalf("register attempt %d: %d", index, response.Code)
		}
	}
	spoof := accountRequestFrom(app, "POST", "/api/v1/auth/register", `{"username":"valid.user","email":"valid@example.com","password":"correct horse battery staple"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"X-Forwarded-For": "198.51.100.9", "X-Real-IP": "198.51.100.10", "Forwarded": "for=198.51.100.11"})
	if spoof.Code != 429 {
		t.Fatalf("untrusted forwarded header bypass: %d %s", spoof.Code, spoof.Body.String())
	}
	now = now.Add(time.Hour)
	cookies, csrf = prepareAccount(t, app)
	recovered := accountRequest(app, "POST", "/api/v1/auth/register", `{"username":"valid.user","email":"valid@example.com","password":"correct horse battery staple"}`, cookies, csrf)
	if recovered.Code != 201 {
		t.Fatalf("registration window: %d %s", recovered.Code, recovered.Body.String())
	}
}

func TestLocalAccountSuccessClearsFailureCountAndIPLimit(t *testing.T) {
	now := time.Now().UTC()
	app, _ := accountFixture(t, func() time.Time { return now })
	registerAccount(t, app, "clear.user", "clear@example.com", "correct horse battery staple")
	cookies, csrf := prepareAccount(t, app)
	fail := `{"username":"CLEAR.USER","password":"wrong password long enough"}`
	for i := 0; i < 9; i++ {
		if r := accountRequest(app, "POST", "/api/v1/auth/login", fail, cookies, csrf); r.Code != 401 {
			t.Fatalf("failure %d: %d", i, r.Code)
		}
	}
	login := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"clear.user","password":"correct horse battery staple"}`, cookies, csrf)
	if login.Code != 200 {
		t.Fatalf("login before threshold: %d", login.Code)
	}
	cookies, csrf = prepareAccount(t, app)
	for i := 0; i < 10; i++ {
		if r := accountRequest(app, "POST", "/api/v1/auth/login", fail, cookies, csrf); r.Code != 401 {
			t.Fatalf("success failed to clear counter at %d: %d", i, r.Code)
		}
	}
	if r := accountRequest(app, "POST", "/api/v1/auth/login", fail, cookies, csrf); r.Code != 429 {
		t.Fatalf("username limit: %d", r.Code)
	}
	// The same IP cannot evade the minute limit with distinct usernames or headers.
	for i := 0; i < 39; i++ {
		body := fmt.Sprintf(`{"username":"other.%d","password":"wrong password long enough"}`, i)
		if r := accountRequest(app, "POST", "/api/v1/auth/login", body, cookies, csrf); r.Code != 401 {
			t.Fatalf("IP admitted %d: %d", i, r.Code)
		}
	}
	r := accountRequestFrom(app, "POST", "/api/v1/auth/login", `{"username":"last.user","password":"wrong password long enough"}`, cookies, csrf, "192.0.2.1:1234", map[string]string{"X-Forwarded-For": "198.51.100.4"})
	if r.Code != 429 {
		t.Fatalf("IP limit bypass: %d", r.Code)
	}
	now = now.Add(time.Minute)
	if r := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"last.user","password":"wrong password long enough"}`, cookies, csrf); r.Code != 401 {
		t.Fatalf("IP window recovery: %d", r.Code)
	}
}

func TestLocalAccountCSRFAndServiceFailure(t *testing.T) {
	app, _ := accountFixture(t, nil)
	cookies, csrf := prepareAccount(t, app)
	body := `{"username":"csrf.user","email":"csrf@example.com","password":"correct horse battery staple"}`
	for _, headers := range []map[string]string{{"X-CSRF-Token": ""}, {"Origin": "https://evil.example"}, {"Origin": ""}, {"Origin": "null"}} {
		response := accountRequestFrom(app, "POST", "/api/v1/auth/register", body, cookies, csrf, "192.0.2.1:1234", headers)
		if response.Code != 403 {
			t.Fatalf("CSRF rejected: %d %s", response.Code, response.Body.String())
		}
	}
	accepted := accountRequestFrom(app, "POST", "/api/v1/auth/register", body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Origin": "", "Referer": "http://127.0.0.1:5173/register"})
	if accepted.Code != 201 {
		t.Fatalf("same-origin Referer: %d %s", accepted.Code, accepted.Body.String())
	}
	// A stale CSRF token cannot act under a newly issued session.
	rejected := accountRequest(app, "POST", "/api/v1/auth/logout", "", accepted.Result().Cookies(), csrf)
	if rejected.Code != 403 {
		t.Fatalf("old CSRF upgraded: %d", rejected.Code)
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", accepted.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("bad CSRF changed session: %d", current.Code)
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
	failed := accountRequest(app, "GET", "/api/v1/auth/session", "", accepted.Result().Cookies(), "")
	if failed.Code != 503 || len(failed.Result().Cookies()) != 0 {
		t.Fatalf("database failure misclassified or cleared cookie: %d %s", failed.Code, failed.Body.String())
	}
}

func TestLocalAccountHTTPFieldContract(t *testing.T) {
	app, _ := accountFixture(t, nil)
	cookies, csrf := prepareAccount(t, app)
	for _, body := range []string{
		`{"username":"ab","email":"valid@example.com","password":"correct horse battery staple"}`,
		`{"username":"Kelvin","email":"valid@example.com","password":"correct horse battery staple"}`,
		`{"username":"valid.user","email":"K@example.com","password":"correct horse battery staple"}`,
		`{"username":"valid.user","email":"valid@example.com","password":"short"}`,
		`{"username":"valid.user","email":"valid@example.com","password":"correct horse battery staple","display_name":""}`,
		`{"username":"valid.user","email":"valid@example.com","password":"correct horse battery staple","display_name":null}`,
		`{"username":"valid.user","email":"valid@example.com","password":"correct horse battery staple","display_name":"a\nb"}`,
	} {
		r := accountRequest(app, "POST", "/api/v1/auth/register", body, cookies, csrf)
		if r.Code != 400 || !strings.Contains(r.Body.String(), "invalid_account_fields") {
			t.Fatalf("field contract: %d %s", r.Code, r.Body.String())
		}
	}
	password := strings.Repeat("🔐", 128)
	registered := registerAccount(t, app, "abc", "unicode+tag@example.com", password)
	if !strings.Contains(registered.Body.String(), `"display_name":"abc"`) {
		t.Fatal("display name default")
	}
	cookies, csrf = prepareAccount(t, app)
	body, _ := json.Marshal(map[string]string{"username": "ABC", "password": password})
	if r := accountRequest(app, "POST", "/api/v1/auth/login", string(body), cookies, csrf); r.Code != 200 {
		t.Fatalf("full Unicode password round trip: %d", r.Code)
	}
}

func TestLocalAccountStoredSecretsAndAbsoluteExpiry(t *testing.T) {
	now := time.Now().UTC()
	app, db := accountFixture(t, func() time.Time { return now })
	password := " identical password for independent salts "
	first := registerAccount(t, app, "salt.first", "salt.first@example.com", password)
	registerAccount(t, app, "salt.second", "salt.second@example.com", password)
	// This database-boundary evidence checks the required persisted security
	// material, which cannot be observed through the intentionally safe HTTP DTO.
	rows, err := db.Query("SELECT password_hash FROM rcc_accounts ORDER BY username")
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[0] == hashes[1] {
		t.Fatal("password salts not independent")
	}
	for _, hash := range hashes {
		if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") || strings.Contains(hash, password) {
			t.Fatal("password not stored with required Argon2id parameters")
		}
	}
	for _, cookie := range first.Result().Cookies() {
		if cookie.MaxAge <= 0 {
			continue
		}
		var rawMatches int
		if err := db.QueryRow("SELECT COUNT(*) FROM rcc_login_sessions WHERE token_hash = ? OR csrf_hash = ?", cookie.Value, cookie.Value).Scan(&rawMatches); err != nil {
			t.Fatal(err)
		}
		if rawMatches != 0 {
			t.Fatal("raw session token stored")
		}
		altered := *cookie
		altered.Value = "A" + cookie.Value[1:]
		if altered.Value == cookie.Value {
			altered.Value = "B" + cookie.Value[1:]
		}
		if response := accountRequest(app, "GET", "/api/v1/auth/session", "", []*http.Cookie{&altered}, ""); response.Code != 401 {
			t.Fatal("tampered cookie acquired identity")
		}
	}
	if strings.Contains(first.Body.String(), "password_hash") || strings.Contains(first.Body.String(), "token_hash") {
		t.Fatal("private authentication fields leaked")
	}
	// Set up recent activity close to the absolute limit without implementing T2's
	// activity API here, then observe the initial session's fixed 8-hour boundary.
	if _, err := db.Exec("UPDATE rcc_login_sessions SET last_active_at = ?", now.Add(7*time.Hour+59*time.Minute)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * time.Hour)
	if response := accountRequest(app, "GET", "/api/v1/auth/session", "", first.Result().Cookies(), ""); response.Code != 401 {
		t.Fatalf("absolute expiry: %d", response.Code)
	}
}

func TestLocalAccountHTTPSCookiesAndPreauthExpiry(t *testing.T) {
	now := time.Now().UTC()
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	settings := integrationConfig(driver)
	settings.AccountPublicOrigin = "https://config.example.test"
	settings.AccountInsecureHTTP = false
	app, err := newApplicationWithClock(ctx, settings, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	cookies, csrf := prepareAccount(t, app)
	for _, cookie := range cookies {
		if cookie.Name != "__Host-rcc-preauth" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/" {
			t.Fatal("HTTPS preauth cookie attributes")
		}
	}
	now = now.Add(10 * time.Minute)
	body := `{"username":"https.user","email":"https@example.com","password":"correct horse battery staple"}`
	expired := accountRequestFrom(app, "POST", "/api/v1/auth/register", body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Origin": "https://config.example.test"})
	if expired.Code != 403 {
		t.Fatalf("preauth expiry: %d", expired.Code)
	}
	cookies, csrf = prepareAccount(t, app)
	registered := accountRequestFrom(app, "POST", "/api/v1/auth/register", body, cookies, csrf, "192.0.2.1:1234", map[string]string{"Origin": "https://config.example.test"})
	if registered.Code != 201 {
		t.Fatalf("https registration: %d %s", registered.Code, registered.Body.String())
	}
	for _, cookie := range registered.Result().Cookies() {
		if cookie.MaxAge > 0 && (cookie.Name != "__Host-rcc-session" || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Domain != "" || cookie.Path != "/" || cookie.MaxAge != 28800) {
			t.Fatal("HTTPS session cookie attributes")
		}
	}
}

func TestLocalAccountDatabaseLockTimeoutIs504(t *testing.T) {
	ctx, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	settings := integrationConfig(driver)
	settings.MySQL.ReadTimeout = time.Second
	app, err := newApplication(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cookies, csrf := prepareAccount(t, app)
	lock, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback()
	if _, err := lock.Exec("SELECT id FROM rcc_auth_control_lock WHERE id = 1 FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	body := `{"username":"timeout.user","email":"timeout@example.com","password":"correct horse battery staple"}`
	timedOut := accountRequest(app, "POST", "/api/v1/auth/register", body, cookies, csrf)
	if timedOut.Code != 504 || !strings.Contains(timedOut.Body.String(), `"auth_timeout"`) {
		t.Fatalf("lock timeout: %d %s", timedOut.Code, timedOut.Body.String())
	}
	if err := lock.Rollback(); err != nil {
		t.Fatal(err)
	}
	retried := accountRequest(app, "POST", "/api/v1/auth/register", body, cookies, csrf)
	if retried.Code != 201 {
		t.Fatalf("uncommitted timeout left partial registration: %d %s", retried.Code, retried.Body.String())
	}
}

func TestLocalAccountSuccessfulLoginPreservesOutstandingReservations(t *testing.T) {
	app, db := accountFixture(t, nil)
	registerAccount(t, app, "pending.user", "pending@example.com", "correct horse battery staple")
	cookies, csrf := prepareAccount(t, app)
	wrong := `{"username":"pending.user","password":"wrong password long enough"}`
	if response := accountRequest(app, "POST", "/api/v1/auth/login", wrong, cookies, csrf); response.Code != 401 {
		t.Fatal("failure setup")
	}
	// A second process may have admitted nine attempts before completing password
	// work. Model that external persisted state, then use a real successful login.
	if _, err := db.Exec("UPDATE rcc_auth_rate_limits SET attempts = 0, in_flight = 9 WHERE bucket_key LIKE 'login-user:%'"); err != nil {
		t.Fatal(err)
	}
	success := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"pending.user","password":"correct horse battery staple"}`, cookies, csrf)
	if success.Code != 200 {
		t.Fatalf("successful competing login: %d %s", success.Code, success.Body.String())
	}
	// The other process completes its nine rejected attempts after this success.
	// These fixture transitions are not identity setup or ORM interaction mocks.
	completed, err := db.Exec("UPDATE rcc_auth_rate_limits SET attempts = attempts + in_flight, in_flight = 0 WHERE bucket_key LIKE 'login-user:%'")
	if err != nil {
		t.Fatal(err)
	}
	if count, _ := completed.RowsAffected(); count != 1 {
		t.Fatal("success deleted the other process's reservation window")
	}
	cookies, csrf = prepareAccount(t, app)
	if response := accountRequest(app, "POST", "/api/v1/auth/login", wrong, cookies, csrf); response.Code != 401 {
		t.Fatalf("tenth failure: %d", response.Code)
	}
	if response := accountRequest(app, "POST", "/api/v1/auth/login", wrong, cookies, csrf); response.Code != 429 {
		t.Fatalf("outstanding failures were erased: %d", response.Code)
	}
}

func TestLocalAccountLoginSettlementFailurePreservesPreviousSession(t *testing.T) {
	app, db := accountFixture(t, nil)
	previous := registerAccount(t, app, "settlement.user", "settlement@example.com", "correct horse battery staple")
	cookies, csrf := prepareAccount(t, app)
	failure := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"settlement.user","password":"wrong password long enough"}`, cookies, csrf)
	if failure.Code != 401 {
		t.Fatal("failed login setup")
	}
	// Make the success counter update fail at the real database boundary.
	if _, err := db.Exec("ALTER TABLE rcc_auth_rate_limits ADD CONSTRAINT injected_settlement_failure CHECK (bucket_key NOT LIKE 'login-user:%' OR attempts >= 1)"); err != nil {
		t.Fatal(err)
	}
	loginCookies := append(cookies, previous.Result().Cookies()...)
	correct := `{"username":"settlement.user","password":"correct horse battery staple"}`
	failed := accountRequest(app, "POST", "/api/v1/auth/login", correct, loginCookies, csrf)
	if failed.Code != 503 {
		t.Fatalf("settlement failure: %d %s", failed.Code, failed.Body.String())
	}
	if current := accountRequest(app, "GET", "/api/v1/auth/session", "", previous.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("failed login revoked original session: %d", current.Code)
	}
	if _, err := db.Exec("ALTER TABLE rcc_auth_rate_limits DROP CHECK injected_settlement_failure"); err != nil {
		t.Fatal(err)
	}
	retried := accountRequest(app, "POST", "/api/v1/auth/login", correct, loginCookies, csrf)
	if retried.Code != 200 {
		t.Fatalf("failed login consumed preparation: %d %s", retried.Code, retried.Body.String())
	}
}

func TestLocalAccountCancelledLoginReleasesReservation(t *testing.T) {
	app, db := accountFixture(t, nil)
	registerAccount(t, app, "cancel.user", "cancel@example.com", "correct horse battery staple")
	cookies, csrf := prepareAccount(t, app)
	// Set the encoded hash's supported work factor high enough to cancel at the
	// external request boundary while real Argon2 work is still running. The
	// attempted password is deliberately wrong regardless of this fixture hash.
	if _, err := db.Exec("UPDATE rcc_accounts SET password_hash = REPLACE(password_hash, 'm=19456,t=2', 'm=65536,t=5') WHERE username = 'cancel.user'"); err != nil {
		t.Fatal(err)
	}
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"username":"cancel.user","password":"wrong password long enough"}`)).WithContext(requestContext)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("X-CSRF-Token", csrf)
	req.RemoteAddr = "192.0.2.1:1234"
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	done := make(chan struct{})
	go func() { defer close(done); app.Handler().ServeHTTP(httptest.NewRecorder(), req) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var inFlight int
		if err := db.QueryRow("SELECT COALESCE(SUM(in_flight),0) FROM rcc_auth_rate_limits WHERE bucket_key LIKE 'login-user:%'").Scan(&inFlight); err != nil {
			t.Fatal(err)
		}
		if inFlight == 1 {
			cancel()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("login did not reach reserved password work")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled request did not finish bounded cleanup")
	}
	if _, err := db.Exec("UPDATE rcc_accounts SET password_hash = REPLACE(password_hash, 'm=65536,t=5', 'm=19456,t=2') WHERE username = 'cancel.user'"); err != nil {
		t.Fatal(err)
	}

	// A fresh successful login clears completed failures. It must not inherit a
	// leaked in-flight slot from the earlier cancelled request.
	signedIn := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"cancel.user","password":"correct horse battery staple"}`, cookies, csrf)
	if signedIn.Code != 200 {
		t.Fatalf("login after cancellation: %d %s", signedIn.Code, signedIn.Body.String())
	}
	cookies, csrf = prepareAccount(t, app)
	for index := 0; index < 10; index++ {
		response := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"cancel.user","password":"wrong password long enough"}`, cookies, csrf)
		if response.Code != 401 {
			t.Fatalf("cancelled request retained a reservation at %d: %d %s", index, response.Code, response.Body.String())
		}
	}
	if response := accountRequest(app, "POST", "/api/v1/auth/login", `{"username":"cancel.user","password":"wrong password long enough"}`, cookies, csrf); response.Code != 429 {
		t.Fatalf("failed-attempt limit: %d", response.Code)
	}

}
