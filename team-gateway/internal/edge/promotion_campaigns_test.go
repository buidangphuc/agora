package edge_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	listCampaignsPath  = "/platform.promotion.v1.FlashSaleService/ListCampaigns"
	cancelCampaignPath = "/platform.promotion.v1.FlashSaleService/CancelCampaign"
	updateCampaignPath = "/platform.promotion.v1.FlashSaleService/UpdateCampaign"
	createCampaignPath = "/platform.promotion.v1.FlashSaleService/CreateCampaign"
)

type campaignAdminUpstream struct {
	promotionv1.FlashSaleServiceClient
	listCalls   atomic.Int64
	cancelCalls atomic.Int64
	updateCalls atomic.Int64
	err         error
	gotList     atomic.Pointer[promotionv1.ListCampaignsRequest]
	gotCancel   atomic.Pointer[promotionv1.CancelCampaignRequest]
	gotUpdate   atomic.Pointer[promotionv1.UpdateCampaignRequest]
	gotPID      atomic.Value
}

func (c *campaignAdminUpstream) record(ctx context.Context) {
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if p := md.Get("x-principal-id"); len(p) > 0 {
			c.gotPID.Store(p[0])
		}
	}
}

func (c *campaignAdminUpstream) ListCampaigns(ctx context.Context, in *promotionv1.ListCampaignsRequest, _ ...grpc.CallOption) (*promotionv1.ListCampaignsResponse, error) {
	c.listCalls.Add(1)
	c.gotList.Store(in)
	c.record(ctx)
	if c.err != nil {
		return nil, c.err
	}
	return &promotionv1.ListCampaignsResponse{
		Campaigns:         []*promotionv1.FlashSaleCampaign{{Id: "c-1"}},
		FlashSaleDisabled: true,
	}, nil
}

func (c *campaignAdminUpstream) CancelCampaign(ctx context.Context, in *promotionv1.CancelCampaignRequest, _ ...grpc.CallOption) (*promotionv1.CancelCampaignResponse, error) {
	c.cancelCalls.Add(1)
	c.gotCancel.Store(in)
	c.record(ctx)
	if c.err != nil {
		return nil, c.err
	}
	return &promotionv1.CancelCampaignResponse{Campaign: &promotionv1.FlashSaleCampaign{Id: in.GetCampaignId()}}, nil
}

func (c *campaignAdminUpstream) UpdateCampaign(ctx context.Context, in *promotionv1.UpdateCampaignRequest, _ ...grpc.CallOption) (*promotionv1.UpdateCampaignResponse, error) {
	c.updateCalls.Add(1)
	c.gotUpdate.Store(in)
	c.record(ctx)
	if c.err != nil {
		return nil, c.err
	}
	return &promotionv1.UpdateCampaignResponse{Campaign: &promotionv1.FlashSaleCampaign{Id: in.GetCampaignId()}}, nil
}

func (c *campaignAdminUpstream) CreateCampaign(_ context.Context, _ *promotionv1.CreateCampaignRequest, _ ...grpc.CallOption) (*promotionv1.CreateCampaignResponse, error) {
	return nil, c.err
}

func (c *campaignAdminUpstream) total() int64 {
	return c.listCalls.Load() + c.cancelCalls.Load() + c.updateCalls.Load()
}

var campaignAdminCalls = []struct{ name, path, body string }{
	{"ListCampaigns", listCampaignsPath, `{}`},
	{"CancelCampaign", cancelCampaignPath, `{"campaign_id":"c-1"}`},
	{"UpdateCampaign", updateCampaignPath, `{"campaign_id":"c-1","sale_price":900}`},
}

func callCampaign(t *testing.T, up *campaignAdminUpstream, path, body string, scopes []string) (int, string) {
	t.Helper()
	return callWith(t, &upstream.Clients{FlashSale: up}, path, body, scopes)
}

// Scenario "Non-admin cannot list all campaigns": anonymous is UNAUTHENTICATED, no upstream call.
func TestNonAdminCannotListAllCampaigns_AnonymousUnauthenticated(t *testing.T) {
	for _, c := range campaignAdminCalls {
		up := &campaignAdminUpstream{}
		code, body := callCampaign(t, up, c.path, c.body, nil)
		if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
			t.Errorf("%s anonymous = %d %s, want 401 unauthenticated", c.name, code, body)
		}
		if n := up.total(); n != 0 {
			t.Errorf("%s anonymous reached promotion %d times, want 0", c.name, n)
		}
	}
}

// Scenario "Non-admin cannot list all campaigns": buyer and seller are PERMISSION_DENIED, no campaign returned.
func TestNonAdminCannotListAllCampaigns_BuyerSellerPermissionDenied(t *testing.T) {
	for _, c := range campaignAdminCalls {
		for _, who := range []struct {
			name   string
			scopes []string
		}{
			{"buyer", []string{"listing.read"}},
			{"seller", []string{"listing.read", "seller"}},
		} {
			up := &campaignAdminUpstream{}
			code, body := callCampaign(t, up, c.path, c.body, who.scopes)
			if code != http.StatusForbidden || !strings.Contains(body, "permission_denied") {
				t.Errorf("%s %s = %d %s, want 403 permission_denied", who.name, c.name, code, body)
			}
			if strings.Contains(body, "c-1") {
				t.Errorf("%s %s leaked a campaign: %s", who.name, c.name, body)
			}
			if n := up.total(); n != 0 {
				t.Errorf("%s %s reached promotion %d times, want 0", who.name, c.name, n)
			}
		}
	}
}

