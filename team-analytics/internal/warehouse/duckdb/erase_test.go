package duckdb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const erSecret = "test-tombstone-secret"

var (
	erAt        = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	pseudonymRE = regexp.MustCompile(`^erased:[0-9a-f]{32}$`)
)

func erOpen(t *testing.T, secret string) *Writer {
	t.Helper()
	w, err := Open(context.Background(), "", WithTombstoneSecret(secret))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func erTrack(id, user, session, anon, ga string) *warehouse.TrackingRecord {
	r := &warehouse.TrackingRecord{
		EventID: id, EventType: "view", ListingID: "lst-" + id, SessionID: session, AnonymousID: anon,
		OccurredAt: erAt, GaClientID: ga, Properties: map[string]string{"device": "dev-" + id},
		SearchQuery: "q-" + id, Currency: "VND", Value: 7, Quantity: 1,
	}
	if user != "" {
		r.PrincipalID, r.PrincipalType = user, "user"
	}
	return r
}

func erWrite(t *testing.T, w *Writer, recs ...*warehouse.TrackingRecord) {
	t.Helper()
	if err := w.Write(context.Background(), recs); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func erErase(t *testing.T, w *Writer, user string) warehouse.EraseCounts {
	t.Helper()
	c, err := w.EraseUser(context.Background(), user)
	if err != nil {
		t.Fatalf("EraseUser(%q): %v", user, err)
	}
	return c
}

func erScalar(t *testing.T, w *Writer, q string, args ...any) int {
	t.Helper()
	var n int
	if err := w.DB().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func erStrings(t *testing.T, w *Writer, q string, args ...any) []string {
	t.Helper()
	rows, err := w.DB().Query(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func erOccurrences(t *testing.T, w *Writer, needle string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, tbl := range erStrings(t, w, `SELECT table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema = 'main'`) {
		n := erScalar(t, w, fmt.Sprintf(`SELECT count(*) FROM %s t WHERE to_json(t)::VARCHAR LIKE '%%' || ? || '%%'`, tbl), needle)
		if n > 0 {
			out[tbl] = n
		}
	}
	return out
}

func erRequireNoTrace(t *testing.T, w *Writer, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if got := erOccurrences(t, w, n); len(got) > 0 {
			t.Errorf("%q still present: %v", n, got)
		}
	}
}

func erSnapshot(t *testing.T, w *Writer, q string, args ...any) string {
	t.Helper()
	return strings.Join(erStrings(t, w, q, args...), "\n")
}

func erHMAC(secret, user string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(user))
	return hex.EncodeToString(m.Sum(nil))
}

func erPseudonym(t *testing.T, w *Writer, user string) string {
	t.Helper()
	var p string
	if err := w.DB().QueryRow(`SELECT pseudonym FROM erased_users WHERE user_hmac = ?`, erHMAC(erSecret, user)).Scan(&p); err != nil {
		t.Fatalf("no tombstone for %q: %v", user, err)
	}
	return p
}

func erEng(id, fact, user, listing, seller string) *warehouse.EngagementFactRecord {
	return &warehouse.EngagementFactRecord{EventID: id, Fact: fact, UserID: user, ListingID: listing, SellerID: seller, OccurredAt: erAt}
}

// Scenario: AUE-12 (boot DDL) erased_users exists after Open with the three columns
func TestErasureTombstoneTableExistsAtBoot(t *testing.T) {
	w := erOpen(t, erSecret)
	cols := erStrings(t, w, `SELECT column_name FROM information_schema.columns WHERE table_name = 'erased_users' ORDER BY ordinal_position`)
	if strings.Join(cols, ",") != "user_hmac,pseudonym,erased_at" {
		t.Fatalf("erased_users columns = %v", cols)
	}
	pk := erScalar(t, w, `SELECT count(*) FROM duckdb_constraints() WHERE table_name = 'erased_users' AND constraint_type = 'PRIMARY KEY'`)
	if pk != 1 {
		t.Errorf("erased_users primary key constraints = %d, want 1", pk)
	}
}

// Scenario: AUE-6 Re-key keeps aggregates and removes identifiers
func TestErasureRekeysTrackingAndStitchedAnonymousRows(t *testing.T) {
	w := erOpen(t, erSecret)
	var recs []*warehouse.TrackingRecord
	for i := range 25 {
		recs = append(recs, erTrack(fmt.Sprintf("u-a1-%d", i), "user-erase-1", "sess-erase-1", "anon-erase-A1", "GA1.1.111"))
		recs = append(recs, erTrack(fmt.Sprintf("u-a2-%d", i), "user-erase-1", "sess-erase-2", "anon-erase-A2", "GA1.1.222"))
	}
	for i := range 10 {
		recs = append(recs, erTrack(fmt.Sprintf("p-a1-%d", i), "", "sess-pre-1", "anon-erase-A1", "GA1.1.111"))
		recs = append(recs, erTrack(fmt.Sprintf("p-a2-%d", i), "", "sess-pre-2", "anon-erase-A2", "GA1.1.222"))
	}
	erWrite(t, w, recs...)
	erWrite(t, w, erTrack("other-1", "user-keep-1", "sess-keep", "anon-keep", "GA1.1.999"))
	const shape = `SELECT event_id || '|' || event_type || '|' || listing_id || '|' || occurred_at::VARCHAR || '|' || value::VARCHAR || '|' || search_query FROM tracking_events ORDER BY event_id`
	before := erSnapshot(t, w, shape)

	counts := erErase(t, w, "user-erase-1")

	if got := erScalar(t, w, `SELECT count(*) FROM tracking_events`); got != 71 {
		t.Errorf("row count = %d, want 71", got)
	}
	if after := erSnapshot(t, w, shape); after != before {
		t.Error("event_type, listing_id, timestamps, value or search_query changed")
	}
	erRequireNoTrace(t, w, "user-erase-1", "anon-erase-A1", "anon-erase-A2", "GA1.1.111", "GA1.1.222", "dev-u-a1-0")
	rows := erStrings(t, w, `SELECT DISTINCT principal_id || '|' || session_id || '|' || anonymous_id FROM tracking_events WHERE event_id LIKE 'u-a%'`)
	if len(rows) != 1 {
		t.Fatalf("user rows carry %d distinct key triples, want 1: %v", len(rows), rows)
	}
	p := strings.Split(rows[0], "|")[0]
	if !pseudonymRE.MatchString(p) || rows[0] != p+"|"+p+"|"+p {
		t.Errorf("principal_id/session_id/anonymous_id = %q, want one erased:<32 hex> pseudonym", rows[0])
	}
	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE anonymous_id = ?`, p); n != 70 {
		t.Errorf("rows carrying the pseudonym = %d, want 70", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE (event_id LIKE 'u-a%' OR event_id LIKE 'p-a%') AND ga_client_id IS NOT NULL`); n != 0 {
		t.Errorf("%d rows keep ga_client_id", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE event_id LIKE 'u-a%' AND properties IS NOT NULL`); n != 0 {
		t.Errorf("%d user rows keep properties", n)
	}
	if counts[warehouse.TableName] != 70 {
		t.Errorf("counts[tracking_events] = %d, want 70", counts[warehouse.TableName])
	}
}

// Scenario: AUE-7 Rows of other users are untouched
func TestErasureLeavesOtherUsersRowsUntouched(t *testing.T) {
	w := erOpen(t, erSecret)
	var recs []*warehouse.TrackingRecord
	for i := range 20 {
		recs = append(recs, erTrack(fmt.Sprintf("v-%d", i), "user-other-V", "sess-V", "anon-V", "GA1.1.V"))
	}
	recs = append(recs, erTrack("u-1", "user-erase-7", "sess-U", "anon-U", "GA1.1.U"))
	erWrite(t, w, recs...)
	const q = `SELECT to_json(t)::VARCHAR FROM tracking_events t WHERE principal_id = 'user-other-V' ORDER BY event_id`
	before := erSnapshot(t, w, q)

	erErase(t, w, "user-erase-7")

	if after := erSnapshot(t, w, q); after != before || before == "" {
		t.Error("rows of user V changed")
	}
}

// Scenario: AUE-40 A service principal row with the same id is not re-keyed
func TestErasureLeavesServicePrincipalRows(t *testing.T) {
	w := erOpen(t, erSecret)
	svc := erTrack("svc-1", "", "sess-svc", "anon-svc", "")
	svc.PrincipalID, svc.PrincipalType = "user-erase-40", "service"
	erWrite(t, w, svc, erTrack("u-1", "user-erase-40", "sess-U", "anon-U", ""))

	erErase(t, w, "user-erase-40")

	if got := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE event_id = 'svc-1' AND principal_id = 'user-erase-40' AND principal_type = 'service'`); got != 1 {
		t.Errorf("service principal row changed (matching rows = %d)", got)
	}
	if got := erScalar(t, w, `SELECT count(*) FROM erased_users`); got != 1 {
		t.Errorf("tombstones = %d, want 1 (the purge still commits)", got)
	}
}

// Scenario: AUE-8 Boundary of a user with no rows
func TestErasureBoundaryUserWithNoRows(t *testing.T) {
	w := erOpen(t, erSecret)
	erWrite(t, w, erTrack("v-1", "user-other-V", "sess-V", "anon-V", "GA1.1.V"))
	const q = `SELECT to_json(t)::VARCHAR FROM tracking_events t ORDER BY event_id`
	before := erSnapshot(t, w, q)

	counts := erErase(t, w, "user-erase-8")

	if n := erScalar(t, w, `SELECT count(*) FROM erased_users`); n != 1 {
		t.Errorf("tombstones = %d, want 1", n)
	}
	if erSnapshot(t, w, q) != before {
		t.Error("an unrelated row changed")
	}
	for tbl, n := range counts {
		if n != 0 {
			t.Errorf("counts[%s] = %d, want 0", tbl, n)
		}
	}
}

// Scenario: AUE-5 Redelivery is a no-op
func TestErasureRedeliveryIsIdempotent(t *testing.T) {
	w := erOpen(t, erSecret)
	erWrite(t, w, erTrack("u-1", "user-erase-5", "sess-U", "anon-U", "GA1.1.U"))
	erErase(t, w, "user-erase-5")
	const q = `SELECT to_json(t)::VARCHAR FROM tracking_events t ORDER BY event_id`
	before := erSnapshot(t, w, q)
	pseudo := erPseudonym(t, w, "user-erase-5")

	counts := erErase(t, w, "user-erase-5")

	if erSnapshot(t, w, q) != before {
		t.Error("second purge changed rows")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM erased_users`); n != 1 {
		t.Errorf("tombstones = %d, want 1", n)
	}
	if got := erPseudonym(t, w, "user-erase-5"); got != pseudo {
		t.Errorf("pseudonym changed on redelivery: %q -> %q", pseudo, got)
	}
	for tbl, n := range counts {
		if n != 0 {
			t.Errorf("redelivery counts[%s] = %d, want 0", tbl, n)
		}
	}
}

// Scenario: AUE-9 Orders are kept with a pseudonymous buyer
func TestErasureKeepsOrdersWithPseudonymousBuyer(t *testing.T) {
	w := erOpen(t, erSecret)
	var facts []*warehouse.OrderFactRecord
	for i := range 3 {
		facts = append(facts, &warehouse.OrderFactRecord{
			EventID: fmt.Sprintf("o-%d", i), OrderID: "order-9", ListingID: "lst-1", VariantID: "v-1", SellerID: "seller-9",
			Quantity: int32(i + 1), UnitPrice: 1000 * int64(i+1), Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "user-erase-9",
		})
	}
	if err := w.WriteOrderFacts(context.Background(), facts); err != nil {
		t.Fatal(err)
	}
	const shape = `SELECT order_id || '|' || quantity::VARCHAR || '|' || unit_price::VARCHAR || '|' || occurred_at::VARCHAR || '|' || seller_id FROM order_facts ORDER BY event_id`
	const gmv = `SELECT sum(quantity * unit_price)::INTEGER FROM order_facts WHERE status = 'PAID'`
	before, gmvBefore := erSnapshot(t, w, shape), erScalar(t, w, gmv)

	counts := erErase(t, w, "user-erase-9")

	if erSnapshot(t, w, shape) != before || erScalar(t, w, gmv) != gmvBefore {
		t.Error("order_id, quantity, unit_price, occurred_at or GMV changed")
	}
	pseudo := erPseudonym(t, w, "user-erase-9")
	if !pseudonymRE.MatchString(pseudo) {
		t.Fatalf("pseudonym %q", pseudo)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM order_facts WHERE buyer_id = ?`, pseudo); n != 3 {
		t.Errorf("lines with the pseudonymous buyer = %d, want 3", n)
	}
	erRequireNoTrace(t, w, "user-erase-9")
	if counts[warehouse.OrderFactsTableName] != 3 {
		t.Errorf("counts[order_facts] = %d, want 3", counts[warehouse.OrderFactsTableName])
	}
}

// Scenario: AUE-10 Favourites and follows are deleted
func TestErasureDeletesFavouritesAndFollows(t *testing.T) {
	w := erOpen(t, erSecret)
	batch := []*warehouse.EngagementFactRecord{
		erEng("f1", warehouse.FactFavoriteAdded, "user-erase-10", "lst-1", ""),
		erEng("f2", warehouse.FactFavoriteAdded, "user-erase-10", "lst-2", ""),
		erEng("f3", warehouse.FactFavoriteRemoved, "user-erase-10", "lst-2", ""),
		erEng("f4", warehouse.FactFavoriteAdded, "user-erase-10", "lst-3", ""),
		erEng("s1", warehouse.FactSellerFollowed, "user-erase-10", "", "seller-1"),
		erEng("s2", warehouse.FactSellerUnfollowed, "user-erase-10", "", "seller-1"),
		erEng("v1", warehouse.FactFavoriteAdded, "user-other-V", "lst-1", ""),
		erEng("v2", warehouse.FactSellerFollowed, "user-other-V", "", "seller-1"),
	}
	if err := w.WriteEngagementFacts(context.Background(), batch); err != nil {
		t.Fatal(err)
	}
	const vq = `SELECT to_json(t)::VARCHAR FROM engagement_facts t WHERE user_id = 'user-other-V' ORDER BY event_id`
	vBefore := erSnapshot(t, w, vq)

	counts := erErase(t, w, "user-erase-10")

	erRequireNoTrace(t, w, "user-erase-10")
	if n := erScalar(t, w, `SELECT count(*) FROM favorites_current WHERE user_id = 'user-erase-10'`); n != 0 {
		t.Errorf("favorites_current rows = %d", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM follows_current WHERE user_id = 'user-erase-10'`); n != 0 {
		t.Errorf("follows_current rows = %d", n)
	}
	if erSnapshot(t, w, vq) != vBefore || vBefore == "" {
		t.Error("facts of V changed")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM engagement_facts`); n != 2 {
		t.Errorf("engagement_facts rows = %d, want 2 (only V's)", n)
	}
	if counts[warehouse.EngagementFactsTableName] != 6 {
		t.Errorf("counts[engagement_facts] = %d, want 6", counts[warehouse.EngagementFactsTableName])
	}
}

// Scenario: AUE-11 An erased seller is re-keyed everywhere
func TestErasureRekeysSellerAndReviewRowsEverywhere(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "lst-s1", SellerID: "seller-erase-11", Status: "LISTING_STATUS_PUBLISHED", UpdatedAt: erAt},
		{ListingID: "lst-s2", SellerID: "seller-erase-11", Status: "LISTING_STATUS_HELD", UpdatedAt: erAt},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "o1", OrderID: "ord", ListingID: "lst-s1", SellerID: "seller-erase-11", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "buyer-keep"},
	}); err != nil {
		t.Fatal(err)
	}
	review := erEng("r1", warehouse.FactReviewCreated, "seller-erase-11", "lst-x", "seller-keep")
	review.Rating, review.ReviewID = 5, "rev-1"
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		erEng("e1", warehouse.FactFavoriteAdded, "buyer-keep", "lst-s1", "seller-erase-11"), review,
	}); err != nil {
		t.Fatal(err)
	}
	const listings = `SELECT listing_id || '|' || status FROM listing_sellers ORDER BY listing_id`
	before := erSnapshot(t, w, listings)

	counts := erErase(t, w, "seller-erase-11")

	erRequireNoTrace(t, w, "seller-erase-11")
	if erSnapshot(t, w, listings) != before {
		t.Error("listing_id or status of listing_sellers changed")
	}
	pseudo := erPseudonym(t, w, "seller-erase-11")
	if n := erScalar(t, w, `SELECT count(*) FROM listing_sellers WHERE seller_id = ?`, pseudo); n != 2 {
		t.Errorf("listing_sellers with the pseudonym = %d, want 2", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM engagement_facts WHERE event_id = 'r1' AND user_id = ?`, pseudo); n != 1 {
		t.Error("review_created user_id not re-keyed to the pseudonym")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM engagement_facts WHERE event_id = 'e1' AND seller_id = ?`, pseudo); n != 1 {
		t.Error("engagement_facts.seller_id not re-keyed")
	}
	if counts[warehouse.ListingSellersTableName] != 2 {
		t.Errorf("counts[listing_sellers] = %d, want 2", counts[warehouse.ListingSellersTableName])
	}
}

