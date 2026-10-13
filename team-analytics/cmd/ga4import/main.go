// Command ga4import asks the running consumer to import one day of the GA4
// export (POST /internal/ga4import). The import itself runs inside the consumer
// process so DuckDB keeps a single writer (add-ga4-export-import).
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/buidangphuc/team-analytics/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := app.RunGA4Import(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
