//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

type maintenanceFixture struct {
	app           *adminApplication
	db            *sql.DB
	binary        string
	environment   []string
	settings      config.Config
	databaseOwner *sql.DB
}

func newMaintenanceFixture(t *testing.T) maintenanceFixture {
	t.Helper()
	ctx, driver := startCurrentIntegrationMySQL(t)
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
	binary := filepath.Join(t.TempDir(), "account-maintain")
	build := exec.Command("go", "build", "-o", binary, "../account-maintain")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build maintenance: %v %s", err, output)
	}
	environment := []string{"PATH=" + os.Getenv("PATH")}
	for _, value := range integrationEnvironment(driver, "invalid-http-address") {
		if strings.HasPrefix(value, "MYSQL_") {
			environment = append(environment, value)
		}
	}
	ownerConfig := *driver
	ownerConfig.User = "root"
	owner, err := sql.Open("mysql", ownerConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	return maintenanceFixture{app, db, binary, environment, integrationConfig(driver), owner}
}

func (f maintenanceFixture) command(input string, args ...string) *exec.Cmd {
	cmd := exec.Command(f.binary, args...)
	cmd.Env = f.environment
	cmd.Stdin = strings.NewReader(input)
	return cmd
}

func (f maintenanceFixture) run(t *testing.T, input string, args ...string) string {
	t.Helper()
	output, err := f.command(input, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("maintenance %s: %v %s", args[0], err, output)
	}
	return string(output)
}

func TestAccountMaintenanceResetPasswordRevokesEverySession(t *testing.T) {
	f := newMaintenanceFixture(t)
	first := registerAccount(t, f.app, "reset.user", "reset@example.com", "current password long enough")
	second := loginAccount(t, f.app, "reset.user", "current password long enough")
	id := accountID(t, first)
	password := " 新的密码 unchanged spaces \n"
	output := f.run(t, password, "reset-password", "--username", " RESET.USER ", "--password-stdin")
	if strings.Contains(output, password) || strings.Contains(output, "reset@example.com") {
		t.Fatal("maintenance disclosed secret")
	}
	for _, session := range []*httptest.ResponseRecorder{first, second} {
		if current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 401 {
			t.Fatalf("reset kept session: %d", current.Code)
		}
	}
	cookies, csrf := prepareAccount(t, f.app)
	if old := accountRequest(f.app, "POST", "/api/v1/auth/login", `{"username":"reset.user","password":"current password long enough"}`, cookies, csrf); old.Code != 401 {
		t.Fatalf("old password: %d", old.Code)
	}
	fresh := loginAccount(t, f.app, "reset.user", password)
	if accountID(t, fresh) != id {
		t.Fatal("reset changed identity")
	}
	f.run(t, password, "reset-password", "--id", id, "--password-stdin")
	if current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", fresh.Result().Cookies(), ""); current.Code != 401 {
		t.Fatalf("same-password reset kept session: %d", current.Code)
	}
	loginAccount(t, f.app, "reset.user", password)
}

func TestAccountMaintenanceDisableAndEnableDoNotReviveSessions(t *testing.T) {
	f := newMaintenanceFixture(t)
	first := registerAccount(t, f.app, "status.user", "status@example.com", "current password long enough")
	second := loginAccount(t, f.app, "status.user", "current password long enough")
	id := accountID(t, first)
	f.run(t, "", "disable", "--id", id)
	for _, session := range []*httptest.ResponseRecorder{first, second} {
		current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), "")
		if current.Code != 401 || !strings.Contains(current.Body.String(), `"account_disabled"`) {
			t.Fatalf("disabled session cannot destroy drafts: %d %s", current.Code, current.Body.String())
		}
	}
	cookies, csrf := prepareAccount(t, f.app)
	disabled := accountRequest(f.app, "POST", "/api/v1/auth/login", `{"username":"status.user","password":"current password long enough"}`, cookies, csrf)
	if disabled.Code != 401 || !strings.Contains(disabled.Body.String(), `"invalid_credentials"`) {
		t.Fatalf("disabled login: %d %s", disabled.Code, disabled.Body.String())
	}
	f.run(t, "", "enable", "--username", "status.user")
	for _, session := range []*httptest.ResponseRecorder{first, second} {
		if current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), ""); current.Code != 401 || !strings.Contains(current.Body.String(), `"session_invalid"`) {
			t.Fatalf("enable revived session: %d %s", current.Code, current.Body.String())
		}
	}
	fresh := loginAccount(t, f.app, "status.user", "current password long enough")
	if accountID(t, fresh) != id {
		t.Fatal("enable changed identity")
	}
	// A repeated enable is an idempotent status operation, not a new revocation.
	f.run(t, "", "enable", "--id", id)
	if current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", fresh.Result().Cookies(), ""); current.Code != 200 {
		t.Fatalf("idempotent enable: %d", current.Code)
	}
}

