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

func counters(t *testing.T, w *Writer) (hour time.Time, decode, dup int64, rows int) {
	t.Helper()
	if err := w.DB().QueryRow("SELECT count(*) FROM tracking_ingest_counters").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows == 0 {
		return
	}
	if err := w.DB().QueryRow("SELECT hour, decode_failures, duplicates_skipped FROM tracking_ingest_counters").Scan(&hour, &decode, &dup); err != nil {
		t.Fatal(err)
	}
	return
}

// A redelivered or in-batch duplicate is counted against the sink's UTC hour; a
// clean batch writes no counter row.
func TestWriteCountsSkippedDuplicates(t *testing.T) {
	ctx := context.Background()
	ing := time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)
	w := openAt(t, "", ing)
	defer w.Close()
	at := ing.Add(-time.Minute)

	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "view", OccurredAt: at},
		{EventID: "e2", EventType: "view", OccurredAt: at},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, rows := counters(t, w); rows != 0 {
		t.Fatalf("clean batch wrote %d counter rows, want 0", rows)
	}

	// e1 redelivered, e3 new, e3 repeated inside the same batch.
	if err := w.Write(ctx, []*warehouse.TrackingRecord{
		{EventID: "e1", EventType: "view", OccurredAt: at},
		{EventID: "e3", EventType: "view", OccurredAt: at},
		{EventID: "e3", EventType: "view", OccurredAt: at},
	}); err != nil {
		t.Fatal(err)
	}
	hour, _, dup, rows := counters(t, w)
	if rows != 1 || dup != 2 || !hour.Equal(time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("counters = hour %v dup %d rows %d, want 10:00 / 2 / 1", hour, dup, rows)
	}
	if n := count(t, w, "tracking_events"); n != 3 {
		t.Fatalf("rows = %d, want 3", n)
	}

	// Counters accumulate in the same hour (ON CONFLICT DO UPDATE).
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "e2", EventType: "view", OccurredAt: at}}); err != nil {
		t.Fatal(err)
	}
	if _, _, dup, rows = counters(t, w); rows != 1 || dup != 3 {
		t.Fatalf("after third batch dup = %d rows = %d, want 3 / 1", dup, rows)
	}
}

func TestRecordDecodeFailuresAccumulates(t *testing.T) {
	ctx := context.Background()
	w := openAt(t, "", time.Now())
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 59, 0, 0, time.UTC)
	for i := 0; i < 2; i++ {
		if err := w.RecordDecodeFailures(ctx, at, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.RecordDecodeFailures(ctx, at.Add(2*time.Minute), 1); err != nil { // next hour
		t.Fatal(err)
	}
	var total, rows int64
	if err := w.DB().QueryRow("SELECT sum(decode_failures), count(*) FROM tracking_ingest_counters").Scan(&total, &rows); err != nil {
		t.Fatal(err)
	}
	if total != 3 || rows != 2 {
		t.Fatalf("decode failures = %d over %d hours, want 3 over 2", total, rows)
	}
	if err := w.RecordDecodeFailures(ctx, at, 0); err != nil {
		t.Fatal(err)
	}
}

func engRec(id, fact, user, listing, seller string, at time.Time) *warehouse.EngagementFactRecord {
	return &warehouse.EngagementFactRecord{EventID: id, Fact: fact, UserID: user, ListingID: listing, SellerID: seller, OccurredAt: at}
}

func currentPairs(t *testing.T, w *Writer, q string) []string {
	t.Helper()
	rows, err := w.DB().Query(q)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			t.Fatal(err)
		}
		out = append(out, a+"|"+b)
	}
	return out
}

func TestEngagementFactsIdempotentWithRatingAndIngestedAt(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	ingest := at.Add(time.Minute)
	w := openAt(t, "", ingest)
	review := engRec("e2", warehouse.FactReviewCreated, "u1", "l1", "s1", at)
	review.Rating = 4
	batch := []*warehouse.EngagementFactRecord{engRec("e1", warehouse.FactFavoriteAdded, "u1", "l1", "", at), review}
	for i := 0; i < 3; i++ {
		if err := w.WriteEngagementFacts(ctx, batch); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, warehouse.EngagementFactsTableName); got != 2 {
		t.Fatalf("rows = %d, want 2", got)
	}
	var rating *int
	var ing time.Time
	if err := w.DB().QueryRow("SELECT rating, ingested_at FROM engagement_facts WHERE event_id='e2'").Scan(&rating, &ing); err != nil {
		t.Fatal(err)
	}
	if rating == nil || *rating != 4 || !ing.Equal(ingest) {
		t.Fatalf("rating=%v ingested_at=%v", rating, ing)
	}
	var n int
	if err := w.DB().QueryRow("SELECT count(*) FROM engagement_facts WHERE event_id='e1' AND rating IS NULL").Scan(&n); err != nil || n != 1 {
		t.Fatalf("non-review rating must be NULL (n=%d err=%v)", n, err)
	}
}

