package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/repository"
)

// pendingStockEvents decodes every pending ListingStockChanged outbox row, in
// creation order.
func pendingStockEvents(t *testing.T, pool *pgxpool.Pool) []*listingv1.ListingStockChanged {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT aggregate_id, payload FROM outbox_events
		WHERE status = 'pending' AND event_type = 'platform.listing.v1.ListingStockChanged'
		ORDER BY created_at, event_id`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	var out []*listingv1.ListingStockChanged
	for rows.Next() {
		var key string
		var payload []byte
		if err := rows.Scan(&key, &payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(payload, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetType() != "platform.listing.v1.ListingStockChanged" || env.GetOccurredAt() == nil {
			t.Fatalf("bad envelope: %+v", &env)
		}
		var ev listingv1.ListingStockChanged
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatalf("unmarshal ListingStockChanged: %v", err)
		}
		if key != ev.GetListingId() {
			t.Fatalf("outbox key %q != listing_id %q", key, ev.GetListingId())
		}
		out = append(out, &ev)
	}
	return out
}

func stockEventRepo(t *testing.T) (*repository.PostgresListingRepository, *pgxpool.Pool) {
	t.Helper()
	repo, pool := newPGRepo(t)
	repo.WithStockEvents(repository.NewOutboxStore(pool), handler.NewStockEventBuilder())
	return repo, pool
}

// One pending row per real change, carrying the stock after it; none for an
// idempotent repeat reserve, a repeated or unknown release, or a commit.
func TestPG_StockEvents_RealChangesOnly(t *testing.T) {
	repo, pool := stockEventRepo(t)
	ctx := context.Background()
	seedPGListing(t, repo, "L1", 10)
	seedPGListing(t, repo, "L2", 5)

	wantStocks := func(step string, want ...int32) {
		t.Helper()
		got := pendingStockEvents(t, pool)
		if len(got) != len(want) {
			t.Fatalf("%s: %d events, want %d", step, len(got), len(want))
		}
		for i, w := range want {
			if got[i].GetStock() != w {
				t.Fatalf("%s: event %d stock = %d, want %d", step, i, got[i].GetStock(), w)
			}
		}
	}

	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, farFuture); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	wantStocks("reserve", 8)
	_ = repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, farFuture) // repeat: no-op
	wantStocks("repeat reserve", 8)
	if err := repo.CommitReservation(ctx, "r1"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	wantStocks("commit", 8)
	if _, err := repo.ReleaseReservation(ctx, "r1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	wantStocks("release", 8, 10)
	_, _ = repo.ReleaseReservation(ctx, "r1")    // repeat: no-op
	_, _ = repo.ReleaseReservation(ctx, "never") // unknown: no-op
	wantStocks("no-op releases", 8, 10)
	if err := repo.ReserveStockIdempotent(ctx, "big", "L1", "", 99, farFuture); !errors.Is(err, repository.ErrOutOfStock) {
		t.Fatalf("oversized reserve: %v", err)
	}
	wantStocks("refused reserve", 8, 10)

	// Sweep: one row per affected listing with its final stock.
	past := time.Now().Add(-time.Minute)
	for _, r := range []struct {
		id, listing string
		qty         int32
	}{{"a", "L1", 1}, {"b", "L1", 2}, {"c", "L2", 1}} {
		if err := repo.ReserveStockIdempotent(ctx, r.id, r.listing, "", r.qty, past); err != nil {
			t.Fatalf("reserve %s: %v", r.id, err)
		}
	}
	wantStocks("3 reserves", 8, 10, 9, 7, 4)
	if n, err := repo.SweepExpiredReservations(ctx, time.Now()); err != nil || n != 3 {
		t.Fatalf("sweep n=%d err=%v", n, err)
	}
	got := pendingStockEvents(t, pool)
	if len(got) != 7 {
		t.Fatalf("after sweep: %d events, want 7 (one per listing)", len(got))
	}
	byListing := map[string]int32{}
	for _, ev := range got[5:] {
		byListing[ev.GetListingId()] = ev.GetStock()
	}
	if byListing["L1"] != 10 || byListing["L2"] != 5 {
		t.Fatalf("sweep events = %v, want L1:10 L2:5", byListing)
	}
	if n, _ := repo.SweepExpiredReservations(ctx, time.Now()); n != 0 || len(pendingStockEvents(t, pool)) != 7 {
		t.Fatalf("an idempotent sweep must write nothing")
	}
}

// A variant change announces the listing's stock and only the changed variant
// with its stock after the change.
func TestPG_StockEvents_Variant(t *testing.T) {
	repo, pool := stockEventRepo(t)
	ctx := context.Background()
	if _, err := repo.Create(ctx, repository.Listing{
		ID: "L1", Title: "L1", Currency: "VND", Status: "published", Stock: 3,
		Variants: []repository.Variant{{ID: "V1", Name: "red", SKU: "r", Stock: 6}, {ID: "V2", Name: "blue", SKU: "b", Stock: 4}},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "V1", 2, farFuture); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := repo.ReleaseReservation(ctx, "r1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	got := pendingStockEvents(t, pool)
	if len(got) != 2 {
		t.Fatalf("%d events, want 2", len(got))
	}
	for i, want := range []int32{4, 6} {
		ev := got[i]
		if ev.GetStock() != 3 || len(ev.GetVariants()) != 1 || ev.GetVariants()[0].GetId() != "V1" || ev.GetVariants()[0].GetStock() != want {
			t.Fatalf("event %d = %+v, want stock 3 and only V1 at %d", i, ev, want)
		}
	}
}

// A failing outbox write rolls the stock change back: stock unchanged, no
// reservation row, no event — for reserve, release and sweep.
func TestPG_StockEvents_RollbackLeavesNothing(t *testing.T) {
	repo, pool := newPGRepo(t)
	ctx := context.Background()
	seedPGListing(t, repo, "L1", 10)
	boom := errors.New("outbox down")
	failing := func(context.Context, repository.StockSnapshot) (repository.OutboxRow, error) {
		return repository.OutboxRow{}, boom
	}
	countRows := func(q string) int {
		var n int
		if err := pool.QueryRow(ctx, q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}

	repo.WithStockEvents(repository.NewOutboxStore(pool), failing)
	if err := repo.ReserveStockIdempotent(ctx, "r1", "L1", "", 2, farFuture); !errors.Is(err, boom) {
		t.Fatalf("reserve: want builder error, got %v", err)
	}
	if got := pgStock(t, repo, "L1"); got != 10 {
		t.Fatalf("stock = %d, want 10", got)
	}
	if n := countRows(`SELECT count(*) FROM reservations`); n != 0 {
		t.Fatalf("reservations = %d, want 0", n)
	}

	// Set up an active and an expired reservation without events, then fail.
	repo.WithStockEvents(nil, nil)
	if err := repo.ReserveStockIdempotent(ctx, "r2", "L1", "", 2, farFuture); err != nil {
		t.Fatalf("reserve r2: %v", err)
	}
	if err := repo.ReserveStockIdempotent(ctx, "r3", "L1", "", 3, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("reserve r3: %v", err)
	}
	repo.WithStockEvents(repository.NewOutboxStore(pool), failing)
	if _, err := repo.ReleaseReservation(ctx, "r2"); !errors.Is(err, boom) {
		t.Fatalf("release: want builder error, got %v", err)
	}
	if _, err := repo.SweepExpiredReservations(ctx, time.Now()); !errors.Is(err, boom) {
		t.Fatalf("sweep: want builder error, got %v", err)
	}
	if got := pgStock(t, repo, "L1"); got != 5 {
		t.Fatalf("stock = %d, want 5 (both holds kept)", got)
	}
	if n := countRows(`SELECT count(*) FROM reservations WHERE status = 'active'`); n != 2 {
		t.Fatalf("active reservations = %d, want 2", n)
	}
	if n := countRows(`SELECT count(*) FROM outbox_events`); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}
