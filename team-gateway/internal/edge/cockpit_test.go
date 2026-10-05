package edge_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buidangphuc/team-gateway/internal/edge"
)

// decodeCockpit runs the handler and decodes its JSON body, asserting the
// content type and status the frozen frontend contract depends on.
func decodeCockpit(t *testing.T, h http.Handler) edge.CockpitMetricsResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}

	var body edge.CockpitMetricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}

// TestCockpitDegradedNoPrometheus: with no PROMETHEUS_URL the handler must still
// return the exact shape with zeroed metrics — never random data.
func TestCockpitDegradedNoPrometheus(t *testing.T) {
	body := decodeCockpit(t, edge.NewCockpitHandler(""))

	if len(body.Services) == 0 {
		t.Fatal("expected roster services even in degraded mode")
	}
	if body.PrometheusAvailable {
		t.Error("degraded mode must report prometheus_available=false")
	}
	if body.TotalRPS != 0 || body.AvgLatencyMs != nil {
		t.Errorf("degraded mode should have zero total_rps and null avg latency, got %v / %v", body.TotalRPS, body.AvgLatencyMs)
	}
	for _, s := range body.Services {
		if s.RPS != 0 || s.P95Latency != nil || s.P99Latency != nil || s.ErrorRate != nil || s.Status != "UNKNOWN" {
			t.Errorf("degraded mode should null %s metrics and report UNKNOWN, got %+v", s.Name, s)
		}
	}
	// Shape must stay intact for the frozen CockpitView contract.
	if body.RecentTraces == nil {
		t.Error("recent_traces must be present")
	}
}

// TestCockpitSourcedFromPrometheus: a fake Prometheus returns per-rpc_service
// vectors; the handler must map them onto the roster (non-zero, not random).
func TestCockpitSourcedFromPrometheus(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		var sample string
		switch {
		case strings.Contains(q, "rate(marketplace_rpc_client_duration_milliseconds_count[1m])"):
			sample = "42.5" // RPS
		case strings.Contains(q, "0.95"):
			sample = "13.2" // p95
		case strings.Contains(q, "0.99"):
			sample = "27.9" // p99
		default:
			sample = "0.01" // error-rate
		}
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[` +
			`{"metric":{"rpc_service":"platform.search.v1.SearchService"},"value":[1700000000,"` + sample + `"]}]}}`))
	}))
	defer prom.Close()

	body := decodeCockpit(t, edge.NewCockpitHandler(prom.URL))

	var search *edge.ServiceHealth
	for i := range body.Services {
		if body.Services[i].Name == "team-search" {
			search = &body.Services[i]
		}
	}
	if search == nil {
		t.Fatal("team-search row missing")
	}
	if search.RPS != 42.5 {
		t.Errorf("team-search RPS: want 42.5 from Prometheus, got %v", search.RPS)
	}
	if search.P95Latency == nil || *search.P95Latency != 13.2 || search.P99Latency == nil || *search.P99Latency != 27.9 {
		t.Errorf("team-search latency mapping wrong: %+v", *search)
	}
	if !body.PrometheusAvailable {
		t.Error("prometheus_available should be true when Prometheus answers")
	}
	if search.Status != "HEALTHY" {
		t.Errorf("team-search status: want HEALTHY, got %s", search.Status)
	}
	if body.TotalRPS < 42.5 {
		t.Errorf("total_rps should include search RPS, got %v", body.TotalRPS)
	}
}

// TestCockpitNeverFabricatesBusinessNumbersOrTraces: there is no order-domain or
// trace source, so orders/revenue must be JSON null and recent_traces empty —
// in both degraded and Prometheus-backed modes. No invented placeholder figures.
func TestCockpitNeverFabricatesBusinessNumbersOrTraces(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer prom.Close()

	for name, h := range map[string]http.Handler{
		"degraded": edge.NewCockpitHandler(""),
		"prom":     edge.NewCockpitHandler(prom.URL),
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		for _, k := range []string{"total_orders_24h", "total_revenue_24h"} {
			if string(raw[k]) != "null" {
				t.Errorf("%s: %s must be null, got %s", name, k, raw[k])
			}
		}
		if string(raw["recent_traces"]) != "[]" {
			t.Errorf("%s: recent_traces must be [], got %s", name, raw["recent_traces"])
		}
	}
}

// TestCockpitEmptyPrometheusIsNoDataNotZero: Prometheus up but no series for a
// row → NO_DATA with null latency/error-rate, not a fabricated 0.0 / healthy.
func TestCockpitEmptyPrometheusIsNoDataNotZero(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer prom.Close()

	body := decodeCockpit(t, edge.NewCockpitHandler(prom.URL))
	if !body.PrometheusAvailable {
		t.Fatal("prometheus answered; prometheus_available should be true")
	}
	if body.AvgLatencyMs != nil {
		t.Errorf("avg latency must be null without samples, got %v", *body.AvgLatencyMs)
	}
	for _, s := range body.Services {
		if s.P95Latency != nil || s.P99Latency != nil || s.ErrorRate != nil {
			t.Errorf("%s: want null latency/error without samples, got %+v", s.Name, s)
		}
		if s.Name != "team-gateway" && s.Status != "NO_DATA" {
			t.Errorf("%s: want NO_DATA without series, got %s", s.Name, s.Status)
		}
	}
}
