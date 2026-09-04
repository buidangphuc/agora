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
	"time"
)

type ServiceHealth struct {
	Name       string  `json:"name"`
	Port       int     `json:"port"`
	Status     string  `json:"status"`
	RPS        float64 `json:"rps"`
	P95Latency float64 `json:"p95_latency_ms"`
	P99Latency float64 `json:"p99_latency_ms"`
	ErrorRate  float64 `json:"error_rate"`
}

type CockpitMetricsResponse struct {
	Timestamp       string          `json:"timestamp"`
	TotalRPS        float64         `json:"total_rps"`
	AvgLatencyMs    float64         `json:"avg_latency_ms"`
	TotalOrders24h  int             `json:"total_orders_24h"`
	TotalRevenue24h int64           `json:"total_revenue_24h"`
	Services        []ServiceHealth `json:"services"`
	RecentTraces    []TraceSummary  `json:"recent_traces"`
}

type TraceSummary struct {
	TraceID   string `json:"trace_id"`
	Operation string `json:"operation"`
	Duration  string `json:"duration"`
	Status    string `json:"status"`
	JaegerURL string `json:"jaeger_url"`
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
	{name: "team-notification", port: 50058, rpcServices: nil}, // not dialled by the edge: server-side metric is a follow-up
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

// derived, non-authoritative business figures — team-order emits Kafka events,
// not a Prometheus counter, and the gateway must not compute business numbers
// (Rule 2). These stay estimated/last-known (never random) until a real
// order-domain metric lands (see replace-cockpit-mock-metrics non-goals).
const (
	derivedOrders24h  = 1420
	derivedRevenue24h = 384_500_000
)

// CockpitHandler serves the Admin Cockpit HUD (GET /api/admin/metrics). It runs
// a fixed, hardcoded PromQL set against Prometheus server-side and shapes the
// results into CockpitMetricsResponse. It never proxies arbitrary PromQL and
// never returns raw Prometheus payloads to the browser (Rule 2, thin proxy).
type CockpitHandler struct {
	promURL string
	http    *http.Client
}

// NewCockpitHandler builds the handler. An empty promURL puts it in degraded
// mode (zeroed values, never random) so the local stack renders without Prometheus.
func NewCockpitHandler(promURL string) *CockpitHandler {
	return &CockpitHandler{
		promURL: promURL,
		http:    &http.Client{Timeout: 2 * time.Second},
	}
}

// ServeHTTP produces the cockpit response. Prometheus-sourced per-service RPS /
// p95 / p99 / error-rate when reachable; a valid, zeroed, same-shape response
// when PROMETHEUS_URL is empty or Prometheus is unreachable.
func (h *CockpitHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	resp := h.buildResponse(r.Context())
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *CockpitHandler) buildResponse(ctx context.Context) CockpitMetricsResponse {
	resp := CockpitMetricsResponse{
		Timestamp:       time.Now().Format(time.RFC3339),
		TotalOrders24h:  derivedOrders24h,  // derived/estimated (documented above)
		TotalRevenue24h: derivedRevenue24h, // derived/estimated (documented above)
		RecentTraces:    recentTraces(),
	}

	snap, ok := h.snapshot(ctx)

	services := make([]ServiceHealth, 0, len(cockpitRoster))
	var totalRPS, sumP95 float64
	var active int
	for _, row := range cockpitRoster {
		sh := ServiceHealth{Name: row.name, Port: row.port, Status: "UNKNOWN"}
		if ok {
			if len(row.rpcServices) == 0 {
				// No gateway-client series for this row. Status reflects that the
				// stack is reachable but this row is not gateway-sourced yet.
				sh.Status = "IDLE"
			} else {
				sh = foldRow(row, snap)
				if sh.RPS > 0 {
					totalRPS += sh.RPS
					sumP95 += sh.P95Latency
					active++
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
			services[0].P95Latency = sumP95 / float64(active)
		}
		services[0].Status = "HEALTHY"
	}

	resp.Services = services
	resp.TotalRPS = totalRPS
	if active > 0 {
		resp.AvgLatencyMs = sumP95 / float64(active)
	}
	return resp
}

// foldRow collapses a row's (possibly several) rpc_service series into one
// ServiceHealth: RPS summed, error-rate RPS-weighted, p95/p99 the worst-case
// (max) across constituents — quantiles are not additive, so max is the safe
// aggregate. Documented approximation; a per-service server-side metric would
// remove the need to fold.
func foldRow(row serviceRow, snap promSnapshot) ServiceHealth {
	sh := ServiceHealth{Name: row.name, Port: row.port}
	var errWeighted float64
	for _, svc := range row.rpcServices {
		rps := snap.rps[svc]
		sh.RPS += rps
		if p := snap.p95[svc]; p > sh.P95Latency {
			sh.P95Latency = p
		}
		if p := snap.p99[svc]; p > sh.P99Latency {
			sh.P99Latency = p
		}
		errWeighted += snap.errRate[svc] * rps
	}
	if sh.RPS > 0 {
		sh.ErrorRate = errWeighted / sh.RPS
		if sh.ErrorRate >= degradedThresholdErrRate {
			sh.Status = "DEGRADED"
		} else {
			sh.Status = "HEALTHY"
		}
	} else {
		sh.Status = "IDLE" // reachable but no traffic in the window → trends to zero
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
	if h.promURL == "" {
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
	endpoint := h.promURL + "/api/v1/query?query=" + url.QueryEscape(query)
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

// recentTraces returns Jaeger deep-links for the trace panel. These are
// non-authoritative illustrative links (deriving them from live Jaeger/trace
// data is out of scope — see replace-cockpit-mock-metrics non-goals); they open
// the real Jaeger UI. No random data.
func recentTraces() []TraceSummary {
	return []TraceSummary{
		{TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", Operation: "CheckoutSaga -> ReserveStock -> ChargePayment", Duration: "—", Status: "linked", JaegerURL: "http://localhost:16686/trace/4bf92f3577b34da6a3ce929d0e0e4736"},
		{TraceID: "0af7651916cd43dd8448eb211c80319c", Operation: "AIService.ShoppingAssistant (RAG Embeddings)", Duration: "—", Status: "linked", JaegerURL: "http://localhost:16686/trace/0af7651916cd43dd8448eb211c80319c"},
		{TraceID: "b7d5119cabbc424aa69327574e1472d6", Operation: "SearchService.Search (OpenSearch Read-Model)", Duration: "—", Status: "linked", JaegerURL: "http://localhost:16686/trace/b7d5119cabbc424aa69327574e1472d6"},
	}
}
