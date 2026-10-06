package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

// cockpitFixture holds a JWKS-backed Edge so the handler's admin gate runs the
// real resolve path (RS256 verify), plus tokens for each persona.
type cockpitFixture struct {
	edge *edge.Edge
	key  *rsa.PrivateKey
}

func newCockpitFixture(t *testing.T) *cockpitFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read", "search:read"}, time.Second, 0, 1000, 1000)
	return &cockpitFixture{edge: e, key: key}
}

func (f *cockpitFixture) token(t *testing.T, scopes ...string) string {
	t.Helper()
	claims := &token.Claims{
		Type:   "user",
		Scopes: scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *cockpitFixture) handler(cfg edge.CockpitConfig, a analyticsv1.AnalyticsQueryServiceClient) http.Handler {
	return edge.NewCockpitHandler(f.edge, cfg, a)
}

// get calls the handler with an optional Authorization header.
func (f *cockpitFixture) get(h http.Handler, auth string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/admin/metrics", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// decodeCockpit runs the handler as an admin and decodes its JSON body,
// asserting the content type and status the frontend contract depends on.
func decodeCockpit(t *testing.T, f *cockpitFixture, h http.Handler) edge.CockpitMetricsResponse {
	t.Helper()
	w := f.get(h, "Bearer "+f.token(t, "admin"))

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
	f := newCockpitFixture(t)
	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{}, nil))

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
	f := newCockpitFixture(t)
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

	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{PrometheusURL: prom.URL}, nil))

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

// TestCockpitNeverFabricatesBusinessNumbersOrTraces: with no analytics client and
// no Jaeger configured, orders/revenue must be JSON null and the lists empty —
// in both degraded and Prometheus-backed modes. No invented placeholder figures.
func TestCockpitNeverFabricatesBusinessNumbersOrTraces(t *testing.T) {
	f := newCockpitFixture(t)
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer prom.Close()

	for name, h := range map[string]http.Handler{
		"degraded": f.handler(edge.CockpitConfig{}, nil),
		"prom":     f.handler(edge.CockpitConfig{PrometheusURL: prom.URL}, nil),
	} {
		w := f.get(h, "Bearer "+f.token(t, "admin"))
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		for _, k := range []string{"total_orders_24h", "total_revenue_24h"} {
			if string(raw[k]) != "null" {
				t.Errorf("%s: %s must be null, got %s", name, k, raw[k])
			}
		}
		for _, k := range []string{"recent_traces", "recent_orders"} {
			if string(raw[k]) != "[]" {
				t.Errorf("%s: %s must be [], got %s", name, k, raw[k])
			}
		}
	}
}

// TestCockpitEmptyPrometheusIsNoDataNotZero: Prometheus up but no series for a
// row → NO_DATA with null latency/error-rate, not a fabricated 0.0 / healthy.
func TestCockpitEmptyPrometheusIsNoDataNotZero(t *testing.T) {
	f := newCockpitFixture(t)
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer prom.Close()

	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{PrometheusURL: prom.URL}, nil))
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

// fakeAnalytics stands in for team-analytics' AnalyticsQueryService; it records
// the principal metadata it was called with.
type fakeAnalytics struct {
	analyticsv1.AnalyticsQueryServiceClient
	calls   atomic.Int32
	err     error
	summary *analyticsv1.GetPlatformOrderSummaryResponse
	recent  *analyticsv1.ListRecentOrdersResponse
	window  time.Duration
	limit   int32
	md      metadata.MD
}

func (a *fakeAnalytics) GetPlatformOrderSummary(ctx context.Context, in *analyticsv1.GetPlatformOrderSummaryRequest, _ ...grpc.CallOption) (*analyticsv1.GetPlatformOrderSummaryResponse, error) {
	a.calls.Add(1)
	a.window = in.GetWindow().AsDuration()
	a.md, _ = metadata.FromOutgoingContext(ctx)
	if a.err != nil {
		return nil, a.err
	}
	return a.summary, nil
}

func (a *fakeAnalytics) ListRecentOrders(_ context.Context, in *analyticsv1.ListRecentOrdersRequest, _ ...grpc.CallOption) (*analyticsv1.ListRecentOrdersResponse, error) {
	a.calls.Add(1)
	a.limit = in.GetLimit()
	if a.err != nil {
		return nil, a.err
	}
	return a.recent, nil
}