func TestAccountMaintenanceResetDisabledAccountPreservesDraftDestruction(t *testing.T) {
	f := newMaintenanceFixture(t)
	old := registerAccount(t, f.app, "disabled.reset", "disabled-reset@example.com", "current password long enough")
	id := accountID(t, old)
	f.run(t, "", "disable", "--id", id)
	f.run(t, "replacement password long enough", "reset-password", "--id", id, "--password-stdin")
	current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", old.Result().Cookies(), "")
	if current.Code != 401 || !strings.Contains(current.Body.String(), `"account_disabled"`) {
		t.Fatalf("reset lost disabled draft destruction: %d %s", current.Code, current.Body.String())
	}
	cookies, csrf := prepareAccount(t, f.app)
	response := accountRequest(f.app, "POST", "/api/v1/auth/login", `{"username":"disabled.reset","password":"replacement password long enough"}`, cookies, csrf)
	if response.Code != 401 {
		t.Fatalf("reset enabled disabled account: %d", response.Code)
	}
	f.run(t, "", "enable", "--id", id)
	current = accountRequest(f.app, "GET", "/api/v1/auth/session", "", old.Result().Cookies(), "")
	if current.Code != 401 || !strings.Contains(current.Body.String(), `"session_invalid"`) {
		t.Fatalf("enable revived reset session: %d %s", current.Code, current.Body.String())
	}
	loginAccount(t, f.app, "disabled.reset", "replacement password long enough")
}

func TestAccountMaintenanceCorrectEmailPreservesOwnershipAndSessions(t *testing.T) {
	f := newMaintenanceFixture(t)
	first := registerAccount(t, f.app, "email.user", "original@example.com", "current password long enough")
	second := loginAccount(t, f.app, "email.user", "current password long enough")
	registerAccount(t, f.app, "occupied.user", "occupied@example.com", "current password long enough")
	id := accountID(t, first)
	sessions := []*httptest.ResponseRecorder{first, second}
	beforeSessions := make([]*httptest.ResponseRecorder, len(sessions))
	for i, session := range sessions {
		beforeSessions[i] = accountRequest(f.app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), "")
		if beforeSessions[i].Code != 200 {
			t.Fatal("initial session unavailable")
		}
	}
	for _, invalid := range []string{"", "not-an-email@", " name@example.com,other@example.com ", " OCCUPIED@example.com "} {
		output, err := f.command(invalid, "set-email", "--id", id, "--email-stdin").CombinedOutput()
		if err == nil {
			t.Fatal("accepted invalid or occupied email")
		}
		if invalid != "" && strings.Contains(string(output), strings.TrimSpace(invalid)) {
			t.Fatal("error disclosed email")
		}
	}
	if unchanged := accountRequest(f.app, "GET", "/api/v1/auth/session", "", first.Result().Cookies(), ""); unchanged.Code != 200 || !strings.Contains(unchanged.Body.String(), `"email":"original@example.com"`) {
		t.Fatal("rejected email correction changed profile")
	}
	f.run(t, " Corrected+Tag@Example.com \n", "set-email", "--id", id, "--email-stdin")
	for i, session := range sessions {
		current := accountRequest(f.app, "GET", "/api/v1/auth/session", "", session.Result().Cookies(), "")
		if current.Code != 200 {
			t.Fatalf("email change revoked session: %d", current.Code)
		}
		var identity struct {
			Account struct {
				ID, Username, Email string
				EmailVerified       bool `json:"email_verified"`
			} `json:"account"`
			ExpiresAt     string `json:"expires_at"`
			IdleExpiresAt string `json:"idle_expires_at"`
		}
		if err := json.Unmarshal(current.Body.Bytes(), &identity); err != nil {
			t.Fatal(err)
		}
		if identity.Account.ID != id || identity.Account.Username != "email.user" || identity.Account.Email != "corrected+tag@example.com" || identity.Account.EmailVerified {
			t.Fatalf("email change altered account ownership or verification")
		}
		var before struct {
			ExpiresAt     string `json:"expires_at"`
			IdleExpiresAt string `json:"idle_expires_at"`
		}
		if err := json.Unmarshal(beforeSessions[i].Body.Bytes(), &before); err != nil {
			t.Fatal(err)
		}
		if before.ExpiresAt == "" || before.IdleExpiresAt == "" || identity.ExpiresAt != before.ExpiresAt || identity.IdleExpiresAt != before.IdleExpiresAt {
			t.Fatal("email correction extended session")
		}
	}
	registerAccount(t, f.app, "released.user", "original@example.com", "current password long enough")
	output := f.run(t, "", "lookup", "--username", " EMAIL.USER ")
	if !strings.Contains(output, id) || strings.Contains(output, "@") {
		t.Fatal("lookup failed or disclosed email")
	}
	f.run(t, "", "disable", "--id", id)
	cookies, csrf := prepareAccount(t, f.app)
	for _, body := range []string{
		`{"username":"email.user","email":"fresh@example.com","password":"current password long enough"}`,
		`{"username":"fresh.user","email":"corrected+tag@example.com","password":"current password long enough"}`,
	} {
		if response := accountRequest(f.app, "POST", "/api/v1/auth/register", body, cookies, csrf); response.Code != 409 {
			t.Fatalf("disabled account released unique field: %d", response.Code)
		}
	}
}

