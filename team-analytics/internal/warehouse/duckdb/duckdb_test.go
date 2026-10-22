package duckdb

import (
	"context"
	"database/sql"
	"path/filepath"
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
	// Scenario "Analytics review hidden: the Parquet export includes hidden rows".
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{hiddenRec("h-exp", "r-exp", "l1", at)}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	defer func() {
		var n int
		q := "SELECT COUNT(*) FROM read_parquet('" + dir + "/engagement_facts.parquet') WHERE fact='review_hidden' AND review_id='r-exp'"
		if err := w.DB().QueryRow(q).Scan(&n); err != nil || n != 1 {
			t.Errorf("Analytics review hidden: the Parquet export includes hidden rows: n=%d err=%v", n, err)
		}
	}()
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

func buyerOf(t *testing.T, w *Writer, eventID string) sql.NullString {
	t.Helper()
	var b sql.NullString
	if err := w.DB().QueryRow("SELECT buyer_id FROM order_facts WHERE event_id = ?", eventID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestWriteOrderFactsStoresBuyerAndNullForEmpty(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "with", OrderID: "o1", ListingID: "l1", OccurredAt: at, Status: "PAID", BuyerID: "u-7"},
		{EventID: "without", OrderID: "o2", ListingID: "l1", OccurredAt: at, Status: "PAID"},
	}); err != nil {
		t.Fatal(err)
	}
	if b := buyerOf(t, w, "with"); !b.Valid || b.String != "u-7" {
		t.Errorf("buyer = %+v, want u-7", b)
	}
	if b := buyerOf(t, w, "without"); b.Valid {
		t.Errorf("empty buyer stored as %q, want NULL", b.String)
	}
}

// A database created before buyer_id existed keeps its rows (NULL buyer) and accepts new ones.
func TestOpenMigratesOrderFactsWithoutBuyerColumn(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	w := openAt(t, path, time.Now())
	if _, err := w.DB().Exec("ALTER TABLE order_facts DROP COLUMN buyer_id"); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if _, err := w.DB().Exec(`INSERT INTO order_facts (event_id, order_id, listing_id, variant_id, seller_id, quantity, unit_price, currency, occurred_at, status)
		VALUES ('old', 'o0', 'l0', '', 's', 1, 1, 'VND', ?, 'PAID')`, at); err != nil {
		t.Fatal(err)
	}
	w.Close()

	w2 := openAt(t, path, time.Now())
	defer w2.Close()
	if got := count(t, w2, warehouse.OrderFactsTableName); got != 1 {
		t.Fatalf("rows after migration = %d, want 1", got)
	}
	if b := buyerOf(t, w2, "old"); b.Valid {
		t.Errorf("old row buyer = %q, want NULL", b.String)
	}
	if err := w2.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{{EventID: "new", OrderID: "o1", OccurredAt: at, Status: "PAID", BuyerID: "u-1"}}); err != nil {
		t.Fatal(err)
	}
	if b := buyerOf(t, w2, "new"); b.String != "u-1" {
		t.Errorf("new row buyer = %+v", b)
	}
}

func TestOrderFactsParquetExportHasBuyerColumn(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{{EventID: "e", OrderID: "o1", OccurredAt: at, Status: "PAID", BuyerID: "u-9"}}); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "order_facts.parquet")
	if err := w.ExportRelation(ctx, warehouse.OrderFactsTableName, dst); err != nil {
		t.Fatal(err)
	}
	var b string
	if err := w.DB().QueryRow("SELECT buyer_id FROM read_parquet('" + dst + "')").Scan(&b); err != nil || b != "u-9" {
		t.Fatalf("exported buyer_id = %q, %v", b, err)
	}
}

