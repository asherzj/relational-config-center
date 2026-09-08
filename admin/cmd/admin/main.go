package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func main() {
	os.Exit(runMain(os.Stderr))
}

func runMain(standardError *os.File) int {
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintf(standardError, "configuration error: %v\n", err)
		return 1
	}

	application, err := newApplication(context.Background(), settings)
	if err != nil {
		if errors.Is(err, mysqladapter.ErrAuthenticationSchemaIncomplete) {
			fmt.Fprintln(standardError, "startup error: authentication schema is incomplete; apply migration 007 (see deploy/mysql/migrations/README.md)")
		} else if errors.Is(err, mysqladapter.ErrAccountRoleSchemaIncomplete) {
			fmt.Fprintln(standardError, "startup error: account role schema is incomplete; apply migration 008 (see deploy/mysql/migrations/README.md)")
		} else if errors.Is(err, mysqladapter.ErrRecordVersionSchemaIncomplete) {
			fmt.Fprintln(standardError, "startup error: record version schema is incomplete; apply migration 009 (see deploy/mysql/migrations/README.md)")
		} else if errors.Is(err, mysqladapter.ErrReleaseSchemaIncomplete) {
			fmt.Fprintln(standardError, "startup error: release order schema is incomplete; apply migrations 010, 011 and 012 (see deploy/mysql/migrations/README.md)")
		} else {
			fmt.Fprintln(standardError, "startup error: Managed Data Source is unavailable")
		}
		return 1
	}

	if err := serveAdmin(application, settings.HTTPAddr, productionShutdownTimeout); err != nil {
		switch {
		case errors.Is(err, errApplicationClose):
			fmt.Fprintln(standardError, "shutdown error: failed to close Managed Data Source")
		default:
			fmt.Fprintln(standardError, "server error: Admin HTTP server failed")
		}
		return 1
	}
	return 0
}