// Scenario "Non-admin cannot list all campaigns" (admin side): ListCampaigns is exposed and forwarded with the principal.
func TestAdminListCampaignsForwarded(t *testing.T) {
	up := &campaignAdminUpstream{}
	code, body := callCampaign(t, up, listCampaignsPath, `{"statuses":["CAMPAIGN_STATUS_ACTIVE"]}`, []string{"admin", "promotion.admin"})
	if code != http.StatusOK {
		t.Fatalf("admin ListCampaigns = %d %s, want 200 (not 501)", code, body)
	}
	if !strings.Contains(body, "c-1") || !strings.Contains(body, "flashSaleDisabled") && !strings.Contains(body, "flash_sale_disabled") {
		t.Fatalf("response not passed through: %s", body)
	}
	g := up.gotList.Load()
	if g == nil || len(g.GetStatuses()) != 1 || g.GetStatuses()[0] != promotionv1.CampaignStatus_CAMPAIGN_STATUS_ACTIVE {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("principal id on upstream call = %q, want user-1", pid)
	}
}

// Scenario "Non-admin cannot list all campaigns" (admin side): CancelCampaign is exposed and forwarded.
func TestAdminCancelCampaignForwarded(t *testing.T) {
	up := &campaignAdminUpstream{}
	code, body := callCampaign(t, up, cancelCampaignPath, `{"campaign_id":"c-1"}`, []string{"admin", "promotion.admin"})
	if code != http.StatusOK {
		t.Fatalf("admin CancelCampaign = %d %s, want 200 (not 501)", code, body)
	}
	if g := up.gotCancel.Load(); g.GetCampaignId() != "c-1" {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("principal id on upstream call = %q, want user-1", pid)
	}
}

// Scenario "Non-admin cannot list all campaigns" (admin side): UpdateCampaign is exposed and forwarded.
func TestAdminUpdateCampaignForwarded(t *testing.T) {
	up := &campaignAdminUpstream{}
	code, body := callCampaign(t, up, updateCampaignPath, `{"campaign_id":"c-1","sale_price":900}`, []string{"admin", "promotion.admin"})
	if code != http.StatusOK {
		t.Fatalf("admin UpdateCampaign = %d %s, want 200 (not 501)", code, body)
	}
	g := up.gotUpdate.Load()
	if g.GetCampaignId() != "c-1" || g.GetSalePrice() != 900 {
		t.Fatalf("forwarded request altered: %v", g)
	}
	if pid, _ := up.gotPID.Load().(string); pid != "user-1" {
		t.Fatalf("principal id on upstream call = %q, want user-1", pid)
	}
}

// Scenario "Non-admin cannot list all campaigns": admin writes are never retried and codes pass through.
func TestAdminCampaignWritesNotRetried_CodesPassThrough(t *testing.T) {
	for _, c := range campaignAdminCalls[1:] {
		up := &campaignAdminUpstream{err: status.Error(codes.Unavailable, "promotion down")}
		code, body := callCampaign(t, up, c.path, c.body, []string{"admin", "promotion.admin"})
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", c.name, code, body)
		}
		if n := up.total(); n != 1 {
			t.Errorf("%s upstream calls = %d, want exactly 1", c.name, n)
		}
	}
	for _, tc := range []struct {
		code codes.Code
		http int
		text string
	}{
		{codes.NotFound, http.StatusNotFound, "not_found"},
		{codes.FailedPrecondition, http.StatusBadRequest, "failed_precondition"},
	} {
		for _, c := range campaignAdminCalls {
			up := &campaignAdminUpstream{err: status.Error(tc.code, "x")}
			code, body := callCampaign(t, up, c.path, c.body, []string{"admin", "promotion.admin"})
			if code != tc.http || !strings.Contains(body, tc.text) {
				t.Errorf("%s %v = %d %s, want %d %s", c.name, tc.code, code, body, tc.http, tc.text)
			}
		}
	}
}

// Scenario "Non-admin cannot list all campaigns": ErrorInfo reason survives, raw upstream text does not.
func TestAdminCampaignErrorInfoReasonPassThrough(t *testing.T) {
	cases := []struct{ name, path, body string }{
		{"CreateCampaign", createCampaignPath, `{"listing_id":"l-1"}`},
	}
	cases = append(cases, campaignAdminCalls...)
	for _, reason := range []string{"AUDIT_UNAVAILABLE", "LISTING_UNAVAILABLE"} {
		for _, c := range cases {
			up := &campaignAdminUpstream{err: errWithInfo(t, codes.Unavailable, rawUpstreamText, "promotion.platform", reason)}
			code, body := callCampaign(t, up, c.path, c.body, []string{"admin", "promotion.admin"})
			if code != http.StatusServiceUnavailable {
				t.Errorf("%s %s = %d %s, want 503", c.name, reason, code, body)
			}
			if !strings.Contains(body, reason) || !strings.Contains(body, "promotion.platform") {
				t.Errorf("%s lost ErrorInfo %s: %s", c.name, reason, body)
			}
			if strings.Contains(body, rawUpstreamText) || strings.Contains(body, "10.0.0.7") {
				t.Errorf("%s leaked upstream text: %s", c.name, body)
			}
		}
	}
}
