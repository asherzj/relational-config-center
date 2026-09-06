package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAdminProcessRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	binaryPath := buildAdminProcess(t)

	process := exec.Command(binaryPath)
	process.Env = []string{"PATH=" + os.Getenv("PATH"), "ADMIN_API_TOKEN=process-token"}
	output, err := process.CombinedOutput()
	if err == nil {
		t.Fatalf("expected invalid configuration to stop startup, got success with output %q", output)
	}
	if !strings.Contains(string(output), "configuration error: MYSQL_HOST is required") {
		t.Fatalf("expected a safe configuration error, got %q", output)
	}
}

func TestAdminProcessRejectsUnavailableRequiredDatabase(t *testing.T) {
	t.Parallel()

	process := exec.Command(buildAdminProcess(t))
	process.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"ADMIN_HTTP_ADDR=127.0.0.1:0",
		"ADMIN_API_TOKEN=process-token",
		"ADMIN_PUBLIC_ORIGIN=https://config.example.test",
		"MYSQL_HOST=127.0.0.1",
		"MYSQL_PORT=1",
		"MYSQL_DATABASE=rcc_test",
		"MYSQL_USER=rcc_admin",
		"MYSQL_PASSWORD=rcc_password",
		"MYSQL_TLS_MODE=false",
		"MYSQL_CONNECT_TIMEOUT=250ms",
		"MYSQL_READ_TIMEOUT=250ms",
		"MYSQL_WRITE_TIMEOUT=250ms",
	}
	output, err := process.CombinedOutput()
	if err == nil {
		t.Fatalf("expected unavailable MySQL to stop startup, got success with output %q", output)
	}
	if !strings.Contains(string(output), "startup error: Managed Data Source is unavailable") {
		t.Fatalf("expected a safe startup error, got %q", output)
	}
	if strings.Contains(string(output), "rcc_password") {
		t.Fatalf("startup error exposed database credentials: %q", output)
	}
}

func TestAdminProcessRejectsMissingAuthenticationToken(t *testing.T) {
	t.Parallel()

	process := exec.Command(buildAdminProcess(t))
	process.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"ADMIN_HTTP_ADDR=127.0.0.1:0",
		"MYSQL_HOST=127.0.0.1",
		"MYSQL_PORT=1",
		"MYSQL_DATABASE=rcc_test",
		"MYSQL_USER=rcc_admin",
		"MYSQL_PASSWORD=database-secret",
		"MYSQL_TLS_MODE=false",
	}
	output, err := process.CombinedOutput()
	if err == nil {
		t.Fatalf("expected missing authentication token to stop startup, got success with output %q", output)
	}
	if !strings.Contains(string(output), "configuration error: ADMIN_API_TOKEN is required") {
		t.Fatalf("expected a safe authentication configuration error, got %q", output)
	}
	if strings.Contains(string(output), "database-secret") {
		t.Fatalf("configuration error exposed database credentials: %q", output)
	}
}

func buildAdminProcess(t *testing.T) string {
	t.Helper()

	binaryName := "admin"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)

	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build admin process: %v\n%s", err, output)
	}
	return binaryPath
}
