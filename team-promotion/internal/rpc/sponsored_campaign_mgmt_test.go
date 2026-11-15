package rpc_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

var mgmtEpoch = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func seedAd(t *testing.T, r *rig, c store.AdCampaign) store.AdCampaign {
	t.Helper()
	if c.SellerID == "" {
		c.SellerID = "seller-1"
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = mgmtEpoch
	}
	if c.Bid == 0 {
		c.Bid = 800
	}
	if c.Budget == 0 {
		c.Budget = 5000
	}
	if c.ListingID == "" {
		c.ListingID = "listing-" + c.ID
	}
	out, err := r.adRepo.Create(t.Context(), c)
	if err != nil {
		t.Fatalf("seed %s: %v", c.ID, err)
	}
	return out
}

func storedAd(t *testing.T, r *rig, id string) store.AdCampaign {
	t.Helper()
	c, err := r.adRepo.GetByID(t.Context(), id)
	if err != nil {
		t.Fatalf("GetByID %s: %v", id, err)
	}
	return c
}

func servedCampaignIDs(t *testing.T, r *rig, category string) []string {
	t.Helper()
	resp, err := r.ads.ListSponsoredSlots(anonCtx(t), &promotionv1.ListSponsoredSlotsRequest{Placement: "search", CategoryId: category, Limit: 4})
	if err != nil {
		t.Fatalf("ListSponsoredSlots: %v", err)
	}
	var ids []string
	for _, s := range resp.GetSlots() {
		ids = append(ids, s.GetCampaignId())
	}
	return ids
}

func idsInclude(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func TestListMyAdCampaignsReturnsOwnNewestFirstWithCounters(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "old", CreatedAt: mgmtEpoch.Add(-time.Hour)})
	seedAd(t, r, store.AdCampaign{ID: "new"})
	slots, err := r.ads.ListSponsoredSlots(anonCtx(t), &promotionv1.ListSponsoredSlotsRequest{Placement: "home", Limit: 4})
	if err != nil || len(slots.GetSlots()) != 2 {
		t.Fatalf("serve: %v %v", slots, err)
	}
	var oldToken string
	for _, s := range slots.GetSlots() {
		if s.GetCampaignId() == "old" {
			oldToken = s.GetImpressionToken()
		}
	}
	if resp, err := r.ads.RecordAdClick(buyerCtx(t), &promotionv1.RecordAdClickRequest{ImpressionToken: oldToken}); err != nil || !resp.GetCharged() {
		t.Fatalf("click: %v %v", resp, err)
	}

	resp, err := r.ads.ListMyAdCampaigns(sellerCtx(t), &promotionv1.ListMyAdCampaignsRequest{})
	if err != nil {
		t.Fatalf("ListMyAdCampaigns: %v", err)
	}
	got := resp.GetCampaigns()
	if len(got) != 2 || got[0].GetId() != "new" || got[1].GetId() != "old" {
		t.Fatalf("campaigns = %v, want [new old]", got)
	}
	if got[1].GetSpent() != 800 || got[1].GetClicks() != 1 || got[1].GetImpressions() != 1 || got[1].GetListingId() != "listing-old" {
		t.Fatalf("old = %v, want spent 800 clicks 1 impressions 1", got[1])
	}
	if got[0].GetSpent() != 0 || got[0].GetStatus() != promotionv1.AdCampaignStatus_AD_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("new = %v, want ACTIVE spent 0", got[0])
	}
}

func TestListMyAdCampaignsExcludesOtherSellers(t *testing.T) {
	r := startRig(t, rigOpts{})
	for _, id := range []string{"t1", "t2", "t3"} {
		seedAd(t, r, store.AdCampaign{ID: id, SellerID: "seller-2"})
	}
	resp, err := r.ads.ListMyAdCampaigns(sellerCtx(t), &promotionv1.ListMyAdCampaignsRequest{})
	if err != nil {
		t.Fatalf("ListMyAdCampaigns: %v", err)
	}
	if len(resp.GetCampaigns()) != 0 {
		t.Fatalf("campaigns = %v, want none", resp.GetCampaigns())
	}
}

func TestListMyAdCampaignsEmpty(t *testing.T) {
	r := startRig(t, rigOpts{})
	resp, err := r.ads.ListMyAdCampaigns(sellerCtx(t), &promotionv1.ListMyAdCampaignsRequest{})
	if err != nil {
		t.Fatalf("ListMyAdCampaigns: %v", err)
	}
	if len(resp.GetCampaigns()) != 0 || resp.GetPage().GetNextCursor() != "" {
		t.Fatalf("resp = %v, want empty with empty cursor", resp)
	}
}

func TestListMyAdCampaignsRequiresSellerScope(t *testing.T) {
	r := startRig(t, rigOpts{})
	_, err := r.ads.ListMyAdCampaigns(buyerCtx(t), &promotionv1.ListMyAdCampaignsRequest{})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
}

func TestListMyAdCampaignsUnauthenticated(t *testing.T) {
	r := startRig(t, rigOpts{})
	for name, ctx := range map[string]context.Context{"no principal": t.Context(), "anonymous": anonCtx(t)} {
		_, err := r.ads.ListMyAdCampaigns(ctx, &promotionv1.ListMyAdCampaignsRequest{})
		if code(err) != codes.Unauthenticated {
			t.Errorf("%s: got %v, want Unauthenticated (err=%v)", name, code(err), err)
		}
	}
}

