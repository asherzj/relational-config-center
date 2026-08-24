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
	HTTPAddr     string
	APIToken     string
	AuthDisabled bool
	CORSOrigins  []string
	Operator     string
	MySQL        MySQL
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
	authDisabled, err := strictBoolean("ADMIN_AUTH_DISABLED", false)
	if err != nil {
		return Config{}, err
	}
	if authDisabled && !explicitLoopbackAddress(httpAddr) {
		return Config{}, fmt.Errorf("ADMIN_AUTH_DISABLED requires ADMIN_HTTP_ADDR to use an explicit loopback address")
	}
	apiToken := strings.TrimSpace(os.Getenv("ADMIN_API_TOKEN"))
	if !authDisabled && apiToken == "" {
		return Config{}, fmt.Errorf("ADMIN_API_TOKEN is required when authentication is enabled")
	}
	corsOrigins, err := corsOriginsFromEnvironment()
	if err != nil {
		return Config{}, err
	}

	host := strings.TrimSpace(os.Getenv("MYSQL_HOST"))
	if host == "" {
		return Config{}, fmt.Errorf("MYSQL_HOST is required")
	}

	port, err := integer("MYSQL_PORT", 3306)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("MYSQL_PORT must be an integer from 1 to 65535")
	}
	database, err := required("MYSQL_DATABASE")
	if err != nil {
		return Config{}, err
	}
	user, err := required("MYSQL_USER")
	if err != nil {
		return Config{}, err
	}
	password, err := required("MYSQL_PASSWORD")
	if err != nil {
		return Config{}, err
	}
	tlsMode := valueOrDefault("MYSQL_TLS_MODE", "true")
	if !validTLSMode(tlsMode) {
		return Config{}, fmt.Errorf("MYSQL_TLS_MODE must be one of true, false, skip-verify, or preferred")
	}

	maxOpen, err := positiveInteger("MYSQL_MAX_OPEN_CONNS", 10)
	if err != nil {
		return Config{}, err
	}
	maxIdle, err := nonNegativeInteger("MYSQL_MAX_IDLE_CONNS", 10)
	if err != nil {
		return Config{}, err
	}
	if maxIdle > maxOpen {
		return Config{}, fmt.Errorf("MYSQL_MAX_IDLE_CONNS cannot exceed MYSQL_MAX_OPEN_CONNS")
	}
	connectionMaxLife, err := positiveDuration("MYSQL_CONN_MAX_LIFETIME", 3*time.Minute)
	if err != nil {
		return Config{}, err
	}
	connectionMaxIdle, err := positiveDuration("MYSQL_CONN_MAX_IDLE_TIME", time.Minute)
	if err != nil {
		return Config{}, err
	}
	connectTimeout, err := positiveDuration("MYSQL_CONNECT_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	readTimeout, err := positiveDuration("MYSQL_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	writeTimeout, err := positiveDuration("MYSQL_WRITE_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:     httpAddr,
		APIToken:     apiToken,
		AuthDisabled: authDisabled,
		CORSOrigins:  corsOrigins,
		Operator:     valueOrDefault("ADMIN_OPERATOR", "admin"),
		MySQL: MySQL{
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
		},
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

func corsOriginsFromEnvironment() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv("ADMIN_CORS_ORIGINS"))
	if raw == "" {
		return nil, nil
	}
	origins := make([]string, 0)
	seen := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		parsed, err := url.Parse(origin)
		if err != nil || strings.Contains(origin, "*") || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.String() != origin {
			return nil, fmt.Errorf("ADMIN_CORS_ORIGINS must contain exact HTTP origins")
		}
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
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

func explicitLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
