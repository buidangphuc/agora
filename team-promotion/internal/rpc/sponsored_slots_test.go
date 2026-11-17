package rpc_test

import (
	"testing"

	"google.golang.org/grpc/codes"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func TestListSponsoredSlotsPublicAndMinimal(t *testing.T) {
	r := startRig(t, rigOpts{})
	created, err := r.ads.CreateAdCampaign(sellerCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 1000, Budget: 5000})
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}

	for name, ctx := range map[string]func() (*promotionv1.ListSponsoredSlotsResponse, error){
		"anonymous principal": func() (*promotionv1.ListSponsoredSlotsResponse, error) {
			return r.ads.ListSponsoredSlots(anonCtx(t), &promotionv1.ListSponsoredSlotsRequest{Placement: "home", Limit: 2})
		},
		"no principal at all": func() (*promotionv1.ListSponsoredSlotsResponse, error) {
			return r.ads.ListSponsoredSlots(t.Context(), &promotionv1.ListSponsoredSlotsRequest{Placement: "home", Limit: 2})
		},
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := ctx()
			if err != nil {
				t.Fatalf("ListSponsoredSlots: %v", err)
			}
			if len(resp.GetSlots()) != 1 {
				t.Fatalf("slots = %v, want 1", resp.GetSlots())
			}
			slot := resp.GetSlots()[0]
			if slot.GetListingId() != "l1" || slot.GetCampaignId() != created.GetCampaign().GetId() || slot.GetImpressionToken() == "" {
				t.Fatalf("slot = %v, want listing l1, campaign %s and a token", slot, created.GetCampaign().GetId())
			}
			if n := slot.ProtoReflect().Descriptor().Fields().Len(); n != 3 {
				t.Fatalf("SponsoredSlot has %d fields, want only listing_id, campaign_id, impression_token", n)
			}
			if len(resp.GetListingIds()) != 0 {
				t.Fatalf("placement call returned legacy listing_ids %v", resp.GetListingIds())
			}
		})
	}
}

func TestListSponsoredSlotsInvalidArgs(t *testing.T) {
	r := startRig(t, rigOpts{})
	for name, req := range map[string]*promotionv1.ListSponsoredSlotsRequest{
		"unknown placement": {Placement: "sidebar", Limit: 2},
		"limit above 4":     {Placement: "home", Limit: 5},
		"negative limit":    {Placement: "home", Limit: -1},
	} {
		if _, err := r.ads.ListSponsoredSlots(anonCtx(t), req); code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument (err=%v)", name, code(err), err)
		}
	}
}
