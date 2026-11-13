package rpc_test

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/forecaststore"
	"github.com/buidangphuc/team-analytics/internal/rpc"
)

const fcKey = "fc:v1:seller:s1:l1"

func fcPayload(model string, cold bool, pts []string) string {
	return fmt.Sprintf(`{"seller_id":"s1","listing_id":"l1","model_version":%q,"generated_at":"2026-11-13T02:45:00Z","is_cold_start":%v,"daily_forecasts":[%s]}`,
		model, cold, strings.Join(pts, ","))
}

func fcPoints(n int, p10, p50, p90 int) []string {
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, fmt.Sprintf(`{"date":"2026-11-%02d","p10":%d,"p50":%d,"p90":%d}`, 14+i, p10+i, p50+i, p90+i))
	}
	return out
}

func sellerCtx() context.Context { return asPrincipal("s1", "user", "seller") }

func forecastReq(horizon, lead int32, level float64) *analyticsv1.GetDemandForecastRequest {
	return &analyticsv1.GetDemandForecastRequest{SellerId: "s1", ListingId: "l1", HorizonDays: horizon, LeadTimeDays: lead, ServiceLevel: level}
}

// Scenario: A forecast key is served as written
func TestGetDemandForecastServedAsWritten(t *testing.T) {
	f := startFakeRedis(t)
	f.set(fcKey, fcPayload("seasonal_naive_v1", false, fcPoints(14, 1, 3, 5)))
	repo := &spyRepo{}
	h := mustHandler(repo, rpc.WithForecastStore(f.store(t)))
	resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 0, 0))
	if err != nil {
		t.Fatalf("GetDemandForecast: %v", err)
	}
	if got := resp.GetDailyForecasts(); len(got) != 14 {
		t.Fatalf("points = %d, want 14", len(got))
	} else if got[0].GetDate() != "2026-11-14" || got[0].GetP10() != 1 || got[0].GetP50() != 3 || got[0].GetP90() != 5 || got[13].GetP50() != 16 {
		t.Errorf("points not as stored: first=%v last=%v", got[0], got[13])
	}
	if resp.GetModelVersion() != "seasonal_naive_v1" || resp.GetIsColdStart() {
		t.Errorf("model=%q cold=%v", resp.GetModelVersion(), resp.GetIsColdStart())
	}
	want := time.Date(2026, 11, 13, 2, 45, 0, 0, time.UTC)
	if resp.GetGeneratedAt() == nil || !resp.GetGeneratedAt().AsTime().Equal(want) {
		t.Errorf("generated_at = %v, want %v", resp.GetGeneratedAt(), want)
	}
	if calls := f.getCalls(); len(calls) != 1 || calls[0] != fcKey {
		t.Errorf("redis reads = %v, want exactly [%s]", calls, fcKey)
	}
	if repo.calls != 0 {
		t.Errorf("warehouse repository calls = %d, want 0", repo.calls)
	}
}

// Scenario: The horizon truncates and never pads
func TestGetDemandForecastHorizonTruncatesNeverPads(t *testing.T) {
	f := startFakeRedis(t)
	f.set(fcKey, fcPayload("seasonal_naive_v1", false, fcPoints(14, 1, 3, 5)))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	cases := []struct {
		horizon int32
		want    int
	}{{7, 7}, {40, 14}, {0, 14}, {1, 1}}
	for _, c := range cases {
		resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(c.horizon, 0, 0))
		if err != nil {
			t.Fatalf("horizon %d: %v", c.horizon, err)
		}
		if got := len(resp.GetDailyForecasts()); got != c.want {
			t.Errorf("horizon %d: points = %d, want %d", c.horizon, got, c.want)
		}
		for i, p := range resp.GetDailyForecasts() {
			if p.GetP10() != float64(1+i) {
				t.Errorf("horizon %d: point %d = %v, not the stored point", c.horizon, i, p)
			}
		}
	}
}

