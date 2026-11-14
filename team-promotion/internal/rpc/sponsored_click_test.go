package rpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

func servedClickToken(t *testing.T, r *rig) (token, listing string) {
	t.Helper()
	if _, err := r.ads.CreateAdCampaign(sellerCtx(t), &promotionv1.CreateAdCampaignRequest{ListingId: "l1", Bid: 800, Budget: 5000}); err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	resp, err := r.ads.ListSponsoredSlots(anonCtx(t), &promotionv1.ListSponsoredSlotsRequest{Placement: "home", Limit: 2})
	if err != nil || len(resp.GetSlots()) != 1 {
		t.Fatalf("serve: %v %v", resp, err)
	}
	return resp.GetSlots()[0].GetImpressionToken(), resp.GetSlots()[0].GetListingId()
}

func TestRecordAdClickEmptyTokenInvalid(t *testing.T) {
	r := startRig(t, rigOpts{})
	for name, ctx := range map[string]context.Context{"anonymous": anonCtx(t), "buyer": buyerCtx(t), "no principal": t.Context()} {
		_, err := r.ads.RecordAdClick(ctx, &promotionv1.RecordAdClickRequest{ImpressionToken: ""})
		if code(err) != codes.InvalidArgument {
			t.Errorf("%s: got %v, want InvalidArgument (err=%v)", name, code(err), err)
		}
	}
}

func TestRecordAdClickUnknownTokenNotFound(t *testing.T) {
	r := startRig(t, rigOpts{})
	_, err := r.ads.RecordAdClick(anonCtx(t), &promotionv1.RecordAdClickRequest{ImpressionToken: "never-issued"})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound (err=%v)", code(err), err)
	}
}

func TestRecordAdClickNeedsNoPrincipalOrScope(t *testing.T) {
	for name, ctxf := range map[string]func(*testing.T) context.Context{
		"anonymous":    anonCtx,
		"buyer":        buyerCtx,
		"no principal": func(t *testing.T) context.Context { return t.Context() },
	} {
		t.Run(name, func(t *testing.T) {
			r := startRig(t, rigOpts{})
			token, listing := servedClickToken(t, r)
			resp, err := r.ads.RecordAdClick(ctxf(t), &promotionv1.RecordAdClickRequest{ImpressionToken: token})
			if err != nil {
				t.Fatalf("RecordAdClick: %v", err)
			}
			if !resp.GetCharged() || resp.GetListingId() != listing {
				t.Fatalf("resp = %v, want charged for %s", resp, listing)
			}
			again, err := r.ads.RecordAdClick(ctxf(t), &promotionv1.RecordAdClickRequest{ImpressionToken: token})
			if err != nil || again.GetCharged() || again.GetListingId() != listing {
				t.Fatalf("replay = %v err=%v, want uncharged for %s", again, err, listing)
			}
		})
	}
}

func TestRecordAdClickForwardedSellerPrincipalNotCharged(t *testing.T) {
	r := startRig(t, rigOpts{})
	token, listing := servedClickToken(t, r)
	resp, err := r.ads.RecordAdClick(sellerCtx(t), &promotionv1.RecordAdClickRequest{ImpressionToken: token})
	if err != nil || resp.GetCharged() || resp.GetListingId() != listing {
		t.Fatalf("own click = %v err=%v, want uncharged for %s", resp, err, listing)
	}
	resp, err = r.ads.RecordAdClick(buyerCtx(t), &promotionv1.RecordAdClickRequest{ImpressionToken: token})
	if err != nil || !resp.GetCharged() {
		t.Fatalf("buyer click after own click = %v err=%v, want charged", resp, err)
	}
}
