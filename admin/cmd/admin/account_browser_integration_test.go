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
	_, driver := startIntegrationMySQLWithRequirement(t, true, "../../../deploy/mysql/init/001-schema.sql", localManagedTableFixture)
	db := deliveryDB(t, driver)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	_, port, _ := net.SplitHostPort(address)
	origin := "http://" + address
	admin := accountProcessCommand(t, buildIntegrationAdmin(t), driver, "ADMIN_PUBLIC_ORIGIN="+origin)
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
	browser.Env = append(os.Environ(), "RCC_E2E_ORIGIN="+origin)
	result, err := browser.CombinedOutput()
	if err != nil {
		t.Fatalf("browser system path: %v %s", err, result)
	}
	var evidence struct {
		AccountID   string   `json:"account_id"`
		TemplateKey string   `json:"template_key"`
		Checks      []string `json:"checks"`
	}
	if err := json.Unmarshal(result, &evidence); err != nil {
		t.Fatalf("browser evidence: %v %s", err, result)
	}
	var creator, modifier, body string
	if err := db.QueryRow("SELECT creator,modifier,body FROM notification_templates WHERE template_key=?", evidence.TemplateKey).Scan(&creator, &modifier, &body); err != nil {
		t.Fatal(err)
	}
	if len(evidence.AccountID) != 36 || creator != evidence.AccountID || modifier != evidence.AccountID || body != "browser system configuration" {
		t.Fatal("real database Operator/content did not match browser account")
	}
	admin.stop(t)
	for _, secret := range []string{"browser.secret@example.com", "browser password long enough", "browser system configuration"} {
		if strings.Contains(admin.output.String(), secret) || strings.Contains(output.String(), secret) {
			t.Fatal("process/proxy log exposed sensitive material")
		}
	}
	t.Logf("browser → Vite same-origin proxy → Admin → MySQL: %s; creator/modifier match current Account ID", strings.Join(evidence.Checks, ", "))
}
