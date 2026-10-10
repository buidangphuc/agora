package query_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func TestDuckDBRepository_OrderFactsAndFunnel(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	d1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

	// Write tracking events
	trackingEvents := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "impression", OccurredAt: d1, ListingID: "lst-a", Price: 500, Currency: "VND", EventGroupID: "grp-1"},
		{EventID: "e2", EventType: "impression", OccurredAt: d1, ListingID: "lst-b", Price: 100, Currency: "VND", EventGroupID: "grp-1"},
		{EventID: "e3", EventType: "view", OccurredAt: d1, ListingID: "lst-a"},
		{EventID: "e4", EventType: "add_to_cart", OccurredAt: d1, ListingID: "lst-a"},
		{EventID: "e5", EventType: "view", OccurredAt: d2, ListingID: "lst-b"},
		{EventID: "e6", EventType: "begin_checkout", OccurredAt: d2, ListingID: "lst-a", Value: 600, Currency: "VND"},
		{EventID: "e7", EventType: "purchase", OccurredAt: d2, ListingID: "lst-a", Price: 500, Quantity: 1, Value: 600, Currency: "VND", TransactionID: "tx-100"},
	}
	if err := w.Write(ctx, trackingEvents); err != nil {
		t.Fatalf("Write tracking events: %v", err)
	}

	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "lst-a", SellerID: "seller-1", UpdatedAt: d1},
		{ListingID: "lst-b", SellerID: "seller-1", UpdatedAt: d1},
	}); err != nil {
		t.Fatalf("UpsertListingSellers: %v", err)
	}

	// Write order facts
	orderFacts := []*warehouse.OrderFactRecord{
		{
			EventID:    "evt-1",
			OrderID:    "o-1",
			ListingID:  "lst-a",
			VariantID:  "sku-a",
			SellerID:   "seller-1",
			Quantity:   2,
			UnitPrice:  500,
			Currency:   "VND",
			OccurredAt: d1,
			Status:     "PAID",
		},
		{
			EventID:    "evt-2",
			OrderID:    "o-2",
			ListingID:  "lst-a",
			VariantID:  "sku-a",
			SellerID:   "seller-1",
			Quantity:   1,
			UnitPrice:  3000,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
		{
			EventID:    "evt-3",
			OrderID:    "o-3",
			ListingID:  "lst-b",
			VariantID:  "sku-b",
			SellerID:   "seller-1",
			Quantity:   5,
			UnitPrice:  100,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
		// seller-2 facts
		{
			EventID:    "evt-4",
			OrderID:    "o-9",
			ListingID:  "lst-z",
			VariantID:  "sku-z",
			SellerID:   "seller-2",
			Quantity:   1,
			UnitPrice:  99999,
			Currency:   "VND",
			OccurredAt: d2,
			Status:     "PAID",
		},
	}
	if err := w.WriteOrderFacts(ctx, orderFacts); err != nil {
		t.Fatalf("WriteOrderFacts: %v", err)
	}

	repo := query.NewDuckDBRepository(w.DB())
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	// Test Funnel
	funnel, err := repo.SellerFunnel(ctx, "seller-1", from, to)
	if err != nil {
		t.Fatalf("SellerFunnel: %v", err)
	}
	if funnel.Impressions != 2 || funnel.Views != 2 || funnel.Adds != 1 || funnel.Orders != 3 || funnel.BeginCheckouts != 1 || funnel.Purchases != 1 {
		t.Errorf("funnel = %+v, want impressions=2 views=2 adds=1 orders=3 begin_checkouts=1 purchases=1", funnel)
	}

	// Test GA4 view
	var ga4EventName, ga4ItemID string
	var ga4Price int64
	row := w.DB().QueryRowContext(ctx, "SELECT event_name, item_id, price FROM ga4_events WHERE event_id = 'e1'")
	if err := row.Scan(&ga4EventName, &ga4ItemID, &ga4Price); err != nil {
		t.Fatalf("query ga4_events view: %v", err)
	}
	if ga4EventName != "view_item_list" || ga4ItemID != "lst-a" || ga4Price != 500 {
		t.Errorf("ga4_events record = (%s, %s, %d), want (view_item_list, lst-a, 500)", ga4EventName, ga4ItemID, ga4Price)
	}

	// Test Revenue Breakdown
	breakdown, err := repo.RevenueBreakdown(ctx, "seller-1", from, to, 10)
	if err != nil {
		t.Fatalf("RevenueBreakdown: %v", err)
	}
	if len(breakdown.Days) != 2 {
		t.Fatalf("days len = %d, want 2", len(breakdown.Days))
	}
	if breakdown.Days[0].Day != "2026-09-01" || breakdown.Days[0].Revenue != 1000 || breakdown.Days[0].OrderCount != 1 {
		t.Errorf("day[0] = %+v, want 2026-09-01/1000/1", breakdown.Days[0])
	}
	if breakdown.Days[1].Day != "2026-09-02" || breakdown.Days[1].Revenue != 3500 || breakdown.Days[1].OrderCount != 2 {
		t.Errorf("day[1] = %+v, want 2026-09-02/3500/2", breakdown.Days[1])
	}

	if len(breakdown.TopSkus) != 2 {
		t.Fatalf("top_skus len = %d, want 2", len(breakdown.TopSkus))
	}
	if breakdown.TopSkus[0].SKU != "sku-a" || breakdown.TopSkus[0].Revenue != 4000 || breakdown.TopSkus[0].UnitsSold != 3 {
		t.Errorf("top_skus[0] = %+v, want sku-a/4000/3", breakdown.TopSkus[0])
	}
	if breakdown.TopSkus[1].SKU != "sku-b" || breakdown.TopSkus[1].Revenue != 500 || breakdown.TopSkus[1].UnitsSold != 5 {
		t.Errorf("top_skus[1] = %+v, want sku-b/500/5", breakdown.TopSkus[1])
	}

	// Isolation test for seller-2
	b2, err := repo.RevenueBreakdown(ctx, "seller-2", from, to, 10)
	if err != nil {
		t.Fatalf("RevenueBreakdown seller-2: %v", err)
	}
	if len(b2.TopSkus) != 1 || b2.TopSkus[0].SKU != "sku-z" || b2.TopSkus[0].Revenue != 99999 {
		t.Errorf("seller-2 top_skus = %+v, want single sku-z/99999", b2.TopSkus)
	}

	// Test Demand Forecast
	fc, err := repo.DemandForecast(ctx, "seller-1", "lst-a", 14)
	if err != nil {
		t.Fatalf("DemandForecast: %v", err)
	}
	if fc.SellerID != "seller-1" || fc.ListingID != "lst-a" {
		t.Errorf("fc seller/listing = %s/%s, want seller-1/lst-a", fc.SellerID, fc.ListingID)
	}
	if len(fc.DailyForecast) != 14 {
		t.Fatalf("daily forecast len = %d, want 14", len(fc.DailyForecast))
	}
	if fc.DailyForecast[0].P10 > fc.DailyForecast[0].P50 || fc.DailyForecast[0].P50 > fc.DailyForecast[0].P90 {
		t.Errorf("invalid quantile ordering: p10=%v, p50=%v, p90=%v", fc.DailyForecast[0].P10, fc.DailyForecast[0].P50, fc.DailyForecast[0].P90)
	}
}

func TestDuckDBRepository_UpsertListingSellers(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	rec := func(id, seller string, at time.Time) *warehouse.ListingSellerRecord {
		return &warehouse.ListingSellerRecord{ListingID: id, SellerID: seller, UpdatedAt: at}
	}
	// Same batch twice (redelivery) plus a duplicate inside the batch: one row.
	for i := 0; i < 2; i++ {
		if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "s1", t1), rec("l1", "s1", t1), rec("l2", "s2", t1)}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	count := func() (n int) {
		if err := w.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM listing_sellers").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
	seller := func(id string) (s string) {
		if err := w.DB().QueryRowContext(ctx, "SELECT seller_id FROM listing_sellers WHERE listing_id = ?", id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	// A newer event refreshes the row; an older (out-of-order) one does not.
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "s9", t2)}); err != nil {
		t.Fatal(err)
	}
	if got := seller("l1"); got != "s9" {
		t.Fatalf("seller after newer event = %q, want s9", got)
	}
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "s1", t1)}); err != nil {
		t.Fatal(err)
	}
	if got := seller("l1"); got != "s9" {
		t.Fatalf("seller after stale event = %q, want s9 (unchanged)", got)
	}
	if n := count(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
}

