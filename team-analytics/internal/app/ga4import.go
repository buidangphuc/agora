package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/buidangphuc/team-analytics/internal/config"
)

const defaultTriggerURL = "http://localhost:9109/internal/ga4import"

// RunGA4Import is the ga4import CLI: a thin client of the consumer's
// POST /internal/ga4import trigger. The import itself runs inside the consumer
// process so DuckDB keeps a single writer (add-ga4-export-import). It returns
// the process exit code.
func RunGA4Import(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runGA4Import(ctx, args, stdout, stderr)
}

func runGA4Import(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ga4import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	date := fs.String("date", "", "export date YYYY-MM-DD (default: yesterday, UTC)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if _, err := config.Load(); err != nil {
		fmt.Fprintf(stderr, "load settings: %v\n", err)
		return 1
	}
	day := *date
	if day == "" {
		day = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}
	base := strings.TrimSpace(os.Getenv("GA4_IMPORT_TRIGGER_URL"))
	if base == "" {
		base = defaultTriggerURL
	}
	u, err := url.Parse(base)
	if err != nil {
		fmt.Fprintf(stderr, "invalid GA4_IMPORT_TRIGGER_URL %q: %v\n", base, err)
		return 1
	}
	q := u.Query()
	q.Set("date", day)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil) //nolint:gosec // the trigger URL is operator configuration (GA4_IMPORT_TRIGGER_URL)
	if err != nil {
		fmt.Fprintf(stderr, "ga4import %s: %v\n", day, err)
		return 1
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req) //nolint:gosec // the trigger URL is operator configuration (GA4_IMPORT_TRIGGER_URL)
	if err != nil {
		fmt.Fprintf(stderr, "ga4import %s: trigger unreachable: %v\n", day, err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "ga4import %s failed: %s: %s\n", day, resp.Status, strings.TrimSpace(string(body)))
		return 1
	}
	fmt.Fprintf(stdout, "ga4import %s: %s\n", day, strings.TrimSpace(string(body)))
	return 0
}