func TestAccountMaintenanceIndependentConnectionAndAtomicFailures(t *testing.T) {
	f := newMaintenanceFixture(t)
	current := registerAccount(t, f.app, "offline.user", "offline@example.com", "current password long enough")
	id := accountID(t, current)
	// The normal Admin must fail here, but maintenance needs only its control
	// tables and MYSQL_* configuration, without HTTP origin or a login session.
	if _, err := f.db.Exec("DROP TABLE rcc_table_policies"); err != nil {
		t.Fatal(err)
	}
	if app, err := newApplication(context.Background(), f.settings); err == nil {
		_ = app.Close()
		t.Fatal("normal Admin ignored missing Policy Catalog")
	}
	f.run(t, "", "lookup", "--id", id)
	f.run(t, "replacement password long enough", "reset-password", "--id", id, "--password-stdin")
	current = loginAccount(t, f.app, "offline.user", "replacement password long enough")
	// Fail after the account UPDATE at the real database transaction boundary.
	// A failed reset must preserve both the prior password and every session.
	// Only fault setup uses this disposable container's root connection: MySQL
	// with binary logging requires SUPER to create a trigger. The CLI retains
	// the ordinary database-scoped account throughout this test.
	if _, err := f.databaseOwner.Exec(`CREATE TRIGGER reject_session_revoke BEFORE DELETE ON rcc_login_sessions FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='sensitive-storage-canary@example.com'`); err != nil {
		t.Fatal(err)
	}
	output, err := f.command("another replacement password", "reset-password", "--id", id, "--password-stdin").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "account storage unavailable") || strings.Contains(string(output), "sensitive-storage-canary") || strings.Contains(string(output), "another replacement password") {
		t.Fatal("reset failure missing safe diagnostic")
	}
	if session := accountRequest(f.app, "GET", "/api/v1/auth/session", "", current.Result().Cookies(), ""); session.Code != 200 {
		t.Fatalf("failed reset revoked session: %d", session.Code)
	}
	loginAccount(t, f.app, "offline.user", "replacement password long enough")
	if _, err := f.databaseOwner.Exec("DROP TRIGGER reject_session_revoke"); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"too short", strings.Repeat("界", 14), strings.Repeat("界", 129), string([]byte{0xff}) + "invalid utf8 password", strings.Repeat("x", 513)} {
		output, err := f.command(input, "reset-password", "--id", id, "--password-stdin").CombinedOutput()
		if err == nil || strings.Contains(string(output), input) {
			t.Fatal("invalid password accepted or disclosed")
		}
	}
	canary := "argv-secret-never-echo"
	for _, args := range [][]string{
		{"reset-password", "--id", id, "--password", canary},
		{"reset-password", "--id", id, "--password-stdin", canary},
		{"reset-password", "--id", id, "--password-stdin=" + canary},
		{"reset-password", "--id", id},
		{"lookup", "--id", id, "--username", "offline.user"},
	} {
		output, err := f.command("", args...).CombinedOutput()
		if err == nil || strings.Contains(string(output), canary) {
			t.Fatal("unsafe command arguments accepted or echoed")
		}
	}
	output, err = f.command("", "lookup", "--username", "missing.user").CombinedOutput()
	if err == nil || strings.TrimSpace(string(output)) != "account not found" {
		t.Fatal("missing account diagnostic")
	}
	if session := accountRequest(f.app, "GET", "/api/v1/auth/session", "", current.Result().Cookies(), ""); session.Code != 200 {
		t.Fatalf("invalid input changed account: %d", session.Code)
	}
	if _, err := f.db.Exec("RENAME TABLE rcc_login_sessions TO unavailable_sessions"); err != nil {
		t.Fatal(err)
	}
	output, err = f.command("replacement password long enough", "reset-password", "--id", id, "--password-stdin").CombinedOutput()
	if err == nil || !strings.Contains(string(output), "schema-migrate status") {
		t.Fatal("missing schema lacks actionable diagnostic")
	}
	if _, err := f.db.Exec("RENAME TABLE unavailable_sessions TO rcc_login_sessions"); err != nil {
		t.Fatal(err)
	}
	if session := accountRequest(f.app, "GET", "/api/v1/auth/session", "", current.Result().Cookies(), ""); session.Code != 200 {
		t.Fatalf("missing schema left partial reset: %d", session.Code)
	}
}
