package app_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	"github.com/buidangphuc/team-promotion/internal/app"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type fakeListings struct {
	mu      sync.Mutex
	sellers map[string]string
	failOn  string
	calls   int
}

func (f *fakeListings) GetListing(_ context.Context, in *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	id := in.GetId()
	if id == f.failOn {
		return nil, status.Error(codes.Unavailable, "team-domain down")
	}
	seller, ok := f.sellers[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "listing not found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: id, SellerId: seller}}, nil
}

func seedLegacy(t *testing.T, pool *pgxpool.Pool, id, listing string) {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`INSERT INTO flash_sale_campaigns (id, listing_id, variant_id, sale_price, stock_cap, starts_at, ends_at)
		 VALUES ($1, $2, 'v', 100, 5, $3, $4)`, id, listing, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("seed legacy %s: %v", id, err)
	}
}

func sellerOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(t.Context(), `SELECT seller_id FROM flash_sale_campaigns WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("read seller_id %s: %v", id, err)
	}
	return s
}

func backfillRepo(t *testing.T) (*pgxpool.Pool, *store.PostgresFlashSaleRepository) {
	t.Helper()
	pool := openPool(t, migratedURL(t))
	return pool, store.NewPostgresFlashSaleRepository(pool, nil)
}

func TestBackfillCampaignSellersIdempotent(t *testing.T) {
	pool, repo := backfillRepo(t)
	seedLegacy(t, pool, "c-1", "l-1")
	seedLegacy(t, pool, "c-2", "l-2")
	seedLegacy(t, pool, "c-3", "l-3")
	listings := &fakeListings{sellers: map[string]string{"l-1": "A", "l-2": "A", "l-3": "B"}}

	first, err := app.BackfillCampaignSellers(t.Context(), repo, listings)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if first.Updated != 3 || first.Skipped != 0 {
		t.Fatalf("first summary = %+v, want 3 updated 0 skipped", first)
	}
	for id, want := range map[string]string{"c-1": "A", "c-2": "A", "c-3": "B"} {
		if got := sellerOf(t, pool, id); got != want {
			t.Errorf("%s seller_id = %q, want %q", id, got, want)
		}
	}
	second, err := app.BackfillCampaignSellers(t.Context(), repo, listings)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.Updated != 0 {
		t.Fatalf("second summary = %+v, want 0 updated", second)
	}
	for id, want := range map[string]string{"c-1": "A", "c-2": "A", "c-3": "B"} {
		if got := sellerOf(t, pool, id); got != want {
			t.Errorf("%s changed on rerun: %q, want %q", id, got, want)
		}
	}
}

func TestBackfillCampaignSellersSkipsMissingListing(t *testing.T) {
	pool, repo := backfillRepo(t)
	seedLegacy(t, pool, "c-1", "l-1")
	seedLegacy(t, pool, "c-gone", "l-gone")
	listings := &fakeListings{sellers: map[string]string{"l-1": "A"}}

	sum, err := app.BackfillCampaignSellers(t.Context(), repo, listings)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Updated != 1 || sum.Skipped != 1 {
		t.Fatalf("summary = %+v, want 1 updated 1 skipped", sum)
	}
	if got := sellerOf(t, pool, "c-gone"); got != "" {
		t.Fatalf("deleted-listing campaign seller_id = %q, want empty", got)
	}
	mine, err := repo.ListMyCampaigns(t.Context(), "A", nil, time.Now(), 0, 50)
	if err != nil || len(mine) != 1 || mine[0].ID != "c-1" {
		t.Fatalf("seller A campaigns = %v err=%v, want only c-1", mine, err)
	}
}

func TestBackfillCampaignSellersStopsOnLookupFailure(t *testing.T) {
	pool, repo := backfillRepo(t)
	seedLegacy(t, pool, "c-1", "l-1")
	seedLegacy(t, pool, "c-2", "l-2")
	seedLegacy(t, pool, "c-3", "l-3")
	listings := &fakeListings{sellers: map[string]string{"l-1": "A", "l-2": "A", "l-3": "B"}, failOn: "l-3"}

	if _, err := app.BackfillCampaignSellers(t.Context(), repo, listings); err == nil {
		t.Fatal("run with team-domain down returned nil, want error")
	}
	for _, id := range []string{"c-1", "c-2", "c-3"} {
		if got := sellerOf(t, pool, id); got != "" {
			t.Errorf("%s seller_id = %q after failed batch, want empty", id, got)
		}
	}

	listings.failOn = ""
	sum, err := app.BackfillCampaignSellers(t.Context(), repo, listings)
	if err != nil {
		t.Fatalf("re-run after recovery: %v", err)
	}
	if sum.Updated != 3 {
		t.Fatalf("recovery summary = %+v, want 3 updated", sum)
	}
	if got := sellerOf(t, pool, "c-3"); got != "B" {
		t.Fatalf("c-3 seller_id = %q, want B", got)
	}
}

func TestBackfillCampaignSellersStopsOnLookupFailure_FinishedBatchesStayIntact(t *testing.T) {
	pool, repo := backfillRepo(t)
	sellers := map[string]string{}
	for i := 1; i <= 101; i++ {
		id := fmt.Sprintf("c-%03d", i)
		seedLegacy(t, pool, id, "l-"+id)
		sellers["l-"+id] = "A"
	}
	listings := &fakeListings{sellers: sellers, failOn: "l-c-101"}

	if _, err := app.BackfillCampaignSellers(t.Context(), repo, listings); err == nil {
		t.Fatal("want error when the lookup for the 101st campaign fails")
	}
	if got := sellerOf(t, pool, "c-100"); got != "A" {
		t.Errorf("c-100 (first batch of 100) seller_id = %q, want A", got)
	}
	if got := sellerOf(t, pool, "c-101"); got != "" {
		t.Errorf("c-101 (failed batch) seller_id = %q, want empty", got)
	}
}

func TestBackfillCampaignSellersSubcommandRegistered(t *testing.T) {
	migrateEnv(t, closedDatabaseURL)
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "127.0.0.1:1")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := app.Execute(ctx, []string{"backfill-campaign-sellers"})
	if err == nil {
		t.Fatal("want an error against an unreachable database")
	}
	if strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("subcommand not registered: %v", err)
	}
}
