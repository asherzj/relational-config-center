//go:build integration && browser

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This intentionally separate system target requires installed Web dependencies
// and Chromium. It fails (never skips) if that browser environment is unavailable.
func TestAccountBrowserSystemPath(t *testing.T) {
	_, driver := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql", localManagedTableFixture, "../../../docs/verification/fixtures/stage1_acceptance.sql", "testdata/014-batch-browser.sql")
	db := deliveryDB(t, driver)
	maintenance := filepath.Join(t.TempDir(), "account-maintain")
	build := exec.Command("go", "build", "-o", maintenance, "../account-maintain")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build maintenance: %v %s", err, output)
	}
	fixtureEnvironment := append(os.Environ(), integrationEnvironment(driver, "invalid-http-address")...)
	fixtureEnvironment = append(fixtureEnvironment, "RCC_ACCOUNT_MAINTAIN="+maintenance)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	_, port, _ := net.SplitHostPort(address)
	origin := "http://" + address
	// The combined browser scripts register more than ten independent actors.
	admin := accountProcessCommand(t, buildIntegrationAdmin(t), driver, "ADMIN_PUBLIC_ORIGIN="+origin, "ADMIN_REGISTER_LIMIT=20")
	admin.ready(t)
	web, err := filepath.Abs("../../../web")
	if err != nil {
		t.Fatal(err)
	}
	proxy := exec.Command("node", filepath.Join(web, "node_modules/vite/bin/vite.js"), "--host", "127.0.0.1", "--port", port, "--strictPort")
	proxy.Dir = web
	proxy.Env = append(os.Environ(), "RCC_ADMIN_URL="+admin.origin)
	output := &synchronizedBuffer{}
	proxy.Stdout = output
	proxy.Stderr = output
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proxy.Process.Kill(); proxy.Wait() })
	deadline := time.Now().Add(15 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		r, err := (&http.Client{Timeout: time.Second}).Get(origin)
		if err == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("same-origin Vite proxy unavailable: %s", output.String())
	}
	browser := exec.Command("node", filepath.Join(web, "e2e/accounts.mjs"))
	browser.Dir = web
	browser.Env = append(fixtureEnvironment, "RCC_E2E_ORIGIN="+origin)
	result, err := browser.CombinedOutput()
	if err != nil {
		t.Fatalf("browser system path: %v %s", err, result)
	}
	var evidence struct {
		RunSuffix   string   `json:"run_suffix"`
		AccountID   string   `json:"account_id"`
		TemplateKey string   `json:"template_key"`
		Checks      []string `json:"checks"`
	}
	if err := json.Unmarshal(result, &evidence); err != nil {
		t.Fatalf("browser evidence: %v %s", err, result)
	}
	if len(evidence.RunSuffix) != 12 || strings.Trim(evidence.RunSuffix, "0123456789abcdef") != "" {
		t.Fatal("browser evidence omitted its unique account fixture suffix")
	}
	var creator, modifier, body string
	if err := db.QueryRow("SELECT creator,modifier,body FROM notification_templates WHERE template_key=?", evidence.TemplateKey).Scan(&creator, &modifier, &body); err != nil {
		t.Fatal(err)
	}
	if len(evidence.AccountID) != 36 || creator != evidence.AccountID || modifier != evidence.AccountID || body != "browser system configuration" {
		t.Fatal("real database Operator/content did not match browser account")
	}
	prepareManagementBrowserPolicies(t, admin, maintenance, fixtureEnvironment)
	// The shared rollback fixture references the mutation policy created above.
	rollbackFixture, err := os.ReadFile(filepath.Join(web, "e2e/fixtures/release-rollbacks.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(rollbackFixture), ";") {
		if strings.TrimSpace(statement) != "" {
			deliveryExec(t, db, statement)
		}
	}
	for _, script := range []string{"unsaved-changes.cjs", "rule-clarity.cjs", "release-drafts.cjs", "release-approvals.cjs", "release-batches.cjs", "release-rollbacks.cjs"} {
		t.Run(script, func(t *testing.T) {
			command := exec.Command("node", filepath.Join(web, "e2e", script))
			command.Dir = web
			output := t.TempDir()
			if root := os.Getenv("RCC_E2E_OUTPUT"); root != "" {
				output = filepath.Join(root, script)
				if err := os.MkdirAll(output, 0755); err != nil {
					t.Fatal(err)
				}
			}
			command.Env = append(fixtureEnvironment, "RCC_WEB_URL="+origin, "RCC_E2E_OUTPUT="+output)
			result, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("authenticated management acceptance %s: %v %s", script, err, result)
			}
			t.Logf("%s: %s", script, result)
		})
	}
	var remainingDrafts, fixtureRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM rcc_query_policies WHERE code LIKE 'stage2_unsaved_query_%'").Scan(&remainingDrafts); err != nil || remainingDrafts != 0 {
		t.Fatalf("management acceptance left disposable rule drafts: count=%d err=%v", remainingDrafts, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM stage1_acceptance_items").Scan(&fixtureRows); err != nil || fixtureRows != 5 {
		t.Fatalf("invalid management write changed the fixture: count=%d err=%v", fixtureRows, err)
	}
	admin.stop(t)
	for _, secret := range []string{"browser." + evidence.RunSuffix + "@example.invalid", "roles." + evidence.RunSuffix + "@example.invalid", "browser password long enough", "browser system configuration"} {
		if strings.Contains(admin.output.String(), secret) || strings.Contains(output.String(), secret) {
			t.Fatal("process/proxy log exposed sensitive material")
		}
	}
	t.Logf("browser → Vite same-origin proxy → Admin → MySQL: %s; creator/modifier match current Account ID", strings.Join(evidence.Checks, ", "))
}

func prepareManagementBrowserPolicies(t *testing.T, admin *accountProcess, maintenance string, environment []string) {
	t.Helper()
	cookies, csrf, identity := processCredentials(t, admin, "/api/v1/auth/register", `{"username":"browser.setup","email":"browser.setup@example.invalid","password":"browser setup password long enough"}`)

	var account struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal(identity, &account); err != nil {
		t.Fatal(err)
	}
	grant := exec.Command(maintenance, "grant-admin", "--id", account.Account.ID)
	grant.Env = environment
	if output, err := grant.CombinedOutput(); err != nil {
		t.Fatalf("grant browser setup administrator: %v %s", err, output)
	}
	for _, request := range []struct {
		path string
		body string
		want int
	}{
		{"/api/v1/mutation-policies", `{"code":"stage1_mutation_v1","name":"Browser acceptance mutation","description":"Isolated fixture","type_code":"single_table_mutation","allow_add":true,"allow_modify":true,"allow_delete":true,"create_operator_field":"created_by","create_time_field":"created_at","modify_operator_field":"updated_by","modify_time_field":"updated_at"}`, http.StatusCreated},
		{"/api/v1/mutation-policies/stage1_mutation_v1/activate", "", http.StatusOK},
		{"/api/v1/table-policies", `{"table_name":"stage1_acceptance_items","query_policy_code":"notification_page_query_v1","mutation_policy_code":"stage1_mutation_v1"}`, http.StatusCreated},
		{"/api/v1/table-policies/stage1_acceptance_items/enable", "", http.StatusOK},
		{"/api/v1/mutation-policies/stage1_mutation_v1/deprecate", "", http.StatusOK},
	} {
		status, _, _ := admin.request(t, http.MethodPost, request.path, request.body, cookies, csrf)
		if status != request.want {
			t.Fatalf("prepare management acceptance %s: status=%d want=%d", request.path, status, request.want)
		}
	}
}
