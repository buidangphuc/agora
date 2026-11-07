package edge

import (
	"slices"
	"sort"
	"testing"

	"connectrpc.com/connect"

	"github.com/buidangphuc/team-gateway/generated/platform/analytics/v1/analyticsv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/audit/v1/auditv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/engagement/v1/engagementv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/listing/v1/listingv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/order/v1/orderv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/promotion/v1/promotionv1connect"
	"github.com/buidangphuc/team-gateway/generated/platform/verification/v1/verificationv1connect"
)

func TestOnlyAdminsCanReadTheChannelFunnel_PolicyEntry(t *testing.T) {
	if got := adminProcedures[analyticsv1connect.AnalyticsQueryServiceGetChannelFunnelProcedure]; got != adminScope {
		t.Fatalf("GetChannelFunnel scope = %q, want %q", got, adminScope)
	}
}

// The admin policy set is pinned: changing it is a spec change.
// Scenario: User enforcement edge: Admin policy set is pinned.
// Scenario: Dispute edge: The admin procedure set is pinned.
func TestAdminProceduresPinnedSet(t *testing.T) {
	want := []string{
		analyticsv1connect.AnalyticsQueryServiceGetTrackingQualityReportProcedure,
		analyticsv1connect.AnalyticsQueryServiceGetRecommendationPerformanceProcedure,
		analyticsv1connect.AnalyticsQueryServiceGetSearchPerformanceProcedure,
		analyticsv1connect.AnalyticsQueryServiceGetChannelFunnelProcedure, // Only admins can read the channel funnel
		auditv1connect.AuditServiceQueryAuditLogProcedure,
		engagementv1connect.EngagementServiceResolveDisputeProcedure,
		engagementv1connect.EngagementServiceListDisputesProcedure,  // engagement-dispute-escalation
		engagementv1connect.EngagementServiceHideReviewProcedure,    // HideReview is admin-gated
		engagementv1connect.EngagementServiceListReportsProcedure,   // listing reports
		engagementv1connect.EngagementServiceResolveReportProcedure, // listing reports
		orderv1connect.OrderServiceForceFailSagaProcedure,
		verificationv1connect.VerificationServiceReviewKycProcedure,
		listingv1connect.ListingServiceModerateListingProcedure,     // domain-listing-moderation: admin-gated
		listingv1connect.ListingServiceListModerationQueueProcedure, // domain-listing-moderation: admin-gated
		identityv1connect.AccountAdminServiceListUsersProcedure,
		identityv1connect.AccountAdminServiceSetUserStatusProcedure,
		promotionv1connect.VoucherServiceDeactivateVoucherProcedure,
		promotionv1connect.VoucherServiceUpdateVoucherProcedure,
		promotionv1connect.FlashSaleServiceListCampaignsProcedure,
		promotionv1connect.FlashSaleServiceCancelCampaignProcedure,
		promotionv1connect.FlashSaleServiceUpdateCampaignProcedure,
	}
	var got []string
	for k := range adminProcedures {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("adminProcedures = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("adminProcedures = %v, want %v", got, want)
		}
	}
}

func TestPolicy_GetSearchPerformanceAdminOnly(t *testing.T) {
	proc := analyticsv1connect.AnalyticsQueryServiceGetSearchPerformanceProcedure
	if got := adminProcedures[proc]; got != adminScope {
		t.Fatalf("GetSearchPerformance scope = %q, want %q", got, adminScope)
	}
	anon := resolvedPrincipal{id: "anonymous", ptype: "anonymous"}
	buyer := resolvedPrincipal{id: "u-1", ptype: "user", scopes: []string{"listing.read"}}
	admin := resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"admin"}}
	if c := connect.CodeOf(requireProcedureScope(proc, anon)); c != connect.CodeUnauthenticated {
		t.Errorf("anonymous = %v, want unauthenticated", c)
	}
	if c := connect.CodeOf(requireProcedureScope(proc, buyer)); c != connect.CodePermissionDenied {
		t.Errorf("buyer = %v, want permission_denied", c)
	}
	if err := requireProcedureScope(proc, admin); err != nil {
		t.Errorf("admin: %v", err)
	}
}

