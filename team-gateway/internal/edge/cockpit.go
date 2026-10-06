package edge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
)

// ServiceHealth is one cockpit row. RPS is a plain number (an absent rate series
// genuinely means no calls). The latency and error-rate fields are pointers: nil
// serialises to JSON null and means "no sample in the window / source
// unavailable" — never a fabricated 0. Status is one of HEALTHY, DEGRADED, IDLE
// (reachable, no traffic), NO_DATA (no gateway-side series for this row) or
// UNKNOWN (Prometheus unreachable).
type ServiceHealth struct {
	Name       string   `json:"name"`
	Port       int      `json:"port"`
	Status     string   `json:"status"`
	RPS        float64  `json:"rps"`
	P95Latency *float64 `json:"p95_latency_ms"`
	P99Latency *float64 `json:"p99_latency_ms"`
	ErrorRate  *float64 `json:"error_rate"`
}

// CockpitMetricsResponse is the shaped cockpit payload. PrometheusAvailable is
// false when PROMETHEUS_URL is unset/unreachable — consumers must then render an
// unavailable state rather than the (meaningless) zero RPS. TotalOrders24h and
// TotalRevenue24h come from team-analytics' order_facts and are null when it is
// unavailable (never 0-as-real); RecentOrders is empty then. RecentTraces comes
// from the Jaeger query API and is empty when Jaeger is unset/unreachable.
type CockpitMetricsResponse struct {
	Timestamp           string          `json:"timestamp"`
	PrometheusAvailable bool            `json:"prometheus_available"`
	TotalRPS            float64         `json:"total_rps"`
	AvgLatencyMs        *float64        `json:"avg_latency_ms"`
	TotalOrders24h      *int            `json:"total_orders_24h"`
	TotalRevenue24h     *int64          `json:"total_revenue_24h"`
	Services            []ServiceHealth `json:"services"`
	RecentOrders        []RecentOrder   `json:"recent_orders"`
	RecentTraces        []TraceSummary  `json:"recent_traces"`
}

// RecentOrder is one paid order as reported by team-analytics (minor units, no
// buyer PII). The gateway copies it; it computes nothing.
type RecentOrder struct {
	OrderID  string `json:"order_id"`
	SellerID string `json:"seller_id"`
	Total    int64  `json:"total"`
	PaidAt   string `json:"paid_at"`
}

// TraceSummary is one recent gateway trace from Jaeger.
type TraceSummary struct {
	TraceID    string  `json:"trace_id"`
	Operation  string  `json:"operation"`
	SpanCount  int     `json:"span_count"`
	DurationMs float64 `json:"duration_ms"`
	StartedAt  string  `json:"started_at"`
	JaegerURL  string  `json:"jaeger_url"`
}

// serviceRow describes one cockpit row: the friendly service name + port the HUD
// shows, and the OTel gRPC `rpc_service` label(s) the gateway's otelgrpc client
// instrumentation emits for it (see add-prometheus-infra). The gateway legitimately
// holds this routing knowledge (Rules 1 & 2) — it is the same upstream table as
// internal/upstream, not business logic. Rows with no rpcServices have no
// gateway-client RED series (the edge does not dial them, or is itself the edge),
// so they are sourced as derived/zero and documented as such.
type serviceRow struct {
	name        string
	port        int
	rpcServices []string
}

// cockpitRoster is the fixed set of rows the HUD renders, in display order. The
// same 10 services as before; the rpc_service mappings mirror internal/upstream.
var cockpitRoster = []serviceRow{
	{name: "team-gateway", port: 8080, rpcServices: nil}, // the edge itself: aggregate-derived (sum of upstreams)
	{name: "team-domain", port: 50051, rpcServices: []string{"platform.listing.v1.ListingService"}},
	{name: "team-search", port: 50052, rpcServices: []string{"platform.search.v1.SearchService"}},
	{name: "team-identity", port: 50053, rpcServices: []string{"platform.identity.v1.AuthService", "platform.identity.v1.AddressService"}},
	{name: "team-engagement", port: 50054, rpcServices: []string{"platform.engagement.v1.EngagementService"}},
	{name: "team-order", port: 50055, rpcServices: []string{"platform.order.v1.CartService", "platform.order.v1.OrderService"}},
	{name: "team-payment", port: 50056, rpcServices: []string{"platform.payment.v1.PaymentService"}},
	{name: "team-chat", port: 50057, rpcServices: []string{"platform.chat.v1.ChatService"}},
	{name: "team-notification", port: 50058, rpcServices: []string{"platform.notification.v1.NotificationService"}},
	{name: "team-ai", port: 8000, rpcServices: []string{"platform.ai.v1.AIService"}},
}