// upstreamCounter is an httptest server that counts hits, standing in for
// Prometheus / Jaeger in the "no upstream call before the gate" assertions.
func upstreamCounter(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// TestCockpitAdminGate: no/invalid token → 401, a token without admin → 403,
// admin → 200; for 401 and 403 no upstream (Prometheus, Jaeger, analytics) is
// called.
func TestCockpitAdminGate(t *testing.T) {
	f := newCockpitFixture(t)
	prom, promHits := upstreamCounter(t, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	jaeger, jaegerHits := upstreamCounter(t, `{"data":[]}`)
	fa := &fakeAnalytics{summary: &analyticsv1.GetPlatformOrderSummaryResponse{}, recent: &analyticsv1.ListRecentOrdersResponse{}}
	h := f.handler(edge.CockpitConfig{PrometheusURL: prom.URL, JaegerQueryURL: jaeger.URL}, fa)

	cases := []struct {
		name, auth string
		want       int
		upstream   bool
	}{
		{"anonymous", "", http.StatusUnauthorized, false},
		{"garbage token", "Bearer not-a-jwt", http.StatusUnauthorized, false},
		{"empty bearer", "Bearer ", http.StatusUnauthorized, false},
		{"buyer", "Bearer " + f.token(t, "order.read", "search:read"), http.StatusForbidden, false},
		{"admin", "Bearer " + f.token(t, "admin"), http.StatusOK, true},
	}
	for _, c := range cases {
		promHits.Store(0)
		jaegerHits.Store(0)
		fa.calls.Store(0)
		w := f.get(h, c.auth)
		if w.Code != c.want {
			t.Errorf("%s: status = %d, want %d", c.name, w.Code, c.want)
		}
		called := promHits.Load() > 0 || jaegerHits.Load() > 0 || fa.calls.Load() > 0
		if called != c.upstream {
			t.Errorf("%s: upstream called = %v, want %v (prom=%d jaeger=%d analytics=%d)",
				c.name, called, c.upstream, promHits.Load(), jaegerHits.Load(), fa.calls.Load())
		}
		if !c.upstream && strings.Contains(w.Body.String(), "total_rps") {
			t.Errorf("%s: rejected response must not carry metrics: %s", c.name, w.Body.String())
		}
	}
}

// TestCockpitOrdersFromAnalytics: the handler copies the analytics RPC results
// verbatim, asks for the fixed 24h window / limit 5, and forwards the admin
// principal so analytics can enforce the scope again.
func TestCockpitOrdersFromAnalytics(t *testing.T) {
	f := newCockpitFixture(t)
	paid := time.Date(2026, 10, 5, 8, 30, 0, 0, time.UTC)
	fa := &fakeAnalytics{
		summary: &analyticsv1.GetPlatformOrderSummaryResponse{OrderCount: 7, Gmv: 2100000},
		recent: &analyticsv1.ListRecentOrdersResponse{Orders: []*analyticsv1.RecentOrder{
			{OrderId: "o-9", SellerId: "s-1", Total: 300000, PaidAt: timestamppb.New(paid)},
			{OrderId: "o-8", SellerId: "s-2", Total: 50000, PaidAt: timestamppb.New(paid.Add(-time.Minute))},
		}},
	}
	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{}, fa))

	if body.TotalOrders24h == nil || *body.TotalOrders24h != 7 {
		t.Errorf("total_orders_24h = %v, want 7", body.TotalOrders24h)
	}
	if body.TotalRevenue24h == nil || *body.TotalRevenue24h != 2100000 {
		t.Errorf("total_revenue_24h = %v, want 2100000", body.TotalRevenue24h)
	}
	if len(body.RecentOrders) != 2 || body.RecentOrders[0] != (edge.RecentOrder{
		OrderID: "o-9", SellerID: "s-1", Total: 300000, PaidAt: "2026-10-05T08:30:00Z",
	}) {
		t.Errorf("recent_orders = %+v", body.RecentOrders)
	}
	if fa.window != 24*time.Hour || fa.limit != 5 {
		t.Errorf("fixed query = window %v limit %d, want 24h / 5", fa.window, fa.limit)
	}
	if got := fa.md.Get("x-principal-scopes"); len(got) != 1 || got[0] != "admin" {
		t.Errorf("forwarded principal scopes = %v, want [admin]", got)
	}
}

