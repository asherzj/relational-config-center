// Package mysql implements managed-table persistence with GORM and MySQL.
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Options controls the underlying database/sql pool.
type Options struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// Open creates a GORM handle backed by a configured database/sql pool.
func Open(ctx context.Context, options Options) (*gorm.DB, *sql.DB, error) {
	driverConfig, err := drivermysql.ParseDSN(options.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parse MySQL DSN: %w", err)
	}
	driverConfig.ParseTime = true
	driverConfig.Loc = time.UTC
	// MySQL normally reports only changed rows. Generic updates need matched-row
	// semantics so an idempotent update is not mistaken for a missing row.
	driverConfig.ClientFoundRows = true

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: driverConfig.FormatDSN(),
	}), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open MySQL: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("obtain database/sql pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(options.MaxOpenConns)
	sqlDB.SetMaxIdleConns(options.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(options.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(options.ConnMaxIdleTime)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping MySQL: %w", err)
	}
	return db, sqlDB, nil
}