// Scenario: AUE-35 A failed order or engagement re-key rolls back as a whole
func TestErasureRollsBackWhenEngagementRekeyFails(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	erWrite(t, w, erTrack("u-1", "user-erase-35", "sess-U", "anon-U", "GA1.1.U"))
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "o1", OrderID: "ord", ListingID: "l", SellerID: "s", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "user-erase-35"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		erEng("e1", warehouse.FactFavoriteAdded, "user-erase-35", "l", ""),
	}); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE eng_backup AS SELECT * FROM engagement_facts`,
		`DROP TABLE engagement_facts CASCADE`,
		`CREATE VIEW engagement_facts AS SELECT * FROM eng_backup`,
	} {
		if _, err := w.DB().Exec(stmt); err != nil {
			t.Fatalf("inject failure: %v", err)
		}
	}

	if _, err := w.EraseUser(ctx, "user-erase-35"); err == nil {
		t.Fatal("EraseUser succeeded although engagement_facts cannot be modified")
	}

	if n := erScalar(t, w, `SELECT count(*) FROM order_facts WHERE buyer_id = 'user-erase-35'`); n != 1 {
		t.Errorf("order_facts rows with the original buyer = %d, want 1 (rolled back)", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE principal_id = 'user-erase-35'`); n != 1 {
		t.Errorf("tracking rows with the original id = %d, want 1 (rolled back)", n)
	}
	if n := erScalar(t, w, `SELECT count(*) FROM erased_users`); n != 0 {
		t.Errorf("tombstones = %d, want 0", n)
	}
}

