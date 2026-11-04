package rpc_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type campaignRig struct {
	*rig
	rec *recordingAudit
}

func newCampaignRig(t *testing.T) *campaignRig {
	t.Helper()
	rec := &recordingAudit{}
	return &campaignRig{rig: startRig(t, rigOpts{audit: rec}), rec: rec}
}

func (c *campaignRig) seed(t *testing.T, listingID string, startsIn, endsIn time.Duration) string {
	t.Helper()
	now := time.Now()
	got, err := c.cf.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: listingID, SalePrice: 50000, StockCap: 10,
		StartsAt: now.Add(startsIn), EndsAt: now.Add(endsIn),
	}, campaignRow)
	if err != nil {
		t.Fatalf("seed campaign: %v", err)
	}
	return got.ID
}

func (c *campaignRig) list(t *testing.T, statuses ...promotionv1.CampaignStatus) *promotionv1.ListCampaignsResponse {
	t.Helper()
	resp, err := c.fc.ListCampaigns(adminCtx(t), &promotionv1.ListCampaignsRequest{Statuses: statuses})
	if err != nil {
		t.Fatalf("ListCampaigns: %v", err)
	}
	return resp
}

func (c *campaignRig) statusOf(t *testing.T, id string) promotionv1.CampaignStatus {
	t.Helper()
	for _, cp := range c.list(t).GetCampaigns() {
		if cp.GetId() == id {
			return cp.GetStatus()
		}
	}
	t.Fatalf("campaign %s not listed", id)
	return 0
}

func requireInfo(t *testing.T, err error, reason string) {
	t.Helper()
	st := status.Convert(err)
	for _, d := range st.Details() {
		if info, ok := d.(*errdetails.ErrorInfo); ok && info.GetDomain() == "promotion.platform" && info.GetReason() == reason {
			return
		}
	}
	t.Fatalf("want ErrorInfo promotion.platform/%s, got %v %q %v", reason, st.Code(), st.Message(), st.Details())
}

func TestListCampaignsRequiresAdmin_NonAdminCannotListAllCampaigns(t *testing.T) {
	c := newCampaignRig(t)
	c.seed(t, "l1", -time.Hour, time.Hour)
	for _, tc := range []struct {
		name string
		want codes.Code
		call func() (*promotionv1.ListCampaignsResponse, error)
	}{
		{"anonymous", codes.Unauthenticated, func() (*promotionv1.ListCampaignsResponse, error) {
			return c.fc.ListCampaigns(anonCtx(t), &promotionv1.ListCampaignsRequest{})
		}},
		{"seller", codes.PermissionDenied, func() (*promotionv1.ListCampaignsResponse, error) {
			return c.fc.ListCampaigns(sellerCtx(t), &promotionv1.ListCampaignsRequest{})
		}},
		{"buyer", codes.PermissionDenied, func() (*promotionv1.ListCampaignsResponse, error) {
			return c.fc.ListCampaigns(buyerCtx(t), &promotionv1.ListCampaignsRequest{})
		}},
	} {
		resp, err := tc.call()
		if code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, code(err), tc.want)
		}
		if len(resp.GetCampaigns()) != 0 {
			t.Errorf("%s: leaked %d campaigns", tc.name, len(resp.GetCampaigns()))
		}
	}
}

func TestListCampaignsAdminFilterAndPaging_NonAdminCannotListAllCampaigns(t *testing.T) {
	c := newCampaignRig(t)
	sched := c.seed(t, "l1", time.Hour, 2*time.Hour)
	act := c.seed(t, "l1", -time.Hour, time.Hour)
	ended := c.seed(t, "l2", -3*time.Hour, -2*time.Hour)

	only := c.list(t, promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED).GetCampaigns()
	if len(only) != 1 || only[0].GetId() != sched || only[0].GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED {
		t.Fatalf("scheduled filter = %v", only)
	}

	all := c.list(t).GetCampaigns()
	if len(all) != 3 || all[0].GetId() != sched || all[1].GetId() != act || all[2].GetId() != ended {
		t.Fatalf("newest-first order wrong: %v", all)
	}
	for _, cp := range all {
		if cp.GetStatus() == promotionv1.CampaignStatus_CAMPAIGN_STATUS_UNSPECIFIED {
			t.Fatalf("campaign %s has UNSPECIFIED status", cp.GetId())
		}
	}

	p1, err := c.fc.ListCampaigns(adminCtx(t), &promotionv1.ListCampaignsRequest{Page: &commonv1.PageRequest{PageSize: 2}})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1.GetCampaigns()) != 2 || p1.GetPage().GetNextCursor() == "" {
		t.Fatalf("page 1 = %d campaigns cursor %q", len(p1.GetCampaigns()), p1.GetPage().GetNextCursor())
	}
	p2, err := c.fc.ListCampaigns(adminCtx(t), &promotionv1.ListCampaignsRequest{Page: &commonv1.PageRequest{PageSize: 2, Cursor: p1.GetPage().GetNextCursor()}})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(p2.GetCampaigns()) != 1 || p2.GetPage().GetNextCursor() != "" {
		t.Fatalf("page 2 = %d campaigns cursor %q", len(p2.GetCampaigns()), p2.GetPage().GetNextCursor())
	}
}

