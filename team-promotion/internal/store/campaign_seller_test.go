package store_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

func seedOwned(t *testing.T, repo *store.PostgresFlashSaleRepository, id, seller string, starts, ends time.Time) store.FlashSaleCampaign {
	t.Helper()
	c, err := repo.Create(t.Context(), store.FlashSaleCampaign{
		ID: id, ListingID: "listing-" + id, VariantID: "v", SalePrice: 100, StockCap: 5, StartsAt: starts, EndsAt: ends, SellerID: seller,
	}, campaignEvent)
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
	return c
}

func TestCampaignSellerCreateStoresOwner_NewCampaignRecordsListingOwner(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	created := seedOwned(t, repo, "c-a", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))
	if created.SellerID != "seller-A" {
		t.Fatalf("created SellerID = %q, want seller-A", created.SellerID)
	}
	got, err := repo.GetByID(t.Context(), "c-a")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.SellerID != "seller-A" {
		t.Fatalf("persisted SellerID = %q, want seller-A", got.SellerID)
	}
}

func TestCampaignSellerCreateIdempotentStoresOwner_NewCampaignRecordsListingOwner(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	c, _, err := repo.CreateIdempotent(t.Context(), "admin-1", "key-0001", "hash", store.FlashSaleCampaign{
		ID: "c-idem", ListingID: "l", SalePrice: 1, StockCap: 1, StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), SellerID: "seller-B",
	}, now, campaignEvent)
	if err != nil {
		t.Fatalf("CreateIdempotent: %v", err)
	}
	got, err := repo.GetByID(t.Context(), c.ID)
	if err != nil || got.SellerID != "seller-B" {
		t.Fatalf("persisted SellerID = %q err=%v, want seller-B", got.SellerID, err)
	}
}

func TestListMyCampaignsScopesToSeller_SellerSeesOnlyOwnWithStatus(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "a-sched", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "a-active", "seller-A", now.Add(-time.Hour), now.Add(time.Hour))
	seedOwned(t, repo, "b-active", "seller-B", now.Add(-time.Hour), now.Add(time.Hour))
	seedOwned(t, repo, "legacy", "", now.Add(-time.Hour), now.Add(time.Hour))

	got, err := repo.ListMyCampaigns(t.Context(), "seller-A", nil, now, 0, 50)
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if !equalIDs(ids(got), []string{"a-sched", "a-active"}) {
		t.Fatalf("seller-A list = %v, want [a-sched a-active]", ids(got))
	}
	for _, c := range got {
		if c.SellerID != "seller-A" {
			t.Errorf("%s SellerID = %q, want seller-A", c.ID, c.SellerID)
		}
	}
	none, err := repo.ListMyCampaigns(t.Context(), "seller-Z", nil, now, 0, 50)
	if err != nil || len(none) != 0 {
		t.Fatalf("seller with no campaigns = %v err=%v, want empty", ids(none), err)
	}
}

func TestListMyCampaignsStatusFilter_StatusFilterNarrowsTheList(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "a-sched", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "a-active", "seller-A", now.Add(-time.Hour), now.Add(time.Hour))
	seedOwned(t, repo, "a-ended", "seller-A", now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	seedOwned(t, repo, "b-sched", "seller-B", now.Add(time.Hour), now.Add(2*time.Hour))

	got, err := repo.ListMyCampaigns(t.Context(), "seller-A", []string{store.CampaignStatusScheduled}, now, 0, 50)
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if !equalIDs(ids(got), []string{"a-sched"}) {
		t.Fatalf("scheduled filter = %v, want [a-sched]", ids(got))
	}
	got, err = repo.ListMyCampaigns(t.Context(), "seller-A", []string{store.CampaignStatusActive, store.CampaignStatusEnded}, now, 0, 50)
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if !equalIDs(ids(got), []string{"a-active", "a-ended"}) {
		t.Fatalf("active+ended filter = %v, want [a-active a-ended]", ids(got))
	}
}

func TestListMyCampaignsPaging_PagingBoundaryAt21Campaigns(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	var want []string
	for i := 0; i < 21; i++ {
		id := fmt.Sprintf("a-%02d", i)
		seedOwned(t, repo, id, "seller-A", now.Add(time.Duration(100-i)*time.Hour), now.Add(200*time.Hour))
		want = append(want, id)
	}
	seedOwned(t, repo, "b-1", "seller-B", now.Add(500*time.Hour), now.Add(600*time.Hour))

	p1, err := repo.ListMyCampaigns(t.Context(), "seller-A", nil, now, 0, 21)
	if err != nil || !equalIDs(ids(p1), want) {
		t.Fatalf("first 21 = %v err=%v, want all of seller-A newest start first", ids(p1), err)
	}
	p2, err := repo.ListMyCampaigns(t.Context(), "seller-A", nil, now, 20, 21)
	if err != nil || !equalIDs(ids(p2), want[20:]) {
		t.Fatalf("offset 20 = %v err=%v, want [%s]", ids(p2), err, want[20])
	}
}

