package rpc_test

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func (c *campaignRig) seedFor(t *testing.T, seller, listingID string, startsIn, endsIn time.Duration) string {
	t.Helper()
	now := time.Now()
	got, err := c.cf.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: listingID, SalePrice: 50000, StockCap: 10, StockSold: 2, SellerID: seller,
		StartsAt: now.Add(startsIn), EndsAt: now.Add(endsIn),
	}, campaignRow)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return got.ID
}

func TestListMyCampaignsOwn_SellerSeesOnlyTheirOwnCampaignsWithStatus(t *testing.T) {
	c := newCampaignRig(t)
	sched := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)
	act := c.seedFor(t, "seller-1", "l1", -time.Hour, time.Hour)
	c.seedFor(t, "seller-2", "l2", -time.Hour, time.Hour)
	c.seedFor(t, "", "l2", -time.Hour, time.Hour)

	resp, err := c.fc.ListMyCampaigns(sellerCtx(t), &promotionv1.ListMyCampaignsRequest{})
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	got := resp.GetCampaigns()
	if len(got) != 2 || got[0].GetId() != sched || got[1].GetId() != act {
		t.Fatalf("campaigns = %v, want [%s %s] newest start first", got, sched, act)
	}
	if got[0].GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED || got[1].GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("statuses = %v %v, want SCHEDULED ACTIVE", got[0].GetStatus(), got[1].GetStatus())
	}
	for _, cp := range got {
		if cp.GetSellerId() != "seller-1" || cp.GetStockCap() != 10 || cp.GetStockSold() != 2 {
			t.Errorf("%s = seller %q cap %d sold %d, want seller-1 10 2", cp.GetId(), cp.GetSellerId(), cp.GetStockCap(), cp.GetStockSold())
		}
	}
	c.rec.requireNone(t, "ListMyCampaigns")
}

func TestListMyCampaignsStatusFilter_StatusFilterNarrowsTheList(t *testing.T) {
	c := newCampaignRig(t)
	sched := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)
	c.seedFor(t, "seller-1", "l1", -time.Hour, time.Hour)

	resp, err := c.fc.ListMyCampaigns(sellerCtx(t), &promotionv1.ListMyCampaignsRequest{
		Statuses: []promotionv1.CampaignStatus{promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED},
	})
	if err != nil {
		t.Fatalf("ListMyCampaigns: %v", err)
	}
	if len(resp.GetCampaigns()) != 1 || resp.GetCampaigns()[0].GetId() != sched {
		t.Fatalf("filtered = %v, want only %s", resp.GetCampaigns(), sched)
	}
}

func TestListMyCampaignsPaging_PagingBoundaryAt21Campaigns(t *testing.T) {
	c := newCampaignRig(t)
	for i := 0; i < 21; i++ {
		c.seedFor(t, "seller-1", "l1", time.Duration(i+1)*time.Hour, 100*time.Hour)
	}
	c.seedFor(t, "seller-2", "l2", time.Hour, 100*time.Hour)

	p1, err := c.fc.ListMyCampaigns(sellerCtx(t), &promotionv1.ListMyCampaignsRequest{Page: &commonv1.PageRequest{PageSize: 20}})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.GetCampaigns()) != 20 || p1.GetPage().GetNextCursor() == "" {
		t.Fatalf("page 1 = %d cursor %q, want 20 and a cursor", len(p1.GetCampaigns()), p1.GetPage().GetNextCursor())
	}
	p2, err := c.fc.ListMyCampaigns(sellerCtx(t), &promotionv1.ListMyCampaignsRequest{Page: &commonv1.PageRequest{PageSize: 20, Cursor: p1.GetPage().GetNextCursor()}})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.GetCampaigns()) != 1 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page 2 = %d cursor %q, want 1 and no cursor", len(p2.GetCampaigns()), p2.GetPage().GetNextCursor())
	}
	seen := map[string]bool{}
	for _, cp := range append(p1.GetCampaigns(), p2.GetCampaigns()...) {
		if seen[cp.GetId()] {
			t.Fatalf("repeated campaign %s", cp.GetId())
		}
		seen[cp.GetId()] = true
	}
}