// Scenario "DuckDB migrates a legacy database": a table lacking the ecommerce
// columns is opened twice; every new column exists once, prior rows keep NULLs.
func TestOpenMigratesLegacyTrackingColumns_DuckDBMigratesALegacyDatabase(t *testing.T) {
	ctx := context.Background()
	newCols := []string{"item_list_id", "position", "event_group_id", "transaction_id", "currency", "value"}
	path := filepath.Join(t.TempDir(), "legacy.duckdb")
	first := openAt(t, path, time.Now())
	for _, q := range []string{"DROP VIEW tracking_events_resolved", "DROP VIEW tracking_identity", "DROP VIEW ga4_events"} {
		if _, err := first.DB().Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, c := range newCols {
		if _, err := first.DB().Exec("ALTER TABLE tracking_events DROP COLUMN " + c); err != nil {
			t.Fatalf("drop %s: %v", c, err)
		}
	}
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	if _, err := first.DB().Exec("INSERT INTO tracking_events (event_id, event_type, occurred_at) VALUES ('old','view',?)", at); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	// two opens in a row
	second := openAt(t, path, time.Now())
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	w := openAt(t, path, time.Now())
	defer w.Close()

	for _, c := range newCols {
		var n int
		if err := w.DB().QueryRow("SELECT count(*) FROM information_schema.columns WHERE table_name='tracking_events' AND column_name=?", c).Scan(&n); err != nil || n != 1 {
			t.Errorf("column %s count = %d, %v; want 1", c, n, err)
		}
	}
	var nulls int
	if err := w.DB().QueryRow("SELECT count(*) FROM tracking_events WHERE event_id='old' AND item_list_id IS NULL AND position IS NULL AND event_group_id IS NULL AND transaction_id IS NULL AND currency IS NULL AND value IS NULL").Scan(&nulls); err != nil || nulls != 1 {
		t.Fatalf("legacy row preserved with NULL new columns: n=%d err=%v", nulls, err)
	}

	if err := w.Write(ctx, []*warehouse.TrackingRecord{{
		EventID: "new", EventType: "purchase", OccurredAt: at, ItemListID: "home", Position: 3,
		EventGroupID: "g1", TransactionID: "o-1", Currency: "VND", Value: 500,
	}}); err != nil {
		t.Fatal(err)
	}
	var list, tx, cur string
	var pos, val int64
	if err := w.DB().QueryRow("SELECT item_list_id, transaction_id, currency, value FROM ga4_events WHERE event_id='new'").Scan(&list, &tx, &cur, &val); err != nil {
		t.Fatalf("ga4_events read: %v", err)
	}
	if list != "home" || tx != "o-1" || cur != "VND" || val != 500 {
		t.Fatalf("ga4_events = %q %q %q %d", list, tx, cur, val)
	}
	if err := w.DB().QueryRow("SELECT position FROM tracking_events WHERE event_id='new'").Scan(&pos); err != nil || pos != 3 {
		t.Fatalf("position = %d, %v; want 3", pos, err)
	}
}

// Scenario: Consent and client id are persisted (DuckDB round-trip of all 5 columns)
func TestConsentAndClientIdArePersisted_DuckDBRoundTrip(t *testing.T) {
	ctx := context.Background()
	w, err := Open(ctx, filepath.Join(t.TempDir(), "w.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	rec := &warehouse.TrackingRecord{
		EventID: "e-consent", EventType: "view", ListingID: "l1",
		OccurredAt: time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
		GaClientID: "111.222", ConsentVersion: 1,
		ConsentAnalytics: true, ConsentMarketing: false, ConsentPersonalization: true,
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{rec}); err != nil {
		t.Fatal(err)
	}
	var (
		client  string
		version int
		a, m, p bool
	)
	err = w.DB().QueryRow("SELECT ga_client_id, consent_version, consent_analytics, consent_marketing, consent_personalization FROM tracking_events WHERE event_id = 'e-consent'").
		Scan(&client, &version, &a, &m, &p)
	if err != nil {
		t.Fatal(err)
	}
	if client != "111.222" || version != 1 || !a || m || !p {
		t.Errorf("row = %q v%d a=%v m=%v p=%v, want 111.222 v1 a=true m=false p=true", client, version, a, m, p)
	}
}

func hiddenRec(id, reviewID, listing string, at time.Time) *warehouse.EngagementFactRecord {
	return &warehouse.EngagementFactRecord{EventID: id, Fact: warehouse.FactReviewHidden, ListingID: listing, ReviewID: reviewID, OccurredAt: at}
}

type factRow struct {
	reviewID sql.NullString
	rating   sql.NullInt64
	listing  string
}

func factOf(t *testing.T, w *Writer, eventID string) factRow {
	t.Helper()
	var r factRow
	if err := w.DB().QueryRow("SELECT review_id, rating, listing_id FROM engagement_facts WHERE event_id = ?", eventID).Scan(&r.reviewID, &r.rating, &r.listing); err != nil {
		t.Fatalf("event %s: %v", eventID, err)
	}
	return r
}

// Scenario: Analytics review hidden: a review_created row stores its review id.
func TestReviewIDStoredForCreatedAndHidden(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	w := openAt(t, "", at)
	defer w.Close()
	created := &warehouse.EngagementFactRecord{EventID: "c1", Fact: warehouse.FactReviewCreated, UserID: "u1", ListingID: "l-1", SellerID: "s1", Rating: 4, ReviewID: "r-1", OccurredAt: at}
	hidden := hiddenRec("h1", "r-1", "l-1", at.Add(time.Hour))
	hidden.Rating = 3 // a hidden row never carries a rating
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{created, hidden}); err != nil {
		t.Fatal(err)
	}
	c := factOf(t, w, "c1")
	if !c.reviewID.Valid || c.reviewID.String != "r-1" || !c.rating.Valid || c.rating.Int64 != 4 || c.listing != "l-1" {
		t.Fatalf("created row = %+v", c)
	}
	h := factOf(t, w, "h1")
	if !h.reviewID.Valid || h.reviewID.String != "r-1" || h.rating.Valid || h.listing != c.listing {
		t.Fatalf("hidden row = %+v (rating must be NULL)", h)
	}
	var n int
	if err := w.DB().QueryRow("SELECT count(*) FROM engagement_facts WHERE fact='review_hidden' AND user_id='' AND seller_id=''").Scan(&n); err != nil || n != 1 {
		t.Fatalf("hidden row user/seller must be empty (n=%d err=%v)", n, err)
	}
}

// Scenario: Analytics review hidden: the review_created row is kept.
func TestReviewHiddenKeepsReviewCreated(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	w := openAt(t, "", at)
	defer w.Close()
	created := &warehouse.EngagementFactRecord{EventID: "c1", Fact: warehouse.FactReviewCreated, UserID: "u1", ListingID: "l-1", Rating: 4, ReviewID: "r-1", OccurredAt: at}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{created}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{hiddenRec("h1", "r-1", "l-1", at.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	if got := count(t, w, "engagement_facts WHERE review_id = 'r-1'"); got != 2 {
		t.Fatalf("rows for review = %d, want 2", got)
	}
	c := factOf(t, w, "c1")
	if !c.rating.Valid || c.rating.Int64 != 4 {
		t.Fatalf("created rating = %+v, want 4", c.rating)
	}
}

// Scenario: Analytics review hidden: a replayed event is stored once.
func TestReviewHiddenIdempotent(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	w := openAt(t, "", at)
	defer w.Close()
	for i := 0; i < 2; i++ {
		if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{hiddenRec("h1", "r-1", "l-1", at)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(t, w, "engagement_facts WHERE fact='review_hidden' AND event_id='h1'"); got != 1 {
		t.Fatalf("hidden rows = %d, want 1", got)
	}
}

// Scenarios: Analytics review hidden: opening an existing database adds review_id once;
// a legacy review_created row is not given a review id without a backfill.
func TestOpenAddsReviewIDColumnIdempotently(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "w.duckdb")
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	w := openAt(t, path, at)
	if _, err := w.DB().Exec("ALTER TABLE engagement_facts DROP COLUMN review_id"); err != nil {
		t.Fatalf("drop review_id (fresh schema must have it): %v", err)
	}
	if _, err := w.DB().Exec(`INSERT INTO engagement_facts (event_id, fact, user_id, listing_id, seller_id, rating, occurred_at)
		VALUES ('old', 'review_created', 'u1', 'l0', 's1', 5, ?)`, at); err != nil {
		t.Fatal(err)
	}
	w.Close()

	for i := 0; i < 2; i++ {
		w2, err := Open(ctx, path)
		if err != nil {
			t.Fatalf("open #%d: %v", i+1, err)
		}
		var cols int
		if err := w2.DB().QueryRow("SELECT count(*) FROM information_schema.columns WHERE table_name='engagement_facts' AND column_name='review_id'").Scan(&cols); err != nil || cols != 1 {
			t.Fatalf("open #%d: review_id columns = %d err=%v", i+1, cols, err)
		}
		if got := count(t, w2, warehouse.EngagementFactsTableName); got != 1 {
			t.Fatalf("open #%d: rows = %d, want 1", i+1, got)
		}
		w2.Close()
	}

	w3 := openAt(t, path, at)
	defer w3.Close()
	if err := w3.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{hiddenRec("h2", "r-2", "l2", at)}); err != nil {
		t.Fatal(err)
	}
	old := factOf(t, w3, "old")
	if old.reviewID.Valid || !old.rating.Valid || old.rating.Int64 != 5 {
		t.Fatalf("legacy row = %+v, want NULL review_id and rating 5", old)
	}
	if h := factOf(t, w3, "h2"); h.reviewID.String != "r-2" {
		t.Fatalf("new hidden row = %+v", h)
	}
}

// Scenario: Analytics review hidden: the favorites and follows views ignore review_hidden.
func TestReviewHiddenDoesNotAffectCurrentViews(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	w := openAt(t, "", t0)
	defer w.Close()
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		engRec("a1", warehouse.FactFavoriteAdded, "u1", "l1", "", t0),
		engRec("s1", warehouse.FactSellerFollowed, "u1", "", "s1", t0),
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{hiddenRec("h1", "r-1", "l1", t0.Add(time.Minute))}); err != nil {
		t.Fatal(err)
	}
	if fav := currentPairs(t, w, "SELECT user_id, listing_id FROM favorites_current ORDER BY 1, 2"); len(fav) != 1 || fav[0] != "u1|l1" {
		t.Fatalf("favorites_current = %v", fav)
	}
	if fol := currentPairs(t, w, "SELECT user_id, seller_id FROM follows_current ORDER BY 1, 2"); len(fol) != 1 || fol[0] != "u1|s1" {
		t.Fatalf("follows_current = %v", fol)
	}
}
