package store_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/events"
	"github.com/buidangphuc/team-domain/internal/store"
)

func erase(t *testing.T, w *store.PgTxWriter, user, ev string, seen *[]store.Listing) int {
	t.Helper()
	n, err := w.EraseSellerListings(t.Context(), user, ev, holdEnqueue(seen))
	if err != nil {
		t.Fatalf("EraseSellerListings: %v", err)
	}
	return n
}

func countWhere(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), q, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", q, err)
	}
	return n
}

func listingsOf(t *testing.T, pool *pgxpool.Pool, seller string) int {
	return countWhere(t, pool, `SELECT count(*) FROM listings WHERE seller_id = $1`, seller)
}

func totalOutbox(t *testing.T, pool *pgxpool.Pool) int {
	return countWhere(t, pool, `SELECT count(*) FROM outbox_events`)
}

func seedVariants(t *testing.T, pool *pgxpool.Pool, listingID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO listing_variants (id, listing_id, name) VALUES ($1, $2, $3)`,
			fmt.Sprintf("%s-v%d", listingID, i), listingID, "v"); err != nil {
			t.Fatalf("seed variant: %v", err)
		}
	}
}

func seedSixStatuses(t *testing.T, repo *store.PostgresListingRepository, pool *pgxpool.Pool, seller string) []string {
	t.Helper()
	ids := []string{"P1", "P2", "R1", "D1", "H1", "BN"}
	statuses := []string{"published", "published", "pending_review", "draft", "held", "banned"}
	for i, id := range ids {
		seedOwned(t, repo, pool, id, seller, statuses[i])
	}
	return ids
}

func seedShop(t *testing.T, pool *pgxpool.Pool, seller string, bundles int) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO storefronts (seller_id, slug) VALUES ($1, $2)`, seller, "slug-"+seller); err != nil {
		t.Fatalf("seed storefront: %v", err)
	}
	for i := 0; i < bundles; i++ {
		if _, err := pool.Exec(t.Context(),
			`INSERT INTO bundles (id, seller_id, title) VALUES ($1, $2, 'b')`,
			fmt.Sprintf("bun-%s-%d", seller, i), seller); err != nil {
			t.Fatalf("seed bundle: %v", err)
		}
	}
}

