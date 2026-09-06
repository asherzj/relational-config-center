//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func TestDatabaseTableListDiscoversOnlyOrdinaryBaseTables(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/002-discovery-fixture.sql",
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/database-tables", nil)
	app.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Tables []struct {
			TableName             string  `json:"table_name"`
			TableComment          string  `json:"table_comment"`
			PolicyExists          bool    `json:"policy_exists"`
			PolicyEnabled         bool    `json:"policy_enabled"`
			Compatible            bool    `json:"compatible"`
			IncompatibilityReason *string `json:"incompatibility_reason"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(response.Tables) != 5 {
		t.Fatalf("expected five ordinary base tables, got %#v", response.Tables)
	}

	expectedNames := []string{"composite_key", "managed_alpha", "missing_primary_key", "uppercase_id", "wrong_primary_key"}
	for index, expectedName := range expectedNames {
		if response.Tables[index].TableName != expectedName {
			t.Fatalf("expected sorted table %q at index %d, got %#v", expectedName, index, response.Tables)
		}
	}

	alpha := response.Tables[1]
	if alpha.TableComment != "Alpha configuration" || !alpha.PolicyExists || !alpha.PolicyEnabled || !alpha.Compatible || alpha.IncompatibilityReason != nil {
		t.Fatalf("unexpected compatible table metadata: %#v", alpha)
	}

	assertReason(t, response.Tables[0].IncompatibilityReason, "composite_primary_key")
	assertReason(t, response.Tables[2].IncompatibilityReason, "missing_primary_key")
	assertReason(t, response.Tables[3].IncompatibilityReason, "primary_key_must_be_id")
	assertReason(t, response.Tables[4].IncompatibilityReason, "primary_key_must_be_id")
	if response.Tables[2].PolicyExists || response.Tables[2].PolicyEnabled {
		t.Fatalf("table without a Policy reported Policy state: %#v", response.Tables[2])
	}
}

func TestDatabaseTableDetailReturnsLiveMetadataAndStableNotFoundError(t *testing.T) {
	app := startIntegrationApplication(t,
		"../../../deploy/mysql/init/001-schema.sql",
		"testdata/002-discovery-fixture.sql",
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/database-tables/managed_alpha", nil)
	app.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	const expected = `{"table_name":"managed_alpha","table_comment":"Alpha configuration","policy_exists":true,"policy_enabled":true,"compatible":true,"incompatibility_reason":null}`
	if strings.TrimSpace(recorder.Body.String()) != expected {
		t.Fatalf("unexpected table detail: %s", recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/v1/database-tables/does_not_exist", nil)
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected HTTP 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var notFound struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &notFound); err != nil {
		t.Fatalf("decode not-found error: %v", err)
	}
	if notFound.Error.Code != "database_table_not_found" || notFound.Error.Message != "database table not found" || notFound.Error.RequestID == "" || recorder.Header().Get("X-Request-ID") != notFound.Error.RequestID {
		t.Fatalf("unexpected not-found error: %s", recorder.Body.String())
	}
}

func TestHealthDistinguishesRunningProcessFromRequiredInfrastructure(t *testing.T) {
	app := startIntegrationApplication(t, "../../../deploy/mysql/init/001-schema.sql")

	assertHealth(t, app, "/health/live", http.StatusOK, `{"status":"live"}`)
	assertHealth(t, app, "/health/ready", http.StatusOK, `{"status":"ready"}`)

	if err := app.Close(); err != nil {
		t.Fatalf("make Managed Data Source unavailable: %v", err)
	}
	assertHealth(t, app, "/health/live", http.StatusOK, `{"status":"live"}`)
	assertHealth(t, app, "/health/ready", http.StatusServiceUnavailable, `{"status":"not_ready"}`)
}

func TestStartupRequiresTheProtectedPolicyCatalog(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t)

	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err == nil {
		_ = app.Close()
		t.Fatal("expected startup to fail when the Policy Catalog is absent")
	}
	if !strings.Contains(err.Error(), "Policy Catalog unavailable") {
		t.Fatalf("expected Policy Catalog startup error, got %v", err)
	}
}

func TestAdminProcessServesUnauthenticatedLiveness(t *testing.T) {
	ctx, driverConfig := startIntegrationMySQL(t, "../../../deploy/mysql/init/001-schema.sql")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Admin HTTP address: %v", err)
	}
	httpAddress := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release Admin HTTP address: %v", err)
	}

	process := exec.Command(buildIntegrationAdmin(t))
	process.Env = append([]string{"PATH=" + os.Getenv("PATH")}, integrationEnvironment(driverConfig, httpAddress)...)
	var output bytes.Buffer
	process.Stdout = &output
	process.Stderr = &output
	if err := process.Start(); err != nil {
		t.Fatalf("start Admin process: %v", err)
	}
	t.Cleanup(func() {
		if process.ProcessState == nil || !process.ProcessState.Exited() {
			_ = process.Process.Kill()
			_, _ = process.Process.Wait()
		}
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+httpAddress+"/health/live", nil)
		if err != nil {
			t.Fatalf("build liveness request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if process.ProcessState != nil && process.ProcessState.Exited() {
			t.Fatalf("Admin exited before serving liveness: %s", output.String())
		}
		if time.Now().After(deadline) {
			t.Fatalf("Admin did not serve liveness: %s", output.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func assertReason(t *testing.T, actual *string, expected string) {
	t.Helper()
	if actual == nil || *actual != expected {
		t.Fatalf("expected incompatibility reason %q, got %v", expected, actual)
	}
}

func assertHealth(t *testing.T, app *adminApplication, path string, status int, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	app.Handler().ServeHTTP(recorder, request)
	if recorder.Code != status || strings.TrimSpace(recorder.Body.String()) != body {
		t.Fatalf("GET %s: expected %d %s, got %d %s", path, status, body, recorder.Code, recorder.Body.String())
	}
}

func startIntegrationApplication(t *testing.T, scripts ...string) *adminApplication {
	t.Helper()
	ctx, driverConfig := startIntegrationMySQL(t, scripts...)
	app, err := newApplication(ctx, integrationConfig(driverConfig))
	if err != nil {
		t.Fatalf("start Admin: %v", err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func startIntegrationMySQL(t *testing.T, scripts ...string) (context.Context, *mysqldriver.Config) {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	options := []testcontainers.ContainerCustomizer{
		tcmysql.WithDatabase("rcc_test"),
		tcmysql.WithUsername("rcc_admin"),
		tcmysql.WithPassword("rcc_password"),
	}
	if len(scripts) > 0 {
		options = append(options, tcmysql.WithScripts(scripts...))
	}
	container, err := tcmysql.Run(ctx, "mysql:8.4", options...)
	if err != nil {
		t.Fatalf("start MySQL 8.4 Testcontainer: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			t.Errorf("terminate MySQL container: %v", err)
		}
	})

	driverConfig, err := mysqldriver.ParseDSN(container.MustConnectionString(ctx, "parseTime=true"))
	if err != nil {
		t.Fatalf("parse Testcontainer connection string: %v", err)
	}
	return ctx, driverConfig
}

func integrationConfig(driverConfig *mysqldriver.Config) config.Config {
	return config.Config{
		AccountPublicOrigin: "http://127.0.0.1:5173",
		AccountInsecureHTTP: true,
		HTTPAddr:            "127.0.0.1:0",
		AuthDisabled:        true,
		Operator:            "integration-test",
		MySQL: config.MySQL{
			Network:            driverConfig.Net,
			Address:            driverConfig.Addr,
			Database:           driverConfig.DBName,
			User:               driverConfig.User,
			Password:           driverConfig.Passwd,
			TLSMode:            "false",
			MaxOpenConnections: 4,
			MaxIdleConnections: 4,
			ConnectionMaxLife:  3 * time.Minute,
			ConnectionMaxIdle:  time.Minute,
			ConnectTimeout:     5 * time.Second,
			ReadTimeout:        5 * time.Second,
			WriteTimeout:       5 * time.Second,
		},
	}
}

func integrationEnvironment(driverConfig *mysqldriver.Config, httpAddress string) []string {
	host, port, _ := net.SplitHostPort(driverConfig.Addr)
	return []string{
		"ADMIN_HTTP_ADDR=" + httpAddress,
		"ADMIN_API_TOKEN=integration-token",
		"ADMIN_PUBLIC_ORIGIN=http://127.0.0.1:5173",
		"ADMIN_ALLOW_LOCAL_HTTP=true",
		"ADMIN_OPERATOR=integration-test",
		"MYSQL_HOST=" + host,
		"MYSQL_PORT=" + port,
		"MYSQL_DATABASE=" + driverConfig.DBName,
		"MYSQL_USER=" + driverConfig.User,
		"MYSQL_PASSWORD=" + driverConfig.Passwd,
		"MYSQL_TLS_MODE=false",
	}
}

func buildIntegrationAdmin(t *testing.T) string {
	t.Helper()
	binaryName := "admin"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	command := exec.Command("go", "build", "-o", binaryPath, ".")
	command.Dir = "."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Admin process: %v\n%s", err, output)
	}
	return binaryPath
}