// TestCockpitAnalyticsUnavailable: when team-analytics fails the figures are
// null (not 0), recent_orders is empty, and the rest of the response still
// comes back.
func TestCockpitAnalyticsUnavailable(t *testing.T) {
	f := newCockpitFixture(t)
	fa := &fakeAnalytics{err: status.Error(codes.Unavailable, "down")}
	w := f.get(f.handler(edge.CockpitConfig{}, fa), "Bearer "+f.token(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"total_orders_24h", "total_revenue_24h"} {
		if string(raw[k]) != "null" {
			t.Errorf("%s = %s, want null", k, raw[k])
		}
	}
	if string(raw["recent_orders"]) != "[]" {
		t.Errorf("recent_orders = %s, want []", raw["recent_orders"])
	}
	if _, ok := raw["services"]; !ok {
		t.Error("services must still be returned")
	}

	// A permission denial from analytics is also "unavailable", never a number.
	fa = &fakeAnalytics{err: errors.New("boom")}
	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{}, fa))
	if body.TotalOrders24h != nil || body.TotalRevenue24h != nil {
		t.Errorf("figures must be nil on error, got %v / %v", body.TotalOrders24h, body.TotalRevenue24h)
	}
}

const jaegerFixture = `{"data":[
 {"traceID":"aaa111","spans":[
   {"spanID":"s1","operationName":"child","startTime":1759650000100000,"duration":300000,"references":[{"refType":"CHILD_OF","spanID":"s0"}]},
   {"spanID":"s0","operationName":"POST /api/orders","startTime":1759650000000000,"duration":500000,"references":[]}]},
 {"traceID":"bbb222","spans":[
   {"spanID":"t0","operationName":"GET /api/search","startTime":1759650100000000,"duration":20000,"references":[]}]}
]}`

// TestCockpitTracesFromJaeger: the fixed query is sent, traces are shaped
// (root operation, span count, duration, start, UI link) newest first.
func TestCockpitTracesFromJaeger(t *testing.T) {
	f := newCockpitFixture(t)
	var gotQuery string
	jaeger := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = w.Write([]byte(jaegerFixture))
	}))
	defer jaeger.Close()

	body := decodeCockpit(t, f, f.handler(edge.CockpitConfig{
		JaegerQueryURL: jaeger.URL, JaegerUIURL: "http://localhost:16686/",
	}, nil))

	for _, want := range []string{"service=team-gateway", "lookback=1h", "limit=5"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("jaeger query %q missing %q", gotQuery, want)
		}
	}
	if len(body.RecentTraces) != 2 {
		t.Fatalf("recent_traces len = %d, want 2", len(body.RecentTraces))
	}
	newest, older := body.RecentTraces[0], body.RecentTraces[1]
	if newest.TraceID != "bbb222" || newest.Operation != "GET /api/search" || newest.SpanCount != 1 || newest.DurationMs != 20 {
		t.Errorf("newest trace = %+v", newest)
	}
	if older.TraceID != "aaa111" || older.Operation != "POST /api/orders" || older.SpanCount != 2 || older.DurationMs != 500 {
		t.Errorf("older trace = %+v (root op must be the parentless span)", older)
	}
	if older.JaegerURL != "http://localhost:16686/trace/aaa111" {
		t.Errorf("jaeger_url = %q", older.JaegerURL)
	}
	if older.StartedAt != "2025-10-05T07:40:00Z" {
		t.Errorf("started_at = %q", older.StartedAt)
	}
}

// TestCockpitJaegerUnavailable: unset, down, non-200 and garbage all leave
// recent_traces empty and the response intact.
func TestCockpitJaegerUnavailable(t *testing.T) {
	f := newCockpitFixture(t)
	down := httptest.NewServer(http.NotFoundHandler())
	downURL := down.URL
	down.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer bad.Close()
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()

	for name, url := range map[string]string{"unset": "", "down": downURL, "garbage": bad.URL, "404": notFound.URL} {
		w := f.get(f.handler(edge.CockpitConfig{JaegerQueryURL: url}, nil), "Bearer "+f.token(t, "admin"))
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(raw["recent_traces"]) != "[]" {
			t.Errorf("%s: recent_traces = %s, want []", name, raw["recent_traces"])
		}
		if _, ok := raw["services"]; !ok {
			t.Errorf("%s: services missing", name)
		}
	}
}
