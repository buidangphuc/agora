package edge_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	listMyCampaignsPath         = "/platform.promotion.v1.FlashSaleService/ListMyCampaigns"
	cancelMyCampaignPath        = "/platform.promotion.v1.FlashSaleService/CancelMyCampaign"
	listMyShopVouchersPath      = "/platform.promotion.v1.VoucherService/ListMyShopVouchers"
	deactivateMyShopVoucherPath = "/platform.promotion.v1.VoucherService/DeactivateMyShopVoucher"
)

var sellerScopes = []string{"listing.read", "listing.write"}

type sellerFlashUpstream struct {
	promotionv1.FlashSaleServiceClient
	calls     atomic.Int64
	err       error
	gotList   atomic.Pointer[promotionv1.ListMyCampaignsRequest]
	gotCancel atomic.Pointer[promotionv1.CancelMyCampaignRequest]
}

func (s *sellerFlashUpstream) ListMyCampaigns(_ context.Context, in *promotionv1.ListMyCampaignsRequest, _ ...grpc.CallOption) (*promotionv1.ListMyCampaignsResponse, error) {
	s.calls.Add(1)
	s.gotList.Store(in)
	if s.err != nil {
		return nil, s.err
	}
	return &promotionv1.ListMyCampaignsResponse{Campaigns: []*promotionv1.FlashSaleCampaign{{Id: "c-mine"}}}, nil
}

func (s *sellerFlashUpstream) CancelMyCampaign(_ context.Context, in *promotionv1.CancelMyCampaignRequest, _ ...grpc.CallOption) (*promotionv1.CancelMyCampaignResponse, error) {
	s.calls.Add(1)
	s.gotCancel.Store(in)
	if s.err != nil {
		return nil, s.err
	}
	return &promotionv1.CancelMyCampaignResponse{Campaign: &promotionv1.FlashSaleCampaign{Id: in.GetCampaignId()}}, nil
}

func (s *sellerFlashUpstream) ListCampaigns(context.Context, *promotionv1.ListCampaignsRequest, ...grpc.CallOption) (*promotionv1.ListCampaignsResponse, error) {
	s.calls.Add(1)
	return &promotionv1.ListCampaignsResponse{}, nil
}

type sellerVoucherUpstream struct {
	promotionv1.VoucherServiceClient
	calls atomic.Int64
	err   error
}

func (s *sellerVoucherUpstream) ListMyShopVouchers(context.Context, *promotionv1.ListMyShopVouchersRequest, ...grpc.CallOption) (*promotionv1.ListMyShopVouchersResponse, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return &promotionv1.ListMyShopVouchersResponse{}, nil
}

func (s *sellerVoucherUpstream) DeactivateMyShopVoucher(_ context.Context, _ *promotionv1.DeactivateMyShopVoucherRequest, _ ...grpc.CallOption) (*promotionv1.DeactivateMyShopVoucherResponse, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return &promotionv1.DeactivateMyShopVoucherResponse{}, nil
}

func (s *sellerVoucherUpstream) ListVouchers(context.Context, *promotionv1.ListVouchersRequest, ...grpc.CallOption) (*promotionv1.ListVouchersResponse, error) {
	s.calls.Add(1)
	return &promotionv1.ListVouchersResponse{}, nil
}

func TestSellerPromotionForwarders_SellerReachesSellerRPCsNotAdminOnes(t *testing.T) {
	fs := &sellerFlashUpstream{}
	vs := &sellerVoucherUpstream{}
	clients := &upstream.Clients{FlashSale: fs, Voucher: vs}

	code, body := callWith(t, clients, listMyCampaignsPath, `{}`, sellerScopes)
	if code != http.StatusOK || !strings.Contains(body, "c-mine") {
		t.Fatalf("seller ListMyCampaigns = %d %s, want 200 with the campaign (not 501)", code, body)
	}
	code, body = callWith(t, clients, cancelMyCampaignPath, `{"campaign_id":"c-mine"}`, sellerScopes)
	if code != http.StatusOK {
		t.Fatalf("seller CancelMyCampaign = %d %s, want 200", code, body)
	}
	if g := fs.gotCancel.Load(); g == nil || g.GetCampaignId() != "c-mine" {
		t.Fatalf("forwarded cancel request altered: %v", g)
	}
	code, body = callWith(t, clients, listMyShopVouchersPath, `{}`, sellerScopes)
	if code != http.StatusOK {
		t.Fatalf("seller ListMyShopVouchers = %d %s, want 200", code, body)
	}
	code, body = callWith(t, clients, deactivateMyShopVoucherPath, `{"voucher_id":"v-1"}`, sellerScopes)
	if code != http.StatusOK {
		t.Fatalf("seller DeactivateMyShopVoucher = %d %s, want 200", code, body)
	}

	before := fs.calls.Load() + vs.calls.Load()
	for _, p := range []string{listCampaignsPath} {
		code, body = callWith(t, clients, p, `{}`, sellerScopes)
		if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
			t.Errorf("seller %s = %d %s, want 403 permission_denied", p, code, body)
		}
	}
	if after := fs.calls.Load() + vs.calls.Load(); after != before {
		t.Errorf("admin RPCs reached promotion for a seller (%d calls)", after-before)
	}
}

func TestSellerPromotionForwarders_AnonymousRejectedWithoutUpstreamCall(t *testing.T) {
	fs := &sellerFlashUpstream{}
	vs := &sellerVoucherUpstream{}
	for _, p := range []string{listMyCampaignsPath, cancelMyCampaignPath, listMyShopVouchersPath, deactivateMyShopVoucherPath} {
		code, body := callWith(t, &upstream.Clients{FlashSale: fs, Voucher: vs}, p, `{}`, nil)
		if code != http.StatusUnauthorized {
			t.Errorf("anonymous %s = %d %s, want 401", p, code, body)
		}
	}
	if n := fs.calls.Load() + vs.calls.Load(); n != 0 {
		t.Errorf("anonymous reached promotion %d times, want 0", n)
	}
}

func assertSellerRPCsUnavailable(t *testing.T) {
	t.Helper()
	down := status.Error(codes.Unavailable, "connection error: dial tcp 10.0.0.9:9090: connection refused")
	clients := &upstream.Clients{FlashSale: &sellerFlashUpstream{err: down}, Voucher: &sellerVoucherUpstream{err: down}}
	for _, p := range []string{listMyShopVouchersPath, listMyCampaignsPath, cancelMyCampaignPath, deactivateMyShopVoucherPath} {
		code, body := callWith(t, clients, p, `{"campaign_id":"c-1","voucher_id":"v-1"}`, sellerScopes)
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", p, code, body)
		}
		if !strings.Contains(body, "PROMOTION_UNAVAILABLE") || !strings.Contains(body, "promotion.platform") {
			t.Errorf("%s missing PROMOTION_UNAVAILABLE ErrorInfo: %s", p, body)
		}
		if strings.Contains(body, "10.0.0.9") {
			t.Errorf("%s leaked upstream address: %s", p, body)
		}
	}
}

func TestSellerPromotionForwarders_PromotionDownSurfacesAsUnavailable(t *testing.T) {
	assertSellerRPCsUnavailable(t)
}

func TestPromotionSellerRPCsUnavailable(t *testing.T) {
	assertSellerRPCsUnavailable(t)
}