func TestListCampaignsReportsCancelledAt_NonAdminCannotListAllCampaigns(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", -time.Hour, time.Hour)
	if _, err := c.fc.CancelCampaign(adminCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got := c.list(t, promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED).GetCampaigns()
	if len(got) != 1 || got[0].GetId() != id || got[0].GetCancelledAt() == nil {
		t.Fatalf("cancelled list = %v", got)
	}
}

func TestCancelCampaignErrors(t *testing.T) {
	c := newCampaignRig(t)
	ended := c.seed(t, "l1", -3*time.Hour, -time.Hour)
	for _, tc := range []struct {
		name string
		id   string
		want codes.Code
	}{
		{"ended", ended, codes.FailedPrecondition},
		{"unknown", "no-such-campaign", codes.NotFound},
		{"empty id", "", codes.InvalidArgument},
	} {
		_, err := c.fc.CancelCampaign(adminCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: tc.id})
		if code(err) != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, code(err), tc.want)
		}
	}
}

func TestCancelCampaignAdminSucceeds_CancelAnEndedOrUnknownCampaign(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", -time.Hour, time.Hour)
	resp, err := c.fc.CancelCampaign(adminCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	cp := resp.GetCampaign()
	if cp.GetId() != id || cp.GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED || cp.GetCancelledAt() == nil {
		t.Fatalf("cancel response = %v", cp)
	}
	if got := c.statusOf(t, id); got != promotionv1.CampaignStatus_CAMPAIGN_STATUS_CANCELLED {
		t.Fatalf("status after cancel = %v", got)
	}
	if evs := c.rec.got(); len(evs) != 1 || evs[0].ActorID != "admin-1" {
		t.Fatalf("audit events = %+v", evs)
	}
}

func TestCancelCampaignRequiresAdmin(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", -time.Hour, time.Hour)
	for _, tc := range []struct {
		name string
		want codes.Code
		ctx  func() error
	}{
		{"seller", codes.PermissionDenied, func() error {
			_, err := c.fc.CancelCampaign(sellerCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id})
			return err
		}},
		{"anonymous", codes.Unauthenticated, func() error {
			_, err := c.fc.CancelCampaign(anonCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id})
			return err
		}},
	} {
		if got := code(tc.ctx()); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
	if got := c.statusOf(t, id); got != promotionv1.CampaignStatus_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("campaign status = %v, want ACTIVE", got)
	}
	c.rec.requireNone(t, "refused cancel")
}

func TestCampaignWritesAuditDownErrorInfo(t *testing.T) {
	writes := map[string]func(*testing.T, *campaignRig, string) error{
		"cancel": func(t *testing.T, c *campaignRig, id string) error {
			_, err := c.fc.CancelCampaign(adminCtx(t), &promotionv1.CancelCampaignRequest{CampaignId: id})
			return err
		},
		"update": func(t *testing.T, c *campaignRig, id string) error {
			_, err := c.fc.UpdateCampaign(adminCtx(t), &promotionv1.UpdateCampaignRequest{
				CampaignId: id, SalePrice: 40000, StockCap: 5,
				StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
			})
			return err
		},
	}
	for name, write := range writes {
		t.Run(name, func(t *testing.T) {
			c := newCampaignRig(t)
			id := c.seed(t, "l1", time.Hour, 2*time.Hour)
			c.rec.mu.Lock()
			c.rec.err = errors.New("connection refused")
			c.rec.mu.Unlock()
			err := write(t, c, id)
			if code(err) != codes.Unavailable {
				t.Fatalf("got %v, want Unavailable", err)
			}
			requireInfo(t, err, "AUDIT_UNAVAILABLE")
			if got := c.statusOf(t, id); got != promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED {
				t.Fatalf("campaign changed to %v", got)
			}
		})
	}
}

func TestUpdateCampaignRequiresAdmin(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", time.Hour, 2*time.Hour)
	req := &promotionv1.UpdateCampaignRequest{
		CampaignId: id, SalePrice: 40000, StockCap: 5,
		StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
	}
	if _, err := c.fc.UpdateCampaign(sellerCtx(t), req); code(err) != codes.PermissionDenied {
		t.Fatalf("seller: got %v, want PermissionDenied", code(err))
	}
	if _, err := c.fc.UpdateCampaign(anonCtx(t), req); code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: got %v, want Unauthenticated", code(err))
	}
	got := c.list(t).GetCampaigns()
	if len(got) != 1 || got[0].GetSalePrice() != 50000 || got[0].GetStockCap() != 10 {
		t.Fatalf("campaign changed: %v", got)
	}
	c.rec.requireNone(t, "refused update")
}

