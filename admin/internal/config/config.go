// Package config loads Admin process settings from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contains process and MySQL settings.
type Config struct {
	Address         string
	MySQLDSN        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Load reads environment variables with local-development defaults.
func Load() (Config, error) {
	config := Config{
		Address:         envOr("RCC_ADMIN_ADDR", "127.0.0.1:8080"),
		MySQLDSN:        envOr("RCC_MYSQL_DSN", "rcc_admin:rcc_admin@tcp(127.0.0.1:3306)/rcc?charset=utf8mb4"),
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
	var err error
	if config.MaxOpenConns, err = intEnv("RCC_MYSQL_MAX_OPEN_CONNS", config.MaxOpenConns); err != nil {
		return Config{}, err
	}
	if config.MaxIdleConns, err = intEnv("RCC_MYSQL_MAX_IDLE_CONNS", config.MaxIdleConns); err != nil {
		return Config{}, err
	}
	if config.ConnMaxLifetime, err = durationEnv("RCC_MYSQL_CONN_MAX_LIFETIME", config.ConnMaxLifetime); err != nil {
		return Config{}, err
	}
	if config.ConnMaxIdleTime, err = durationEnv("RCC_MYSQL_CONN_MAX_IDLE_TIME", config.ConnMaxIdleTime); err != nil {
		return Config{}, err
	}
	if config.MaxOpenConns < 1 {
		return Config{}, fmt.Errorf("RCC_MYSQL_MAX_OPEN_CONNS must be at least 1")
	}
	if config.MaxIdleConns < 0 || config.MaxIdleConns > config.MaxOpenConns {
		return Config{}, fmt.Errorf("RCC_MYSQL_MAX_IDLE_CONNS must be between 0 and RCC_MYSQL_MAX_OPEN_CONNS")
	}
	return config, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func intEnv(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return parsed, nil
}