// Scenario: The horizon truncates and never pads (default 14, clamp 28)
func TestGetDemandForecastHorizonDefaultAndClamp(t *testing.T) {
	f := startFakeRedis(t)
	f.set(fcKey, fcPayload("seasonal_naive_v1", false, fcPoints(30, 1, 3, 5)))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	for _, c := range []struct {
		horizon int32
		want    int
	}{{0, 14}, {40, 28}, {28, 28}} {
		resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(c.horizon, 0, 0))
		if err != nil {
			t.Fatalf("horizon %d: %v", c.horizon, err)
		}
		if got := len(resp.GetDailyForecasts()); got != c.want {
			t.Errorf("horizon %d: points = %d, want %d", c.horizon, got, c.want)
		}
	}
}

// Scenario: Reorder point and safety stock are derived from the stored points
func TestGetDemandForecastReorderPoint(t *testing.T) {
	f := startFakeRedis(t)
	var pts []string
	for i := 0; i < 14; i++ {
		pts = append(pts, fmt.Sprintf(`{"date":"2026-11-%02d","p10":2,"p50":4,"p90":6}`, 14+i))
	}
	f.set(fcKey, fcPayload("seasonal_naive_v1", false, pts))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 3, 0.95))
	if err != nil {
		t.Fatalf("GetDemandForecast: %v", err)
	}
	if math.Abs(resp.GetSafetyStock()-3.3) > 1e-9 {
		t.Errorf("safety_stock = %v, want 3.3", resp.GetSafetyStock())
	}
	if math.Abs(resp.GetSuggestedReorderPoint()-15.3) > 1e-9 {
		t.Errorf("suggested_reorder_point = %v, want 15.3", resp.GetSuggestedReorderPoint())
	}
}

// Scenario: An invalid listing or seller id is rejected
func TestGetDemandForecastRejectsBadIDs(t *testing.T) {
	f := startFakeRedis(t)
	f.set(fcKey, fcPayload("seasonal_naive_v1", false, fcPoints(14, 1, 3, 5)))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	admin := asPrincipal("a1", "user", "admin")
	cases := map[string]*analyticsv1.GetDemandForecastRequest{
		"wildcard listing": {SellerId: "s1", ListingId: "x:*"},
		"empty seller":     {SellerId: "", ListingId: "l1"},
		"empty listing":    {SellerId: "s1", ListingId: ""},
		"blank listing":    {SellerId: "s1", ListingId: "   "},
		"colon seller":     {SellerId: "s1:evil", ListingId: "l1"},
	}
	for name, req := range cases {
		if _, err := h.GetDemandForecast(admin, req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code = %v, want InvalidArgument", name, status.Code(err))
		}
	}
	if calls := f.getCalls(); len(calls) != 0 {
		t.Errorf("redis reads = %v, want none", calls)
	}
}

// Scenario: Another seller's forecast is denied
func TestGetDemandForecastDeniedWithoutRedisRead(t *testing.T) {
	f := startFakeRedis(t)
	f.set("fc:v1:seller:s2:l1", fcPayload("seasonal_naive_v1", false, fcPoints(14, 1, 3, 5)))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	req := &analyticsv1.GetDemandForecastRequest{SellerId: "s2", ListingId: "l1"}
	if _, err := h.GetDemandForecast(sellerCtx(), req); status.Code(err) != codes.PermissionDenied {
		t.Errorf("other seller: code = %v, want PermissionDenied", status.Code(err))
	}
	if _, err := h.GetDemandForecast(context.Background(), req); status.Code(err) != codes.Unauthenticated {
		t.Errorf("anonymous: code = %v, want Unauthenticated", status.Code(err))
	}
	if calls := f.getCalls(); len(calls) != 0 {
		t.Errorf("redis reads on deny = %v, want none", calls)
	}
	resp, err := h.GetDemandForecast(asPrincipal("a1", "user", "admin"), req)
	if err != nil || len(resp.GetDailyForecasts()) != 14 {
		t.Errorf("admin: err=%v points=%d, want 14", err, len(resp.GetDailyForecasts()))
	}
}

