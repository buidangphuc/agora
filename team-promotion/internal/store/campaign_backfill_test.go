package store_test

import (
	"testing"
	"time"
)

func TestBackfillCampaignSellersStoreListsOnlyUnassignedInKeysetOrder_FillsOldCampaignsOnce(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-3", "", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-1", "", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-2", "seller-X", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-4", "", now.Add(time.Hour), now.Add(2*time.Hour))

	first, err := repo.ListUnassignedCampaigns(t.Context(), "", 2)
	if err != nil {
		t.Fatalf("ListUnassignedCampaigns: %v", err)
	}
	if !equalIDs(ids(first), []string{"c-1", "c-3"}) {
		t.Fatalf("first batch = %v, want [c-1 c-3] (unassigned only, id order, limit 2)", ids(first))
	}
	if first[0].ListingID != "listing-c-1" {
		t.Fatalf("listing id not loaded: %q", first[0].ListingID)
	}
	next, err := repo.ListUnassignedCampaigns(t.Context(), "c-3", 2)
	if err != nil {
		t.Fatalf("ListUnassignedCampaigns after c-3: %v", err)
	}
	if !equalIDs(ids(next), []string{"c-4"}) {
		t.Fatalf("second batch = %v, want [c-4]", ids(next))
	}
}

func TestBackfillCampaignSellersStoreAssignIsAtomicAndIdempotent_FillsOldCampaignsOnce(t *testing.T) {
	_, repo := newCampaignRepo(t)
	now := campaignClock
	seedOwned(t, repo, "c-1", "", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-2", "", now.Add(time.Hour), now.Add(2*time.Hour))
	seedOwned(t, repo, "c-3", "seller-keep", now.Add(time.Hour), now.Add(2*time.Hour))

	n, err := repo.AssignCampaignSellers(t.Context(), map[string]string{"c-1": "A", "c-2": "B", "c-3": "overwrite"})
	if err != nil {
		t.Fatalf("AssignCampaignSellers: %v", err)
	}
	if n != 2 {
		t.Fatalf("updated = %d, want 2 (an already assigned row is never overwritten)", n)
	}
	for id, want := range map[string]string{"c-1": "A", "c-2": "B", "c-3": "seller-keep"} {
		got, err := repo.GetByID(t.Context(), id)
		if err != nil || got.SellerID != want {
			t.Fatalf("%s SellerID = %q err=%v, want %q", id, got.SellerID, err, want)
		}
	}
	again, err := repo.AssignCampaignSellers(t.Context(), map[string]string{"c-1": "Z", "c-2": "Z"})
	if err != nil || again != 0 {
		t.Fatalf("second assign = %d err=%v, want 0 updated", again, err)
	}
	got, _ := repo.GetByID(t.Context(), "c-1")
	if got.SellerID != "A" {
		t.Fatalf("c-1 changed on rerun: %q", got.SellerID)
	}
}
