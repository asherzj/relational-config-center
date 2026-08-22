package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	"github.com/asherzj/relational-config-center/admin/internal/config"
	"github.com/asherzj/relational-config-center/admin/internal/domain"
	"github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	httpapi "github.com/asherzj/relational-config-center/admin/internal/interfaces/http"
)

func main() {
	if err := run(); err != nil {
		slog.Error("admin stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()
	db, sqlDB, err := mysql.Open(startupContext, mysql.Options{
		DSN:             settings.MySQLDSN,
		MaxOpenConns:    settings.MaxOpenConns,
		MaxIdleConns:    settings.MaxIdleConns,
		ConnMaxLifetime: settings.ConnMaxLifetime,
		ConnMaxIdleTime: settings.ConnMaxIdleTime,
	})
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	policies, err := mysql.NewPolicyCatalog(db).Load(startupContext)
	if err != nil {
		return fmt.Errorf("load table policies: %w", err)
	}
	registry, err := domain.NewRegistry(policies...)
	if err != nil {
		return fmt.Errorf("build table policy registry: %w", err)
	}
	service := application.NewService(registry, mysql.NewRepository(db))
	server := &http.Server{
		Addr:              settings.Address,
		Handler:           httpapi.NewRouter(service, sqlDB),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("admin listening", "address", settings.Address)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case serveErr := <-serverErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve Admin HTTP: %w", serveErr)
		}
		return nil
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		return fmt.Errorf("shutdown Admin HTTP: %w", err)
	}
	return nil
}