func TestUpdateCampaignAdminSucceeds_SellerCannotEdit(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", time.Hour, 2*time.Hour)
	resp, err := c.fc.UpdateCampaign(adminCtx(t), &promotionv1.UpdateCampaignRequest{
		CampaignId: id, SalePrice: 40000, StockCap: 5,
		StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	cp := resp.GetCampaign()
	if cp.GetId() != id || cp.GetSalePrice() != 40000 || cp.GetStockCap() != 5 || cp.GetStatus() != promotionv1.CampaignStatus_CAMPAIGN_STATUS_SCHEDULED {
		t.Fatalf("update response = %v", cp)
	}
	if evs := c.rec.got(); len(evs) != 1 {
		t.Fatalf("audit events = %+v", evs)
	}
}

func TestUpdateCampaignNotFound(t *testing.T) {
	c := newCampaignRig(t)
	_, err := c.fc.UpdateCampaign(adminCtx(t), &promotionv1.UpdateCampaignRequest{
		CampaignId: "no-such-campaign", SalePrice: 40000, StockCap: 5,
		StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
	})
	if code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	c.rec.requireNone(t, "update unknown")
}

func TestUpdateCampaignStartedIsFailedPrecondition_SellerCannotEdit(t *testing.T) {
	c := newCampaignRig(t)
	id := c.seed(t, "l1", -time.Hour, time.Hour)
	_, err := c.fc.UpdateCampaign(adminCtx(t), &promotionv1.UpdateCampaignRequest{
		CampaignId: id, SalePrice: 40000, StockCap: 5,
		StartsAt: timestamppb.New(time.Now().Add(3 * time.Hour)), EndsAt: timestamppb.New(time.Now().Add(4 * time.Hour)),
	})
	if code(err) != codes.FailedPrecondition {
		t.Fatalf("got %v, want FailedPrecondition", err)
	}
}

func createReq(listingID, key string) *promotionv1.CreateCampaignRequest {
	return &promotionv1.CreateCampaignRequest{
		ListingId: listingID, SalePrice: 50000, StockCap: 10,
		StartsAt:       timestamppb.New(time.Now().Add(time.Hour)),
		EndsAt:         timestamppb.New(time.Now().Add(2 * time.Hour)),
		IdempotencyKey: key,
	}
}

func TestCreateCampaignSellerOwnershipDenied(t *testing.T) {
	c := newCampaignRig(t)
	_, err := c.fc.CreateCampaign(sellerCtx(t), createReq("l2", ""))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", err)
	}
	if n := c.cf.count(); n != 0 {
		t.Fatalf("%d campaigns created", n)
	}
}

func TestCreateCampaignIdempotencyKeyRequired(t *testing.T) {
	for name, key := range map[string]string{"empty": "", "seven chars": "abcdefg", "contains space": "abcd efgh"} {
		t.Run(name, func(t *testing.T) {
			c := newCampaignRig(t)
			_, err := c.fc.CreateCampaign(adminCtx(t), createReq("l1", key))
			st := status.Convert(err)
			if st.Code() != codes.InvalidArgument || !strings.Contains(st.Message(), "idempotency_key") {
				t.Fatalf("got %v %q, want InvalidArgument naming idempotency_key", st.Code(), st.Message())
			}
			if n := c.cf.count(); n != 0 {
				t.Fatalf("%d campaigns created", n)
			}
			c.rec.requireNone(t, name)
		})
	}
}

func TestCreateCampaignAdminValidKeyOK_MissingOrMalformedKey(t *testing.T) {
	c := newCampaignRig(t)
	for _, key := range []string{"abcdefgh", strings.Repeat("A", 64), "key_-12345"} {
		if _, err := c.fc.CreateCampaign(adminCtx(t), createReq("l1", key)); err != nil {
			t.Fatalf("key %q: %v", key, err)
		}
	}
	if _, err := c.fc.CreateCampaign(adminCtx(t), createReq("l1", strings.Repeat("A", 65))); code(err) != codes.InvalidArgument {
		t.Fatalf("65-char key: got %v, want InvalidArgument", code(err))
	}
}

func TestCreateCampaignSellerNoKeyRequired(t *testing.T) {
	c := newCampaignRig(t)
	resp, err := c.fc.CreateCampaign(sellerCtx(t), createReq("l1", ""))
	if err != nil {
		t.Fatalf("seller create: %v", err)
	}
	if resp.GetCampaign().GetListingId() != "l1" {
		t.Fatalf("campaign = %v", resp.GetCampaign())
	}
	if n := c.cf.count(); n != 1 {
		t.Fatalf("%d campaigns created, want 1", n)
	}
}