// Scenario: No key means cold start without points
func TestGetDemandForecastNoKeyColdStart(t *testing.T) {
	f := startFakeRedis(t)
	repo := &spyRepo{}
	h := mustHandler(repo, rpc.WithForecastStore(f.store(t)))
	resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 3, 0.95))
	if err != nil {
		t.Fatalf("GetDemandForecast: %v", err)
	}
	if !resp.GetIsColdStart() || len(resp.GetDailyForecasts()) != 0 || resp.GetSuggestedReorderPoint() != 0 || resp.GetSafetyStock() != 0 || resp.GetGeneratedAt() != nil {
		t.Errorf("want bare cold start, got %+v", resp)
	}
	if repo.calls != 0 {
		t.Errorf("warehouse repository calls = %d, want 0 (no order_facts fallback)", repo.calls)
	}
}

// Scenario: A key marked cold start carries no invented numbers
func TestGetDemandForecastStoredColdStart(t *testing.T) {
	f := startFakeRedis(t)
	f.set(fcKey, fcPayload("baseline_v0", true, fcPoints(5, 0, 1, 2)))
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
	resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 0, 0))
	if err != nil {
		t.Fatalf("GetDemandForecast: %v", err)
	}
	if !resp.GetIsColdStart() || resp.GetModelVersion() != "baseline_v0" || len(resp.GetDailyForecasts()) != 5 {
		t.Errorf("cold=%v model=%q points=%d, want true/baseline_v0/5", resp.GetIsColdStart(), resp.GetModelVersion(), len(resp.GetDailyForecasts()))
	}
}

// Scenario: Redis unreachable returns UNAVAILABLE
func TestGetDemandForecastStoreDown(t *testing.T) {
	f := startFakeRedis(t)
	st := f.store(t)
	_ = f.ln.Close()
	h := mustHandler(&spyRepo{}, rpc.WithForecastStore(st))
	resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 3, 0.95))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
	if len(resp.GetDailyForecasts()) != 0 || resp.GetSuggestedReorderPoint() != 0 {
		t.Errorf("response carries numbers: %+v", resp)
	}
}

// Scenario: A malformed payload returns UNAVAILABLE (handler mapping)
func TestGetDemandForecastMalformedPayloadUnavailable(t *testing.T) {
	bad := map[string]string{
		"invalid json":  `{not json`,
		"p10 above p50": fcPayload("seasonal_naive_v1", false, []string{`{"date":"2026-11-14","p10":5,"p50":3,"p90":6}`}),
		"negative":      fcPayload("seasonal_naive_v1", false, []string{`{"date":"2026-11-14","p10":-1,"p50":3,"p90":6}`}),
		"p50 above p90": fcPayload("seasonal_naive_v1", false, []string{`{"date":"2026-11-14","p10":1,"p50":7,"p90":6}`}),
	}
	for name, v := range bad {
		f := startFakeRedis(t)
		f.set(fcKey, v)
		h := mustHandler(&spyRepo{}, rpc.WithForecastStore(f.store(t)))
		resp, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 3, 0.95))
		if status.Code(err) != codes.Unavailable {
			t.Errorf("%s: code = %v, want Unavailable", name, status.Code(err))
		}
		if len(resp.GetDailyForecasts()) != 0 {
			t.Errorf("%s: response carries points", name)
		}
	}
}

// Scenario: An unset store address returns UNAVAILABLE
func TestGetDemandForecastStoreUnconfigured(t *testing.T) {
	empty, err := forecaststore.New("", 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]*rpc.Handler{
		"empty address": mustHandler(&spyRepo{}, rpc.WithForecastStore(empty)),
		"no store":      mustHandler(&spyRepo{}),
	} {
		_, err := h.GetDemandForecast(sellerCtx(), forecastReq(14, 3, 0.95))
		if status.Code(err) != codes.Unavailable {
			t.Errorf("%s: code = %v, want Unavailable", name, status.Code(err))
		}
	}
	repo := &spyRepo{}
	h := mustHandler(repo, rpc.WithForecastStore(empty))
	if _, err := h.GetSellerFunnel(sellerCtx(), &analyticsv1.GetSellerFunnelRequest{SellerId: "s1"}); err != nil {
		t.Errorf("other query RPC failed: %v", err)
	}
}
