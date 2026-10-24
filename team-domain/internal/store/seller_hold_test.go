package store_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-domain/internal/store"
)

var holdT0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// holdEnqueue records the listing each outbox row was built from and writes a
// distinct row per listing (aggregate_id = listing id).
func holdEnqueue(seen *[]store.Listing) store.EnqueueFn {
	return func(l store.Listing) (store.OutboxRow, error) {
		*seen = append(*seen, l)
		return store.OutboxRow{
			EventID: uuid.NewString(), AggregateType: "Listing", AggregateID: l.ID,
			EventType: "platform.listing.v1.ListingChanged", Payload: []byte{0x01},
		}, nil
	}
}

func seedOwned(t *testing.T, repo *store.PostgresListingRepository, pool *pgxpool.Pool, id, seller, status string) {
	t.Helper()
	if _, err := repo.Create(t.Context(), store.Listing{
		ID: id, Title: id, Currency: "VND", Status: "published", Stock: 1, SellerID: seller,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE listings SET status = $2 WHERE id = $1`, id, status); err != nil {
		t.Fatalf("force status %s: %v", id, err)
	}
}

// seedHeld creates a listing already held with the given prior status.
func seedHeld(t *testing.T, repo *store.PostgresListingRepository, pool *pgxpool.Pool, id, seller, prior string) {
	t.Helper()
	seedOwned(t, repo, pool, id, seller, "held")
	if _, err := pool.Exec(t.Context(),
		`UPDATE listings SET held_prior_status = $2, moderation_reason_code = 'seller_enforcement' WHERE id = $1`, id, prior); err != nil {
		t.Fatalf("force held %s: %v", id, err)
	}
}

type holdRow struct{ status, prior, reason string }

func holdState(t *testing.T, pool *pgxpool.Pool, id string) holdRow {
	t.Helper()
	var r holdRow
	if err := pool.QueryRow(t.Context(),
		`SELECT status, COALESCE(held_prior_status,''), moderation_reason_code FROM listings WHERE id = $1`, id).
		Scan(&r.status, &r.prior, &r.reason); err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	return r
}

func processed(t *testing.T, pool *pgxpool.Pool, eventID string) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&n); err != nil {
		t.Fatalf("processed_events: %v", err)
	}
	return n == 1
}

func appliedAt(t *testing.T, pool *pgxpool.Pool, user string) (time.Time, bool) {
	t.Helper()
	var ts time.Time
	err := pool.QueryRow(t.Context(),
		`SELECT changed_at FROM seller_status_applied WHERE user_id = $1`, user).Scan(&ts)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func hold(t *testing.T, w *store.PgTxWriter, user, ev string, at time.Time, seen *[]store.Listing) int {
	t.Helper()
	n, err := w.HoldSellerListings(t.Context(), user, ev, at, holdEnqueue(seen))
	if err != nil {
		t.Fatalf("HoldSellerListings: %v", err)
	}
	return n
}

func restore(t *testing.T, w *store.PgTxWriter, user, ev string, at time.Time, seen *[]store.Listing) int {
	t.Helper()
	n, err := w.RestoreSellerListings(t.Context(), user, ev, at, holdEnqueue(seen))
	if err != nil {
		t.Fatalf("RestoreSellerListings: %v", err)
	}
	return n
}

func expectState(t *testing.T, pool *pgxpool.Pool, id string, want holdRow) {
	t.Helper()
	if got := holdState(t, pool, id); got != want {
		t.Errorf("%s: got %+v want %+v", id, got, want)
	}
}

// Scenario: Seller hold: Suspension holds published and pending listings
func TestHoldSellerListingsHoldsPublishedAndPending(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	seedOwned(t, repo, pool, "P2", "S1", "published")
	seedOwned(t, repo, pool, "R1", "S1", "pending_review")
	seedOwned(t, repo, pool, "D1", "S1", "draft")
	seedOwned(t, repo, pool, "X1", "S1", "rejected")

	var seen []store.Listing
	if n := hold(t, w, "S1", "ev1", holdT0, &seen); n != 3 {
		t.Errorf("changed = %d, want 3", n)
	}
	held := func(prior string) holdRow { return holdRow{"held", prior, "seller_enforcement"} }
	expectState(t, pool, "P1", held("published"))
	expectState(t, pool, "P2", held("published"))
	expectState(t, pool, "R1", held("pending_review"))
	expectState(t, pool, "D1", holdRow{"draft", "", ""})
	expectState(t, pool, "X1", holdRow{"rejected", "", ""})
	for _, id := range []string{"P1", "P2", "R1"} {
		if c := outboxCount(t, pool, id); c != 1 {
			t.Errorf("outbox %s = %d, want 1", id, c)
		}
	}
	for _, id := range []string{"D1", "X1"} {
		if c := outboxCount(t, pool, id); c != 0 {
			t.Errorf("outbox %s = %d, want 0", id, c)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("enqueue called %d times, want 3", len(seen))
	}
	for _, l := range seen {
		if l.Status != "held" || l.ModerationReasonCode != "seller_enforcement" {
			t.Errorf("enqueue saw %s status=%q reason=%q, want held/seller_enforcement", l.ID, l.Status, l.ModerationReasonCode)
		}
	}
	if !processed(t, pool, "ev1") {
		t.Error("event id not recorded in processed_events")
	}
	if ts, ok := appliedAt(t, pool, "S1"); !ok || !ts.Equal(holdT0) {
		t.Errorf("seller_status_applied = %v (found=%v), want %v", ts, ok, holdT0)
	}
}

// Scenario: Seller hold: Suspension holds published and pending listings (R2: batches of 500, no cap)
func TestHoldSellerListingsBeyondOneBatch(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "seed", "SB", "draft")
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO listings (id, title, price, currency, status, stock, seller_id)
		 SELECT 'B' || g, 'b', 1, 'VND', 'published', 1, 'SB' FROM generate_series(1, 1200) g`); err != nil {
		t.Fatalf("bulk seed: %v", err)
	}
	var seen []store.Listing
	if n := hold(t, w, "SB", "evb", holdT0, &seen); n != 1200 {
		t.Errorf("changed = %d, want 1200", n)
	}
	var held, rows int
	_ = pool.QueryRow(t.Context(), `SELECT count(*) FROM listings WHERE seller_id='SB' AND status='held'`).Scan(&held)
	_ = pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events WHERE aggregate_id LIKE 'B%'`).Scan(&rows)
	if held != 1200 || rows != 1200 {
		t.Errorf("held=%d outbox=%d, want 1200/1200", held, rows)
	}
}

// Scenario: Seller hold: Suspension to ban keeps the original prior status
func TestHoldSellerListingsKeepsOriginalPriorStatus(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	hold(t, w, "S1", "ev-susp", holdT0, &seen)
	hold(t, w, "S1", "ev-ban", holdT0.Add(time.Minute), &seen)
	expectState(t, pool, "P1", holdRow{"held", "published", "seller_enforcement"})
	if c := outboxCount(t, pool, "P1"); c != 1 {
		t.Errorf("outbox = %d, want 1", c)
	}
	if !processed(t, pool, "ev-ban") {
		t.Error("second event id not recorded")
	}
}

// Scenario: Seller hold: Admin-banned listing is not touched
func TestHoldSellerListingsSkipsBannedListing(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "BN", "S1", "banned")
	var seen []store.Listing
	hold(t, w, "S1", "ev1", holdT0, &seen)
	expectState(t, pool, "BN", holdRow{"banned", "", ""})
	if c := outboxCount(t, pool, "BN"); c != 0 {
		t.Errorf("outbox = %d, want 0", c)
	}
	if !processed(t, pool, "ev1") {
		t.Error("event id not recorded")
	}
}

// Scenario: Seller hold: Other sellers' listings are not touched
func TestHoldSellerListingsOnlyTargetsUser(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "A1", "SA", "published")
	seedOwned(t, repo, pool, "B1", "SB", "published")
	seedOwned(t, repo, pool, "B2", "SB", "pending_review")
	var seen []store.Listing
	hold(t, w, "SA", "ev1", holdT0, &seen)
	expectState(t, pool, "A1", holdRow{"held", "published", "seller_enforcement"})
	expectState(t, pool, "B1", holdRow{"published", "", ""})
	expectState(t, pool, "B2", holdRow{"pending_review", "", ""})
	if outboxCount(t, pool, "B1")+outboxCount(t, pool, "B2") != 0 {
		t.Error("outbox rows written for another seller")
	}
}

// Scenario: Seller hold: Reinstatement restores prior statuses
func TestRestoreSellerListingsReturnsPriorStatus(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedHeld(t, repo, pool, "H1", "S1", "published")
	seedHeld(t, repo, pool, "H2", "S1", "pending_review")
	var seen []store.Listing
	if n := restore(t, w, "S1", "ev-act", holdT0, &seen); n != 2 {
		t.Errorf("changed = %d, want 2", n)
	}
	expectState(t, pool, "H1", holdRow{"published", "", ""})
	expectState(t, pool, "H2", holdRow{"pending_review", "", ""})
	if outboxCount(t, pool, "H1") != 1 || outboxCount(t, pool, "H2") != 1 {
		t.Error("want one outbox row per restored listing")
	}
	if !processed(t, pool, "ev-act") {
		t.Error("event id not recorded")
	}
	if ts, ok := appliedAt(t, pool, "S1"); !ok || !ts.Equal(holdT0) {
		t.Errorf("seller_status_applied = %v (found=%v), want %v", ts, ok, holdT0)
	}
}

// Scenario: Seller hold: Admin-banned listing is not resurrected
func TestRestoreSellerListingsLeavesBannedListing(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedHeld(t, repo, pool, "H1", "S1", "published")
	seedOwned(t, repo, pool, "BN", "S1", "banned")
	var seen []store.Listing
	restore(t, w, "S1", "ev1", holdT0, &seen)
	expectState(t, pool, "H1", holdRow{"published", "", ""})
	expectState(t, pool, "BN", holdRow{"banned", "", ""})
	if c := outboxCount(t, pool, "BN"); c != 0 {
		t.Errorf("outbox for banned = %d, want 0", c)
	}
}

// Scenario: Seller hold: Restore with nothing held is a no-op
func TestRestoreSellerListingsNothingHeldNoop(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	if n := restore(t, w, "S1", "ev1", holdT0, &seen); n != 0 {
		t.Errorf("changed = %d, want 0", n)
	}
	expectState(t, pool, "P1", holdRow{"published", "", ""})
	if c := outboxCount(t, pool, "P1"); c != 0 {
		t.Errorf("outbox = %d, want 0", c)
	}
	if len(seen) != 0 {
		t.Errorf("enqueue called %d times, want 0", len(seen))
	}
}

// Scenario: Seller hold: Restore does not touch other sellers' listings
func TestRestoreSellerListingsOnlyTargetsUser(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedHeld(t, repo, pool, "A1", "SA", "published")
	seedHeld(t, repo, pool, "B1", "SB", "published")
	var seen []store.Listing
	restore(t, w, "SA", "ev1", holdT0, &seen)
	expectState(t, pool, "A1", holdRow{"published", "", ""})
	expectState(t, pool, "B1", holdRow{"held", "published", "seller_enforcement"})
	if c := outboxCount(t, pool, "B1"); c != 0 {
		t.Errorf("outbox for other seller = %d, want 0", c)
	}
}

// Scenario: Seller hold: Re-driven older suspension does not re-hold a reinstated seller
func TestSellerStatusStaleEventSkipped(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	t1, t2 := holdT0, holdT0.Add(time.Hour)
	var seen []store.Listing
	restore(t, w, "S1", "ev-active", t2, &seen)
	hold(t, w, "S1", "ev-old-susp", t1, &seen)
	expectState(t, pool, "P1", holdRow{"published", "", ""})
	if c := outboxCount(t, pool, "P1"); c != 0 {
		t.Errorf("outbox = %d, want 0", c)
	}
	if !processed(t, pool, "ev-old-susp") {
		t.Error("stale event id must still be recorded as processed")
	}
	if ts, ok := appliedAt(t, pool, "S1"); !ok || !ts.Equal(t2) {
		t.Errorf("stored changed_at = %v (found=%v), want %v", ts, ok, t2)
	}
}

// Scenario: Seller hold: Event with the same changed_at is stale
func TestSellerStatusEqualChangedAtSkipped(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	restore(t, w, "S1", "ev-active", holdT0, &seen)
	hold(t, w, "S1", "ev-same", holdT0, &seen)
	expectState(t, pool, "P1", holdRow{"published", "", ""})
	if c := outboxCount(t, pool, "P1"); c != 0 {
		t.Errorf("outbox = %d, want 0", c)
	}
	if !processed(t, pool, "ev-same") {
		t.Error("event id must still be recorded")
	}
}

// Scenario: Seller hold: Redelivered event changes nothing twice (repository half: same event id)
func TestHoldSellerListingsSameEventIDIsIdempotent(t *testing.T) {
	repo, pool := newPGRepo(t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	hold(t, w, "S1", "ev1", holdT0, &seen)
	// A redelivery must succeed without writing anything more, even if a newer
	// changed_at were somehow carried.
	hold(t, w, "S1", "ev1", holdT0.Add(time.Minute), &seen)
	expectState(t, pool, "P1", holdRow{"held", "published", "seller_enforcement"})
	if c := outboxCount(t, pool, "P1"); c != 1 {
		t.Errorf("outbox = %d, want 1", c)
	}
	_ = fmt.Sprint()
}
