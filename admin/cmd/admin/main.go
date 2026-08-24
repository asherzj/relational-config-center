package main

import (
	"context"
	"errors"
	"fmt"
	"os"

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
		fmt.Fprintln(standardError, "startup error: Managed Data Source is unavailable")
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