// Scenario: AUE-36 A forged event for another user does not touch unrelated rows
func TestErasureMatchesWholeColumnValueOnly(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	erWrite(t, w, erTrack("v-1", "user-erase-36-long", "sess-V", "anon-V", "GA1.1.V"))
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "o1", OrderID: "ord", ListingID: "l", SellerID: "xuser-erase-36-long", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "user-erase-36-long"},
	}); err != nil {
		t.Fatal(err)
	}
	snap := func() string {
		return erSnapshot(t, w, `SELECT to_json(t)::VARCHAR FROM tracking_events t UNION ALL SELECT to_json(t)::VARCHAR FROM order_facts t ORDER BY 1`)
	}
	before := snap()

	erErase(t, w, "user-erase-36")
	erErase(t, w, "er-erase-36-long")
	erErase(t, w, "user-erase-36-lon")

	if snap() != before {
		t.Error("rows of V changed for a prefix or suffix id")
	}
}

// Scenario: AUE-13 The tombstone holds no clear id
func TestErasureTombstoneHoldsNoClearID(t *testing.T) {
	w := erOpen(t, erSecret)
	erWrite(t, w, erTrack("u-1", "user-erase-13", "sess-U", "anon-U", ""))
	erErase(t, w, "user-erase-13")

	row := erStrings(t, w, `SELECT user_hmac || '|' || pseudonym || '|' || erased_at::VARCHAR FROM erased_users`)
	if len(row) != 1 {
		t.Fatalf("erased_users rows = %d", len(row))
	}
	if strings.Contains(row[0], "user-erase-13") {
		t.Errorf("tombstone row %q contains the clear id", row[0])
	}
	parts := strings.Split(row[0], "|")
	if len(parts[0]) != 64 || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(parts[0]) {
		t.Errorf("user_hmac %q is not 64 hex characters", parts[0])
	}
	if parts[0] != erHMAC(erSecret, "user-erase-13") {
		t.Errorf("user_hmac is not HMAC-SHA256(user_id, ERASURE_TOMBSTONE_SECRET)")
	}

	other := erOpen(t, "another-secret")
	erWrite(t, other, erTrack("u-1", "user-erase-13", "sess-U", "anon-U", ""))
	erErase(t, other, "user-erase-13")
	got := erStrings(t, other, `SELECT user_hmac FROM erased_users`)
	if len(got) != 1 || got[0] == parts[0] {
		t.Errorf("user_hmac does not depend on the secret: %v", got)
	}
}

