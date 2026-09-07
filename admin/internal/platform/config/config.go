package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains deployment-owned Admin settings.
type Config struct {
	AccountRegisterLimit     int
	AccountLoginIPLimit      int
	AccountLoginFailureLimit int
	AccountPublicOrigin      string
	AccountInsecureHTTP      bool
	AccountTrustedProxies    []string
	HTTPAddr                 string
	MySQL                    MySQL
}

// MySQL contains the one deployment-owned Managed Data Source configuration.
type MySQL struct {
	Network            string
	Address            string
	Database           string
	User               string
	Password           string
	TLSMode            string
	MaxOpenConnections int
	MaxIdleConnections int
	ConnectionMaxLife  time.Duration
	ConnectionMaxIdle  time.Duration
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
}

// Load reads structured settings from the process environment.
func Load() (Config, error) {
	httpAddr := valueOrDefault("ADMIN_HTTP_ADDR", "127.0.0.1:8080")
	if !validHTTPAddress(httpAddr) {
		return Config{}, fmt.Errorf("ADMIN_HTTP_ADDR must be an explicit host and numeric port")
	}
	if err := rejectRemovedSettings(); err != nil {
		return Config{}, err
	}
	mysql, err := LoadMySQL()
	if err != nil {
		return Config{}, err
	}

	registrationLimit, err := positiveInteger("ADMIN_REGISTER_LIMIT", 10)
	if err != nil {
		return Config{}, err
	}
	loginIPLimit, err := positiveInteger("ADMIN_LOGIN_IP_LIMIT", 60)
	if err != nil {
		return Config{}, err
	}
	loginFailureLimit, err := positiveInteger("ADMIN_LOGIN_FAILURE_LIMIT", 10)
	if err != nil {
		return Config{}, err
	}
	origin, insecure, proxies, err := accountHTTPEnvironment()
	if err != nil {
		return Config{}, err
	}
	return Config{
		AccountRegisterLimit: registrationLimit, AccountLoginIPLimit: loginIPLimit, AccountLoginFailureLimit: loginFailureLimit,
		AccountPublicOrigin: origin, AccountInsecureHTTP: insecure, AccountTrustedProxies: proxies,
		HTTPAddr: httpAddr,
		MySQL:    mysql,
	}, nil
}

func validHTTPAddress(address string) bool {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return false
	}
	port, err := strconv.Atoi(portText)
	return err == nil && port >= 0 && port <= 65535
}

func rejectRemovedSettings() error {
	for _, name := range []string{"ADMIN_API_TOKEN", "ADMIN_AUTH_DISABLED", "ADMIN_OPERATOR", "ADMIN_CORS_ORIGINS"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return fmt.Errorf("%s has been removed; configure ADMIN_PUBLIC_ORIGIN and use Local Account sessions", name)
		}
	}
	return nil
}

