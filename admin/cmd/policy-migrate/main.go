package main

import (
	"context"
	"fmt"
	"os"

	mysqladapter "github.com/asherzj/relational-config-center/admin/internal/infrastructure/mysql"
	"github.com/asherzj/relational-config-center/admin/internal/platform/config"
)

func main() {
	os.Exit(run(os.Stderr))
}

func run(standardError *os.File) int {
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintf(standardError, "configuration error: %v\n", err)
		return 1
	}
	adapter, err := mysqladapter.Open(context.Background(), settings.MySQL)
	if err != nil {
		fmt.Fprintln(standardError, "migration error: Policy Catalog is unavailable")
		return 1
	}
	defer func() { _ = adapter.Close() }()
	if err := adapter.MigrateLegacyTablePolicies(context.Background(), settings.Operator); err != nil {
		fmt.Fprintf(standardError, "migration error: %v\n", err)
		return 1
	}
	if err := adapter.ContractLegacyTablePolicies(context.Background()); err != nil {
		fmt.Fprintf(standardError, "contraction error (no legacy columns were dropped): %v\n", err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "legacy Table Policies backfilled and contracted to Code assignments successfully")
	return 0
}
