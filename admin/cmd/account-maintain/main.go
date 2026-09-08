package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/asherzj/relational-config-center/admin/internal/application"
	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/asherzj/relational-config-center/admin/internal/infrastructure/password"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

const usage = "usage: account-maintain <lookup|reset-password|disable|enable|set-email|grant-admin> <--id UUID|--username NAME> [--password-stdin|--email-stdin]"

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, input *os.File, output, diagnostics io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(output, usage)
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(diagnostics, usage)
		return 2
	}
	action := args[0]
	if action != "lookup" && action != "reset-password" && action != "disable" && action != "enable" && action != "set-email" && action != "grant-admin" {
		fmt.Fprintln(diagnostics, usage)
		return 2
	}
	flags := flag.NewFlagSet("account-maintain", flag.ContinueOnError)
	// Flag errors can include a supplied value. Never print parser errors or argv.
	flags.SetOutput(io.Discard)
	id := flags.String("id", "", "Account ID")
	username := flags.String("username", "", "username")
	passwordStdin := flags.Bool("password-stdin", false, "read the exact UTF-8 password through EOF")
	emailStdin := flags.Bool("email-stdin", false, "read the corrected email through EOF")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || (*id == "") == (*username == "") || *passwordStdin != (action == "reset-password") || *emailStdin != (action == "set-email") {
		fmt.Fprintln(diagnostics, usage)
		return 2
	}
	var fieldInput string
	if *passwordStdin || *emailStdin {
		stat, err := input.Stat()
		if err != nil || stat.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(diagnostics, "input error: pipe or redirect field bytes to stdin; terminal input is not accepted")
			return 2
		}
		raw, err := io.ReadAll(io.LimitReader(input, 513))
		if err != nil || len(raw) > 512 {
			fmt.Fprintln(diagnostics, "input error: unable to read account field (512-byte input limit)")
			return 2
		}
		fieldInput = string(raw)
	}
	settings, err := config.LoadMySQL()
	if err != nil {
		fmt.Fprintf(diagnostics, "configuration error: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adapter, err := mysqladapter.OpenMaintenance(ctx, settings)
	if err != nil {
		fmt.Fprintln(diagnostics, "database unavailable: verify MYSQL_* connection settings and database permissions")
		return 1
	}
	defer func() { _ = adapter.Close() }()
	maintenance := application.NewAccountMaintenance(adapter, password.NewArgon2id())
	selector := application.AccountSelector{ID: *id, Username: *username}
	var account application.LocalAccountSummary
	if action == "grant-admin" {
		account, err = maintenance.GrantAdmin(ctx, selector)
	} else if action == "reset-password" {
		account, err = maintenance.ResetPassword(ctx, selector, fieldInput)
	} else if action == "disable" || action == "enable" {
		account, err = maintenance.SetEnabled(ctx, selector, action == "enable")
	} else if action == "set-email" {
		account, err = maintenance.CorrectEmail(ctx, selector, fieldInput)
	} else {
		account, err = maintenance.Find(ctx, selector)
	}
	if err != nil {
		return reportError(diagnostics, err)
	}
	if err := json.NewEncoder(output).Encode(struct {
		Action  string                          `json:"action"`
		Account application.LocalAccountSummary `json:"account"`
	}{action, account}); err != nil {
		fmt.Fprintln(diagnostics, "output error: operation completed; verify its outcome before retrying")
		return 1
	}
	return 0
}

func reportError(output io.Writer, err error) int {
	switch {
	case errors.Is(err, application.ErrLastAdministrator):
		fmt.Fprintln(output, "cannot remove the last enabled administrator; grant ADMIN to another enabled account first")
	case errors.Is(err, application.ErrAccountDisabled):
		fmt.Fprintln(output, "account is disabled; explicitly enable it before granting ADMIN")
	case errors.Is(err, application.ErrAccountFields):
		fmt.Fprintln(output, "invalid account fields: verify selector and account field rules")
	case errors.Is(err, application.ErrAccountNotFound):
		fmt.Fprintln(output, "account not found")
	case errors.Is(err, application.ErrAccountConflict):
		fmt.Fprintln(output, "account field conflict: email is already occupied")
	case errors.Is(err, application.ErrAuthTimeout):
		fmt.Fprintln(output, "database operation timed out: verify outcome before retrying")
	default:
		fmt.Fprintln(output, "account storage unavailable: verify database permissions and local-account control schema (migration 007); verify outcome before retrying")
	}
	return 1
}
