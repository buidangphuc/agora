package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The CLI is a thin client of the consumer's POST /internal/ga4import trigger.

func cliEnv(t *testing.T, url string) {
	t.Helper()
	t.Setenv("GA4_IMPORT_TRIGGER_URL", url)
	for _, k := range []string{"GA4_EXPORT_SOURCE", "GA4_EXPORT_DATASET", "GA4_EXPORT_FIXTURE_DIR", "GA4_RECONCILE_MAX_DIFF_RATIO"} {
		t.Setenv(k, "")
	}
}

// Scenario: The import runs inside the consumer without a second writer (CLI side: exit 0 on 200, POST with date)
func TestCLICallsTriggerAndExitsZeroOn200_TheImportRunsInsideTheConsumer(t *testing.T) {
	var gotMethod, gotDate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotDate = r.Method, r.URL.Query().Get("date")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"rows":4}`))
	}))
	defer srv.Close()
	cliEnv(t, srv.URL+"/internal/ga4import")
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	if gotMethod != http.MethodPost || gotDate != "2024-03-05" {
		t.Errorf("trigger got %s date=%q, want POST date=2024-03-05", gotMethod, gotDate)
	}
}

// Scenario: The import runs inside the consumer without a second writer (default date is yesterday UTC)
func TestCLIDefaultsDateToYesterdayUTC_TheImportRunsInsideTheConsumer(t *testing.T) {
	var gotDate string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDate = r.URL.Query().Get("date")
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	var out, errb bytes.Buffer
	if code := run(context.Background(), nil, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
	want := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if gotDate != want {
		t.Errorf("date = %q, want %q", gotDate, want)
	}
}

// Scenario: A failed import returns 5xx with the date (CLI exits non-zero with the text on stderr)
func TestCLIExitsNonZeroWithErrorTextOn5xx_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "ga4 export 2024-03-06: events_20240306 not found", http.StatusInternalServerError)
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	var out, errb bytes.Buffer
	code := run(context.Background(), []string{"--date", "2024-03-06"}, &out, &errb)
	if code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errb.String(), "2024-03-06") || !strings.Contains(errb.String(), "not found") {
		t.Errorf("stderr must carry the trigger error text, got %q", errb.String())
	}
}

// Scenario: An invalid date is rejected by the trigger (4xx surfaces as non-zero + text)
func TestCLIExitsNonZeroOn4xx_AnInvalidDateIsRejectedByTheTrigger(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid date \"x\"", http.StatusBadRequest)
	}))
	defer srv.Close()
	cliEnv(t, srv.URL)
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-06"}, &out, &errb); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errb.String(), "invalid date") {
		t.Errorf("stderr = %q", errb.String())
	}
}

// Scenario: A failed import returns 5xx with the date (trigger unreachable is also a failure)
func TestCLIExitsNonZeroWhenTriggerUnreachable_AFailedImportReturns5xxWithTheDate(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	cliEnv(t, url)
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-06"}, &out, &errb); code == 0 || errb.Len() == 0 {
		t.Fatalf("want non-zero exit with stderr text, got %d %q", code, errb.String())
	}
}

// Scenario: The job is a no-op when unconfigured (trigger answers 200 rows=0 -> CLI exit 0)
func TestCLIExitsZeroWhenTriggerReportsNoOp_TheJobIsANoOpWhenUnconfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"rows":0}`))
	}))
	defer srv.Close()
	cliEnv(t, srv.URL) // GA4_EXPORT_DATASET unset
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errb.String())
	}
}

// Scenario: Invalid threshold is rejected at startup (before any trigger call)
func TestInvalidThresholdIsRejectedAtStartup(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()
	cliEnv(t, srv.URL)
	t.Setenv("GA4_RECONCILE_MAX_DIFF_RATIO", "-1")
	t.Setenv("GA4_EXPORT_DATASET", "fixture")
	t.Setenv("GA4_EXPORT_SOURCE", "fixture")
	var out, errb bytes.Buffer
	if code := run(context.Background(), []string{"--date", "2024-03-05"}, &out, &errb); code == 0 {
		t.Fatal("want non-zero exit")
	}
	if !strings.Contains(errb.String(), "GA4_RECONCILE_MAX_DIFF_RATIO") {
		t.Errorf("stderr must name the setting, got %q", errb.String())
	}
	if calls.Load() != 0 {
		t.Errorf("trigger called %d times, want 0", calls.Load())
	}
}
