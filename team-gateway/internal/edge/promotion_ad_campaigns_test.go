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
	listMyAdCampaignsPath = "/platform.promotion.v1.SponsoredService/ListMyAdCampaigns"
	pauseAdCampaignPath   = "/platform.promotion.v1.SponsoredService/PauseAdCampaign"
	resumeAdCampaignPath  = "/platform.promotion.v1.SponsoredService/ResumeAdCampaign"
)

type adCampaignUpstream struct {
	promotionv1.SponsoredServiceClient
	calls atomic.Int64
	err   error
}

func (a *adCampaignUpstream) ListMyAdCampaigns(_ context.Context, _ *promotionv1.ListMyAdCampaignsRequest, _ ...grpc.CallOption) (*promotionv1.ListMyAdCampaignsResponse, error) {
	a.calls.Add(1)
	if a.err != nil {
		return nil, a.err
	}
	return &promotionv1.ListMyAdCampaignsResponse{Campaigns: []*promotionv1.AdCampaign{{Id: "ad-1"}}}, nil
}

func (a *adCampaignUpstream) PauseAdCampaign(_ context.Context, in *promotionv1.PauseAdCampaignRequest, _ ...grpc.CallOption) (*promotionv1.PauseAdCampaignResponse, error) {
	a.calls.Add(1)
	if a.err != nil {
		return nil, a.err
	}
	return &promotionv1.PauseAdCampaignResponse{Campaign: &promotionv1.AdCampaign{Id: in.GetCampaignId()}}, nil
}

func (a *adCampaignUpstream) ResumeAdCampaign(_ context.Context, in *promotionv1.ResumeAdCampaignRequest, _ ...grpc.CallOption) (*promotionv1.ResumeAdCampaignResponse, error) {
	a.calls.Add(1)
	if a.err != nil {
		return nil, a.err
	}
	return &promotionv1.ResumeAdCampaignResponse{Campaign: &promotionv1.AdCampaign{Id: in.GetCampaignId()}}, nil
}

var adCampaignCalls = []struct{ name, path, body string }{
	{"ListMyAdCampaigns", listMyAdCampaignsPath, `{}`},
	{"PauseAdCampaign", pauseAdCampaignPath, `{"campaign_id":"ad-1"}`},
	{"ResumeAdCampaign", resumeAdCampaignPath, `{"campaign_id":"ad-1"}`},
}

func callAd(t *testing.T, up *adCampaignUpstream, path, body string, scopes []string) (int, string) {
	t.Helper()
	return callWith(t, &upstream.Clients{Sponsored: up}, path, body, scopes)
}

// Scenario "Anonymous request is rejected at the edge".
func TestPromotionAdCampaigns_AnonymousRejectedAtEdge(t *testing.T) {
	for _, c := range adCampaignCalls {
		up := &adCampaignUpstream{}
		code, body := callAd(t, up, c.path, c.body, nil)
		if code != http.StatusUnauthorized || !strings.Contains(body, "unauthenticated") {
			t.Errorf("%s anonymous = %d %s, want 401 unauthenticated", c.name, code, body)
		}
		if n := up.calls.Load(); n != 0 {
			t.Errorf("%s anonymous reached promotion %d times, want 0", c.name, n)
		}
	}
}

// Scenario "Upstream NOT_FOUND is passed through" (and authenticated forwarding works).
func TestPromotionAdCampaigns_UpstreamNotFoundPassedThrough(t *testing.T) {
	for _, c := range adCampaignCalls {
		up := &adCampaignUpstream{err: status.Error(codes.NotFound, "campaign not found")}
		code, body := callAd(t, up, c.path, c.body, []string{"listing.read", "seller"})
		if code != http.StatusNotFound || !strings.Contains(body, "not_found") {
			t.Errorf("%s = %d %s, want 404 not_found", c.name, code, body)
		}
		if n := up.calls.Load(); n != 1 {
			t.Errorf("%s upstream calls = %d, want 1", c.name, n)
		}
	}
}

// Scenario "Upstream NOT_FOUND is passed through": success path is forwarded unchanged.
func TestPromotionAdCampaigns_UpstreamNotFoundPassedThrough_SuccessForwarded(t *testing.T) {
	for _, c := range adCampaignCalls {
		up := &adCampaignUpstream{}
		code, body := callAd(t, up, c.path, c.body, []string{"listing.read", "seller"})
		if code != http.StatusOK || !strings.Contains(body, "ad-1") {
			t.Errorf("%s = %d %s, want 200 with ad-1", c.name, code, body)
		}
	}
}

// Scenario "Upstream unavailable is surfaced".
func TestPromotionAdCampaigns_UpstreamUnavailableSurfaced(t *testing.T) {
	for _, c := range adCampaignCalls {
		up := &adCampaignUpstream{err: status.Error(codes.Unavailable, "promotion down")}
		code, body := callAd(t, up, c.path, c.body, []string{"listing.read", "seller"})
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", c.name, code, body)
		}
		if !strings.Contains(body, "PROMOTION_UNAVAILABLE") {
			t.Errorf("%s missing PROMOTION_UNAVAILABLE ErrorInfo: %s", c.name, body)
		}
	}
}