// LoadMySQL configures maintenance connections independently of normal Admin
// HTTP/account readiness. It never loads deployment authentication settings.
func LoadMySQL() (MySQL, error) {

	host := strings.TrimSpace(os.Getenv("MYSQL_HOST"))
	if host == "" {
		return MySQL{}, fmt.Errorf("MYSQL_HOST is required")
	}

	port, err := integer("MYSQL_PORT", 3306)
	if err != nil || port < 1 || port > 65535 {
		return MySQL{}, fmt.Errorf("MYSQL_PORT must be an integer from 1 to 65535")
	}
	database, err := required("MYSQL_DATABASE")
	if err != nil {
		return MySQL{}, err
	}
	user, err := required("MYSQL_USER")
	if err != nil {
		return MySQL{}, err
	}
	password, err := required("MYSQL_PASSWORD")
	if err != nil {
		return MySQL{}, err
	}
	tlsMode := valueOrDefault("MYSQL_TLS_MODE", "true")
	if !validTLSMode(tlsMode) {
		return MySQL{}, fmt.Errorf("MYSQL_TLS_MODE must be one of true, false, skip-verify, or preferred")
	}

	maxOpen, err := positiveInteger("MYSQL_MAX_OPEN_CONNS", 10)
	if err != nil {
		return MySQL{}, err
	}
	maxIdle, err := nonNegativeInteger("MYSQL_MAX_IDLE_CONNS", 10)
	if err != nil {
		return MySQL{}, err
	}
	if maxIdle > maxOpen {
		return MySQL{}, fmt.Errorf("MYSQL_MAX_IDLE_CONNS cannot exceed MYSQL_MAX_OPEN_CONNS")
	}
	connectionMaxLife, err := positiveDuration("MYSQL_CONN_MAX_LIFETIME", 3*time.Minute)
	if err != nil {
		return MySQL{}, err
	}
	connectionMaxIdle, err := positiveDuration("MYSQL_CONN_MAX_IDLE_TIME", time.Minute)
	if err != nil {
		return MySQL{}, err
	}
	connectTimeout, err := positiveDuration("MYSQL_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return MySQL{}, err
	}
	readTimeout, err := positiveDuration("MYSQL_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return MySQL{}, err
	}
	writeTimeout, err := positiveDuration("MYSQL_WRITE_TIMEOUT", 5*time.Second)
	if err != nil {
		return MySQL{}, err
	}

	return MySQL{
		Network:            "tcp",
		Address:            net.JoinHostPort(host, strconv.Itoa(port)),
		Database:           database,
		User:               user,
		Password:           password,
		TLSMode:            tlsMode,
		MaxOpenConnections: maxOpen,
		MaxIdleConnections: maxIdle,
		ConnectionMaxLife:  connectionMaxLife,
		ConnectionMaxIdle:  connectionMaxIdle,
		ConnectTimeout:     connectTimeout,
		ReadTimeout:        readTimeout,
		WriteTimeout:       writeTimeout,
	}, nil
}

func strictBoolean(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func required(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func integer(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func positiveInteger(name string, fallback int) (int, error) {
	value, err := integer(name, fallback)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func nonNegativeInteger(name string, fallback int) (int, error) {
	value, err := integer(name, fallback)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return value, nil
}

func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}

func validTLSMode(value string) bool {
	switch value {
	case "true", "false", "skip-verify", "preferred":
		return true
	default:
		return false
	}
}

func accountHTTPEnvironment() (string, bool, []string, error) {
	origin, err := required("ADMIN_PUBLIC_ORIGIN")
	if err != nil {
		return "", false, nil, err
	}
	insecure, err := strictBoolean("ADMIN_ALLOW_LOCAL_HTTP", false)
	if err != nil {
		return "", false, nil, err
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.String() != origin {
		return "", false, nil, fmt.Errorf("ADMIN_PUBLIC_ORIGIN must be an exact origin")
	}
	if parsed.Scheme != "https" && !(insecure && parsed.Scheme == "http" && net.ParseIP(parsed.Hostname()) != nil && net.ParseIP(parsed.Hostname()).IsLoopback()) {
		return "", false, nil, fmt.Errorf("ADMIN_PUBLIC_ORIGIN requires HTTPS or explicitly allowed loopback HTTP")
	}
	if insecure && parsed.Scheme != "http" {
		return "", false, nil, fmt.Errorf("ADMIN_ALLOW_LOCAL_HTTP requires a loopback HTTP origin")
	}
	var proxies []string
	if raw := strings.TrimSpace(os.Getenv("ADMIN_TRUSTED_PROXIES")); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			cidr := strings.TrimSpace(item)
			if _, _, err := net.ParseCIDR(cidr); err != nil {
				return "", false, nil, fmt.Errorf("ADMIN_TRUSTED_PROXIES must contain CIDRs")
			}
			proxies = append(proxies, cidr)
		}
	}
	return origin, insecure, proxies, nil
}
