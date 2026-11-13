package listing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type capEnt struct {
	mu      sync.Mutex
	ent     listing.Entitlements
	err     error
	calls   int
	sellers []string
}

func (f *capEnt) Get(_ context.Context, sellerID string) (listing.Entitlements, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.sellers = append(f.sellers, sellerID)
	return f.ent, f.err
}

func (f *capEnt) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type capEnv struct {
	repo *storetest.InMemoryListingRepository
	txw  *storetest.InMemoryTxWriter
	svc  *listing.ListingService
	ent  *capEnt
}

func newCapEnv(seller string, statuses []string, maxListings int64) *capEnv {
	seed := []store.Listing{{ID: "other-1", Title: "other", Currency: "VND", Status: "published", SellerID: "someone-else"}}
	for i, st := range statuses {
		seed = append(seed, store.Listing{
			ID: fmt.Sprintf("%s-%d", seller, i), Title: "t", Currency: "VND", Status: st, SellerID: seller,
		})
	}
	repo := storetest.NewInMemoryListingRepository(seed...)
	txw := storetest.NewInMemoryTxWriter(repo)
	ent := &capEnt{ent: listing.Entitlements{MaxListings: maxListings, FeaturedListings: 0}}
	svc := listing.NewListingServiceWithOutbox(repo, txw).WithEntitlements(ent)
	return &capEnv{repo: repo, txw: txw, svc: svc, ent: ent}
}

func repeatStatus(st string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = st
	}
	return out
}

func (e *capEnv) owned(t *testing.T, seller string) int64 {
	t.Helper()
	p, err := e.repo.ListByOwner(context.Background(), seller, "", 100)
	if err != nil {
		t.Fatalf("ListByOwner: %v", err)
	}
	return p.Total
}

func capEnqueue(l store.Listing) (store.OutboxRow, error) {
	return store.OutboxRow{EventID: "ev-" + l.ID, AggregateType: "Listing", AggregateID: l.ID, EventType: "t", Payload: []byte{1}}, nil
}

func capNewListing() store.Listing {
	return store.Listing{Title: "new", Currency: "VND", Status: "draft", Price: 1}
}

func TestCreateRejectedAtPlanCap(t *testing.T) {
	env := newCapEnv("s1", repeatStatus("published", 10), 10)
	_, err := env.svc.CreateWithEvent(context.Background(), capNewListing(), "s1", capEnqueue)
	if !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("err = %v, want store.ErrListingCapReached", err)
	}
	if got := env.owned(t, "s1"); got != 10 {
		t.Fatalf("owned = %d, want 10", got)
	}
	if len(env.txw.Rows()) != 0 {
		t.Fatalf("outbox rows = %d, want 0", len(env.txw.Rows()))
	}
	if len(env.ent.sellers) != 1 || env.ent.sellers[0] != "s1" {
		t.Fatalf("entitlements asked for %v, want [s1]", env.ent.sellers)
	}
}

func TestCreateAllowedBelowPlanCap(t *testing.T) {
	env := newCapEnv("s1", repeatStatus("published", 9), 10)
	out, err := env.svc.CreateWithEvent(context.Background(), capNewListing(), "s1", capEnqueue)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.SellerID != "s1" {
		t.Fatalf("seller = %q, want s1", out.SellerID)
	}
	if got := env.owned(t, "s1"); got != 10 {
		t.Fatalf("owned = %d, want 10", got)
	}
	if len(env.txw.Rows()) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(env.txw.Rows()))
	}
}

func TestCreateUnlimitedPlanNeverBlocked(t *testing.T) {
	env := newCapEnv("s1", repeatStatus("published", 600), -1)
	if _, err := env.svc.CreateWithEvent(context.Background(), capNewListing(), "s1", capEnqueue); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := env.owned(t, "s1"); got != 601 {
		t.Fatalf("owned = %d, want 601", got)
	}
}

