package config_test

import (
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func TestLoadRejectsMissingBearerTokenWhenAuthenticationIsEnabled(t *testing.T) {
	setRequiredMySQL(t)
	t.Setenv("ADMIN_API_TOKEN", "")
	t.Setenv("ADMIN_AUTH_DISABLED", "")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_API_TOKEN is required") {
		t.Fatalf("expected missing API token rejection, got %v", err)
	}
}

func TestLoadAllowsAuthenticationToBeDisabledOnlyOnExplicitLoopback(t *testing.T) {
	for _, test := range []struct {
		name    string
		address string
		wantErr bool
	}{
		{name: "IPv4 loopback", address: "127.0.0.1:8080"},
		{name: "IPv6 loopback", address: "[::1]:8080"},
		{name: "wildcard IPv4", address: "0.0.0.0:8080", wantErr: true},
		{name: "wildcard IPv6", address: "[::]:8080", wantErr: true},
		{name: "hostname is not explicit", address: "localhost:8080", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequiredMySQL(t)
			t.Setenv("ADMIN_HTTP_ADDR", test.address)
			t.Setenv("ADMIN_AUTH_DISABLED", "true")
			t.Setenv("ADMIN_API_TOKEN", "")

			settings, err := config.Load()
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "explicit loopback") {
					t.Fatalf("expected loopback restriction error, got settings %#v and error %v", settings, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("load loopback configuration: %v", err)
			}
			if !settings.AuthDisabled {
				t.Fatal("expected authentication to be disabled")
			}
		})
	}
}

func TestLoadRejectsNonBooleanAuthenticationFlag(t *testing.T) {
	setRequiredMySQL(t)
	t.Setenv("ADMIN_API_TOKEN", "deployment-secret")
	t.Setenv("ADMIN_AUTH_DISABLED", "1")

	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "ADMIN_AUTH_DISABLED must be true or false") {
		t.Fatalf("expected strict boolean rejection, got %v", err)
	}
}

func TestLoadParsesAndValidatesExactCORSOrigins(t *testing.T) {
	setRequiredMySQL(t)
	t.Setenv("ADMIN_API_TOKEN", "deployment-secret")
	t.Setenv("ADMIN_CORS_ORIGINS", " https://admin.example.test,http://127.0.0.1:5173,https://admin.example.test ")

	settings, err := config.Load()
	if err != nil {
		t.Fatalf("load CORS origins: %v", err)
	}
	want := []string{"https://admin.example.test", "http://127.0.0.1:5173"}
	if len(settings.CORSOrigins) != len(want) {
		t.Fatalf("expected origins %#v, got %#v", want, settings.CORSOrigins)
	}
	for index := range want {
		if settings.CORSOrigins[index] != want[index] {
			t.Fatalf("expected origins %#v, got %#v", want, settings.CORSOrigins)
		}
	}

	for _, invalid := range []string{"*", "https://example.test/path", "example.test", "https://*.example.test"} {
		t.Run(invalid, func(t *testing.T) {
			setRequiredMySQL(t)
			t.Setenv("ADMIN_API_TOKEN", "deployment-secret")
			t.Setenv("ADMIN_CORS_ORIGINS", invalid)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ADMIN_CORS_ORIGINS") {
				t.Fatalf("expected invalid CORS origin rejection for %q, got %v", invalid, err)
			}
		})
	}
}

func TestLoadRejectsInvalidHTTPBindAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", ":8080", "127.0.0.1:not-a-port", "127.0.0.1:70000"} {
		t.Run(address, func(t *testing.T) {
			setRequiredMySQL(t)
			t.Setenv("ADMIN_API_TOKEN", "deployment-secret")
			t.Setenv("ADMIN_HTTP_ADDR", address)
			if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ADMIN_HTTP_ADDR") {
				t.Fatalf("expected invalid bind address rejection for %q, got %v", address, err)
			}
		})
	}
}

func setRequiredMySQL(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ADMIN_HTTP_ADDR", "ADMIN_API_TOKEN", "ADMIN_AUTH_DISABLED", "ADMIN_OPERATOR", "ADMIN_CORS_ORIGINS",
		"MYSQL_MAX_OPEN_CONNS", "MYSQL_MAX_IDLE_CONNS", "MYSQL_CONN_MAX_LIFETIME", "MYSQL_CONN_MAX_IDLE_TIME",
		"MYSQL_CONNECT_TIMEOUT", "MYSQL_READ_TIMEOUT", "MYSQL_WRITE_TIMEOUT",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("MYSQL_HOST", "127.0.0.1")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_DATABASE", "rcc_test")
	t.Setenv("MYSQL_USER", "rcc_admin")
	t.Setenv("MYSQL_PASSWORD", "database-secret")
	t.Setenv("MYSQL_TLS_MODE", "false")
}
