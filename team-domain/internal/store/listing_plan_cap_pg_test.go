package store_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-domain/internal/store"
)

func capEvent(l store.Listing) (store.OutboxRow, error) {
	return store.OutboxRow{
		EventID: newID(), AggregateType: "Listing", AggregateID: l.ID,
		EventType: "platform.listing.v1.ListingChanged", Payload: []byte{0x01},
	}, nil
}

func seedStatuses(t *testing.T, repo *store.PostgresListingRepository, pool *pgxpool.Pool, seller string, counts map[string]int) {
	t.Helper()
	for st, n := range counts {
		for i := 0; i < n; i++ {
			seedOwned(t, repo, pool, fmt.Sprintf("%s-%s-%d", seller, st, i), seller, st)
		}
	}
}

func TestCountSellerListingsAllStatuses(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	seedStatuses(t, repo, pool, "sA", map[string]int{"published": 4, "draft": 3, "hidden": 2, "sold_out": 1})
	seedStatuses(t, repo, pool, "sB", map[string]int{"published": 5})

	got, err := repo.CountBySeller(t.Context(), "sA")
	if err != nil {
		t.Fatalf("CountBySeller: %v", err)
	}
	if got != 10 {
		t.Fatalf("count = %d, want 10 (every status counts)", got)
	}

	txw := store.NewPgTxWriter(pool, newOutboxStore(t, pool))
	_, err = txw.CreateCappedTx(t.Context(), store.Listing{ID: newID(), Title: "x", Currency: "VND", Status: "draft", SellerID: "sA"}, 10, capEvent)
	if !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("CreateCappedTx err = %v, want ErrListingCapReached", err)
	}
	if n, _ := repo.CountBySeller(t.Context(), "sA"); n != 10 {
		t.Fatalf("count after reject = %d, want 10", n)
	}
	var outboxRows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events`).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 0 {
		t.Fatalf("outbox rows after reject = %d, want 0", outboxRows)
	}
}

func TestCreateCappedBelowUnlimitedAndZero(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	txw := store.NewPgTxWriter(pool, newOutboxStore(t, pool))
	mk := func(seller string) store.Listing {
		return store.Listing{ID: newID(), Title: "x", Currency: "VND", Status: "draft", SellerID: seller}
	}
	if _, err := txw.CreateCappedTx(t.Context(), mk("z"), 0, capEvent); !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("zero cap err = %v, want ErrListingCapReached", err)
	}
	if _, err := txw.CreateCappedTx(t.Context(), mk("u"), -1, capEvent); err != nil {
		t.Fatalf("unlimited: %v", err)
	}
	if _, err := txw.CreateCappedTx(t.Context(), mk("b"), 1, capEvent); err != nil {
		t.Fatalf("below cap: %v", err)
	}
	if _, err := repo.CreateCapped(t.Context(), mk("b"), 1); !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("repo CreateCapped at cap err = %v, want ErrListingCapReached", err)
	}
	if n, _ := repo.CountBySeller(t.Context(), "b"); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestConcurrentCreatesRespectPlanCap(t *testing.T) {
	repo, pool := newPGRepo(t.Context(), t)
	seedStatuses(t, repo, pool, "sA", map[string]int{"published": 8})
	txw := store.NewPgTxWriter(pool, newOutboxStore(t, pool))

	const workers = 6
	errs := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = txw.CreateCappedTx(t.Context(),
				store.Listing{ID: newID(), Title: "x", Currency: "VND", Status: "draft", SellerID: "sA"}, 10, capEvent)
		}()
	}
	close(start)
	wg.Wait()

	ok, rejected := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, store.ErrListingCapReached):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 2 || rejected != 4 {
		t.Fatalf("ok=%d rejected=%d, want 2 and 4", ok, rejected)
	}
	if n, err := repo.CountBySeller(t.Context(), "sA"); err != nil || n != 10 {
		t.Fatalf("count = %d, err %v; want 10", n, err)
	}
	var outboxRows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_events`).Scan(&outboxRows); err != nil {
		t.Fatal(err)
	}
	if outboxRows != 2 {
		t.Fatalf("outbox rows = %d, want 2", outboxRows)
	}
}