const (
	// Base name of the gateway's OTel gRPC client duration histogram as rendered
	// by the collector's Prometheus exporter (namespace `marketplace`, OTel
	// `rpc.client.duration` unit ms → `_milliseconds`). The exact rendered name
	// settles when add-prometheus-infra is applied against the live collector; if
	// it differs, adjust this constant (and the label constants below).
	rpcHistogram = "marketplace_rpc_client_duration_milliseconds"
	// Label carrying the upstream gRPC service (from OTel `rpc.service`).
	rpcServiceLabel = "rpc_service"
	// Label carrying the numeric gRPC status; "0" == OK.
	rpcStatusLabel = "rpc_grpc_status_code"

	degradedThresholdErrRate = 0.05 // error-rate at/above which a row is DEGRADED
)

// CockpitConfig carries the cockpit's fixed upstream endpoints. Each empty
// value disables that source (its figures render as unavailable, never random).
type CockpitConfig struct {
	PrometheusURL  string // PROMETHEUS_URL
	JaegerQueryURL string // JAEGER_QUERY_URL (server-side only)
	JaegerUIURL    string // JAEGER_UI_URL (browser-facing base for trace links)
}

const (
	// adminScope is the scope identity grants the admin role; the cockpit needs it.
	adminScope = "admin"
	// Fixed cockpit queries: the browser supplies none of these.
	ordersWindow = 24 * time.Hour
	recentOrders = 5
)

// CockpitHandler serves the Admin Cockpit HUD (GET /api/admin/metrics). It
// requires the `admin` scope (verified once by the edge), runs a fixed,
// hardcoded PromQL set against Prometheus, asks team-analytics for the order
// figures and Jaeger for recent traces, and shapes the results into
// CockpitMetricsResponse. It never proxies arbitrary queries and never returns
// raw upstream payloads to the browser (Rule 2, thin proxy).
type CockpitHandler struct {
	edge      *Edge
	cfg       CockpitConfig
	analytics analyticsv1.AnalyticsQueryServiceClient // nil → orders unavailable
	http      *http.Client
}

// NewCockpitHandler builds the handler. Empty URLs / a nil analytics client put
// the matching source in degraded mode so the local stack renders without it.
func NewCockpitHandler(e *Edge, cfg CockpitConfig, analytics analyticsv1.AnalyticsQueryServiceClient) *CockpitHandler {
	return &CockpitHandler{
		edge:      e,
		cfg:       cfg,
		analytics: analytics,
		http:      &http.Client{Timeout: 2 * time.Second},
	}
}

// ServeHTTP gates on the admin scope, then produces the cockpit response. No
// token → 401, a token without `admin` → 403, both before any upstream call.
func (h *CockpitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p, err := h.edge.resolve(r.Header)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeCockpitError(w, http.StatusUnauthorized, "invalid or expired bearer token")
		return
	}
	if p.ptype == "anonymous" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeCockpitError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	if !hasScope(p.scopes, adminScope) {
		writeCockpitError(w, http.StatusForbidden, "insufficient_scope: admin required")
		return
	}

	ctx := withPrincipal(r.Context(), p)
	resp := h.buildResponse(ctx, r.Header)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeCockpitError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// buildResponse fans the three independent sources out concurrently; each one
