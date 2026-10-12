package query_test

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

// Seam under test (add-ga4-export-import / channel-funnel):
//   query.ChannelFunnelRepository (optional interface on the repository, implemented by
//   *DuckDBRepository over the tracking_events_attributed view) and
//   (*query.Service).GetChannelFunnel (admin scope required; from > to -> InvalidArgument).

var cfDay = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

func cfWarehouse(t *testing.T) *duckdb.Writer {
	t.Helper()
	w, err := duckdb.Open(context.Background(), "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func cfSession(t *testing.T, db *sql.DB, client, id, campaign string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO ga4_sessions (export_date, user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at)
VALUES (DATE '2026-10-01', ?, ?, 'google', 'cpc', ?, '/p/1', TIMESTAMP '2026-10-01 00:00:00', TIMESTAMP '2026-10-01 23:59:00')`, client, id, campaign)
	if err != nil {
		t.Fatalf("insert ga4_sessions: %v", err)
	}
}

func cfEvents(client, typ string, n int, tx string) []*warehouse.TrackingRecord {
	var out []*warehouse.TrackingRecord
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s-%s-%d", client, typ, i)
		r := &warehouse.TrackingRecord{EventID: id, EventType: typ, ListingID: "l1", AnonymousID: "a-" + client,
			GaClientID: client, OccurredAt: cfDay.Add(10*time.Hour + time.Duration(i)*time.Minute)}
		if tx != "" {
			r.TransactionID = tx
		}
		out = append(out, r)
	}
	return out
}

func cfSeed(t *testing.T) *query.DuckDBRepository {
	t.Helper()
	w := cfWarehouse(t)
	cfSession(t, w.DB(), "c-spring", "s1", "spring")
	cfSession(t, w.DB(), "c-summer", "s2", "summer")
	var recs []*warehouse.TrackingRecord
	recs = append(recs, cfEvents("c-spring", "view", 4, "")...)
	recs = append(recs, cfEvents("c-summer", "view", 2, "")...)
	recs = append(recs, cfEvents("c-spring", "impression", 3, "")...)
	recs = append(recs, cfEvents("c-spring", "add_to_cart", 2, "")...)
	recs = append(recs, cfEvents("c-spring", "begin_checkout", 1, "")...)
	// two purchase events share a transaction_id: counted once.
	p := cfEvents("c-spring", "purchase", 2, "tx-1")
	p[1].EventID = "c-spring-purchase-dup"
	recs = append(recs, p...)
	if err := w.Write(context.Background(), recs); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return query.NewDuckDBRepository(w.DB())
}

func cfByCampaign(rows []*analyticsv1.ChannelFunnelRow) map[string]*analyticsv1.ChannelFunnelRow {
	m := map[string]*analyticsv1.ChannelFunnelRow{}
	for _, r := range rows {
		m[r.GetCampaign()] = r
	}
	return m
}

func cfReq(from, to time.Time) *analyticsv1.GetChannelFunnelRequest {
	return &analyticsv1.GetChannelFunnelRequest{From: timestamppb.New(from), To: timestamppb.New(to)}
}

// Scenario: Counts are split by channel
func TestChannelFunnel_CountsAreSplitByChannel(t *testing.T) {
	svc := query.NewService(cfSeed(t))
	resp, err := svc.GetChannelFunnel(asPrincipal("admin-1", "user", "admin"), cfReq(cfDay, cfDay.Add(24*time.Hour)))
	if err != nil {
		t.Fatalf("GetChannelFunnel: %v", err)
	}
	rows := cfByCampaign(resp.GetRows())
	spring, summer := rows["spring"], rows["summer"]
	if spring == nil || summer == nil || len(resp.GetRows()) != 2 {
		t.Fatalf("rows = %v, want exactly spring and summer", resp.GetRows())
	}
	if spring.GetViews() != 4 || summer.GetViews() != 2 {
		t.Errorf("views spring=%d summer=%d, want 4 and 2", spring.GetViews(), summer.GetViews())
	}
	if spring.GetSource() != "google" || spring.GetMedium() != "cpc" {
		t.Errorf("spring source/medium = %q/%q, want google/cpc", spring.GetSource(), spring.GetMedium())
	}
	if spring.GetImpressions() != 3 || spring.GetAdds() != 2 || spring.GetBeginCheckouts() != 1 {
		t.Errorf("spring funnel = %v, want impressions 3, adds 2, begin_checkouts 1", spring)
	}
	if spring.GetPurchases() != 1 {
		t.Errorf("spring purchases = %d, want 1 (distinct transaction_id)", spring.GetPurchases())
	}
	if summer.GetImpressions()+summer.GetAdds()+summer.GetBeginCheckouts()+summer.GetPurchases() != 0 {
		t.Errorf("summer has non-view counts: %v", summer)
	}
}

// Scenario: Counts are split by channel (window bound excludes events outside it)
func TestChannelFunnel_CountsAreSplitByChannel_WindowBoundsApply(t *testing.T) {
	svc := query.NewService(cfSeed(t))
	resp, err := svc.GetChannelFunnel(asPrincipal("admin-1", "user", "admin"), cfReq(cfDay, cfDay.Add(10*time.Hour+1*time.Minute)))
	if err != nil {
		t.Fatalf("GetChannelFunnel: %v", err)
	}
	// Only the minute-0 and minute-1 events (10:00, 10:01) fall in the window: spring 2 views.
	if got := cfByCampaign(resp.GetRows())["spring"].GetViews(); got != 2 {
		t.Errorf("spring views in narrowed window = %d, want 2", got)
	}
}

// Scenario: A window with no events returns no rows
func TestChannelFunnel_AWindowWithNoEventsReturnsNoRows(t *testing.T) {
	svc := query.NewService(cfSeed(t))
	resp, err := svc.GetChannelFunnel(asPrincipal("admin-1", "user", "admin"),
		cfReq(cfDay.AddDate(0, 0, 40), cfDay.AddDate(0, 0, 41)))
	if err != nil {
		t.Fatalf("call must succeed, got %v", err)
	}
	if len(resp.GetRows()) != 0 {
		t.Errorf("rows = %v, want empty", resp.GetRows())
	}
}

// Scenario: An invalid window is rejected
func TestChannelFunnel_AnInvalidWindowIsRejected(t *testing.T) {
	svc := query.NewService(cfSeed(t))
	_, err := svc.GetChannelFunnel(asPrincipal("admin-1", "user", "admin"), cfReq(cfDay.Add(time.Hour), cfDay))
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument for from > to", status.Code(err))
	}
}

// Scenario: Only admins can read the channel funnel
func TestChannelFunnel_OnlyAdminsCanReadTheChannelFunnel(t *testing.T) {
	svc := query.NewService(cfSeed(t))
	cases := []struct {
		name string
		req  *analyticsv1.GetChannelFunnelRequest
		ctx  context.Context
		want codes.Code
	}{
		{"buyer", cfReq(cfDay, cfDay.Add(24*time.Hour)), asPrincipal("buyer-1", "user", "buyer"), codes.PermissionDenied},
		{"seller owner even with seller_id set", &analyticsv1.GetChannelFunnelRequest{SellerId: "seller-1", From: timestamppb.New(cfDay), To: timestamppb.New(cfDay.Add(24 * time.Hour))},
			asPrincipal("seller-1", "user", "seller"), codes.PermissionDenied},
		{"service without admin", cfReq(cfDay, cfDay.Add(24*time.Hour)), asPrincipal("svc-x", "service", "order.read"), codes.PermissionDenied},
	}
	for _, c := range cases {
		resp, err := svc.GetChannelFunnel(c.ctx, c.req)
		if status.Code(err) != c.want {
			t.Errorf("%s: code = %v, want %v", c.name, status.Code(err), c.want)
		}
		if len(resp.GetRows()) != 0 {
			t.Errorf("%s: rows returned to a non-admin: %v", c.name, resp.GetRows())
		}
	}
	if _, err := svc.GetChannelFunnel(context.Background(), cfReq(cfDay, cfDay.Add(time.Hour))); status.Code(err) == codes.OK {
		t.Errorf("no principal: call succeeded, want an auth error")
	}
	// the scope check runs before anything else, even with no repository wired
	if _, err := query.NewService(nil).GetChannelFunnel(asPrincipal("buyer-1", "user", "buyer"), cfReq(cfDay, cfDay.Add(time.Hour))); status.Code(err) != codes.PermissionDenied {
		t.Errorf("nil repo: code = %v, want PermissionDenied before repo check", status.Code(err))
	}
}

// Scenario: Revenue is never read from GA4 data
func TestChannelFunnel_RevenueIsNeverReadFromGA4Data(t *testing.T) {
	const orderTotal = 500000
	w := cfWarehouse(t)
	ctx := context.Background()
	cfSession(t, w.DB(), "c-spring", "s1", "spring")
	if _, err := w.DB().Exec(`INSERT INTO ga4_events_daily (day, event_name, events) VALUES (DATE '2026-10-01', 'purchase', 999999)`); err != nil {
		t.Fatalf("seed ga4_events_daily: %v", err)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "p1", EventType: "purchase", OccurredAt: cfDay.Add(10 * time.Hour), ListingID: "lst-a", Value: 777777, Currency: "VND", TransactionID: "o-1", GaClientID: "c-spring"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{{ListingID: "lst-a", SellerID: "seller-1", UpdatedAt: cfDay}}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{{EventID: "e1", OrderID: "o-1", ListingID: "lst-a", VariantID: "sku-a", SellerID: "seller-1",
		Quantity: 1, UnitPrice: orderTotal, Currency: "VND", OccurredAt: cfDay.Add(10 * time.Hour), Status: "PAID"}}); err != nil {
		t.Fatal(err)
	}
	repo := query.NewDuckDBRepository(w.DB())
	from, to := cfDay, cfDay.Add(24*time.Hour)

	b, err := repo.RevenueBreakdown(ctx, "seller-1", from, to, 10)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, d := range b.Days {
		total += d.Revenue
	}
	if total != orderTotal {
		t.Errorf("breakdown revenue = %d, want %d from order_facts", total, orderTotal)
	}
	s, err := repo.PlatformOrderSummary(ctx, cfDay.AddDate(0, -1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.GMV != orderTotal {
		t.Errorf("GMV = %d, want %d", s.GMV, orderTotal)
	}
	// and the channel funnel itself exposes counts only, taken from tracking events.
	rows, err := repo.ChannelFunnel(ctx, "", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Purchases != 1 {
		t.Errorf("channel rows = %+v, want one row with purchases 1 (not the GA4 daily 999999)", rows)
	}
}

// Scenario: Funnel query meets p95 latency
func TestChannelFunnel_FunnelQueryMeetsP95Latency(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 1,000,000 events")
	}
	w := cfWarehouse(t)
	for i := 0; i < 20; i++ {
		cfSession(t, w.DB(), fmt.Sprintf("pc-%d", i), fmt.Sprintf("ps-%d", i), fmt.Sprintf("camp-%d", i%5))
	}
	if _, err := w.DB().Exec(`INSERT INTO tracking_events (event_id, event_type, listing_id, occurred_at, ga_client_id)
SELECT 'p-' || i, (['view','impression','add_to_cart','begin_checkout','purchase'])[1 + (i % 5)], 'l1',
       TIMESTAMP '2026-10-01 00:00:00' + to_seconds((i % 2592000)::BIGINT), 'pc-' || (i % 20)
FROM range(1000000) t(i)`); err != nil {
		// range() columns and the 30-day spread (2,592,000 s) follow the spec scenario.
		t.Fatalf("seed 1M events: %v", err)
	}
	svc := query.NewService(query.NewDuckDBRepository(w.DB()))
	admin := asPrincipal("admin-1", "user", "admin")
	req := cfReq(cfDay, cfDay.AddDate(0, 0, 30))
	var lat []time.Duration
	for i := 0; i < 100; i++ {
		start := time.Now()
		if _, err := svc.GetChannelFunnel(admin, req); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		lat = append(lat, time.Since(start))
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	if p95 := lat[94]; p95 >= 2*time.Second {
		t.Errorf("p95 = %v, want < 2s", p95)
	}
	// guard against a vacuous pass from a no-op implementation
	resp, _ := svc.GetChannelFunnel(admin, req)
	if len(resp.GetRows()) == 0 {
		t.Errorf("seeded 30-day window returned no rows")
	}
}