func TestCreateZeroCapBlocks(t *testing.T) {
	env := newCapEnv("s1", nil, 0)
	_, err := env.svc.CreateWithEvent(context.Background(), capNewListing(), "s1", capEnqueue)
	if !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("err = %v, want store.ErrListingCapReached", err)
	}
	if got := env.owned(t, "s1"); got != 0 {
		t.Fatalf("owned = %d, want 0", got)
	}
}

func TestCreateFailsClosedWhenPromotionUnavailable(t *testing.T) {
	cases := map[string]error{
		"plain connection error": errors.New("connection refused"),
		"wrapped unavailable":    fmt.Errorf("%w: down", listing.ErrEntitlementsUnavailable),
	}
	for name, upstream := range cases {
		t.Run(name, func(t *testing.T) {
			env := newCapEnv("s1", nil, 10)
			env.ent.err = upstream
			_, err := env.svc.CreateWithEvent(context.Background(), capNewListing(), "s1", capEnqueue)
			if !errors.Is(err, listing.ErrEntitlementsUnavailable) {
				t.Fatalf("err = %v, want listing.ErrEntitlementsUnavailable", err)
			}
			if errors.Is(err, store.ErrListingCapReached) {
				t.Fatalf("an upstream failure must not look like a cap rejection")
			}
			if got := env.owned(t, "s1"); got != 0 {
				t.Fatalf("owned = %d, want 0", got)
			}
			if len(env.txw.Rows()) != 0 {
				t.Fatalf("outbox rows = %d, want 0", len(env.txw.Rows()))
			}
		})
	}
}

func TestOverCapSellerKeepsListings(t *testing.T) {
	env := newCapEnv("s1", repeatStatus("draft", 14), 10)
	ctx := context.Background()

	page, err := env.svc.ListMine(ctx, "s1", "", 100)
	if err != nil || len(page.Items) != 14 {
		t.Fatalf("ListMine = %d items, err %v; want 14", len(page.Items), err)
	}
	edit, err := env.svc.Get(ctx, "s1-0")
	if err != nil {
		t.Fatal(err)
	}
	edit.Title = "edited"
	if _, err := env.svc.UpdateWithEvent(ctx, edit, "s1", false, capEnqueue); err != nil {
		t.Fatalf("update while over cap: %v", err)
	}
	if got, _ := env.svc.Get(ctx, "s1-0"); got.Title != "edited" {
		t.Fatalf("title = %q, want edited", got.Title)
	}
	if env.ent.callCount() != 0 {
		t.Fatalf("list/update consulted entitlements %d times", env.ent.callCount())
	}
	_, err = env.svc.CreateWithEvent(ctx, capNewListing(), "s1", capEnqueue)
	if !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("create err = %v, want store.ErrListingCapReached", err)
	}
	if got := env.owned(t, "s1"); got != 14 {
		t.Fatalf("owned = %d, want 14", got)
	}
}

func TestDeleteBelowCapReenablesCreate(t *testing.T) {
	env := newCapEnv("s1", repeatStatus("published", 11), 10)
	ctx := context.Background()
	for _, id := range []string{"s1-0", "s1-1"} {
		if _, err := env.svc.DeleteWithEvent(ctx, id, "s1", false, capEnqueue); err != nil {
			t.Fatalf("delete %s: %v", id, err)
		}
	}
	if _, err := env.svc.CreateWithEvent(ctx, capNewListing(), "s1", capEnqueue); err != nil {
		t.Fatalf("create after deletes: %v", err)
	}
	if got := env.owned(t, "s1"); got != 10 {
		t.Fatalf("owned = %d, want 10", got)
	}
}

func TestCreateWithoutOutboxStillCapped(t *testing.T) {
	repo := storetest.NewInMemoryListingRepository(
		store.Listing{ID: "a", Title: "a", Currency: "VND", Status: "draft", SellerID: "s1"},
	)
	ent := &capEnt{ent: listing.Entitlements{MaxListings: 1}}
	svc := listing.NewListingService(repo).WithEntitlements(ent)
	_, err := svc.Create(context.Background(), capNewListing(), "s1")
	if !errors.Is(err, store.ErrListingCapReached) {
		t.Fatalf("err = %v, want store.ErrListingCapReached", err)
	}
}
