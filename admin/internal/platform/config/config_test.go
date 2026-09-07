package config_test

import (
	"strings"
	"testing"

	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func TestLoadRequiresNoDeploymentIdentity(t *testing.T) {
	setRequiredMySQL(t)
	if _, err := config.Load(); err != nil {
		t.Fatalf("load account configuration: %v", err)
	}
}

func TestMaintenanceConfigNeedsOnlyMySQL(t *testing.T) {
	setRequiredMySQL(t)
	t.Setenv("ADMIN_PUBLIC_ORIGIN", "")
	if _, err := config.LoadMySQL(); err != nil {
		t.Fatalf("maintenance depends on HTTP/account configuration: %v", err)
	}
}

func TestLoadRejectsInvalidHTTPBindAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", ":8080", "127.0.0.1:not-a-port", "127.0.0.1:70000"} {
		t.Run(address, func(t *testing.T) {
			setRequiredMySQL(t)
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
	t.Setenv("ADMIN_PUBLIC_ORIGIN", "https://admin.example.test")
	t.Setenv("ADMIN_ALLOW_LOCAL_HTTP", "")
	t.Setenv("ADMIN_TRUSTED_PROXIES", "")
	t.Setenv("ADMIN_REGISTER_LIMIT", "")
	t.Setenv("ADMIN_LOGIN_IP_LIMIT", "")
	t.Setenv("ADMIN_LOGIN_FAILURE_LIMIT", "")
	t.Setenv("MYSQL_HOST", "127.0.0.1")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_DATABASE", "rcc_test")
	t.Setenv("MYSQL_USER", "rcc_admin")
	t.Setenv("MYSQL_PASSWORD", "database-secret")
	t.Setenv("MYSQL_TLS_MODE", "false")
}

func TestLocalAccountOriginAndLimitsConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, origin, insecure, proxies string
		wantError                       bool
	}{
		{name: "explicit HTTPS", origin: "https://config.example.test"},
		{name: "explicit local HTTP", origin: "http://127.0.0.1:5173", insecure: "true"},
		{name: "IPv6 local HTTP", origin: "http://[::1]:5173", insecure: "true"},
		{name: "missing origin", wantError: true},
		{name: "HTTP without exception", origin: "http://127.0.0.1:5173", wantError: true},
		{name: "remote HTTP", origin: "http://config.example.test", insecure: "true", wantError: true},
		{name: "origin with path", origin: "https://config.example.test/path", wantError: true},
		{name: "credentials in origin", origin: "https://user@config.example.test", wantError: true},
		{name: "proxy CIDR", origin: "https://config.example.test", proxies: "127.0.0.1/32,10.0.0.0/8"},
		{name: "invalid proxy", origin: "https://config.example.test", proxies: "*", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			setRequiredMySQL(t)
			t.Setenv("ADMIN_PUBLIC_ORIGIN", test.origin)
			t.Setenv("ADMIN_ALLOW_LOCAL_HTTP", test.insecure)
			t.Setenv("ADMIN_TRUSTED_PROXIES", test.proxies)
			settings, err := config.Load()
			if (err != nil) != test.wantError {
				t.Fatalf("error=%v wanted error=%v", err, test.wantError)
			}
			if err == nil && (settings.AccountRegisterLimit != 10 || settings.AccountLoginIPLimit != 60 || settings.AccountLoginFailureLimit != 10) {
				t.Fatal("rate defaults")
			}
		})
	}
}
