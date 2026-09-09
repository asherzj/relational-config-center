// release-reset is only for an explicitly identified, stopped dev/test database.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

const usage = "usage: release-reset --environment=<development|test> --target-address=HOST:PORT --target-database=NAME --target-server-uuid=UUID --writers-stopped"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, output, diagnostics io.Writer) int {
	flags := flag.NewFlagSet("release-reset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	environment := flags.String("environment", "", "isolated development or test environment")
	address := flags.String("target-address", "", "exact MYSQL_HOST:MYSQL_PORT or socket address")
	database := flags.String("target-database", "", "exact database name")
	uuid := flags.String("target-server-uuid", "", "verified SELECT @@server_uuid value")
	stopped := flags.Bool("writers-stopped", false, "all writers and retries have been stopped and drained")
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(output, usage)
		return 0
	}
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || (*environment != "development" && *environment != "test") || *address == "" || *database == "" || *uuid == "" || !*stopped {
		fmt.Fprintln(diagnostics, usage)
		return 2
	}
	settings, err := config.LoadMySQL()
	if err != nil {
		fmt.Fprintln(diagnostics, "configuration error: verify MYSQL_* settings")
		return 1
	}
	if settings.Address != *address || settings.Database != *database {
		fmt.Fprintln(diagnostics, "target mismatch: no changes made; verify explicit target against MYSQL_* settings")
		return 1
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 5*time.Minute)
	defer cancel()
	adapter, err := mysqladapter.OpenMaintenance(ctx, settings)
	if err != nil {
		fmt.Fprintln(diagnostics, "database unavailable: verify connection and permissions")
		return 1
	}
	defer adapter.Close()
	report, err := adapter.ResetReleaseHistory(ctx, *database, *uuid)
	if err != nil {
		switch {
		case errors.Is(err, mysqladapter.ErrReleaseResetTarget):
			fmt.Fprintln(diagnostics, "target mismatch: no changes made; verify database and server UUID")
		case errors.Is(err, mysqladapter.ErrReleaseResetSchema):
			fmt.Fprintln(diagnostics, "unsupported schema: no changes made; verify release/version tables, direct TRIGGER grants, and absence of triggers and foreign keys on cleanup tables")
		default:
			fmt.Fprintln(diagnostics, "reset not confirmed: keep writers stopped, resolve database/permission/interruption errors, then rerun the same verified target; a transaction interrupted before commit rolls back")
		}
		return 1
	}
	result := struct {
		Environment string `json:"environment"`
		Address     string `json:"address"`
		mysqladapter.ReleaseResetReport
	}{*environment, *address, report}
	if err := json.NewEncoder(output).Encode(result); err != nil {
		fmt.Fprintln(diagnostics, "output unavailable after commit: keep writers stopped and rerun the same verified target to obtain verification")
		return 1
	}
	return 0
}
