package store_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func insertImpression(t *testing.T, pool *pgxpool.Pool, token, campaign, listing string) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `INSERT INTO ad_impressions (token, campaign_id, listing_id, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`, token, campaign, listing, servingEpoch, servingEpoch.Add(store.ImpressionTTL)); err != nil {
		t.Fatalf("insert impression %s: %v", token, err)
	}
}

type clickOutcome struct {
	res store.ClickResult
	err error
}

func clickAll(repo *store.PostgresAdCampaignRepository, tokens []string) []clickOutcome {
	out := make([]clickOutcome, len(tokens))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, tok := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out[i].res, out[i].err = repo.RecordClick(context.Background(), tok, "buyer-1", servingEpoch.Add(time.Minute))
		}()
	}
	close(start)
	wg.Wait()
	return out
}

func TestConcurrentClicksNeverOverspend(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", category: "C", bid: 500, budget: 2000})
	tokens := make([]string, 20)
	for i := range tokens {
		tokens[i] = "tok-" + string(rune('a'+i))
		insertImpression(t, pool, tokens[i], "c1", "listing-c1")
	}

	charged := 0
	for _, o := range clickAll(repo, tokens) {
		if o.err != nil {
			t.Fatalf("RecordClick: %v", o.err)
		}
		if o.res.ListingID != "listing-c1" {
			t.Fatalf("listing = %q, want listing-c1", o.res.ListingID)
		}
		if o.res.Charged {
			charged++
		}
	}
	if charged != 4 {
		t.Fatalf("charged = %d, want 4", charged)
	}
	c, err := repo.GetByID(t.Context(), "c1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if c.Spent != 2000 || c.Clicks != 4 || c.Status != store.AdCampaignStatusPausedBudget {
		t.Fatalf("spent=%d clicks=%d status=%d, want 2000, 4, PAUSED_BUDGET", c.Spent, c.Clicks, c.Status)
	}
}

func TestConcurrentClicksSameTokenChargeOnce(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", category: "C", bid: 500, budget: 5000})
	insertImpression(t, pool, "same", "c1", "listing-c1")
	tokens := make([]string, 10)
	for i := range tokens {
		tokens[i] = "same"
	}

	charged := 0
	for _, o := range clickAll(repo, tokens) {
		if o.err != nil {
			t.Fatalf("RecordClick: %v", o.err)
		}
		if o.res.Charged {
			charged++
		}
	}
	if charged != 1 {
		t.Fatalf("charged = %d, want 1", charged)
	}
	if c, _ := repo.GetByID(t.Context(), "c1"); c.Spent != 500 || c.Clicks != 1 {
		t.Fatalf("spent=%d clicks=%d, want 500 and 1", c.Spent, c.Clicks)
	}
}

func TestRecordClickRollsBackOnChargeFailure(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", category: "C", bid: 500, budget: 5000})
	insertImpression(t, pool, "tok", "c1", "listing-c1")
	if _, err := pool.Exec(t.Context(), `ALTER TABLE ad_campaigns ADD CONSTRAINT click_always_fails CHECK (clicks = 0)`); err != nil {
		t.Fatalf("add failing constraint: %v", err)
	}

	res, err := repo.RecordClick(t.Context(), "tok", "buyer-1", servingEpoch.Add(time.Minute))
	if err == nil {
		t.Fatalf("RecordClick = %+v, want a database error", res)
	}
	var clicked *time.Time
	if err := pool.QueryRow(t.Context(), `SELECT clicked_at FROM ad_impressions WHERE token = 'tok'`).Scan(&clicked); err != nil {
		t.Fatalf("read token: %v", err)
	}
	if clicked != nil {
		t.Fatalf("token clicked_at = %v, want still unconsumed", clicked)
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE ad_campaigns DROP CONSTRAINT click_always_fails`); err != nil {
		t.Fatalf("drop failing constraint: %v", err)
	}
	retry, err := repo.RecordClick(t.Context(), "tok", "buyer-1", servingEpoch.Add(time.Minute))
	if err != nil || !retry.Charged || retry.ListingID != "listing-c1" {
		t.Fatalf("retry after rollback = %+v err=%v, want charged for listing-c1", retry, err)
	}
}

func TestRecordClickExpiredAndUnknownAtStore(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", category: "C", bid: 500, budget: 5000})
	insertImpression(t, pool, "tok", "c1", "listing-c1")

	res, err := repo.RecordClick(t.Context(), "tok", "", servingEpoch.Add(31*time.Minute))
	if err != nil || res.Charged || res.ListingID != "listing-c1" {
		t.Fatalf("expired: res=%+v err=%v, want uncharged with listing-c1", res, err)
	}
	if _, err := repo.RecordClick(t.Context(), "nope", "", servingEpoch); err == nil {
		t.Fatal("unknown token: want store.ErrImpressionNotFound")
	}
	c, _ := repo.GetByID(t.Context(), "c1")
	if c.Spent != 0 || c.Clicks != 0 {
		t.Fatalf("spent=%d clicks=%d, want 0 and 0", c.Spent, c.Clicks)
	}
}

func TestRecordClickUnknownTokenIsErrImpressionNotFound(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	_, err := repo.RecordClick(t.Context(), "nope", "", servingEpoch)
	if !errors.Is(err, store.ErrImpressionNotFound) {
		t.Fatalf("err = %v, want store.ErrImpressionNotFound", err)
	}
}
