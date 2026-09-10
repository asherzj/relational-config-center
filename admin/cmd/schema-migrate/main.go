package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func main() { os.Exit(run()) }

func run() int {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: schema-migrate status|up|baseline|recover [--timeout=5m] [--lock-timeout=10s]")
		return 2
	}
	operation := os.Args[1]
	if operation != "status" && operation != "up" && operation != "baseline" && operation != "recover" {
		fmt.Fprintln(os.Stderr, "unsupported_operation: use status, up, baseline or recover; down/reset are not supported")
		return 2
	}
	flags := flag.NewFlagSet("schema-migrate", flag.ContinueOnError)
	timeout := flags.Duration("timeout", 5*time.Minute, "total operation deadline")
	lockTimeout := flags.Duration("lock-timeout", 10*time.Second, "maximum migration lock wait")
	if err := flags.Parse(os.Args[2:]); err != nil || flags.NArg() != 0 || *timeout <= 0 || *timeout > time.Hour || *lockTimeout <= 0 || *lockTimeout > *timeout {
		fmt.Fprintln(os.Stderr, "invalid_options: require 0 < lock-timeout <= timeout <= 1h and no extra arguments")
		return 2
	}
	settings, err := config.LoadMySQL()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration_error: %v\n", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	adapter, err := mysqladapter.OpenMaintenance(ctx, settings)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database_unavailable: check maintenance connection, permissions and timeout")
		return 1
	}
	defer adapter.Close()
	var result mysqladapter.SchemaMigrationStatus
	if operation == "status" {
		result, err = adapter.ControlSchemaStatus(ctx)
	} else if operation == "baseline" {
		result, err = adapter.BaselineControlSchema(ctx, mysqladapter.SchemaMigrationOptions{LockTimeout: *lockTimeout})
	} else {
		result, err = adapter.MigrateControlSchema(ctx, mysqladapter.SchemaMigrationOptions{LockTimeout: *lockTimeout, Recover: operation == "recover"})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}