func TestCancelIfScheduledOwnedCancels_SellerCancelsTheirScheduledCampaign(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-1", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))

	at, err := repo.CancelIfScheduledOwned(t.Context(), "c-1", "seller-A", now)
	if err != nil {
		t.Fatalf("CancelIfScheduledOwned: %v", err)
	}
	if !at.Equal(now) {
		t.Fatalf("cancelled_at = %v, want %v", at, now)
	}
	got, err := repo.GetByID(t.Context(), "c-1")
	if err != nil || !got.CancelledAt.Equal(now) {
		t.Fatalf("stored cancelled_at = %v err=%v, want %v", got.CancelledAt, err, now)
	}
	listed, err := repo.ListMyCampaigns(t.Context(), "seller-A", []string{store.CampaignStatusCancelled}, now, 0, 10)
	if err != nil || !equalIDs(ids(listed), []string{"c-1"}) {
		t.Fatalf("cancelled list = %v err=%v, want [c-1]", ids(listed), err)
	}
}

func TestCancelIfScheduledOwnedNotOwner_ForeignAndUnknownDeniedAlike(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-a", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-legacy", "", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-a-active", "seller-A", now.Add(-time.Hour), now.Add(time.Hour))

	cases := []struct{ name, id, caller string }{
		{"foreign seller", "c-a", "seller-B"},
		{"unknown id", "no-such-campaign", "seller-B"},
		{"legacy empty seller_id", "c-legacy", "seller-B"},
		{"legacy empty seller_id and empty caller", "c-legacy", ""},
		{"foreign and not scheduled reports not-owner first", "c-a-active", "seller-B"},
	}
	for _, tc := range cases {
		_, err := repo.CancelIfScheduledOwned(t.Context(), tc.id, tc.caller, now)
		if !errors.Is(err, store.ErrNotOwner) {
			t.Errorf("%s: err = %v, want store.ErrNotOwner", tc.name, err)
		}
	}
	for _, id := range []string{"c-a", "c-legacy"} {
		got, err := repo.GetByID(t.Context(), id)
		if err != nil || !got.CancelledAt.IsZero() {
			t.Errorf("%s changed by a denied cancel: cancelled_at=%v err=%v", id, got.CancelledAt, err)
		}
	}
}

func TestCancelIfScheduledOwnedRefusesStarted_CancellingAnActiveCampaignIsRefused(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-active", "seller-A", now.Add(-time.Hour), now.Add(time.Hour))
	seedOwned(t, repo, "c-ended", "seller-A", now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	seedOwned(t, repo, "c-open", "seller-A", time.Time{}, time.Time{})

	for _, id := range []string{"c-active", "c-ended", "c-open"} {
		_, err := repo.CancelIfScheduledOwned(t.Context(), id, "seller-A", now)
		if !errors.Is(err, store.ErrCampaignNotScheduled) {
			t.Errorf("%s: err = %v, want store.ErrCampaignNotScheduled", id, err)
		}
		got, gerr := repo.GetByID(t.Context(), id)
		if gerr != nil || !got.CancelledAt.IsZero() {
			t.Errorf("%s changed by a refused cancel: cancelled_at=%v err=%v", id, got.CancelledAt, gerr)
		}
	}
}

func TestCancelIfScheduledOwnedIdempotent_CancellingTwiceIsIdempotent(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-1", "seller-A", now.Add(time.Hour), now.Add(2*time.Hour))

	first, err := repo.CancelIfScheduledOwned(t.Context(), "c-1", "seller-A", now)
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	second, err := repo.CancelIfScheduledOwned(t.Context(), "c-1", "seller-A", now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("second cancel after the start time passed: %v", err)
	}
	if !second.Equal(first) {
		t.Fatalf("second cancelled_at = %v, want the first %v", second, first)
	}
	if _, err := repo.CancelIfScheduledOwned(t.Context(), "c-1", "seller-B", now); !errors.Is(err, store.ErrNotOwner) {
		t.Fatalf("foreign caller on a cancelled campaign: err = %v, want store.ErrNotOwner", err)
	}
}

func TestCancelIfScheduledOwnedBoundary_CampaignStartingDuringTheCancelIsNotCancelled(t *testing.T) {
	_, repo := newCampaignRepo(t)
	starts := campaignClock.Add(time.Hour)
	seedOwned(t, repo, "c-edge", "seller-A", starts, starts.Add(time.Hour))

	if _, err := repo.CancelIfScheduledOwned(t.Context(), "c-edge", "seller-A", starts); !errors.Is(err, store.ErrCampaignNotScheduled) {
		t.Fatalf("cancel at starts_at: err = %v, want store.ErrCampaignNotScheduled", err)
	}
	if got, err := repo.GetByID(t.Context(), "c-edge"); err != nil || !got.CancelledAt.IsZero() {
		t.Fatalf("campaign cancelled at its start instant: cancelled_at=%v err=%v", got.CancelledAt, err)
	}
	at, err := repo.CancelIfScheduledOwned(t.Context(), "c-edge", "seller-A", starts.Add(-time.Microsecond))
	if err != nil || !at.Equal(starts.Add(-time.Microsecond)) {
		t.Fatalf("cancel one microsecond before starts_at: at=%v err=%v, want success", at, err)
	}
}