// Scenario: AUE-6 pseudonyms come from crypto/rand per erasure
func TestErasurePseudonymIsRandomPerErasure(t *testing.T) {
	w := erOpen(t, erSecret)
	erWrite(t, w,
		erTrack("a-1", "user-erase-a", "sa", "anon-a", ""),
		erTrack("b-1", "user-erase-b", "sb", "anon-b", ""))
	erErase(t, w, "user-erase-a")
	erErase(t, w, "user-erase-b")
	pa, pb := erPseudonym(t, w, "user-erase-a"), erPseudonym(t, w, "user-erase-b")
	if !pseudonymRE.MatchString(pa) || !pseudonymRE.MatchString(pb) || pa == pb {
		t.Errorf("pseudonyms %q, %q must both match erased:<32 hex> and differ", pa, pb)
	}
}

// Scenario: AUE-12 A buffered event flushed after the purge is re-keyed
func TestErasureWritersRekeyTombstonedUserOnIngest(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	erWrite(t, w, erTrack("seed", "user-erase-12", "sess-U", "anon-U", ""))
	erErase(t, w, "user-erase-12")
	pseudo := erPseudonym(t, w, "user-erase-12")

	erWrite(t, w, erTrack("late-t", "user-erase-12", "sess-late", "anon-late", ""), erTrack("late-v", "user-keep-12", "sess-v", "anon-v", ""))
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "late-o", OrderID: "ord", ListingID: "l", SellerID: "s", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "user-erase-12"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		erEng("late-e", warehouse.FactFavoriteAdded, "user-erase-12", "l", ""),
	}); err != nil {
		t.Fatal(err)
	}

	erRequireNoTrace(t, w, "user-erase-12")
	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE event_id = 'late-t' AND principal_id = ?`, pseudo); n != 1 {
		t.Error("late tracking row not re-keyed to the pseudonym")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM order_facts WHERE event_id = 'late-o' AND buyer_id = ?`, pseudo); n != 1 {
		t.Error("late order fact not re-keyed to the pseudonym")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM engagement_facts WHERE event_id = 'late-e' AND user_id = ?`, pseudo); n != 1 {
		t.Error("late engagement fact not re-keyed to the pseudonym")
	}
}

// Scenario: AUE-11 An erased seller is re-keyed everywhere
func TestErasureWritersRekeyTombstonedSellerOnIngest(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	erWrite(t, w, erTrack("seed", "seller-erase-11", "sess-S", "anon-S", ""))
	erErase(t, w, "seller-erase-11")
	pseudo := erPseudonym(t, w, "seller-erase-11")

	if err := w.UpsertListingSellers(ctx, []*warehouse.ListingSellerRecord{
		{ListingID: "l-11", SellerID: "seller-erase-11", UpdatedAt: erAt, Status: "LISTING_STATUS_DELETED"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "late-o-11", OrderID: "ord", ListingID: "l-11", SellerID: "seller-erase-11", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "buyer-keep-11"},
	}); err != nil {
		t.Fatal(err)
	}

	erRequireNoTrace(t, w, "seller-erase-11")
	if n := erScalar(t, w, `SELECT count(*) FROM listing_sellers WHERE listing_id = 'l-11' AND seller_id = ?`, pseudo); n != 1 {
		t.Error("late listing_sellers row not re-keyed to the pseudonym")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM order_facts WHERE event_id = 'late-o-11' AND seller_id = ? AND buyer_id = 'buyer-keep-11'`, pseudo); n != 1 {
		t.Error("late order fact seller not re-keyed to the pseudonym")
	}
}