func TestPauseAdCampaignStopsServing(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1", CategoryID: "shoes"})
	if !idsInclude(servedCampaignIDs(t, r, "shoes"), "c1") {
		t.Fatal("c1 not served before pause")
	}
	resp, err := r.ads.PauseAdCampaign(sellerCtx(t), &promotionv1.PauseAdCampaignRequest{CampaignId: "c1"})
	if err != nil {
		t.Fatalf("PauseAdCampaign: %v", err)
	}
	if resp.GetCampaign().GetStatus() != promotionv1.AdCampaignStatus_AD_CAMPAIGN_STATUS_PAUSED {
		t.Fatalf("status = %v, want PAUSED", resp.GetCampaign().GetStatus())
	}
	if idsInclude(servedCampaignIDs(t, r, "shoes"), "c1") {
		t.Fatal("c1 still served after pause")
	}
}

func TestPauseAdCampaignOtherSellerNotFound(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1"})
	_, err := r.ads.PauseAdCampaign(authCtx(t, "seller-2", "listing.write"), &promotionv1.PauseAdCampaignRequest{CampaignId: "c1"})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound (err=%v)", code(err), err)
	}
	if got := storedAd(t, r, "c1").Status; got != store.AdCampaignStatusActive {
		t.Fatalf("status = %d, want ACTIVE", got)
	}
}

func TestPauseAdCampaignUnknownIDNotFound(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1", SellerID: "seller-2"})
	_, unknown := r.ads.PauseAdCampaign(sellerCtx(t), &promotionv1.PauseAdCampaignRequest{CampaignId: "nope"})
	_, foreign := r.ads.PauseAdCampaign(sellerCtx(t), &promotionv1.PauseAdCampaignRequest{CampaignId: "c1"})
	if code(unknown) != codes.NotFound || code(foreign) != codes.NotFound {
		t.Fatalf("unknown=%v foreign=%v, want both NotFound", unknown, foreign)
	}
	if status.Convert(unknown).Message() != status.Convert(foreign).Message() {
		t.Fatalf("messages differ: %q vs %q", status.Convert(unknown).Message(), status.Convert(foreign).Message())
	}
}

func TestPauseAdCampaignEmptyIDInvalid(t *testing.T) {
	r := startRig(t, rigOpts{})
	_, err := r.ads.PauseAdCampaign(sellerCtx(t), &promotionv1.PauseAdCampaignRequest{})
	if code(err) != codes.InvalidArgument {
		t.Fatalf("got %v, want InvalidArgument (err=%v)", code(err), err)
	}
}

func TestPauseAdCampaignRequiresSellerScope(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1"})
	_, err := r.ads.PauseAdCampaign(buyerCtx(t), &promotionv1.PauseAdCampaignRequest{CampaignId: "c1"})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	if got := storedAd(t, r, "c1").Status; got != store.AdCampaignStatusActive {
		t.Fatalf("status = %d, want ACTIVE", got)
	}
}

func TestResumeAdCampaignServesAgain(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1", CategoryID: "shoes", Budget: 10000, Spent: 1000, Bid: 800, Status: store.AdCampaignStatusPaused})
	if idsInclude(servedCampaignIDs(t, r, "shoes"), "c1") {
		t.Fatal("c1 served while paused")
	}
	resp, err := r.ads.ResumeAdCampaign(sellerCtx(t), &promotionv1.ResumeAdCampaignRequest{CampaignId: "c1"})
	if err != nil {
		t.Fatalf("ResumeAdCampaign: %v", err)
	}
	if resp.GetCampaign().GetStatus() != promotionv1.AdCampaignStatus_AD_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("status = %v, want ACTIVE", resp.GetCampaign().GetStatus())
	}
	if !idsInclude(servedCampaignIDs(t, r, "shoes"), "c1") {
		t.Fatal("c1 not served after resume")
	}
}

func TestResumeAdCampaignOtherSellerNotFound(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1", Status: store.AdCampaignStatusPaused})
	_, err := r.ads.ResumeAdCampaign(authCtx(t, "seller-2", "listing.write"), &promotionv1.ResumeAdCampaignRequest{CampaignId: "c1"})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound (err=%v)", code(err), err)
	}
	if got := storedAd(t, r, "c1").Status; got != store.AdCampaignStatusPaused {
		t.Fatalf("status = %d, want PAUSED", got)
	}
}

func TestResumeAdCampaignRequiresSellerScope(t *testing.T) {
	r := startRig(t, rigOpts{})
	seedAd(t, r, store.AdCampaign{ID: "c1", Status: store.AdCampaignStatusPaused})
	_, err := r.ads.ResumeAdCampaign(buyerCtx(t), &promotionv1.ResumeAdCampaignRequest{CampaignId: "c1"})
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied (err=%v)", code(err), err)
	}
	if got := storedAd(t, r, "c1").Status; got != store.AdCampaignStatusPaused {
		t.Fatalf("status = %d, want PAUSED", got)
	}
}
