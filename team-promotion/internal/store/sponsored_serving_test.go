package store_test

import (
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

var servingEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type adRow struct {
	id, listing, category string
	bid, budget, spent    int64
	status                int32
	age                   time.Duration
}

func insertAd(t *testing.T, pool *pgxpool.Pool, r adRow) {
	t.Helper()
	if r.listing == "" {
		r.listing = "listing-" + r.id
	}
	if r.status == 0 {
		r.status = store.AdCampaignStatusActive
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status, created_at, spent, category_id)
		VALUES ($1, 'seller-'||$1, $2, $3, $4, $5, $6, $7, $8)`,
		r.id, r.listing, r.budget, r.bid, r.status, servingEpoch.Add(-r.age), r.spent, r.category); err != nil {
		t.Fatalf("insert campaign %s: %v", r.id, err)
	}
}

func servableIDs(t *testing.T, repo *store.PostgresAdCampaignRepository, category string, limit int) []string {
	t.Helper()
	rows, err := repo.ListServable(t.Context(), category, limit)
	if err != nil {
		t.Fatalf("ListServable(%q): %v", category, err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestListServableExcludesNonServable(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "ok", category: "C", bid: 800, budget: 5000})
	insertAd(t, pool, adRow{id: "exact", category: "C", bid: 800, budget: 1000, spent: 200})
	insertAd(t, pool, adRow{id: "paused", category: "C", bid: 900, budget: 5000, status: store.AdCampaignStatusPaused})
	insertAd(t, pool, adRow{id: "paused-budget", category: "C", bid: 900, budget: 5000, status: store.AdCampaignStatusPausedBudget})
	insertAd(t, pool, adRow{id: "ended", category: "C", bid: 900, budget: 5000, status: store.AdCampaignStatusEnded})
	insertAd(t, pool, adRow{id: "short", category: "C", bid: 900, budget: 5000, spent: 4200})

	got := servableIDs(t, repo, "C", 50)
	sort.Strings(got)
	if want := []string{"exact", "ok"}; !equalStrings(got, want) {
		t.Fatalf("servable = %v, want %v", got, want)
	}
}

func TestListServableOrdersByBidThenOldestAndFiltersCategory(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "low", category: "C", bid: 500, budget: 5000})
	insertAd(t, pool, adRow{id: "tie-new", category: "C", bid: 800, budget: 5000, age: time.Hour})
	insertAd(t, pool, adRow{id: "tie-old", category: "C", bid: 800, budget: 5000, age: 24 * time.Hour})
	insertAd(t, pool, adRow{id: "top", category: "C", bid: 900, budget: 5000})
	insertAd(t, pool, adRow{id: "other-cat", category: "D", bid: 1000, budget: 5000})
	insertAd(t, pool, adRow{id: "no-cat", category: "", bid: 950, budget: 5000})

	if got, want := servableIDs(t, repo, "C", 50), []string{"top", "tie-old", "tie-new", "low"}; !equalStrings(got, want) {
		t.Fatalf("category C = %v, want %v", got, want)
	}
	if got, want := servableIDs(t, repo, "", 50), []string{"other-cat", "no-cat", "top", "tie-old", "tie-new", "low"}; !equalStrings(got, want) {
		t.Fatalf("all categories = %v, want %v", got, want)
	}
	if got, want := servableIDs(t, repo, "", 2), []string{"other-cat", "no-cat"}; !equalStrings(got, want) {
		t.Fatalf("limit 2 = %v, want %v", got, want)
	}
}

func impressionsOf(t *testing.T, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(t.Context(), `SELECT impressions FROM ad_campaigns WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatalf("read impressions of %s: %v", id, err)
	}
	return n
}

func tokenCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM ad_impressions`).Scan(&n); err != nil {
		t.Fatalf("count ad_impressions: %v", err)
	}
	return n
}

func TestServeInsertsTokensAndCountsImpressions(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "a", category: "C", bid: 800, budget: 5000})
	insertAd(t, pool, adRow{id: "b", category: "C", bid: 700, budget: 5000})

	err := repo.IssueImpressions(t.Context(), []store.ImpressionIssue{
		{Token: "tok-a", CampaignID: "a", ListingID: "listing-a"},
		{Token: "tok-b", CampaignID: "b", ListingID: "listing-b"},
	})
	if err != nil {
		t.Fatalf("IssueImpressions: %v", err)
	}
	if n := tokenCount(t, pool); n != 2 {
		t.Fatalf("ad_impressions rows = %d, want 2", n)
	}
	rows, err := pool.Query(t.Context(), `SELECT token, campaign_id, listing_id, expires_at - created_at, clicked_at IS NULL FROM ad_impressions ORDER BY token`)
	if err != nil {
		t.Fatalf("read tokens: %v", err)
	}
	defer rows.Close()
	want := map[string][2]string{"tok-a": {"a", "listing-a"}, "tok-b": {"b", "listing-b"}}
	for rows.Next() {
		var tok, camp, lst string
		var ttl time.Duration
		var unclicked bool
		if err := rows.Scan(&tok, &camp, &lst, &ttl, &unclicked); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if w := want[tok]; w[0] != camp || w[1] != lst {
			t.Fatalf("token %s -> campaign %s listing %s, want %v", tok, camp, lst, w)
		}
		if ttl != 30*time.Minute || !unclicked {
			t.Fatalf("token %s ttl = %v unclicked = %v, want 30m true", tok, ttl, unclicked)
		}
	}
	if impressionsOf(t, pool, "a") != 1 || impressionsOf(t, pool, "b") != 1 {
		t.Fatalf("impressions = %d, %d; want 1, 1", impressionsOf(t, pool, "a"), impressionsOf(t, pool, "b"))
	}
}

func TestServeRollsBackOnInsertFailure(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "a", category: "C", bid: 800, budget: 5000})
	insertAd(t, pool, adRow{id: "b", category: "C", bid: 700, budget: 5000})
	if err := repo.IssueImpressions(t.Context(), []store.ImpressionIssue{{Token: "dup", CampaignID: "a", ListingID: "listing-a"}}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	err := repo.IssueImpressions(t.Context(), []store.ImpressionIssue{
		{Token: "fresh", CampaignID: "b", ListingID: "listing-b"},
		{Token: "dup", CampaignID: "a", ListingID: "listing-a"},
	})
	if err == nil {
		t.Fatal("IssueImpressions with a duplicate token succeeded, want an error")
	}
	if n := tokenCount(t, pool); n != 1 {
		t.Fatalf("ad_impressions rows = %d after failed batch, want 1", n)
	}
	if impressionsOf(t, pool, "a") != 1 || impressionsOf(t, pool, "b") != 0 {
		t.Fatalf("impressions a=%d b=%d after failed batch, want 1 and 0", impressionsOf(t, pool, "a"), impressionsOf(t, pool, "b"))
	}
}

func TestAdImpressionsSchemaHoldsNoBuyerData(t *testing.T) {
	pool := newTestPool(t)
	rows, err := pool.Query(t.Context(), `SELECT column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'ad_impressions' ORDER BY column_name`)
	if err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, c)
	}
	want := []string{"campaign_id", "clicked_at", "created_at", "expires_at", "listing_id", "token"}
	if !equalStrings(got, want) {
		t.Fatalf("ad_impressions columns = %v, want %v", got, want)
	}
}
