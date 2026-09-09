//go:build integration

package main

import (
	"database/sql"
	"encoding/json"
	passwordadapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

type accountProcess struct {
	cmd          *exec.Cmd
	output       *synchronizedBuffer
	done         chan struct{}
	waitErr      error
	origin       string
	publicOrigin string
}

func accountProcessCommand(t *testing.T, binary string, driver *mysqldriver.Config, extra ...string) *accountProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	cmd := exec.Command(binary)
	env := append([]string{"PATH=" + os.Getenv("PATH")}, integrationEnvironment(driver, address)...)
	for _, setting := range append([]string{"ADMIN_PUBLIC_ORIGIN=http://" + address}, extra...) {
		key := strings.SplitN(setting, "=", 2)[0] + "="
		for i := len(env) - 1; i >= 0; i-- {
			if strings.HasPrefix(env[i], key) {
				env = append(env[:i], env[i+1:]...)
			}
		}
		env = append(env, setting)
	}
	cmd.Env = env
	p := &accountProcess{cmd: cmd, output: &synchronizedBuffer{}, done: make(chan struct{}), origin: "http://" + address}
	for _, setting := range env {
		if strings.HasPrefix(setting, "ADMIN_PUBLIC_ORIGIN=") {
			p.publicOrigin = strings.TrimPrefix(setting, "ADMIN_PUBLIC_ORIGIN=")
		}
	}
	cmd.Stdout = p.output
	cmd.Stderr = p.output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.waitErr = cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		cmd.Process.Kill()
		<-p.done
	})
	return p
}
func (p *accountProcess) ready(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-p.done:
			err := p.waitErr
			t.Fatalf("Admin exited before readiness: %v %s", err, p.output.String())
		default:
		}
		client := http.Client{Timeout: 100 * time.Millisecond}
		r, err := client.Get(p.origin + "/health/ready")
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("Admin readiness timeout: %s", p.output.String())
}
func (p *accountProcess) stop(t *testing.T) {
	t.Helper()
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// Match serveAdmin's production drain window, plus bounded process-exit
	// overhead. Pending HTTP headers can legitimately outlive a five-second wait.
	select {
	case <-p.done:
		err := p.waitErr
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(productionShutdownTimeout + 2*time.Second):
		t.Fatal("Admin did not stop gracefully")
	}
}
func (p *accountProcess) request(t *testing.T, method, path, body string, cookies []*http.Cookie, csrf string) (int, http.Header, []byte) {
	t.Helper()
	return p.requestWithKey(t, method, path, body, cookies, csrf, "")
}
func (p *accountProcess) requestWithKey(t *testing.T, method, path, body string, cookies []*http.Cookie, csrf, key string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, p.origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", p.publicOrigin)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Idempotency-Key", key)
	for _, c := range cookies {
		if c.MaxAge >= 0 {
			req.AddCookie(c)
		}
	}
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, data
}
func deliveryDB(t *testing.T, driver *mysqldriver.Config) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", driver.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func deliveryExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func deliveryCSRF(t *testing.T, data []byte) string {
	t.Helper()
	var v struct {
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v.CSRF
}
func TestAccountProcessRequiresCompleteAuthenticationSchema(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	binary := buildIntegrationAdmin(t)
	good := accountProcessCommand(t, binary, driver)
	good.ready(t)
	good.stop(t)
	faults := []struct{ name, apply, restore string }{
		{"missing_table", "RENAME TABLE rcc_preauth_credentials TO missing_preauth", "RENAME TABLE missing_preauth TO rcc_preauth_credentials"},
		{"missing_column", "ALTER TABLE rcc_accounts RENAME COLUMN password_version TO missing_version", "ALTER TABLE rcc_accounts RENAME COLUMN missing_version TO password_version"},
		{"missing_uniqueness", "ALTER TABLE rcc_accounts DROP INDEX uq_rcc_accounts_email", "ALTER TABLE rcc_accounts ADD UNIQUE KEY uq_rcc_accounts_email(email)"},
		{"wrong_collation", "ALTER TABLE rcc_accounts MODIFY username VARCHAR(32) CHARACTER SET ascii COLLATE ascii_general_ci NOT NULL", "ALTER TABLE rcc_accounts MODIFY username VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL"},
		{"missing_expiry_index", "ALTER TABLE rcc_login_sessions DROP INDEX ix_rcc_sessions_expiry", "ALTER TABLE rcc_login_sessions ADD KEY ix_rcc_sessions_expiry(expires_at)"},
		{"unexpected_invisible_uniqueness", "ALTER TABLE rcc_login_sessions ADD UNIQUE KEY wrong_invisible_account(account_id) INVISIBLE", "ALTER TABLE rcc_login_sessions DROP INDEX wrong_invisible_account"},
		{"unexpected_expiry_uniqueness", "ALTER TABLE rcc_login_sessions ADD UNIQUE KEY wrong_unique_expiry(expires_at)", "ALTER TABLE rcc_login_sessions DROP INDEX wrong_unique_expiry"},
		{"unexpected_session_uniqueness", "ALTER TABLE rcc_login_sessions ADD UNIQUE KEY wrong_unique_account(account_id)", "ALTER TABLE rcc_login_sessions DROP INDEX wrong_unique_account"},
		{"missing_foreign_key", "ALTER TABLE rcc_login_sessions DROP FOREIGN KEY fk_rcc_sessions_account", "ALTER TABLE rcc_login_sessions ADD CONSTRAINT fk_rcc_sessions_account FOREIGN KEY(account_id) REFERENCES rcc_accounts(id)"},
		{"missing_lock_row", "DELETE FROM rcc_auth_control_lock WHERE id=1", "INSERT INTO rcc_auth_control_lock(id) VALUES(1)"},
	}
	for _, fault := range faults {
		t.Run(fault.name, func(t *testing.T) {
			deliveryExec(t, db, fault.apply)
			defer deliveryExec(t, db, fault.restore)
			p := accountProcessCommand(t, binary, driver)
			select {
			case <-p.done:
				err := p.waitErr
				if err == nil || !strings.Contains(p.output.String(), "authentication schema is incomplete; apply migration 007") {
					t.Fatalf("missing safe migration diagnostic: %v %s", err, p.output.String())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Admin accepted incomplete authentication schema")
			}
		})
	}
}

func processCredentials(t *testing.T, p *accountProcess, path, body string) ([]*http.Cookie, string, []byte) {
	t.Helper()
	status, h, data := p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 200 {
		t.Fatalf("prepare: %d", status)
	}
	prepared := (&http.Response{Header: h}).Cookies()
	status, h, data = p.request(t, "POST", path, body, prepared, deliveryCSRF(t, data))
	if status != 200 && status != 201 {
		t.Fatalf("authenticate: %d", status)
	}
	return (&http.Response{Header: h}).Cookies(), deliveryCSRF(t, data), data
}
func TestAccountProcessRestartPreservesSessionsAndRateWindows(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	binary := buildIntegrationAdmin(t)
	start := func() *accountProcess {
		p := accountProcessCommand(t, binary, driver, "ADMIN_REGISTER_LIMIT=1")
		p.ready(t)
		return p
	}
	p := start()
	body := `{"username":"restart.user","email":"restart@example.com","password":"restart password long enough"}`
	valid, csrf, _ := processCredentials(t, p, "/api/v1/auth/register", body)
	script := exec.Command("python3", "../../../scripts/account-session.py", "--origin", p.origin, "--username", "restart.user", "--password-stdin")
	script.Stdin = strings.NewReader("restart password long enough")
	if output, err := script.CombinedOutput(); err != nil || !strings.Contains(string(output), "Session logged out.") {
		t.Fatalf("delivered script: %v %s", err, output)
	}
	expired, _, _ := processCredentials(t, p, "/api/v1/auth/login", `{"username":"restart.user","password":"restart password long enough"}`)
	// Expiry is explicit persisted fault setup; normal credentials came from HTTP.
	deliveryExec(t, db, "UPDATE rcc_login_sessions SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE token_hash=SHA2(?,256)", expired[0].Value)
	status, h, data := p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 200 {
		t.Fatal(status)
	}
	pre := (&http.Response{Header: h}).Cookies()
	preCSRF := deliveryCSRF(t, data)
	status, _, _ = p.request(t, "POST", "/api/v1/auth/register", body, pre, preCSRF)
	if status != 429 {
		t.Fatalf("initial registration window: %d", status)
	}
	p.stop(t)
	p = start()
	for _, test := range []struct {
		cookies []*http.Cookie
		want    int
	}{{valid, 200}, {expired, 401}, {pre, 401}} {
		status, _, _ = p.request(t, "GET", "/api/v1/table-policies", "", test.cookies, "")
		if status != test.want {
			t.Fatalf("credential after restart: got %d want %d", status, test.want)
		}
	}
	status, h, _ = p.request(t, "POST", "/api/v1/auth/register", body, pre, preCSRF)
	if status != 429 || h.Get("Retry-After") == "" {
		t.Fatalf("restart lost rate window: %d", status)
	}
	status, _, _ = p.request(t, "POST", "/api/v1/auth/activity", "", valid, csrf)
	if status != 200 {
		t.Fatalf("restart lost session CSRF: %d", status)
	}
	// Admission clears only ended records; unexpired sessions and limits survive.
	deliveryExec(t, db, "UPDATE rcc_auth_rate_limits SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND")
	status, _, _ = p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 200 {
		t.Fatal(status)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_login_sessions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("cleanup retained expired or removed valid session: %d %v", count, err)
	}
	status, _, _ = p.request(t, "GET", "/api/v1/table-policies", "", valid, "")
	if status != 200 {
		t.Fatal(status)
	}

	for _, value := range []string{strings.Repeat("A", 43), valid[0].Value + "tampered"} {
		forged := *valid[0]
		forged.Value = value
		if status, _, _ := p.request(t, "GET", "/api/v1/table-policies", "", []*http.Cookie{&forged}, ""); status != 401 {
			t.Fatalf("forged/tampered credential acquired business identity: %d", status)
		}
	}
	if status, _, _ := p.request(t, "POST", "/api/v1/auth/logout", "", valid, csrf); status != 204 {
		t.Fatal(status)
	}
	if status, _, _ := p.request(t, "GET", "/api/v1/table-policies", "", valid, ""); status != 401 {
		t.Fatal("revoked credential read business data")
	}
	fresh, _, _ := processCredentials(t, p, "/api/v1/auth/login", `{"username":"restart.user","password":"restart password long enough"}`)
	if fresh[0].Value == valid[0].Value {
		t.Fatal("login reused old session credential")
	}
	if status, _, _ := p.request(t, "GET", "/api/v1/table-policies", "", fresh, ""); status != 200 {
		t.Fatal("fresh login unavailable")
	}
	p.stop(t)
	for _, secret := range []string{"restart@example.com", "restart password long enough", valid[0].Value, csrf, preCSRF} {
		if strings.Contains(p.output.String(), secret) {
			t.Fatal("process log exposed sensitive material")
		}
	}
}
func TestAccountProcessDatabaseFailuresRemain503And504(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	p := accountProcessCommand(t, buildIntegrationAdmin(t), driver, "MYSQL_READ_TIMEOUT=1s")
	p.ready(t)
	cookies, csrf, _ := processCredentials(t, p, "/api/v1/auth/register", `{"username":"fault.user","email":"fault@example.com","password":"fault password long enough"}`)
	deliveryExec(t, db, "RENAME TABLE rcc_login_sessions TO unavailable_sessions")
	status, h, data := p.request(t, "GET", "/api/v1/table-policies", "", cookies, "")
	if status != 503 || len((&http.Response{Header: h}).Cookies()) != 0 || !strings.Contains(string(data), "auth_unavailable") {
		t.Fatalf("storage unavailable: %d %s", status, data)
	}
	deliveryExec(t, db, "RENAME TABLE unavailable_sessions TO rcc_login_sessions")
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("SELECT id FROM rcc_auth_control_lock WHERE id=1 FOR UPDATE"); err != nil {
		t.Fatal(err)
	}
	status, _, data = p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 504 || !strings.Contains(string(data), "auth_timeout") {
		t.Fatalf("storage timeout: %d %s", status, data)
	}
	tx.Rollback()
	status, _, _ = p.request(t, "POST", "/api/v1/auth/activity", "", cookies, csrf)
	if status != 200 {
		t.Fatalf("fault cleared existing session: %d", status)
	}
	p.stop(t)
}

func TestAccountUpgradeFromLegacyMatchesFreshSchema(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "testdata/001-legacy-policy-schema.sql",
		"../../../deploy/mysql/migrations/001-promote-mutation-capabilities.sql",
		"../../../deploy/mysql/migrations/002-rename-audit-timestamps.sql",
		"../../../deploy/mysql/migrations/003-create-query-policies.sql",
		"../../../deploy/mysql/migrations/004-create-mutation-policies.sql",
		"../../../deploy/mysql/migrations/005-expand-table-policy-code-references.sql",
		"../../../deploy/mysql/migrations/013-policy-audit-timestamps.sql")
	db := deliveryDB(t, driver)
	directory := t.TempDir()
	migrate := directory + "/policy-migrate"
	build := exec.Command("go", "build", "-o", migrate, "../policy-migrate")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build migrate: %v %s", err, out)
	}
	command := exec.Command(migrate)
	command.Env = append([]string{"PATH=" + os.Getenv("PATH"), "POLICY_MIGRATION_OPERATOR=historical-maintainer"}, integrationEnvironment(driver, "invalid-http-address")...)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("legacy contraction: %v %s", err, out)
	}
	ownerDriver := *driver
	ownerDriver.User = "root"
	ownerDriver.MultiStatements = true
	owner := deliveryDB(t, &ownerDriver)
	migration, err := os.ReadFile("../../../deploy/mysql/migrations/007-local-accounts.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(migration))
	// Seed the documented 007 deployment format before any role/release schema.
	// These are real preserved account/session rows, not post-upgrade registration.
	const oldID = "517c20e7-b4a8-4f7a-b562-c78b1f96e4a0"
	const oldToken = "old-upgrade-session-credential-aaaaaaaaaaaaaa"
	const oldCSRF = "old-upgrade-csrf-credential-bbbbbbbbbbbbbbbbb"
	const oldPassword = "old deployment password long enough"
	hash, err := passwordadapter.NewArgon2id().Hash(t.Context(), oldPassword)
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, `INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,enabled,password_version,session_version,created_at) VALUES(?,'legacy.account','legacy@example.com','旧账号',?,1,3,7,UTC_TIMESTAMP(6))`, oldID, hash)
	deliveryExec(t, owner, `INSERT INTO rcc_login_sessions(token_hash,account_id,csrf_hash,password_version,session_version,created_at,last_active_at,expires_at) VALUES(SHA2(?,256),?,SHA2(?,256),3,7,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)+INTERVAL 8 HOUR)`, oldToken, oldID, oldCSRF)
	deliveryExec(t, owner, `CREATE TABLE legacy_policy(id INT PRIMARY KEY,label TEXT,creator VARCHAR(64)) ENGINE=InnoDB`)
	const oldValue = "  旧配置\r\nwith tab\tand trailing  "
	deliveryExec(t, owner, `INSERT INTO legacy_policy VALUES(7,?,'legacy-maintainer')`, oldValue)
	policyQuery := `SELECT p.table_name,p.query_policy_code,p.mutation_policy_code,p.enabled,q.type_code,q.default_order_field,q.default_order_direction,q.default_page_size,q.max_page_size,q.status,m.type_code,m.allow_add,m.allow_modify,m.allow_delete,m.status FROM rcc_table_policies p JOIN rcc_query_policies q ON q.code=p.query_policy_code JOIN rcc_mutation_policies m ON m.code=p.mutation_policy_code WHERE p.table_name=?`
	preservedPolicy := schemaMetadata(t, owner, policyQuery, "legacy_policy")
	binary := buildIntegrationAdmin(t)
	applyRoleMigration(t, owner)
	rejectIncomplete := func(migration string) {
		incomplete := accountProcessCommand(t, binary, driver)
		select {
		case <-incomplete.done:
			if incomplete.waitErr == nil || !strings.Contains(incomplete.output.String(), migration) {
				t.Fatalf("incomplete control upgrade accepted or missing guidance: %v %s", incomplete.waitErr, incomplete.output.String())
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Admin served before control migrations completed")
		}
	}
	rejectIncomplete("009")
	recordVersionMigration, err := os.ReadFile("../../../deploy/mysql/migrations/009-record-versions.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(recordVersionMigration))
	releaseMigration, err := os.ReadFile("../../../deploy/mysql/migrations/010-release-drafts.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(releaseMigration))
	targetMigration, err := os.ReadFile("../../../deploy/mysql/migrations/011-release-targets.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(targetMigration))
	publicationMigration, err := os.ReadFile("../../../deploy/mysql/migrations/012-publication.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(publicationMigration))
	deliveryExec(t, owner, "RENAME TABLE rcc_refresh_notifications TO interrupted_notifications")
	rejectIncomplete("014")
	deliveryExec(t, owner, "RENAME TABLE interrupted_notifications TO rcc_refresh_notifications")
	// Existing technical records may contain multiple old orders for one table.
	// The longest supported old order ID must fit the migration identity too.
	for index, id := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32)} {
		deliveryExec(t, owner, `INSERT INTO rcc_publication_commands(table_name,sequence,order_id,document) VALUES('legacy_policy',?,?,JSON_OBJECT('legacy','unchanged command','sequence',?))`, index+1, id, index+1)
		deliveryExec(t, owner, `INSERT INTO rcc_refresh_notifications(order_id,table_name,table_version,document) VALUES(?,'legacy_policy',?,JSON_OBJECT('id',?,'status','NOT_CONNECTED'))`, id, index+1, id)
	}
	technicalQueries := []string{`SELECT table_name,sequence,order_id,document FROM rcc_publication_commands WHERE table_name=? ORDER BY sequence`, `SELECT order_id,table_name,table_version,document FROM rcc_refresh_notifications WHERE table_name=? ORDER BY order_id`}
	preservedTechnical := []string{schemaMetadata(t, owner, technicalQueries[0], "legacy_policy"), schemaMetadata(t, owner, technicalQueries[1], "legacy_policy")}
	executionMigration, err := os.ReadFile("../../../deploy/mysql/migrations/014-original-order-executions.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(executionMigration))
	deliveryExec(t, owner, string(executionMigration))
	for index, query := range technicalQueries {
		if schemaMetadata(t, owner, query, "legacy_policy") != preservedTechnical[index] {
			t.Fatal("migration rewrote or removed old technical records")
		}
	}
	for _, table := range []string{"rcc_publication_commands", "rcc_refresh_notifications"} {
		var count int
		if err := owner.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE execution_id=CONCAT('legacy:',order_id) AND LENGTH(execution_id)=39").Scan(&count); err != nil || count != 2 {
			t.Fatalf("legacy technical identities: %s count=%d error=%v", table, count, err)
		}
	}

	rejectIncomplete("015")
	reservationMigration, err := os.ReadFile("../../../deploy/mysql/migrations/015-draft-target-reservations.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, owner, string(reservationMigration))
	deliveryExec(t, owner, string(reservationMigration))

	deliveryExec(t, owner, "CREATE DATABASE fresh_accounts CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
	freshDriver := ownerDriver
	freshDriver.DBName = "fresh_accounts"
	fresh := deliveryDB(t, &freshDriver)
	schema, err := os.ReadFile("../../../deploy/mysql/init/001-schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	deliveryExec(t, fresh, string(schema))
	for _, query := range []string{
		`SELECT TABLE_NAME,COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COALESCE(COLUMN_DEFAULT,'<null>'),COALESCE(COLLATION_NAME,''),EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('rcc_accounts','rcc_login_sessions','rcc_preauth_credentials','rcc_auth_rate_limits','rcc_auth_control_lock','rcc_account_role_history','rcc_record_versions','rcc_release_orders','rcc_release_requests','rcc_release_details','rcc_release_executions','rcc_release_targets','rcc_release_table_references','rcc_table_publications','rcc_publication_commands','rcc_refresh_notifications') ORDER BY TABLE_NAME,ORDINAL_POSITION`,
		`SELECT TABLE_NAME,INDEX_NAME,NON_UNIQUE,SEQ_IN_INDEX,COLUMN_NAME,COALESCE(SUB_PART,0) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=? AND TABLE_NAME IN ('rcc_accounts','rcc_login_sessions','rcc_preauth_credentials','rcc_auth_rate_limits','rcc_auth_control_lock','rcc_account_role_history','rcc_record_versions','rcc_release_orders','rcc_release_requests','rcc_release_details','rcc_release_executions','rcc_release_targets','rcc_release_table_references','rcc_table_publications','rcc_publication_commands','rcc_refresh_notifications') ORDER BY TABLE_NAME,INDEX_NAME,SEQ_IN_INDEX`,
		`SELECT TABLE_NAME,COLUMN_NAME,REFERENCED_TABLE_NAME,REFERENCED_COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA=? AND REFERENCED_TABLE_NAME IS NOT NULL ORDER BY TABLE_NAME,COLUMN_NAME`,
	} {
		upgraded := schemaMetadata(t, owner, query, driver.DBName)
		installed := schemaMetadata(t, owner, query, "fresh_accounts")
		if upgraded != installed || upgraded == "" {
			t.Fatalf("fresh/upgrade metadata mismatch:\n%s\n%s", upgraded, installed)
		}
	}
	p := accountProcessCommand(t, binary, driver)
	p.ready(t)
	oldCookies := []*http.Cookie{{Name: "rcc-session-dev", Value: oldToken}}
	status, _, current := p.request(t, "GET", "/api/v1/auth/session", "", oldCookies, "")
	if status != 200 || !strings.Contains(string(current), oldID) || !strings.Contains(string(current), `"roles":["VIEWER"]`) {
		t.Fatalf("old session/identity/default role: %d %s", status, current)
	}
	if status, _, _ := p.request(t, "POST", "/api/v1/query-policies", `{}`, oldCookies, oldCSRF); status != 403 {
		t.Fatalf("old default viewer wrote catalog: %d", status)
	}
	_, _, login := processCredentials(t, p, "/api/v1/auth/login", `{"username":"legacy.account","password":"`+oldPassword+`"}`)
	if !strings.Contains(string(login), oldID) {
		t.Fatal("old credentials changed identity")
	}
	status, _, dataBefore := p.request(t, "POST", "/api/v1/tables/legacy_policy/query", `{}`, oldCookies, oldCSRF)
	var queryResult struct {
		Rows           []map[string]*string `json:"rows"`
		RecordVersions []string             `json:"record_versions"`
	}
	if status != 200 || json.Unmarshal(dataBefore, &queryResult) != nil || len(queryResult.Rows) != 1 || queryResult.Rows[0]["label"] == nil || *queryResult.Rows[0]["label"] != oldValue || *queryResult.Rows[0]["creator"] != "legacy-maintainer" {
		t.Fatalf("upgraded business bytes: %d %s", status, dataBefore)
	}
	if len(queryResult.RecordVersions) != 1 || queryResult.RecordVersions[0] != "0" {
		t.Fatalf("migration fabricated record versions: %s", dataBefore)
	}
	if after := schemaMetadata(t, owner, policyQuery, "legacy_policy"); after != preservedPolicy {
		t.Fatal("upgrade changed existing rule assignment or semantics")
	}
	for _, table := range []string{"rcc_record_versions", "rcc_release_orders", "rcc_publication_commands", "rcc_refresh_notifications", "rcc_account_role_history"} {
		var count int
		expected := 0
		if table == "rcc_publication_commands" || table == "rcc_refresh_notifications" {
			expected = 2
		}
		if err := owner.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != expected {
			t.Fatalf("migration changed %s count: %d %v", table, count, err)
		}
	}
	cookies, _, data := processCredentials(t, p, "/api/v1/auth/register", `{"username":"upgraded.user","email":"upgraded@example.com","password":"upgraded password long enough"}`)
	if status, _, _ := p.request(t, "GET", "/api/v1/table-policies", "", cookies, ""); status != 200 {
		t.Fatal(status)
	}
	p.stop(t)
	// Account maintenance must remain usable when normal Policy Catalog readiness fails.
	deliveryExec(t, db, "RENAME TABLE rcc_query_policies TO unavailable_query_policies")
	maintain := directory + "/account-maintain"
	build = exec.Command("go", "build", "-o", maintain, "../account-maintain")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build maintain: %v %s", err, out)
	}
	bootstrap := exec.Command(maintain, "grant-admin", "--id", oldID)
	bootstrap.Env = append([]string{"PATH=" + os.Getenv("PATH")}, integrationEnvironment(driver, "invalid-http-address")...)
	if out, err := bootstrap.CombinedOutput(); err != nil {
		t.Fatalf("explicit first administrator: %v %s", err, out)
	}
	deliveryExec(t, db, "RENAME TABLE unavailable_query_policies TO rcc_query_policies")
	p = accountProcessCommand(t, binary, driver)
	p.ready(t)
	if status, _, _ := p.request(t, "GET", "/api/v1/account-roles", "", oldCookies, ""); status != 200 {
		t.Fatalf("selected first administrator unavailable: %d", status)
	}
	if status, _, _ := p.request(t, "GET", "/api/v1/account-roles", "", cookies, ""); status != 403 {
		t.Fatalf("bootstrap granted another account: %d", status)
	}
	if status, _, body := p.request(t, "POST", "/api/v1/tables/legacy_policy/rows", `{"content":{"id":"8","label":"old-client"}}`, oldCookies, oldCSRF); status != 404 || !strings.Contains(string(body), "route_not_found") {
		t.Fatalf("old writer bypass: %d %s", status, body)
	}
	p.stop(t)
	deliveryExec(t, db, "RENAME TABLE rcc_query_policies TO unavailable_query_policies")
	command = exec.Command(maintain, "lookup", "--username", "upgraded.user")
	command.Env = append([]string{"PATH=" + os.Getenv("PATH")}, integrationEnvironment(driver, "invalid-http-address")...)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("upgraded maintenance independent readiness: %v %s", err, out)
	}
	var identity struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal(data, &identity); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), identity.Account.ID) {
		t.Fatal("maintenance lookup changed migrated identity")
	}
}
func schemaMetadata(t *testing.T, db *sql.DB, query, schema string) string {
	t.Helper()
	rows, err := db.Query(query, schema)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result strings.Builder
	for rows.Next() {
		values := make([]string, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		result.WriteString(strings.Join(values, "|"))
		result.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result.String()
}

func TestAccountProcessHTTPSCookiesAndSensitiveMaterials(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	proxy := httptest.NewUnstartedServer(nil)
	proxy.StartTLS()
	defer proxy.Close()
	p := accountProcessCommand(t, buildIntegrationAdmin(t), driver, "ADMIN_PUBLIC_ORIGIN="+proxy.URL, "ADMIN_ALLOW_LOCAL_HTTP=false")
	p.ready(t)
	target, err := url.Parse(p.origin)
	if err != nil {
		t.Fatal(err)
	}
	proxy.Config.Handler = httputil.NewSingleHostReverseProxy(target)
	request := func(method, path, body string, cookies []*http.Cookie, csrf string) (int, []*http.Cookie, []byte) {
		req, err := http.NewRequest(method, proxy.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", proxy.URL)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrf)
		for _, c := range cookies {
			if c.MaxAge >= 0 {
				req.AddCookie(c)
			}
		}
		res, err := proxy.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, res.Cookies(), data
	}
	status, pre, data := request("GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 200 {
		t.Fatal(status)
	}
	csrf := deliveryCSRF(t, data)
	status, session, data := request("POST", "/api/v1/auth/register", `{"username":"secure.user","email":"secure@example.com","password":"secure password long enough"}`, pre, csrf)
	if status != 201 {
		t.Fatalf("HTTPS register: %d", status)
	}
	for _, group := range []struct {
		cookies []*http.Cookie
		name    string
	}{{pre, "__Host-rcc-preauth"}, {session, "__Host-rcc-session"}} {
		found := false
		for _, c := range group.cookies {
			if c.MaxAge < 0 {
				continue
			}
			found = true
			if c.Name != group.name || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" {
				t.Fatal("HTTPS Cookie contract")
			}
		}
		if !found {
			t.Fatal("missing HTTPS Cookie")
		}
	}
	if status, _, _ := request("GET", "/api/v1/table-policies", "", session, ""); status != 200 {
		t.Fatal(status)
	}
	sessionCSRF := deliveryCSRF(t, data)
	if status, _, _ := request("POST", "/api/v1/auth/logout", "", session, sessionCSRF); status != 204 {
		t.Fatal(status)
	}
	if status, _, _ := request("GET", "/api/v1/table-policies", "", session, ""); status != 401 {
		t.Fatal(status)
	}
	p.stop(t)
	secrets := []string{"secure@example.com", "secure password long enough", csrf, sessionCSRF}
	for _, c := range append(pre, session...) {
		if c.Value != "" {
			secrets = append(secrets, c.Value)
		}
	}
	for _, secret := range secrets {
		if strings.Contains(p.output.String(), secret) {
			t.Fatal("HTTPS process logs disclosed secret")
		}
	}
}

func TestAccountProcessCapacityNeverEvictsValidState(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")
	db := deliveryDB(t, driver)
	p := accountProcessCommand(t, buildIntegrationAdmin(t), driver)
	p.ready(t)
	cookies, _, _ := processCredentials(t, p, "/api/v1/auth/register", `{"username":"capacity.user","email":"capacity@example.com","password":"capacity password long enough"}`)
	// Fill the bounded pre-login pool using test-owned synthetic state, retaining
	// the independently registered valid session and unfinished rate windows.
	deliveryExec(t, db, `INSERT INTO rcc_preauth_credentials(token_hash,csrf_hash,expires_at) WITH RECURSIVE seq(n) AS(SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<10000) SELECT /*+ SET_VAR(cte_max_recursion_depth=10001) */ SHA2(CONCAT('capacity',n),256),SHA2(CONCAT('csrf',n),256),UTC_TIMESTAMP(6)+INTERVAL 10 MINUTE FROM seq`)
	status, _, _ := p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 503 {
		t.Fatalf("full preauth pool: %d", status)
	}
	if status, _, _ := p.request(t, "GET", "/api/v1/table-policies", "", cookies, ""); status != 200 {
		t.Fatalf("capacity evicted session: %d", status)
	}
	var before int
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_auth_rate_limits").Scan(&before); err != nil || before == 0 {
		t.Fatal("missing rate state")
	}
	deliveryExec(t, db, "UPDATE rcc_preauth_credentials SET expires_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND")
	status, _, _ = p.request(t, "GET", "/api/v1/auth/csrf", "", nil, "")
	if status != 200 {
		t.Fatalf("expired capacity not reclaimed: %d", status)
	}
	var remaining, after int
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_preauth_credentials").Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("bounded cleanup: %d %v", remaining, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_auth_rate_limits").Scan(&after); err != nil || after != before {
		t.Fatal("cleanup changed unfinished rate windows")
	}
	p.stop(t)
}