func TestDuckDBRepository_SellerFunnelScopedBySeller(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "lst-a", SellerID: "seller-a", UpdatedAt: at},
		{ListingID: "lst-b", SellerID: "seller-b", UpdatedAt: at},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "1", EventType: "impression", OccurredAt: at, ListingID: "lst-a"},
		{EventID: "2", EventType: "impression", OccurredAt: at, ListingID: "lst-a"},
		{EventID: "3", EventType: "view", OccurredAt: at, ListingID: "lst-a"},
		{EventID: "4", EventType: "view", OccurredAt: at, ListingID: "lst-b"},
		// Unknown listing and listing-less events belong to no seller.
		{EventID: "5", EventType: "view", OccurredAt: at, ListingID: "lst-unknown"},
		{EventID: "6", EventType: "view", OccurredAt: at},
	}); err != nil {
		t.Fatal(err)
	}

	repo := query.NewDuckDBRepository(w.DB())
	from, to := at.Add(-time.Hour), at.Add(time.Hour)

	a, err := repo.SellerFunnel(ctx, "seller-a", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if a.Impressions != 2 || a.Views != 1 {
		t.Errorf("seller-a funnel = %+v, want impressions=2 views=1", a)
	}
	b, err := repo.SellerFunnel(ctx, "seller-b", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if b.Impressions != 0 || b.Views != 1 {
		t.Errorf("seller-b funnel = %+v, want impressions=0 views=1", b)
	}
	none, err := repo.SellerFunnel(ctx, "seller-none", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if none.Impressions != 0 || none.Views != 0 {
		t.Errorf("unmapped seller funnel = %+v, want zero tracking counts", none)
	}
}

func TestDuckDBWriter_ListingAttributes(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()

	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	attrs := func(id string) (category sql.NullString, price sql.NullInt64) {
		err := w.DB().QueryRowContext(ctx, "SELECT category_id, price FROM listing_sellers WHERE listing_id = ?", id).Scan(&category, &price)
		if err != nil {
			t.Fatal(err)
		}
		return category, price
	}
	rec := func(id, category string, price int64, at time.Time) *warehouse.ListingSellerRecord {
		return &warehouse.ListingSellerRecord{ListingID: id, SellerID: "s1", CategoryID: category, Price: price, UpdatedAt: at}
	}
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "cat-a", 100, t1), rec("l2", "", 0, t1)}); err != nil {
		t.Fatal(err)
	}
	if c, p := attrs("l1"); c.String != "cat-a" || p.Int64 != 100 {
		t.Fatalf("l1 = %v %v, want cat-a 100", c, p)
	}
	if c, p := attrs("l2"); c.Valid || p.Valid {
		t.Fatalf("an unknown category and price must be NULL, got %v %v", c, p)
	}
	// A newer event replaces both attributes; a stale one changes neither.
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "cat-b", 250, t2)}); err != nil {
		t.Fatal(err)
	}
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{rec("l1", "cat-a", 100, t1)}); err != nil {
		t.Fatal(err)
	}
	if c, p := attrs("l1"); c.String != "cat-b" || p.Int64 != 250 {
		t.Fatalf("l1 after newer+stale = %v %v, want cat-b 250", c, p)
	}
}

