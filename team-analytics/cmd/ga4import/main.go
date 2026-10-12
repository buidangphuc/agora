// Command ga4import imports one day of the GA4 export into ga4_sessions
// (add-ga4-export-import). It exits 0 without reading anything while
// GA4_EXPORT_DATASET is unset.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ga4import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	date := fs.String("date", "", "export date YYYY-MM-DD (default: yesterday, UTC)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	s, err := config.LoadSettings()
	if err != nil {
		fmt.Fprintf(stderr, "load settings: %v\n", err)
		return 1
	}
	if strings.TrimSpace(s.GA4.ExportDataset) == "" {
		fmt.Fprintln(stdout, "GA4_EXPORT_DATASET unset: nothing to import")
		return 0
	}
	day := time.Now().UTC().AddDate(0, 0, -1)
	if *date != "" {
		if day, err = time.Parse("2006-01-02", *date); err != nil {
			fmt.Fprintf(stderr, "invalid --date %q: %v\n", *date, err)
			return 2
		}
	}
	if s.Warehouse.Driver != config.DriverDuckDB {
		fmt.Fprintf(stderr, "ga4import supports WAREHOUSE_DRIVER=duckdb only, got %q\n", s.Warehouse.Driver)
		return 1
	}

	var src ga4import.Source
	switch s.GA4.ExportSource {
	case "fixture":
		if s.GA4.FixtureDir == "" {
			fmt.Fprintln(stderr, "GA4_EXPORT_FIXTURE_DIR is required when GA4_EXPORT_SOURCE=fixture")
			return 1
		}
		src = ga4import.NewFixtureSource(s.GA4.FixtureDir)
	default:
		bq, err := ga4import.NewBigQuerySource(ctx, s.Warehouse.BigQueryProject, s.GA4.ExportDataset)
		if err != nil {
			fmt.Fprintf(stderr, "bigquery source: %v\n", err)
			return 1
		}
		defer bq.Close()
		src = bq
	}

	w, err := duckdb.Open(ctx, s.Warehouse.DuckDBPath)
	if err != nil {
		fmt.Fprintf(stderr, "open warehouse: %v\n", err)
		return 1
	}
	defer w.Close()
	res, err := ga4import.Run(ctx, w, src, day)
	if err != nil {
		fmt.Fprintf(stderr, "ga4import failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "ga4import %s: %d events read\n", day.Format("2006-01-02"), res.Rows)
	return 0
}