// Scenario: Erased seller's listings in every status are deleted
func TestEraseSellerListingsDeletesEveryStatus(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	ids := seedSixStatuses(t, repo, pool, "S1")
	seedVariants(t, pool, "P1", 2)

	var seen []store.Listing
	if n := erase(t, w, "S1", "ev1", &seen); n != 6 {
		t.Errorf("erased = %d, want 6", n)
	}
	if got := listingsOf(t, pool, "S1"); got != 0 {
		t.Errorf("listings left = %d, want 0", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM listing_variants WHERE listing_id = 'P1'`); got != 0 {
		t.Errorf("variants left = %d, want 0", got)
	}
	if got := totalOutbox(t, pool); got != 6 {
		t.Errorf("outbox rows = %d, want 6", got)
	}
	for _, id := range ids {
		if c := outboxCount(t, pool, id); c != 1 {
			t.Errorf("outbox %s = %d, want 1", id, c)
		}
	}
	if !processed(t, pool, "ev1") {
		t.Error("event id not recorded in processed_events")
	}
}

// Scenario: Delete events match the existing delete path
func TestEraseSellerListingsDeleteEventsMatchDeletePath(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	seedVariants(t, pool, "P1", 2)

	n, err := w.EraseSellerListings(t.Context(), "S1", "ev1", events.EnqueueDeleted())
	if err != nil || n != 1 {
		t.Fatalf("EraseSellerListings = %d, %v; want 1, nil", n, err)
	}
	var payload []byte
	var evType string
	if err := pool.QueryRow(t.Context(),
		`SELECT event_type, payload FROM outbox_events WHERE aggregate_id = 'P1'`).Scan(&evType, &payload); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if evType != events.ListingChangedEventType {
		t.Errorf("event_type = %q, want %q", evType, events.ListingChangedEventType)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var lc listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &lc); err != nil {
		t.Fatalf("unmarshal ListingChanged: %v", err)
	}
	if lc.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_DELETED {
		t.Errorf("change_type = %v, want DELETED", lc.GetChangeType())
	}
	if lc.GetListing().GetId() != "P1" || len(lc.GetListing().GetVariants()) != 2 {
		t.Errorf("snapshot = id %q variants %d, want P1 with 2 variants", lc.GetListing().GetId(), len(lc.GetListing().GetVariants()))
	}
	if lc.GetListing().GetStatus() != listingv1.ListingStatus_LISTING_STATUS_PUBLISHED {
		t.Errorf("snapshot status = %v, want pre-delete PUBLISHED", lc.GetListing().GetStatus())
	}
	p := env.GetPrincipal()
	if p.GetId() != "service-team-domain" || p.GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_SERVICE {
		t.Errorf("principal = %v, want service service-team-domain", p)
	}
}

// Scenario: Other sellers' listings are not touched
func TestEraseSellerListingsOnlyTargetsUser(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "A1", "SA", "published")
	seedOwned(t, repo, pool, "B1", "SB", "published")
	seedOwned(t, repo, pool, "B2", "SB", "published")
	var seen []store.Listing
	erase(t, w, "SA", "ev1", &seen)
	expectState(t, pool, "B1", holdRow{"published", "", ""})
	expectState(t, pool, "B2", holdRow{"published", "", ""})
	if outboxCount(t, pool, "B1")+outboxCount(t, pool, "B2") != 0 {
		t.Error("outbox rows written for another seller")
	}
	if got := listingsOf(t, pool, "SA"); got != 0 {
		t.Errorf("SA listings left = %d, want 0", got)
	}
}

// Scenario: Batch boundary of 500 listings
func TestEraseSellerListingsBatchBoundary501(t *testing.T) {
	_, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO listings (id, title, price, currency, status, stock, seller_id)
		 SELECT 'B' || g, 'b', 1, 'VND', 'published', 1, 'SB' FROM generate_series(1, 501) g`); err != nil {
		t.Fatalf("bulk seed: %v", err)
	}
	var seen []store.Listing
	if n := erase(t, w, "SB", "evb", &seen); n != 501 {
		t.Errorf("erased = %d, want 501", n)
	}
	if got := listingsOf(t, pool, "SB"); got != 0 {
		t.Errorf("listings left = %d, want 0", got)
	}
	if got := totalOutbox(t, pool); got != 501 {
		t.Errorf("outbox rows = %d, want 501", got)
	}
	if len(seen) != 501 {
		t.Errorf("enqueue called %d times, want 501", len(seen))
	}
}

// Scenario: Storefront and bundles are deleted
func TestEraseSellerListingsDeletesStorefrontBundlesAndStatusRow(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	seedShop(t, pool, "S1", 2)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO seller_status_applied (user_id, changed_at) VALUES ('S1', now())`); err != nil {
		t.Fatalf("seed status row: %v", err)
	}
	var seen []store.Listing
	erase(t, w, "S1", "ev1", &seen)
	if got := countWhere(t, pool, `SELECT count(*) FROM storefronts WHERE seller_id = 'S1'`); got != 0 {
		t.Errorf("storefronts left = %d, want 0", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM bundles WHERE seller_id = 'S1'`); got != 0 {
		t.Errorf("bundles left = %d, want 0", got)
	}
	if _, ok := appliedAt(t, pool, "S1"); ok {
		t.Error("seller_status_applied row still present")
	}
}