// An existing database whose listing_sellers table predates the attribute columns is migrated in place.
func TestDuckDBWriter_ListingAttributesMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wh.duckdb")
	old, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `CREATE TABLE listing_sellers (listing_id VARCHAR PRIMARY KEY, seller_id VARCHAR NOT NULL, updated_at TIMESTAMP NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.ExecContext(ctx, `INSERT INTO listing_sellers VALUES ('l1', 's1', TIMESTAMP '2026-09-01 10:00:00')`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	w, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatalf("open an old database: %v", err)
	}
	defer w.Close()
	var category sql.NullString
	if err := w.DB().QueryRowContext(ctx, "SELECT category_id FROM listing_sellers WHERE listing_id = 'l1'").Scan(&category); err != nil || category.Valid {
		t.Fatalf("migrated row category = %v, err %v; want NULL", category, err)
	}
	// Replaying the same event (equal updated_at) fills the attributes in.
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{{ListingID: "l1", SellerID: "s1", CategoryID: "cat-a", Price: 7, UpdatedAt: at}}); err != nil {
		t.Fatal(err)
	}
	if err := w.DB().QueryRowContext(ctx, "SELECT category_id FROM listing_sellers WHERE listing_id = 'l1'").Scan(&category); err != nil || category.String != "cat-a" {
		t.Fatalf("after replay category = %v, err %v; want cat-a", category, err)
	}
}

// The Parquet export of listing_sellers carries the attribute columns the featurestore reads.
func TestDuckDBWriter_ExportsListingAttributes(t *testing.T) {
	ctx := context.Background()
	w, err := duckdb.Open(ctx, "")
	if err != nil {
		t.Fatalf("duckdb.Open: %v", err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "l1", SellerID: "s1", CategoryID: "cat-a", Price: 4200, UpdatedAt: at},
	}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "listing_sellers.parquet")
	if err := w.ExportRelation(ctx, warehouse.ListingSellersTableName, dst); err != nil {
		t.Fatalf("ExportRelation: %v", err)
	}
	var seller, category string
	var price int64
	q := "SELECT seller_id, category_id, price FROM read_parquet('" + dst + "') WHERE listing_id = 'l1'"
	if err := w.DB().QueryRowContext(ctx, q).Scan(&seller, &category, &price); err != nil {
		t.Fatalf("read the export back: %v", err)
	}
	if seller != "s1" || category != "cat-a" || price != 4200 {
		t.Fatalf("exported row = %q %q %d, want s1 cat-a 4200", seller, category, price)
	}
}
