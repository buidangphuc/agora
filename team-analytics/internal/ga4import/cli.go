package ga4import

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/buidangphuc/team-analytics/internal/config"
)

func RunCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ga4import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	date := fs.String("date", "", "export date YYYY-MM-DD (default: yesterday, UTC)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	settings, err := config.Load()
	if err != nil {
		fmt.Fprintf(stderr, "load settings: %v\n", err)
		return 1
	}
	day := *date
	if day == "" {
		day = ImportDate(time.Now()).Format(dateLayout)
	}
	base := strings.TrimSpace(settings.GA4.ImportTriggerURL)
	u, err := url.Parse(base)
	if err != nil {
		fmt.Fprintf(stderr, "invalid GA4_IMPORT_TRIGGER_URL %q: %v\n", base, err)
		return 1
	}
	q := u.Query()
	q.Set("date", day)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		fmt.Fprintf(stderr, "ga4import %s: %v\n", day, err)
		return 1
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
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