func TestFavoritesAndFollowsCurrentViews(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	w := openAt(t, "", t0)
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		engRec("a1", warehouse.FactFavoriteAdded, "u1", "l1", "", t0),
		engRec("a2", warehouse.FactFavoriteAdded, "u1", "l2", "", t0.Add(time.Second)),
		engRec("a3", warehouse.FactFavoriteRemoved, "u1", "l1", "", t0.Add(2*time.Second)),
		// re-add after removal is current again
		engRec("b1", warehouse.FactFavoriteAdded, "u2", "l1", "", t0),
		engRec("b2", warehouse.FactFavoriteRemoved, "u2", "l1", "", t0.Add(time.Second)),
		engRec("b3", warehouse.FactFavoriteAdded, "u2", "l1", "", t0.Add(2*time.Second)),
		// same-timestamp tie resolves by event_id: f2 (removed) wins over f1
		engRec("f1", warehouse.FactFavoriteAdded, "u3", "l9", "", t0),
		engRec("f2", warehouse.FactFavoriteRemoved, "u3", "l9", "", t0),
		engRec("s1", warehouse.FactSellerFollowed, "u1", "", "s1", t0),
		engRec("s2", warehouse.FactSellerFollowed, "u1", "", "s2", t0),
		engRec("s3", warehouse.FactSellerUnfollowed, "u1", "", "s2", t0.Add(time.Second)),
	}); err != nil {
		t.Fatal(err)
	}
	fav := currentPairs(t, w, "SELECT user_id, listing_id FROM favorites_current ORDER BY 1, 2")
	if len(fav) != 2 || fav[0] != "u1|l2" || fav[1] != "u2|l1" {
		t.Fatalf("favorites_current = %v", fav)
	}
	fol := currentPairs(t, w, "SELECT user_id, seller_id FROM follows_current ORDER BY 1, 2")
	if len(fol) != 1 || fol[0] != "u1|s1" {
		t.Fatalf("follows_current = %v", fol)
	}
}

func TestOpenIsIdempotentForEngagementSchema(t *testing.T) {
	path := t.TempDir() + "/a.duckdb"
	w := openAt(t, path, time.Now())
	if err := w.WriteEngagementFacts(context.Background(), []*warehouse.EngagementFactRecord{engRec("e", warehouse.FactFavoriteAdded, "u", "l", "", time.Now())}); err != nil {
		t.Fatal(err)
	}
	w.Close()
	w2 := openAt(t, path, time.Now())
	if got := count(t, w2, warehouse.EngagementFactsTableName); got != 1 {
		t.Fatalf("rows after reopen = %d", got)
	}
}

func TestExportRelationWritesReadableParquetForTablesAndViews(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.Write(ctx, []*warehouse.TrackingRecord{{EventID: "e1", EventType: "view", ListingID: "l1", OccurredAt: at}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, rel := range []string{warehouse.TableName, warehouse.ResolvedViewName, warehouse.EngagementFactsTableName, warehouse.OrderFactsTableName} {
		dst := dir + "/" + rel + ".parquet"
		if err := w.ExportRelation(ctx, rel, dst); err != nil {
			t.Fatalf("export %s: %v", rel, err)
		}
		var n int
		if err := w.DB().QueryRow("SELECT COUNT(*) FROM read_parquet('" + dst + "')").Scan(&n); err != nil {
			t.Fatalf("read back %s: %v", rel, err)
		}
		if want := count(t, w, rel); n != want {
			t.Errorf("%s: parquet rows = %d, table rows = %d", rel, n, want)
		}
	}
	var user string
	if err := w.DB().QueryRow("SELECT user_key FROM read_parquet('" + dir + "/tracking_events_resolved.parquet')").Scan(&user); err != nil || user == "" {
		t.Errorf("resolved export user_key = %q, %v", user, err)
	}
}
