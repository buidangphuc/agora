package ga4import_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/ga4import"
)

func cliEnv(t *testing.T, url string) {
	t.Helper()
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50059")
	if url != "" {
		t.Setenv("GA4_IMPORT_TRIGGER_URL", url)
	}
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := ga4import.RunCLI(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLICallsTriggerAndExitsZeroOn200_TheImportRunsInsideTheConsumer(t *testing.T) {
	var gotMethod, gotDate string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotMethod, gotDate = r.Method, r.URL.Query().Get("date")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"rows":4}`))
	}))
	defer srv.Close()
	cliEnv(t, srv.URL+"/internal/ga4import")
	code, _, stderr := runCLI("--date", "2024-03-05")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if calls.Load() != 1 || gotMethod != http.MethodPost || gotDate != "2024-03-05" {
		t.Errorf("trigger got %d calls, %s date=%q, want 1 POST date=2024-03-05", calls.Load(), gotMethod, gotDate)
	}
}

func TestCLIDefaultsDateToYesterdayUTC_TheImportRunsInsideTheConsumer(t *testing.T) {
	var gotDate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDate = r.URL.Query().Get("date")
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	before := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	code, _, stderr := runCLI()
	after := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if gotDate != before && gotDate != after {
		t.Errorf("date = %q, want %q", gotDate, before)
	}
}

func TestCLIExitsNonZeroWithErrorTextOn5xx_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ga4 export 2024-03-06: events_20240306 not found", http.StatusInternalServerError)
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	code, _, stderr := runCLI("--date", "2024-03-06")
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(stderr, "2024-03-06") || !strings.Contains(stderr, "not found") {
		t.Errorf("stderr must carry the trigger error text, got %q", stderr)
	}
}

func TestCLIExitsNonZeroOn4xx_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid date \"x\"", http.StatusBadRequest)
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	code, _, stderr := runCLI("--date", "2024-03-06")
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(stderr, "invalid date") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestCLIExitsNonZeroWhenTriggerUnreachable_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	cliEnv(t, url)
	code, _, stderr := runCLI("--date", "2024-03-06")
	if code == 0 || stderr == "" {
		t.Fatalf("want non-zero exit with stderr text, got %d %q", code, stderr)
	}
}

func TestCLIExitsZeroWhenTriggerReportsNoOp_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rows":0}`))
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	code, _, stderr := runCLI("--date", "2024-03-05")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestCLIReadsTriggerAddressFromSettings_TheCLIReadsTheTriggerAddressFromSettings(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	cliEnv(t, srv.URL+"/custom/path")
	code, _, stderr := runCLI("--date", "2024-03-05")
	if code != 0 || calls.Load() != 1 {
		t.Fatalf("exit %d calls %d stderr %q, want exit 0 and one POST to the configured address", code, calls.Load(), stderr)
	}
}

func TestInvalidThresholdIsRejectedAtStartup(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	cliEnv(t, srv.URL)
	t.Setenv("GA4_RECONCILE_MAX_DIFF_RATIO", "-1")
	code, _, stderr := runCLI("--date", "2024-03-05")
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(stderr, "GA4_RECONCILE_MAX_DIFF_RATIO") {
		t.Errorf("stderr must name the setting, got %q", stderr)
	}
	if calls.Load() != 0 {
		t.Errorf("trigger called %d times, want 0", calls.Load())
	}
}