// degrades on its own.
func (h *CockpitHandler) buildResponse(ctx context.Context, header http.Header) CockpitMetricsResponse {
	resp := CockpitMetricsResponse{
		Timestamp:    time.Now().Format(time.RFC3339),
		RecentOrders: []RecentOrder{},
		RecentTraces: []TraceSummary{},
	}

	var (
		wg   sync.WaitGroup
		snap promSnapshot
		ok   bool
	)
	wg.Add(3)
	go func() { defer wg.Done(); snap, ok = h.snapshot(ctx) }()
	go func() { defer wg.Done(); h.fillOrders(ctx, header, &resp) }()
	go func() {
		defer wg.Done()
		if traces := h.recentTraces(ctx); traces != nil {
			resp.RecentTraces = traces
		}
	}()
	wg.Wait()
	resp.PrometheusAvailable = ok

	services := make([]ServiceHealth, 0, len(cockpitRoster))
	var totalRPS, sumP95 float64
	var active int
	for _, row := range cockpitRoster {
		sh := ServiceHealth{Name: row.name, Port: row.port, Status: "UNKNOWN"}
		if ok {
			if len(row.rpcServices) == 0 {
				// No gateway-client series for this row (the edge itself, or a
				// service it does not dial): honestly "no data", not idle.
				sh.Status = "NO_DATA"
			} else {
				sh = foldRow(row, snap)
				if sh.RPS > 0 {
					totalRPS += sh.RPS
					if sh.P95Latency != nil {
						sumP95 += *sh.P95Latency
						active++
					}
				}
			}
		}
		services = append(services, sh)
	}

	// team-gateway (the edge itself) has no client series — surface it as the
	// aggregate of everything it routes, so the "Total Gateway Throughput" row is
	// real (derived from real upstream series), not fabricated.
	if ok && len(services) > 0 && services[0].Name == "team-gateway" {
		services[0].RPS = totalRPS
		if active > 0 {
			avg := sumP95 / float64(active)
			services[0].P95Latency = &avg
		}
		services[0].Status = "HEALTHY"
	}

	resp.Services = services
	resp.TotalRPS = totalRPS
	if active > 0 {
		avg := sumP95 / float64(active)
		resp.AvgLatencyMs = &avg
	}
	return resp
}

// fillOrders copies team-analytics' admin RPC results into resp, forwarding the
// caller's principal (analytics enforces `admin` again). On any failure the
// matching fields stay null/empty — the gateway never computes or invents them.
func (h *CockpitHandler) fillOrders(ctx context.Context, header http.Header, resp *CockpitMetricsResponse) {
	if h.analytics == nil {
		return
	}
	out := h.edge.outgoing(ctx, header)

	var sum *analyticsv1.GetPlatformOrderSummaryResponse
	var list *analyticsv1.ListRecentOrdersResponse
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = h.edge.callRead(out, func(c context.Context) error {
			var e error
			sum, e = h.analytics.GetPlatformOrderSummary(c, &analyticsv1.GetPlatformOrderSummaryRequest{
				Window: durationpb.New(ordersWindow),
			})
			return e
		})
	}()
	go func() {
		defer wg.Done()
		_ = h.edge.callRead(out, func(c context.Context) error {
			var e error
			list, e = h.analytics.ListRecentOrders(c, &analyticsv1.ListRecentOrdersRequest{Limit: recentOrders})
			return e
		})
	}()
	wg.Wait()

	if sum != nil {
		n := int(sum.GetOrderCount())
		gmv := sum.GetGmv()
		resp.TotalOrders24h = &n
		resp.TotalRevenue24h = &gmv
	}
	if list != nil {
		for _, o := range list.GetOrders() {
			resp.RecentOrders = append(resp.RecentOrders, RecentOrder{
				OrderID:  o.GetOrderId(),
				SellerID: o.GetSellerId(),
				Total:    o.GetTotal(),
				PaidAt:   o.GetPaidAt().AsTime().UTC().Format(time.RFC3339),
			})
		}
	}
}

// foldRow collapses a row's (possibly several) rpc_service series into one
// ServiceHealth: RPS summed, error-rate RPS-weighted, p95/p99 the worst-case
// (max) across constituents — quantiles are not additive, so max is the safe
// aggregate. Documented approximation; a per-service server-side metric would
// remove the need to fold.
func foldRow(row serviceRow, snap promSnapshot) ServiceHealth {
	sh := ServiceHealth{Name: row.name, Port: row.port}
	var errWeighted float64
	var seen bool
	for _, svc := range row.rpcServices {
		rps, has := snap.rps[svc]
		if has {
			seen = true
		}
		sh.RPS += rps
		if p, ok := snap.p95[svc]; ok && (sh.P95Latency == nil || p > *sh.P95Latency) {
			sh.P95Latency = &p
		}
		if p, ok := snap.p99[svc]; ok && (sh.P99Latency == nil || p > *sh.P99Latency) {
			sh.P99Latency = &p
		}
		errWeighted += snap.errRate[svc] * rps
	}
	switch {
	case !seen:
		sh.Status = "NO_DATA" // the series has never been scraped for this row
	case sh.RPS > 0:
		// An absent error-rate series with traffic means zero errors (the
		// numerator vector is empty), so 0 is a real value here.
		er := errWeighted / sh.RPS
		sh.ErrorRate = &er
		if er >= degradedThresholdErrRate {
			sh.Status = "DEGRADED"
		} else {
			sh.Status = "HEALTHY"
		}
	default:
		sh.Status = "IDLE" // reachable but no traffic in the window → error-rate undefined (null)
	}
	return sh
}

