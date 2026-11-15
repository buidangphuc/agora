package store_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func statusOf(t *testing.T, repo *store.PostgresAdCampaignRepository, id string) store.AdCampaign {
	t.Helper()
	c, err := repo.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID(%s): %v", id, err)
	}
	return c
}

func TestListBySellerCursorPaging(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	for i := 0; i < 120; i++ {
		if _, err := pool.Exec(t.Context(), `INSERT INTO ad_campaigns (id, seller_id, listing_id, budget, bid, status, created_at)
			VALUES ($1, 'S', $2, 5000, 500, 1, $3)`,
			fmt.Sprintf("c-%03d", i), fmt.Sprintf("l-%03d", i), servingEpoch.Add(-time.Duration(i/7)*time.Minute)); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	insertAd(t, pool, adRow{id: "other"})

	seen := map[string]bool{}
	var order []store.AdCampaign
	var cursor *store.AdCampaignCursor
	var sizes []int
	for page := 0; page < 5; page++ {
		rows, err := repo.ListBySeller(t.Context(), "S", 100, cursor)
		if err != nil {
			t.Fatalf("ListBySeller page %d: %v", page, err)
		}
		if len(rows) == 0 {
			break
		}
		sizes = append(sizes, len(rows))
		for _, r := range rows {
			if r.SellerID != "S" {
				t.Fatalf("leaked seller %q", r.SellerID)
			}
			if seen[r.ID] {
				t.Fatalf("id %s repeated across pages", r.ID)
			}
			seen[r.ID] = true
			order = append(order, r)
		}
		last := rows[len(rows)-1]
		cursor = &store.AdCampaignCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if len(sizes) != 2 || sizes[0] != 100 || sizes[1] != 20 {
		t.Fatalf("page sizes = %v, want [100 20]", sizes)
	}
	if len(seen) != 120 {
		t.Fatalf("distinct ids = %d, want 120", len(seen))
	}
	for i := 1; i < len(order); i++ {
		a, b := order[i-1], order[i]
		if a.CreatedAt.Before(b.CreatedAt) || (a.CreatedAt.Equal(b.CreatedAt) && a.ID < b.ID) {
			t.Fatalf("order violated at %d: (%v,%s) before (%v,%s)", i, a.CreatedAt, a.ID, b.CreatedAt, b.ID)
		}
	}
}

func TestListBySellerCursorEmptyForUnknownSeller(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "a"})
	rows, err := repo.ListBySeller(t.Context(), "nobody", 10, nil)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%d err=%v, want empty", len(rows), err)
	}
}

func TestResumeBoundaryRemainingEqualsBid(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", status: store.AdCampaignStatusPaused, budget: 5000, spent: 4200, bid: 800})
	got, err := repo.Resume(t.Context(), "c1", "seller-c1")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got.Status != store.AdCampaignStatusActive {
		t.Fatalf("returned status = %d, want active", got.Status)
	}
	if s := statusOf(t, repo, "c1").Status; s != store.AdCampaignStatusActive {
		t.Fatalf("persisted status = %d, want active", s)
	}
}

func TestResumeBelowBidStaysPaused(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", status: store.AdCampaignStatusPaused, budget: 5000, spent: 4300, bid: 800})
	got, err := repo.Resume(t.Context(), "c1", "seller-c1")
	if !errors.Is(err, store.ErrAdCampaignState) {
		t.Fatalf("err = %v, want ErrAdCampaignState", err)
	}
	if got.Status != store.AdCampaignStatusPaused {
		t.Fatalf("returned prior status = %d, want paused", got.Status)
	}
	if s := statusOf(t, repo, "c1").Status; s != store.AdCampaignStatusPaused {
		t.Fatalf("persisted status = %d, want paused", s)
	}
}

func TestResumeRejectsBudgetPausedAndEnded(t *testing.T) {
	for _, st := range []int32{store.AdCampaignStatusPausedBudget, store.AdCampaignStatusEnded} {
		pool := newTestPool(t)
		repo := store.NewPostgresAdCampaignRepository(pool)
		insertAd(t, pool, adRow{id: "c1", status: st, budget: 5000, spent: 0, bid: 800})
		got, err := repo.Resume(t.Context(), "c1", "seller-c1")
		if !errors.Is(err, store.ErrAdCampaignState) {
			t.Fatalf("status %d: err = %v, want ErrAdCampaignState", st, err)
		}
		if got.Status != st || statusOf(t, repo, "c1").Status != st {
			t.Fatalf("status %d changed", st)
		}
	}
}

func TestResumeActiveIsIdempotent(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", budget: 5000, spent: 100, bid: 800})
	got, err := repo.Resume(t.Context(), "c1", "seller-c1")
	if err != nil || got.Status != store.AdCampaignStatusActive {
		t.Fatalf("got=%+v err=%v, want active nil", got, err)
	}
}