func TestRequireProcedureScope(t *testing.T) {
	anon := resolvedPrincipal{id: "anonymous", ptype: "anonymous", scopes: []string{"listing.read"}}
	buyer := resolvedPrincipal{id: "u-1", ptype: "user", scopes: []string{"listing.read", "engagement:write"}}
	admin := resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"listing.read", "admin", "promotion.admin"}}
	service := resolvedPrincipal{id: "svc-order", ptype: "service", scopes: []string{"audit.write"}}

	for proc := range adminProcedures {
		for _, c := range []struct {
			name string
			p    resolvedPrincipal
			want connect.Code
		}{
			{"anonymous", anon, connect.CodeUnauthenticated},
			{"buyer", buyer, connect.CodePermissionDenied},
			{"service without admin", service, connect.CodePermissionDenied},
			{"admin", admin, 0},
		} {
			err := requireProcedureScope(proc, c.p)
			if c.want == 0 {
				if err != nil {
					t.Errorf("%s as %s: unexpected %v", proc, c.name, err)
				}
			} else if connect.CodeOf(err) != c.want {
				t.Errorf("%s as %s: got %v, want %v", proc, c.name, err, c.want)
			}
		}
	}
	// Ungated procedures never fail the policy.
	if err := requireProcedureScope(listingv1connect.ListingServiceGetListingProcedure, anon); err != nil {
		t.Errorf("ungated procedure rejected: %v", err)
	}
}

// Scenarios: User enforcement edge: Anonymous caller is rejected at the edge /
// Seller without admin scope is denied (policy level, both AccountAdmin procedures).
func TestAccountAdminProceduresPolicyGate(t *testing.T) {
	anon := resolvedPrincipal{id: "anonymous", ptype: "anonymous"}
	seller := resolvedPrincipal{id: "u-3", ptype: "user", scopes: []string{"listing.read", "listing.write"}}
	admin := resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"admin"}}
	for _, proc := range []string{
		identityv1connect.AccountAdminServiceListUsersProcedure,
		identityv1connect.AccountAdminServiceSetUserStatusProcedure,
	} {
		if c := connect.CodeOf(requireProcedureScope(proc, anon)); c != connect.CodeUnauthenticated {
			t.Errorf("%s anonymous = %v, want unauthenticated", proc, c)
		}
		if c := connect.CodeOf(requireProcedureScope(proc, seller)); c != connect.CodePermissionDenied {
			t.Errorf("%s seller = %v, want permission_denied", proc, c)
		}
		if err := requireProcedureScope(proc, admin); err != nil {
			t.Errorf("%s admin: %v", proc, err)
		}
	}
}

func TestPromotionAdminPolicyEntries(t *testing.T) {
	promotionProcs := []string{
		promotionv1connect.VoucherServiceDeactivateVoucherProcedure,
		promotionv1connect.VoucherServiceUpdateVoucherProcedure,
		promotionv1connect.FlashSaleServiceListCampaignsProcedure,
		promotionv1connect.FlashSaleServiceCancelCampaignProcedure,
		promotionv1connect.FlashSaleServiceUpdateCampaignProcedure,
	}
	adminOnly := resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"listing.read", "admin", "order.admin"}}
	promoAdmin := resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"admin", "promotion.admin"}}
	for _, proc := range promotionProcs {
		if got := adminProcedures[proc]; got != "promotion.admin" {
			t.Errorf("%s scope = %q, want promotion.admin", proc, got)
		}
		if c := connect.CodeOf(requireProcedureScope(proc, adminOnly)); c != connect.CodePermissionDenied {
			t.Errorf("%s admin without promotion.admin = %v, want permission_denied", proc, c)
		}
		if err := requireProcedureScope(proc, promoAdmin); err != nil {
			t.Errorf("%s promotion.admin: %v", proc, err)
		}
	}
	for proc, scope := range adminProcedures {
		if slices.Contains(promotionProcs, proc) {
			continue
		}
		if scope != adminScope {
			t.Errorf("%s scope = %q, want %q", proc, scope, adminScope)
		}
		if err := requireProcedureScope(proc, adminOnly); err != nil {
			t.Errorf("%s admin: %v", proc, err)
		}
	}
}
