package store_test

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-promotion/internal/store"
)

var campaignClock = time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)

func newCampaignRepo(t *testing.T) (*pgxpool.Pool, *store.PostgresFlashSaleRepository) {
	t.Helper()
	pool := newTestPool(t)
	return pool, store.NewPostgresFlashSaleRepository(pool, newOutbox(t, pool))
}

func seedCampaign(t *testing.T, repo *store.PostgresFlashSaleRepository, id string, starts, ends time.Time, capacity int64) store.FlashSaleCampaign {
	t.Helper()
	c, err := repo.Create(t.Context(), store.FlashSaleCampaign{
		ID: id, ListingID: "listing-" + id, VariantID: "v", SalePrice: 100, StockCap: capacity, StartsAt: starts, EndsAt: ends,
	}, campaignEvent)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return c
}

func ids(cs []store.FlashSaleCampaign) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func equalIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestListCampaignsFiltersByStatus_ListFiltersByStatus(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedCampaign(t, repo, "c-sched", now.Add(time.Hour), now.Add(2*time.Hour), 5)
	seedCampaign(t, repo, "c-active", now.Add(-time.Hour), now.Add(time.Hour), 5)
	seedCampaign(t, repo, "c-ended", now.Add(-3*time.Hour), now.Add(-2*time.Hour), 5)
	seedCampaign(t, repo, "c-cancel", now.Add(-time.Hour), now.Add(time.Hour), 5)
	if _, err := repo.Cancel(t.Context(), "c-cancel", now); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	cases := []struct {
		name     string
		statuses []string
		want     []string
	}{
		{"scheduled", []string{store.CampaignStatusScheduled}, []string{"c-sched"}},
		{"active excludes cancelled", []string{store.CampaignStatusActive}, []string{"c-active"}},
		{"ended", []string{store.CampaignStatusEnded}, []string{"c-ended"}},
		{"cancelled", []string{store.CampaignStatusCancelled}, []string{"c-cancel"}},
		{"two statuses newest first", []string{store.CampaignStatusScheduled, store.CampaignStatusEnded}, []string{"c-sched", "c-ended"}},
		{"empty means all", nil, []string{"c-sched", "c-active", "c-cancel", "c-ended"}},
	}
	for _, tc := range cases {
		got, err := repo.ListCampaigns(t.Context(), tc.statuses, now, 0, 50)
		if err != nil {
			t.Fatalf("%s: ListCampaigns: %v", tc.name, err)
		}
		if !equalIDs(ids(got), tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, ids(got), tc.want)
		}
	}
}

func TestListCampaignsWindowEdges_StatusDerivationAtWindowEdges(t *testing.T) {
	_, repo := newCampaignRepo(t)
	starts, ends := campaignClock, campaignClock.Add(time.Hour)
	seedCampaign(t, repo, "c-edge", starts, ends, 5)

	cases := []struct {
		name   string
		now    time.Time
		status string
	}{
		{"just before start", starts.Add(-time.Nanosecond), store.CampaignStatusScheduled},
		{"at starts_at", starts, store.CampaignStatusActive},
		{"just before end", ends.Add(-time.Nanosecond), store.CampaignStatusActive},
		{"at ends_at", ends, store.CampaignStatusEnded},
	}
	for _, tc := range cases {
		got, err := repo.ListCampaigns(t.Context(), []string{tc.status}, tc.now, 0, 10)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !equalIDs(ids(got), []string{"c-edge"}) {
			t.Fatalf("%s: filter %q got %v, want [c-edge]", tc.name, tc.status, ids(got))
		}
	}
}

func TestListCampaignsOrderAndPaging_PagingBoundary(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedCampaign(t, repo, "c-1", now.Add(1*time.Hour), now.Add(10*time.Hour), 5)
	seedCampaign(t, repo, "c-2", now.Add(2*time.Hour), now.Add(10*time.Hour), 5)
	seedCampaign(t, repo, "c-3a", now.Add(3*time.Hour), now.Add(10*time.Hour), 5)
	seedCampaign(t, repo, "c-3b", now.Add(3*time.Hour), now.Add(10*time.Hour), 5)

	cases := []struct {
		name          string
		offset, limit int
		want          []string
	}{
		{"all newest first, ties by id", 0, 50, []string{"c-3a", "c-3b", "c-2", "c-1"}},
		{"first page", 0, 2, []string{"c-3a", "c-3b"}},
		{"second page full", 2, 2, []string{"c-2", "c-1"}},
		{"last partial page", 3, 2, []string{"c-1"}},
		{"offset equals total", 4, 2, []string{}},
		{"offset beyond total", 9, 2, []string{}},
		{"limit one", 1, 1, []string{"c-3b"}},
	}
	for _, tc := range cases {
		got, err := repo.ListCampaigns(t.Context(), nil, now, tc.offset, tc.limit)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !equalIDs(ids(got), tc.want) {
			t.Fatalf("%s: got %v, want %v", tc.name, ids(got), tc.want)
		}
	}
}