// Scenario: AUE-37 A late event for a non-erased user is stored as is
func TestErasureWriterStoresNonErasedUserAsIs(t *testing.T) {
	w := erOpen(t, erSecret)
	erWrite(t, w, erTrack("seed", "user-erase-37", "sess-U", "anon-U", ""))
	erErase(t, w, "user-erase-37")
	before := erScalar(t, w, `SELECT count(*) FROM erased_users`)

	erWrite(t, w, erTrack("late-v", "user-keep-37", "sess-v", "anon-v", ""))

	if n := erScalar(t, w, `SELECT count(*) FROM tracking_events WHERE event_id = 'late-v' AND principal_id = 'user-keep-37'`); n != 1 {
		t.Error("row of a non-erased user was re-keyed")
	}
	if n := erScalar(t, w, `SELECT count(*) FROM erased_users`); n != before {
		t.Errorf("tombstone count changed %d -> %d", before, n)
	}
}

// Scenario: AUE-38 Tombstone lookup failure rejects the batch
func TestErasureWriterTombstoneLookupFailureRollsBackBatch(t *testing.T) {
	w := erOpen(t, erSecret)
	ctx := context.Background()
	if _, err := w.DB().Exec(`DROP TABLE erased_users`); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(ctx, []*warehouse.TrackingRecord{erTrack("t1", "user-keep-38", "s", "a", ""), erTrack("t2", "user-keep-38b", "s", "a", "")}); err == nil {
		t.Error("Write succeeded although the tombstone lookup fails")
	}
	if err := w.WriteOrderFacts(ctx, []*warehouse.OrderFactRecord{
		{EventID: "o1", OrderID: "ord", ListingID: "l", SellerID: "s", Quantity: 1, UnitPrice: 5, Currency: "VND", OccurredAt: erAt, Status: "PAID", BuyerID: "user-keep-38"},
	}); err == nil {
		t.Error("WriteOrderFacts succeeded although the tombstone lookup fails")
	}
	if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{
		erEng("e1", warehouse.FactFavoriteAdded, "user-keep-38", "l", ""),
	}); err == nil {
		t.Error("WriteEngagementFacts succeeded although the tombstone lookup fails")
	}
	for _, tbl := range []string{"tracking_events", "order_facts", "engagement_facts"} {
		if n := erScalar(t, w, `SELECT count(*) FROM `+tbl); n != 0 {
			t.Errorf("%s has %d rows after a rejected batch", tbl, n)
		}
	}
}