// Scenario: Storefront and bundles are deleted (seller with shop data and no listings)
func TestEraseSellerListingsDeletesShopDataWithoutListings(t *testing.T) {
	_, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedShop(t, pool, "S1", 1)
	var seen []store.Listing
	erase(t, w, "S1", "ev1", &seen)
	if got := countWhere(t, pool, `SELECT count(*) FROM storefronts WHERE seller_id = 'S1'`); got != 0 {
		t.Errorf("storefronts left = %d, want 0", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM bundles WHERE seller_id = 'S1'`); got != 0 {
		t.Errorf("bundles left = %d, want 0", got)
	}
}

// Scenario: Another seller's storefront and bundles survive
func TestEraseSellerListingsOtherSellersShopDataSurvives(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "A1", "SA", "published")
	seedShop(t, pool, "SA", 1)
	seedShop(t, pool, "SB", 3)
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO seller_status_applied (user_id, changed_at) VALUES ('SB', now())`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	var seen []store.Listing
	erase(t, w, "SA", "ev1", &seen)
	if got := countWhere(t, pool, `SELECT count(*) FROM storefronts WHERE seller_id = 'SB'`); got != 1 {
		t.Errorf("SB storefronts = %d, want 1", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM bundles WHERE seller_id = 'SB'`); got != 3 {
		t.Errorf("SB bundles = %d, want 3", got)
	}
	if _, ok := appliedAt(t, pool, "SB"); !ok {
		t.Error("SB seller_status_applied row was deleted")
	}
}

// Scenario: Failed shop-data delete rolls back the listings too
func TestEraseSellerListingsShopDataFailureRollsBackListings(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedSixStatuses(t, repo, pool, "S1")
	seedShop(t, pool, "S1", 2)
	if _, err := pool.Exec(t.Context(), `
		CREATE FUNCTION fail_bundle_delete() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'bundle delete blocked'; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER bundles_block BEFORE DELETE ON bundles
		FOR EACH ROW EXECUTE FUNCTION fail_bundle_delete();`); err != nil {
		t.Fatalf("install failure trigger: %v", err)
	}
	var seen []store.Listing
	n, err := w.EraseSellerListings(t.Context(), "S1", "ev1", holdEnqueue(&seen))
	if err == nil {
		t.Fatalf("want error from failing bundle delete, got n=%d", n)
	}
	if got := listingsOf(t, pool, "S1"); got != 6 {
		t.Errorf("listings after rollback = %d, want 6", got)
	}
	if got := countWhere(t, pool, `SELECT count(*) FROM storefronts WHERE seller_id = 'S1'`); got != 1 {
		t.Errorf("storefront after rollback = %d, want 1", got)
	}
	if got := totalOutbox(t, pool); got != 0 {
		t.Errorf("outbox rows after rollback = %d, want 0", got)
	}
	if processed(t, pool, "ev1") {
		t.Error("event id recorded despite rollback")
	}
}

// Scenario: Redelivered event is a no-op
func TestEraseSellerListingsRedeliveredEventIsNoop(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	if n := erase(t, w, "S1", "ev1", &seen); n != 1 {
		t.Fatalf("first erased = %d, want 1", n)
	}
	seedOwned(t, repo, pool, "P2", "S1", "published")
	if n := erase(t, w, "S1", "ev1", &seen); n != 0 {
		t.Errorf("redelivery erased = %d, want 0", n)
	}
	if got := listingsOf(t, pool, "S1"); got != 1 {
		t.Errorf("listings = %d, want 1 (P2 untouched by redelivery)", got)
	}
	if got := totalOutbox(t, pool); got != 1 {
		t.Errorf("outbox rows = %d, want 1", got)
	}
}

// Scenario: Second event for the same user is a no-op
func TestEraseSellerListingsSecondEventSameUserIsNoop(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedOwned(t, repo, pool, "P1", "S1", "published")
	var seen []store.Listing
	erase(t, w, "S1", "ev1", &seen)
	before := totalOutbox(t, pool)
	if n := erase(t, w, "S1", "ev2", &seen); n != 0 {
		t.Errorf("second erased = %d, want 0", n)
	}
	if got := totalOutbox(t, pool); got != before {
		t.Errorf("outbox rows = %d, want %d", got, before)
	}
	if !processed(t, pool, "ev2") {
		t.Error("second event id not recorded")
	}
}

// Scenario: Store failure rolls back and retries
func TestEraseSellerListingsOutboxFailureRollsBackAndRetries(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	seedSixStatuses(t, repo, pool, "S1")

	boom := errors.New("outbox build failed")
	calls := 0
	failing := func(l store.Listing) (store.OutboxRow, error) {
		calls++
		if calls == 4 {
			return store.OutboxRow{}, boom
		}
		return store.OutboxRow{
			EventID: uuid.NewString(), AggregateType: "Listing", AggregateID: l.ID,
			EventType: "platform.listing.v1.ListingChanged", Payload: []byte{0x01},
		}, nil
	}
	if _, err := w.EraseSellerListings(t.Context(), "S1", "ev1", failing); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped %v", err, boom)
	}
	if got := listingsOf(t, pool, "S1"); got != 6 {
		t.Errorf("listings after rollback = %d, want 6", got)
	}
	if got := totalOutbox(t, pool); got != 0 {
		t.Errorf("outbox rows after rollback = %d, want 0", got)
	}
	if processed(t, pool, "ev1") {
		t.Error("event id recorded despite rollback")
	}

	var seen []store.Listing
	if n := erase(t, w, "S1", "ev1", &seen); n != 6 {
		t.Errorf("retry erased = %d, want 6", n)
	}
	if got := listingsOf(t, pool, "S1"); got != 0 {
		t.Errorf("listings after retry = %d, want 0", got)
	}
	if got := totalOutbox(t, pool); got != 6 {
		t.Errorf("outbox rows after retry = %d, want 6", got)
	}
}

// Scenario: Erasure of a user with no listings
func TestEraseSellerListingsNoListings(t *testing.T) {
	_, pool := newPGRepo(t.Context(), t)
	w := modWriter(t, pool)
	var seen []store.Listing
	if n := erase(t, w, "NOBODY", "ev1", &seen); n != 0 {
		t.Errorf("erased = %d, want 0", n)
	}
	if got := totalOutbox(t, pool); got != 0 {
		t.Errorf("outbox rows = %d, want 0", got)
	}
	if len(seen) != 0 {
		t.Errorf("enqueue called %d times, want 0", len(seen))
	}
	if !processed(t, pool, "ev1") {
		t.Error("event id not recorded as processed")
	}
}