func TestCancelCampaign_CancelIsAtomicAndIdempotent(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedCampaign(t, repo, "c-act", now.Add(-time.Hour), now.Add(time.Hour), 5)
	seedCampaign(t, repo, "c-sched", now.Add(time.Hour), now.Add(2*time.Hour), 5)
	seedCampaign(t, repo, "c-ended", now.Add(-2*time.Hour), now.Add(-time.Hour), 5)

	first, err := repo.Cancel(t.Context(), "c-act", now)
	if err != nil || first.IsZero() {
		t.Fatalf("first cancel = %v, %v; want non-zero time", first, err)
	}
	second, err := repo.Cancel(t.Context(), "c-act", now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if !second.Equal(first) {
		t.Fatalf("second cancelled_at = %v, want unchanged %v", second, first)
	}
	if _, err := repo.Cancel(t.Context(), "c-sched", now); err != nil {
		t.Fatalf("cancel scheduled: %v", err)
	}
	if _, err := repo.Cancel(t.Context(), "c-ended", now); !errors.Is(err, store.ErrCampaignEnded) {
		t.Fatalf("cancel ended err = %v, want ErrCampaignEnded", err)
	}
	if _, err := repo.Cancel(t.Context(), "c-nope", now); !errors.Is(err, store.ErrCampaignNotFound) {
		t.Fatalf("cancel unknown err = %v, want ErrCampaignNotFound", err)
	}
	ended, err := repo.ListCampaigns(t.Context(), []string{store.CampaignStatusEnded}, now, 0, 10)
	if err != nil || !equalIDs(ids(ended), []string{"c-ended"}) {
		t.Fatalf("ended campaign after refused cancel = %v, %v", ids(ended), err)
	}
}

func TestCancelRacesReserve(t *testing.T) {
	pool, repo := newCampaignRepo(t)
	holds := store.NewPostgresFlashHoldRepository(pool)
	now := campaignClock
	const capacity = 5
	seedCampaign(t, repo, "c-race", now.Add(-time.Hour), now.Add(time.Hour), capacity)

	const workers = 24
	var cancelled atomic.Bool
	var wg sync.WaitGroup
	start := make(chan struct{})
	type outcome struct {
		afterCancel bool
		err         error
	}
	results := make([]outcome, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			after := cancelled.Load()
			_, _, err := holds.Reserve(t.Context(), store.FlashHold{
				ReservationID: fmt.Sprintf("r-%d", i), CampaignID: "c-race", ListingID: "listing-c-race", VariantID: "v", Quantity: 1,
			}, now)
			results[i] = outcome{afterCancel: after, err: err}
		}()
	}
	var cancelErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = repo.Cancel(t.Context(), "c-race", now)
		cancelled.Store(true)
	}()
	close(start)
	wg.Wait()
	if cancelErr != nil {
		t.Fatalf("cancel: %v", cancelErr)
	}

	for i, r := range results {
		if r.afterCancel && !errors.Is(r.err, store.ErrCampaignCancelled) {
			t.Fatalf("reserve %d started after cancel returned: err = %v, want ErrCampaignCancelled", i, r.err)
		}
		if r.err != nil && !errors.Is(r.err, store.ErrCampaignCancelled) && !errors.Is(r.err, store.ErrFlashStockInsufficient) {
			t.Fatalf("reserve %d: unexpected err %v", i, r.err)
		}
	}
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM flash_sale_holds WHERE campaign_id = 'c-race'`).Scan(&n); err != nil {
		t.Fatalf("count holds: %v", err)
	}
	if n > capacity {
		t.Fatalf("holds = %d, exceeds cap %d", n, capacity)
	}
	_, _, err := holds.Reserve(t.Context(), store.FlashHold{
		ReservationID: "r-late", CampaignID: "c-race", ListingID: "listing-c-race", VariantID: "v", Quantity: 1,
	}, now)
	if !errors.Is(err, store.ErrCampaignCancelled) {
		t.Fatalf("reserve after cancel err = %v, want ErrCampaignCancelled", err)
	}
}

func TestCancelKeepsExistingHoldsAndSold_CancelStopsNewHoldsKeepsExisting(t *testing.T) {
	pool, repo := newCampaignRepo(t)
	holds := store.NewPostgresFlashHoldRepository(pool)
	now := campaignClock
	seedCampaign(t, repo, "c-keep", now.Add(-time.Hour), now.Add(time.Hour), 10)
	for _, id := range []string{"r-sold-1", "r-sold-2", "r-held"} {
		if _, created, err := holds.Reserve(t.Context(), store.FlashHold{
			ReservationID: id, CampaignID: "c-keep", ListingID: "listing-c-keep", VariantID: "v", Quantity: 1,
		}, now); err != nil || !created {
			t.Fatalf("reserve %s: created=%v err=%v", id, created, err)
		}
	}
	for _, id := range []string{"r-sold-1", "r-sold-2"} {
		if ok, err := holds.Commit(t.Context(), id, now); err != nil || !ok {
			t.Fatalf("commit %s: %v %v", id, ok, err)
		}
	}
	if _, err := repo.Cancel(t.Context(), "c-keep", now); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	_, _, err := holds.Reserve(t.Context(), store.FlashHold{
		ReservationID: "r-new", CampaignID: "c-keep", ListingID: "listing-c-keep", VariantID: "v", Quantity: 1,
	}, now)
	if !errors.Is(err, store.ErrCampaignCancelled) {
		t.Fatalf("new reserve err = %v, want ErrCampaignCancelled", err)
	}
	if ok, err := holds.Commit(t.Context(), "r-held", now); err != nil || !ok {
		t.Fatalf("commit pre-cancel hold after cancel: %v %v", ok, err)
	}
	got, err := repo.GetByID(t.Context(), "c-keep")
	if err != nil || got.StockSold != 3 {
		t.Fatalf("stock_sold = %d err=%v, want 3", got.StockSold, err)
	}
	if found, err := holds.Release(t.Context(), "r-sold-1", now); err != nil || !found {
		t.Fatalf("release of existing hold after cancel: %v %v", found, err)
	}
	got, _ = repo.GetByID(t.Context(), "c-keep")
	if got.StockSold != 2 {
		t.Fatalf("stock_sold after release = %d, want 2", got.StockSold)
	}
}

func TestUpdateCampaignStartRace(t *testing.T) {
	_, repo := newCampaignRepo(t)
	starts := campaignClock.Add(time.Hour)
	before := seedCampaign(t, repo, "c-upd", starts, starts.Add(time.Hour), 5)
	edit := store.FlashSaleCampaign{
		SalePrice: 90, StockCap: 50, StartsAt: starts.Add(24 * time.Hour), EndsAt: starts.Add(48 * time.Hour),
	}

	cases := []struct {
		name string
		now  time.Time
	}{
		{"clock passed starts_at after the check", starts.Add(time.Second)},
		{"clock exactly at starts_at", starts},
	}
	for _, tc := range cases {
		if _, err := repo.UpdateIfScheduled(t.Context(), "c-upd", edit, tc.now); !errors.Is(err, store.ErrCampaignNotScheduled) {
			t.Fatalf("%s: err = %v, want ErrCampaignNotScheduled", tc.name, err)
		}
		after, err := repo.GetByID(t.Context(), "c-upd")
		if err != nil {
			t.Fatalf("%s: reload: %v", tc.name, err)
		}
		if after.SalePrice != before.SalePrice || after.StockCap != before.StockCap || !after.StartsAt.Equal(before.StartsAt) || !after.EndsAt.Equal(before.EndsAt) {
			t.Fatalf("%s: stored values changed: %+v, want %+v", tc.name, after, before)
		}
	}

	updated, err := repo.UpdateIfScheduled(t.Context(), "c-upd", edit, campaignClock)
	if err != nil {
		t.Fatalf("update while scheduled: %v", err)
	}
	if updated.SalePrice != 90 || updated.StockCap != 50 || !updated.StartsAt.Equal(edit.StartsAt) || !updated.EndsAt.Equal(edit.EndsAt) {
		t.Fatalf("returned = %+v, want edited values", updated)
	}
	if updated.ListingID != before.ListingID || updated.StockSold != 0 {
		t.Fatalf("update touched listing or sold: %+v", updated)
	}
	stored, _ := repo.GetByID(t.Context(), "c-upd")
	if stored.SalePrice != 90 || stored.StockCap != 50 || !stored.StartsAt.Equal(edit.StartsAt) {
		t.Fatalf("stored = %+v, want edited values", stored)
	}
}

func TestUpdateIfScheduledRefusals_EditStartedCampaignIsRefused(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedCampaign(t, repo, "c-act", now.Add(-time.Hour), now.Add(time.Hour), 5)
	seedCampaign(t, repo, "c-ended", now.Add(-2*time.Hour), now.Add(-time.Hour), 5)
	seedCampaign(t, repo, "c-canc", now.Add(time.Hour), now.Add(2*time.Hour), 5)
	if _, err := repo.Cancel(t.Context(), "c-canc", now); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	edit := store.FlashSaleCampaign{SalePrice: 90, StockCap: 50, StartsAt: now.Add(5 * time.Hour), EndsAt: now.Add(6 * time.Hour)}
	for _, id := range []string{"c-act", "c-ended", "c-canc"} {
		if _, err := repo.UpdateIfScheduled(t.Context(), id, edit, now); !errors.Is(err, store.ErrCampaignNotScheduled) {
			t.Fatalf("%s: err = %v, want ErrCampaignNotScheduled", id, err)
		}
		got, _ := repo.GetByID(t.Context(), id)
		if got.SalePrice != 100 || got.StockCap != 5 {
			t.Fatalf("%s: stored values changed: %+v", id, got)
		}
	}
	if _, err := repo.UpdateIfScheduled(t.Context(), "c-nope", edit, now); !errors.Is(err, store.ErrCampaignNotFound) {
		t.Fatalf("unknown err = %v, want ErrCampaignNotFound", err)
	}
}

func TestCampaignIdempotencyConcurrent(t *testing.T) {
	pool, repo := newCampaignRepo(t)
	now := campaignClock
	const workers = 8
	type outcome struct {
		c       store.FlashSaleCampaign
		created bool
		err     error
	}
	results := make([]outcome, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c, created, err := repo.CreateIdempotent(t.Context(), "admin-a", "retry-key-0001", "hash-1", store.FlashSaleCampaign{
				ListingID: "l-1", VariantID: "v", SalePrice: 100, StockCap: 5, StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour),
			}, now, campaignEvent)
			results[i] = outcome{c, created, err}
		}()
	}
	close(start)
	wg.Wait()

	createdCount := 0
	var id string
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("worker %d: %v", i, r.err)
		}
		if r.created {
			createdCount++
		}
		if id == "" {
			id = r.c.ID
		}
		if r.c.ID == "" || r.c.ID != id {
			t.Fatalf("worker %d returned id %q, want %q", i, r.c.ID, id)
		}
	}
	if createdCount != 1 {
		t.Fatalf("created=true for %d calls, want exactly 1", createdCount)
	}
	var campaigns int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM flash_sale_campaigns`).Scan(&campaigns); err != nil || campaigns != 1 {
		t.Fatalf("campaign rows = %d err=%v, want 1", campaigns, err)
	}
	if n := outboxCount(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

func TestCampaignIdempotencyReplayAndMismatch_RetryAfterTimeoutReturnsSameCampaign(t *testing.T) {
	pool, repo := newCampaignRepo(t)
	now := campaignClock
	in := store.FlashSaleCampaign{ListingID: "l-1", VariantID: "v", SalePrice: 100, StockCap: 5, StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour)}

	if _, err := repo.LookupIdempotent(t.Context(), "admin-a", "retry-key-0001", now); !errors.Is(err, store.ErrIdempotencyNotFound) {
		t.Fatalf("lookup before create err = %v, want ErrIdempotencyNotFound", err)
	}
	first, created, err := repo.CreateIdempotent(t.Context(), "admin-a", "retry-key-0001", "hash-1", in, now, campaignEvent)
	if err != nil || !created {
		t.Fatalf("first create: created=%v err=%v", created, err)
	}
	again, created, err := repo.CreateIdempotent(t.Context(), "admin-a", "retry-key-0001", "hash-1", in, now.Add(time.Minute), campaignEvent)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay: id=%q created=%v err=%v, want id %q created=false", again.ID, created, err, first.ID)
	}
	_, _, err = repo.CreateIdempotent(t.Context(), "admin-a", "retry-key-0001", "hash-2", in, now.Add(time.Minute), campaignEvent)
	if !errors.Is(err, store.ErrIdempotencyMismatch) {
		t.Fatalf("different payload err = %v, want ErrIdempotencyMismatch", err)
	}
	hit, err := repo.LookupIdempotent(t.Context(), "admin-a", "retry-key-0001", now.Add(time.Minute))
	if err != nil || hit.Campaign.ID != first.ID || hit.PayloadHash != "hash-1" {
		t.Fatalf("lookup = %+v err=%v, want campaign %q hash-1", hit, err, first.ID)
	}
	var campaigns int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM flash_sale_campaigns`).Scan(&campaigns); err != nil || campaigns != 1 {
		t.Fatalf("campaign rows = %d err=%v, want 1", campaigns, err)
	}
	if n := outboxCount(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}

	if _, err := repo.Cancel(t.Context(), first.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cancelled, err := repo.LookupIdempotent(t.Context(), "admin-a", "retry-key-0001", now.Add(3*time.Minute))
	if err != nil || cancelled.Campaign.ID != first.ID {
		t.Fatalf("lookup after cancel = %+v err=%v", cancelled, err)
	}
	listed, err := repo.ListCampaigns(t.Context(), []string{store.CampaignStatusCancelled}, now.Add(3*time.Minute), 0, 10)
	if err != nil || !equalIDs(ids(listed), []string{first.ID}) {
		t.Fatalf("cancelled list = %v err=%v", ids(listed), err)
	}
}

func TestCampaignIdempotencyScopeAndExpiry_KeyIsScopedPerAdminAndExpires(t *testing.T) {
	pool, repo := newCampaignRepo(t)
	now := campaignClock
	in := store.FlashSaleCampaign{ListingID: "l-1", VariantID: "v", SalePrice: 100, StockCap: 5, StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(72 * time.Hour)}

	a, created, err := repo.CreateIdempotent(t.Context(), "admin-a", "shared-key-01", "hash-a", in, now, campaignEvent)
	if err != nil || !created {
		t.Fatalf("admin a create: created=%v err=%v", created, err)
	}
	b, created, err := repo.CreateIdempotent(t.Context(), "admin-b", "shared-key-01", "hash-b", in, now.Add(time.Second), campaignEvent)
	if err != nil || !created || b.ID == a.ID {
		t.Fatalf("admin b create: id=%q (a=%q) created=%v err=%v, want its own campaign", b.ID, a.ID, created, err)
	}

	inside := now.Add(24*time.Hour - time.Minute)
	if _, err := repo.LookupIdempotent(t.Context(), "admin-a", "shared-key-01", inside); err != nil {
		t.Fatalf("lookup inside 24h: %v", err)
	}
	if n, err := repo.PurgeIdempotency(t.Context(), inside); err != nil || n != 0 {
		t.Fatalf("purge inside 24h = %d err=%v, want 0", n, err)
	}

	expired := now.Add(24*time.Hour + time.Second)
	if _, err := repo.LookupIdempotent(t.Context(), "admin-a", "shared-key-01", expired); !errors.Is(err, store.ErrIdempotencyNotFound) {
		t.Fatalf("lookup after expiry err = %v, want ErrIdempotencyNotFound", err)
	}
	reuse, created, err := repo.CreateIdempotent(t.Context(), "admin-a", "shared-key-01", "hash-a2", in, expired, campaignEvent)
	if err != nil || !created || reuse.ID == a.ID {
		t.Fatalf("reuse after expiry: id=%q created=%v err=%v, want a new campaign", reuse.ID, created, err)
	}
	var campaigns int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM flash_sale_campaigns`).Scan(&campaigns); err != nil || campaigns != 3 {
		t.Fatalf("campaign rows = %d err=%v, want 3", campaigns, err)
	}
}

