//go:build integration && browser

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestReleaseFlowBrowserSystemPath exercises per-table saved instances through
// a real Chromium session, same-origin Vite proxy, Admin process, and MySQL.
func TestReleaseFlowBrowserSystemPath(t *testing.T) {
	_, driver := startCurrentIntegrationMySQL(t, "testdata/003-policy-fixture.sql")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	_, port, _ := net.SplitHostPort(address)
	origin := "http://" + address
	admin := accountProcessCommand(t, buildIntegrationAdmin(t), driver, "ADMIN_PUBLIC_ORIGIN="+origin, "ADMIN_REGISTER_LIMIT=20")
	admin.ready(t)

	_, _, identity := processCredentials(t, admin, "/api/v1/auth/register", `{"username":"template.browser.admin","email":"template.browser.admin@example.invalid","password":"template browser password long enough"}`)
	var registered struct {
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal(identity, &registered); err != nil {
		t.Fatal(err)
	}
	maintenance := filepath.Join(t.TempDir(), "account-maintain")
	build := exec.Command("go", "build", "-o", maintenance, "../account-maintain")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build maintenance: %v %s", err, output)
	}
	grant := exec.Command(maintenance, "grant-admin", "--id", registered.Account.ID)
	grant.Env = append(os.Environ(), integrationEnvironment(driver, "invalid-http-address")...)
	if output, err := grant.CombinedOutput(); err != nil {
		t.Fatalf("grant browser administrator: %v %s", err, output)
	}

	web, err := filepath.Abs("../../../web")
	if err != nil {
		t.Fatal(err)
	}
	proxy := exec.Command("node", filepath.Join(web, "node_modules/vite/bin/vite.js"), "--host", "127.0.0.1", "--port", port, "--strictPort")
	proxy.Dir = web
	proxy.Env = append(os.Environ(), "RCC_ADMIN_URL="+admin.origin)
	proxyOutput := &synchronizedBuffer{}
	proxy.Stdout, proxy.Stderr = proxyOutput, proxyOutput
	if err := proxy.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Process.Kill(); _, _ = proxy.Process.Wait() })
	deadline := time.Now().Add(15 * time.Second)
	for {
		response, requestErr := (&http.Client{Timeout: time.Second}).Get(origin)
		if requestErr == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("same-origin Vite proxy unavailable: %s", proxyOutput.String())
		}
		time.Sleep(50 * time.Millisecond)
	}

	outputDir := t.TempDir()
	if configured := os.Getenv("RCC_E2E_OUTPUT"); configured != "" {
		outputDir = configured
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	browser := exec.Command("node", filepath.Join(web, "e2e/release-instances.cjs"))
	browser.Dir = web
	browser.Env = append(os.Environ(), "RCC_WEB_URL="+origin, "RCC_E2E_OUTPUT="+outputDir)
	result, err := browser.CombinedOutput()
	if err != nil {
		t.Fatalf("release flow browser path: %v %s", err, result)
	}
	var evidence struct {
		Code   string   `json:"code"`
		Checks []string `json:"checks"`
	}
	if err := json.Unmarshal(result, &evidence); err != nil {
		t.Fatalf("browser evidence: %v %s", err, result)
	}

	t.Logf("browser → Vite proxy → Admin → MySQL: %v", evidence.Checks)
}
