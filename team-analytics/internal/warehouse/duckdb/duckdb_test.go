package duckdb

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

func count(t *testing.T, w *Writer, table string) int {
	t.Helper()
	var n int
	if err := w.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestWriteIsIdempotentOnEventID(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	batch := []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "view", ListingID: "l1", OccurredAt: at, Properties: map[string]string{"k": "v"}},
		{EventID: "e2", EventType: "view", ListingID: "l2", OccurredAt: at},
		{EventID: "e2", EventType: "view", ListingID: "l2", OccurredAt: at}, // dup inside the batch
	}
	for i := 0; i < 3; i++ { // redelivery of the same batch
		if err := w.Write(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, warehouse.TableName); got != 2 {
		t.Fatalf("tracking rows = %d, want 2", got)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "e3", EventType: "view", OccurredAt: at}}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, w, warehouse.TableName); got != 3 {
		t.Fatalf("after new event rows = %d, want 3", got)
	}
}

func TestWriteOrderFactsIsIdempotentOnEventID(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	batch := []*warehouse.OrderFactRecord{
		{EventID: "evt-0", OrderID: "o1", ListingID: "l1", SellerID: "s1", Quantity: 1, UnitPrice: 10, Currency: "VND", OccurredAt: at, Status: "PAID"},
		{EventID: "evt-1", OrderID: "o1", ListingID: "l2", SellerID: "s1", Quantity: 2, UnitPrice: 5, Currency: "VND", OccurredAt: at, Status: "PAID"},
	}
	for i := 0; i < 3; i++ {
		if err := w.WriteOrderFacts(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, warehouse.OrderFactsTableName); got != 2 {
		t.Fatalf("order_facts rows = %d, want 2", got)
	}
}

// Existing volumes may already contain duplicates; Open and further writes must
// still work (no unique index) and not add more.
func TestWriteOnDatabaseWithPreexistingDuplicates(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 2; i++ {
		if _, err := w.DB().Exec("INSERT INTO tracking_events (event_id, event_type, occurred_at) VALUES ('dup','view', now())"); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "dup", EventType: "view", OccurredAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, w, warehouse.TableName); got != 2 {
		t.Fatalf("rows = %d, want 2 (no new dup)", got)
	}
}

func openAt(t *testing.T, path string, now time.Time) *Writer {
	t.Helper()
	w, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return now }
	return w
}

func TestIngestedAtIsSetByTheSinkClock(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ing := time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)
	w := openAt(t, "", ing)
	defer w.Close()
	rec := &warehouse.TrackingRecord{EventID: "e1", EventType: "view", OccurredAt: at}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{rec}); err != nil {
		t.Fatal(err)
	}
	var got time.Time
	if err := w.DB().QueryRow("SELECT ingested_at FROM tracking_events WHERE event_id='e1'").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Equal(ing) || got.Before(at) {
		t.Fatalf("ingested_at = %v, want %v (>= occurred_at %v)", got, ing, at)
	}
}

func TestStitchingViews(t *testing.T) {
	ctx := context.Background()
	w := openAt(t, "", time.Now())
	defer w.Close()
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	batch := []*warehouse.TrackingRecord{
		{EventID: "a1", EventType: "view", AnonymousID: "X", PrincipalType: "anonymous", OccurredAt: t0},
		{EventID: "a2", EventType: "view", AnonymousID: "X", PrincipalID: "u-old", PrincipalType: "user", OccurredAt: t0.Add(time.Minute)},
		{EventID: "a3", EventType: "view", AnonymousID: "X", PrincipalID: "u-new", PrincipalType: "user", OccurredAt: t0.Add(2 * time.Minute)},
		{EventID: "a4", EventType: "view", AnonymousID: "X", PrincipalType: "anonymous", OccurredAt: t0.Add(3 * time.Minute)},
		{EventID: "b1", EventType: "view", AnonymousID: "Y", PrincipalType: "anonymous", OccurredAt: t0},
		{EventID: "s1", EventType: "view", AnonymousID: "S", PrincipalType: "anonymous", OccurredAt: t0},
		{EventID: "s2", EventType: "view", AnonymousID: "S", PrincipalID: "u-solo", PrincipalType: "user", OccurredAt: t0.Add(time.Minute)},
		{EventID: "s3", EventType: "view", AnonymousID: "S", PrincipalType: "anonymous", OccurredAt: t0.Add(2 * time.Minute)},
		{EventID: "c1", EventType: "view", AnonymousID: "", PrincipalID: "u-noanon", PrincipalType: "user", OccurredAt: t0},
	}
	if err := w.Write(ctx, batch); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		// X was seen with two accounts: ambiguous (the anonymous id is client-supplied),
		// so its anonymous rows are not stitched to either.
		"a1": "anon:X",
		"a2": "u-old", // a user row keeps its own principal
		"a3": "u-new",
		"a4": "anon:X",
		"b1": "anon:Y",
		"s1": "u-solo", // pre-login row resolves to the only account seen with S
		"s2": "u-solo",
		"s3": "u-solo", // post-logout anonymous row
		"c1": "u-noanon",
	}
	rows, err := w.DB().Query("SELECT event_id, user_key FROM tracking_events_resolved")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			t.Fatal(err)
		}
		n++
		if want[id] != key {
			t.Errorf("%s user_key = %q, want %q", id, key, want[id])
		}
	}
	if n != len(want) {
		t.Fatalf("resolved rows = %d, want %d (the join must not fan out)", n, len(want))
	}
	var pid string
	if err := w.DB().QueryRow("SELECT principal_id FROM tracking_identity WHERE anonymous_id='S'").Scan(&pid); err != nil || pid != "u-solo" {
		t.Fatalf("tracking_identity S = %q, %v; want u-solo", pid, err)
	}
	var ambiguous int
	if err := w.DB().QueryRow("SELECT count(*) FROM tracking_identity WHERE anonymous_id='X'").Scan(&ambiguous); err != nil || ambiguous != 0 {
		t.Fatalf("tracking_identity must not map the ambiguous X: rows=%d err=%v", ambiguous, err)
	}
}

// An existing volume holds the table without ingested_at and with the old
// ga4_events view: Open must migrate it and create the stitching views.
func TestOpenMigratesLegacyDatabase(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir() + "/legacy.duckdb"
	first := openAt(t, path, time.Now())
	for _, q := range []string{
		"DROP VIEW tracking_events_resolved", "DROP VIEW tracking_identity", "DROP VIEW ga4_events",
		"ALTER TABLE tracking_events DROP COLUMN ingested_at",
		"CREATE VIEW ga4_events AS SELECT event_id FROM tracking_events",
	} {
		if _, err := first.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	w := openAt(t, path, time.Now())
	defer w.Close()
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "e1", EventType: "view", AnonymousID: "Z", OccurredAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	var key string
	if err := w.DB().QueryRow("SELECT user_key FROM tracking_events_resolved").Scan(&key); err != nil || key != "anon:Z" {
		t.Fatalf("user_key = %q, %v", key, err)
	}
}