// promSnapshot holds one poll of the fixed PromQL set, keyed by rpc_service.
type promSnapshot struct {
	rps     map[string]float64
	p95     map[string]float64
	p99     map[string]float64
	errRate map[string]float64
}

// snapshot runs the fixed query set. Returns ok=false (→ degraded/zeroed shape,
// never random) when PROMETHEUS_URL is unset or Prometheus is unreachable.
func (h *CockpitHandler) snapshot(ctx context.Context) (promSnapshot, bool) {
	if h.cfg.PrometheusURL == "" {
		return promSnapshot{}, false
	}
	rps, err := h.queryVector(ctx, fmt.Sprintf(
		"sum by (%s) (rate(%s_count[1m]))", rpcServiceLabel, rpcHistogram))
	if err != nil {
		return promSnapshot{}, false // unreachable → degrade gracefully
	}
	p95, err := h.queryVector(ctx, fmt.Sprintf(
		"histogram_quantile(0.95, sum by (le, %s) (rate(%s_bucket[5m])))", rpcServiceLabel, rpcHistogram))
	if err != nil {
		return promSnapshot{}, false
	}
	p99, err := h.queryVector(ctx, fmt.Sprintf(
		"histogram_quantile(0.99, sum by (le, %s) (rate(%s_bucket[5m])))", rpcServiceLabel, rpcHistogram))
	if err != nil {
		return promSnapshot{}, false
	}
	errRate, err := h.queryVector(ctx, fmt.Sprintf(
		"sum by (%[1]s) (rate(%[2]s_count{%[3]s!=\"0\"}[5m])) / sum by (%[1]s) (rate(%[2]s_count[5m]))",
		rpcServiceLabel, rpcHistogram, rpcStatusLabel))
	if err != nil {
		return promSnapshot{}, false
	}
	return promSnapshot{rps: rps, p95: p95, p99: p99, errRate: errRate}, true
}

// promQueryResponse is the subset of the Prometheus HTTP API instant-query
// response we consume. The gateway parses it server-side and never forwards it.
type promQueryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string  `json:"metric"`
			Value  [2]json.RawMessage `json:"value"` // [ <unix_ts>, "<sample>" ]
		} `json:"result"`
	} `json:"data"`
}

// queryVector runs one instant PromQL query and returns rpc_service → sample.
// Samples that are NaN/absent are skipped so idle series read as zero, not junk.
func (h *CockpitHandler) queryVector(ctx context.Context, query string) (map[string]float64, error) {
	endpoint := h.cfg.PrometheusURL + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := h.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("prometheus query %d: %s", res.StatusCode, string(body))
	}

	var pr promQueryResponse
	if err := json.NewDecoder(res.Body).Decode(&pr); err != nil {
		return nil, err
	}
	if pr.Status != "success" {
		return nil, fmt.Errorf("prometheus status %q", pr.Status)
	}

	out := make(map[string]float64, len(pr.Data.Result))
	for _, s := range pr.Data.Result {
		svc := s.Metric[rpcServiceLabel]
		if svc == "" {
			continue
		}
		// value[1] is a JSON string like "12.34"; unquote then parse.
		var raw string
		if err := json.Unmarshal(s.Value[1], &raw); err != nil {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		// ParseFloat accepts "NaN"/"+Inf"; skip non-finite samples (empty
		// histogram_quantile buckets) so idle series read as zero and JSON
		// encoding never fails on a NaN.
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out[svc] = v
	}
	return out, nil
}