func TestListMyCampaignsAnonymous(t *testing.T) {
	c := newCampaignRig(t)
	c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)
	resp, err := c.fc.ListMyCampaigns(anonCtx(t), &promotionv1.ListMyCampaignsRequest{})
	if code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: code = %v, want Unauthenticated", code(err))
	}
	if len(resp.GetCampaigns()) != 0 {
		t.Fatalf("anonymous leaked %d campaigns", len(resp.GetCampaigns()))
	}
}

func TestListMyCampaignsRequiresListingWrite_CallerWithoutListingWriteIsDenied(t *testing.T) {
	c := newCampaignRig(t)
	c.seedFor(t, "buyer-1", "l1", time.Hour, 2*time.Hour)
	c.seedFor(t, "admin-1", "l1", time.Hour, 2*time.Hour)
	for _, tc := range []struct {
		name string
		call func() (*promotionv1.ListMyCampaignsResponse, error)
	}{
		{"buyer", func() (*promotionv1.ListMyCampaignsResponse, error) {
			return c.fc.ListMyCampaigns(buyerCtx(t), &promotionv1.ListMyCampaignsRequest{})
		}},
		{"admin without listing.write", func() (*promotionv1.ListMyCampaignsResponse, error) {
			return c.fc.ListMyCampaigns(authCtx(t, "admin-1", "admin", "promotion.admin"), &promotionv1.ListMyCampaignsRequest{})
		}},
	} {
		resp, err := tc.call()
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: code = %v, want PermissionDenied", tc.name, code(err))
		}
		if len(resp.GetCampaigns()) != 0 {
			t.Errorf("%s: leaked %d campaigns", tc.name, len(resp.GetCampaigns()))
		}
	}
}

func (c *campaignRig) cancelMine(t *testing.T, id string) (*promotionv1.CancelMyCampaignResponse, error) {
	t.Helper()
	return c.fc.CancelMyCampaign(sellerCtx(t), &promotionv1.CancelMyCampaignRequest{CampaignId: id})
}

func TestCancelMyCampaign_SellerCancelsTheirScheduledCampaign(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)

	resp, err := c.cancelMine(t, id)
	if err != nil {
		t.Fatalf("CancelMyCampaign: %v", err)
	}
	cp := resp.GetCampaign()
	if cp.GetId() != id || cp.GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED || cp.GetCancelledAt() == nil {
		t.Fatalf("response = %v, want CANCELLED with cancelled_at", cp)
	}
	listed, err := c.fc.ListMyCampaigns(sellerCtx(t), &promotionv1.ListMyCampaignsRequest{})
	if err != nil || len(listed.GetCampaigns()) != 1 || listed.GetCampaigns()[0].GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED {
		t.Fatalf("list after cancel = %v err=%v, want one CANCELLED", listed.GetCampaigns(), err)
	}
}

func TestCancelMyCampaignActive_CancellingAnActiveCampaignIsRefused(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seedFor(t, "seller-1", "l1", -time.Hour, time.Hour)

	_, err := c.cancelMine(t, id)
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("code = %v (%v), want FailedPrecondition", code(err), err)
	}
	if st := c.statusOf(t, id); st != promotionv1.CampaignStatus_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("status after refused cancel = %v, want ACTIVE", st)
	}
	if _, cancelled := c.cf.cancelledAt(id); cancelled {
		t.Fatal("refused cancel still recorded cancelled_at")
	}
}