func TestPurgeIdempotency_KeyIsScopedPerAdminAndExpires(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	in := store.FlashSaleCampaign{ListingID: "l-1", VariantID: "v", SalePrice: 100, StockCap: 5, StartsAt: now.Add(48 * time.Hour), EndsAt: now.Add(72 * time.Hour)}
	if _, _, err := repo.CreateIdempotent(t.Context(), "admin-a", "old-key-0001", "h", in, now, campaignEvent); err != nil {
		t.Fatalf("old: %v", err)
	}
	if _, _, err := repo.CreateIdempotent(t.Context(), "admin-a", "new-key-0001", "h", in, now.Add(20*time.Hour), campaignEvent); err != nil {
		t.Fatalf("new: %v", err)
	}
	at := now.Add(25 * time.Hour)
	n, err := repo.PurgeIdempotency(t.Context(), at)
	if err != nil || n != 1 {
		t.Fatalf("purge = %d err=%v, want 1", n, err)
	}
	if _, err := repo.LookupIdempotent(t.Context(), "admin-a", "new-key-0001", at); err != nil {
		t.Fatalf("young record purged: %v", err)
	}
	campaigns, err := repo.ListCampaigns(t.Context(), nil, at, 0, 10)
	if err != nil || len(campaigns) != 2 {
		t.Fatalf("purge removed campaigns: %d err=%v, want 2 kept", len(campaigns), err)
	}
}
