//go:build integration && browser

package main

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestNotificationCenterBrowserSystemPath(t *testing.T) {
	runNotificationBrowserSystemPath(t, "notification-center.cjs")
}
func TestApprovalNotificationsBrowserSystemPath(t *testing.T) {
	runNotificationBrowserSystemPath(t, "approval-notifications.cjs")
}
func runNotificationBrowserSystemPath(t *testing.T, script string) {
	_, driver := startCurrentIntegrationMySQL(t, localManagedTableFixture, "testdata/016-multitable-browser.sql")
	runNotificationBrowserWithMySQL(t, driver, script)
}

func runNotificationBrowserWithMySQL(t *testing.T, driver *mysqldriver.Config, script string) {
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
	browser := exec.Command("node", filepath.Join(web, "e2e", script))
	browser.Dir = web
	browser.Env = append(fixtureEnvironment, "RCC_WEB_URL="+origin)
	result, err := browser.CombinedOutput()
	if err != nil {
		t.Fatalf("browser system path: %v %s", err, result)
	}
	t.Logf("notification center browser: %s", result)
}