func TestCancelMyCampaignForeign_DeniedLikeAnUnknownId(t *testing.T) {
	c := newCampaignRig(t)
	foreign := c.seedFor(t, "seller-2", "l2", time.Hour, 2*time.Hour)
	legacy := c.seedFor(t, "", "l2", time.Hour, 2*time.Hour)

	_, errForeign := c.cancelMine(t, foreign)
	_, errUnknown := c.cancelMine(t, "no-such-campaign")
	_, errLegacy := c.cancelMine(t, legacy)
	_, errMalformed := c.cancelMine(t, "!!not an id!!")
	for name, err := range map[string]error{"foreign": errForeign, "unknown": errUnknown, "legacy": errLegacy, "malformed": errMalformed} {
		if code(err) != codes.PermissionDenied {
			t.Errorf("%s: code = %v (%v), want PermissionDenied", name, code(err), err)
		}
	}
	want := status.Convert(errForeign).Message()
	for name, err := range map[string]error{"unknown": errUnknown, "legacy": errLegacy, "malformed": errMalformed} {
		if got := status.Convert(err).Message(); got != want {
			t.Errorf("%s message = %q, want the foreign message %q (no existence oracle)", name, got, want)
		}
	}
	for _, id := range []string{foreign, legacy} {
		if _, cancelled := c.cf.cancelledAt(id); cancelled {
			t.Errorf("%s cancelled by a denied call", id)
		}
	}
}

func TestCancelMyCampaignIdempotent_CancellingTwiceIsIdempotent(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)

	first, err := c.cancelMine(t, id)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := c.cancelMine(t, id)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.GetCampaign().GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED ||
		!second.GetCampaign().GetCancelledAt().AsTime().Equal(first.GetCampaign().GetCancelledAt().AsTime()) {
		t.Fatalf("second = %v, want CANCELLED with the first cancelled_at %v", second.GetCampaign(), first.GetCampaign().GetCancelledAt())
	}
}

func TestCancelMyCampaignRequiresListingWrite_CallerWithoutListingWriteIsDenied(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seedFor(t, "buyer-1", "l1", time.Hour, 2*time.Hour)
	for _, tc := range []struct {
		name string
		want codes.Code
		err  func() error
	}{
		{"anonymous", codes.Unauthenticated, func() error {
			_, err := c.fc.CancelMyCampaign(anonCtx(t), &promotionv1.CancelMyCampaignRequest{CampaignId: id})
			return err
		}},
		{"buyer", codes.PermissionDenied, func() error {
			_, err := c.fc.CancelMyCampaign(buyerCtx(t), &promotionv1.CancelMyCampaignRequest{CampaignId: id})
			return err
		}},
		{"admin without listing.write", codes.PermissionDenied, func() error {
			_, err := c.fc.CancelMyCampaign(authCtx(t, "admin-1", "admin", "promotion.admin"), &promotionv1.CancelMyCampaignRequest{CampaignId: id})
			return err
		}},
	} {
		if got := code(tc.err()); got != tc.want {
			t.Errorf("%s: code = %v, want %v", tc.name, got, tc.want)
		}
	}
	if _, cancelled := c.cf.cancelledAt(id); cancelled {
		t.Fatal("campaign cancelled by an unauthorised caller")
	}
}

func TestCancelMyCampaignNoAudit_SellerCancellationWritesNoAuditEvent(t *testing.T) {
	c := newCampaignRig(t)
	c.rec.err = status.Error(codes.Unavailable, "team-audit down")
	id := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)

	if _, err := c.cancelMine(t, id); err != nil {
		t.Fatalf("CancelMyCampaign with audit down: %v", err)
	}
	c.rec.requireNone(t, "CancelMyCampaign")
}

func TestCampaignSellerAdminCancelRegression_AdminCancelKeepsItsAdminRules(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seedFor(t, "seller-1", "l1", time.Hour, 2*time.Hour)

	resp, err := c.fc.CancelCampaign(adminCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id})
	if err != nil {
		t.Fatalf("admin CancelCampaign: %v", err)
	}
	if resp.GetCampaign().GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED {
		t.Fatalf("status = %v, want CANCELLED", resp.GetCampaign().GetStatus())
	}
	evs := c.rec.got()
	if len(evs) != 1 || evs[0].Action != "campaign.cancel" || evs[0].TargetID != id {
		t.Fatalf("audit = %+v, want exactly one campaign.cancel for %s", evs, id)
	}
	if _, err := c.fc.CancelCampaign(sellerCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id}); code(err) != codes.PermissionDenied {
		t.Fatalf("seller on the admin RPC: code = %v, want PermissionDenied", code(err))
	}
}