func TestResumeScopedBySeller(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", status: store.AdCampaignStatusPaused, budget: 5000, bid: 800})
	if _, err := repo.Resume(t.Context(), "c1", "intruder"); !errors.Is(err, store.ErrAdCampaignNotFound) {
		t.Fatalf("foreign resume err = %v, want ErrAdCampaignNotFound", err)
	}
	if _, err := repo.Resume(t.Context(), "missing", "seller-c1"); !errors.Is(err, store.ErrAdCampaignNotFound) {
		t.Fatalf("unknown resume err = %v, want ErrAdCampaignNotFound", err)
	}
	if s := statusOf(t, repo, "c1").Status; s != store.AdCampaignStatusPaused {
		t.Fatalf("status = %d, want paused", s)
	}
}

func TestPauseActiveAndBudgetPausedBecomePaused(t *testing.T) {
	for _, st := range []int32{store.AdCampaignStatusActive, store.AdCampaignStatusPausedBudget, store.AdCampaignStatusPaused} {
		pool := newTestPool(t)
		repo := store.NewPostgresAdCampaignRepository(pool)
		insertAd(t, pool, adRow{id: "c1", status: st, budget: 5000, bid: 800})
		got, err := repo.Pause(t.Context(), "c1", "seller-c1")
		if err != nil {
			t.Fatalf("from %d: Pause: %v", st, err)
		}
		if got.Status != store.AdCampaignStatusPaused || statusOf(t, repo, "c1").Status != store.AdCampaignStatusPaused {
			t.Fatalf("from %d: not paused", st)
		}
	}
}

func TestPauseEndedRejected(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", status: store.AdCampaignStatusEnded, budget: 5000, bid: 800})
	got, err := repo.Pause(t.Context(), "c1", "seller-c1")
	if !errors.Is(err, store.ErrAdCampaignState) {
		t.Fatalf("err = %v, want ErrAdCampaignState", err)
	}
	if got.Status != store.AdCampaignStatusEnded || statusOf(t, repo, "c1").Status != store.AdCampaignStatusEnded {
		t.Fatal("ended campaign changed")
	}
}

func TestPauseScopedBySeller(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	insertAd(t, pool, adRow{id: "c1", budget: 5000, bid: 800})
	if _, err := repo.Pause(t.Context(), "c1", "intruder"); !errors.Is(err, store.ErrAdCampaignNotFound) {
		t.Fatalf("foreign pause err = %v, want ErrAdCampaignNotFound", err)
	}
	if _, err := repo.Pause(t.Context(), "missing", "seller-c1"); !errors.Is(err, store.ErrAdCampaignNotFound) {
		t.Fatalf("unknown pause err = %v, want ErrAdCampaignNotFound", err)
	}
	if s := statusOf(t, repo, "c1").Status; s != store.AdCampaignStatusActive {
		t.Fatalf("status = %d, want active", s)
	}
}

func TestResumeVersusClickNeverOverspends(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresAdCampaignRepository(pool)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("r-%03d", i)
		insertAd(t, pool, adRow{id: id, status: store.AdCampaignStatusPaused, budget: 5000, spent: 4200, bid: 800})
		tok := "tok-" + id
		insertImpression(t, pool, tok, id, "listing-"+id)

		start := make(chan struct{})
		var wg sync.WaitGroup
		var resumeErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, resumeErr = repo.Resume(context.Background(), id, "seller-"+id)
		}()
		go func() {
			defer wg.Done()
			<-start
			_, _ = repo.RecordClick(context.Background(), tok, "buyer-1", servingEpoch.Add(time.Minute))
		}()
		close(start)
		wg.Wait()

		if resumeErr != nil {
			t.Fatalf("rep %d: Resume: %v", i, resumeErr)
		}
		c := statusOf(t, repo, id)
		if c.Spent > c.Budget {
			t.Fatalf("rep %d: spent %d > budget %d", i, c.Spent, c.Budget)
		}
		switch c.Status {
		case store.AdCampaignStatusActive:
			if c.Budget-c.Spent < c.Bid {
				t.Fatalf("rep %d: ACTIVE with remaining %d < bid %d", i, c.Budget-c.Spent, c.Bid)
			}
		case store.AdCampaignStatusPausedBudget:
			if c.Spent != 5000 {
				t.Fatalf("rep %d: PAUSED_BUDGET with spent %d, want 5000", i, c.Spent)
			}
		default:
			t.Fatalf("rep %d: final status %d, want ACTIVE or PAUSED_BUDGET", i, c.Status)
		}
	}
}
