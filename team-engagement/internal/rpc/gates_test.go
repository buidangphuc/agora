package rpc_test

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/rpc"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

type gate string

const (
	gateAdmin  gate = "admin"
	gateWrite  gate = "engagement:write"
	gateRead   gate = "engagement:read"
	gatePublic gate = "public"
)

type rpcCall struct {
	name string
	gate gate
	call func(h *rpc.Handler, ctx context.Context) error
}

func e[T any](_ T, err error) error { return err }

const reportListing = "0b9f6d44-6f1c-4f7e-9d5a-2d9a0e0c1b11"

func allCalls() []rpcCall {
	page := &commonv1.PageRequest{PageSize: 10}
	return []rpcCall{
		{"HideReview", gateAdmin, func(h *rpc.Handler, c context.Context) error {
			return e(h.HideReview(c, &engagementv1.HideReviewRequest{ReviewId: "r-1", Reason: "spam"}))
		}},
		{"ResolveDispute", gateAdmin, func(h *rpc.Handler, c context.Context) error {
			return e(h.ResolveDispute(c, &engagementv1.ResolveDisputeRequest{DisputeId: "d-1", Status: engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED}))
		}},
		{"ListDisputes", gateAdmin, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListDisputes(c, &engagementv1.ListDisputesRequest{}))
		}},
		{"ListReports", gateAdmin, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListReports(c, &engagementv1.ListReportsRequest{}))
		}},
		{"ResolveReport", gateAdmin, func(h *rpc.Handler, c context.Context) error {
			return e(h.ResolveReport(c, &engagementv1.ResolveReportRequest{ReportId: "rp-1", Status: engagementv1.ReportStatus_REPORT_STATUS_ACTIONED, ResolutionNote: "n"}))
		}},

		{"AddFavorite", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.AddFavorite(c, &engagementv1.AddFavoriteRequest{ListingId: "l-1"}))
		}},
		{"RemoveFavorite", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.RemoveFavorite(c, &engagementv1.RemoveFavoriteRequest{ListingId: "l-1"}))
		}},
		{"FollowSeller", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.FollowSeller(c, &engagementv1.FollowSellerRequest{SellerId: "s-1"}))
		}},
		{"UnfollowSeller", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.UnfollowSeller(c, &engagementv1.UnfollowSellerRequest{SellerId: "s-1"}))
		}},
		{"CreateReview", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.CreateReview(c, createReviewReq("l-1", "order-1", 5)))
		}},
		{"MarkReviewHelpful", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.MarkReviewHelpful(c, &engagementv1.MarkReviewHelpfulRequest{ReviewId: "r-1"}))
		}},
		{"AskQuestion", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.AskQuestion(c, &engagementv1.AskQuestionRequest{ListingId: "l-1", QuestionText: "ok?"}))
		}},
		{"AnswerQuestion", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.AnswerQuestion(c, &engagementv1.AnswerQuestionRequest{QuestionId: "q-1", AnswerText: "a"}))
		}},
		{"CreateDispute", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.CreateDispute(c, &engagementv1.CreateDisputeRequest{OrderId: "order-1", DefendantId: "seller-1", Reason: "broken"}))
		}},
		{"CheckIn", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.CheckIn(c, &engagementv1.CheckInRequest{}))
		}},
		{"CreateReport", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.CreateReport(c, &engagementv1.CreateReportRequest{TargetType: engagementv1.ReportTargetType_REPORT_TARGET_TYPE_LISTING, TargetId: reportListing, ReasonCode: "counterfeit"}))
		}},
		{"CreateCollection", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.CreateCollection(c, &engagementv1.CreateCollectionRequest{Name: "wish"}))
		}},
		{"AddToCollection", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.AddToCollection(c, &engagementv1.AddToCollectionRequest{CollectionId: "c-1", ListingId: "l-1"}))
		}},
		{"RemoveFromCollection", gateWrite, func(h *rpc.Handler, c context.Context) error {
			return e(h.RemoveFromCollection(c, &engagementv1.RemoveFromCollectionRequest{CollectionId: "c-1", ListingId: "l-1"}))
		}},

		{"IsFavorite", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.IsFavorite(c, &engagementv1.IsFavoriteRequest{ListingId: "l-1"}))
		}},
		{"ListFavorites", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListFavorites(c, &engagementv1.ListFavoritesRequest{Page: page}))
		}},
		{"IsFollowing", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.IsFollowing(c, &engagementv1.IsFollowingRequest{SellerId: "s-1"}))
		}},
		{"ListFollowedSellers", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListFollowedSellers(c, &engagementv1.ListFollowedSellersRequest{Page: page}))
		}},
		{"ListFollowedListings", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListFollowedListings(c, &engagementv1.ListFollowedListingsRequest{Page: page}))
		}},
		{"GetDispute", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetDispute(c, &engagementv1.GetDisputeRequest{DisputeId: "d-1"}))
		}},
		{"ListOrderDisputes", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListOrderDisputes(c, &engagementv1.ListOrderDisputesRequest{OrderId: "order-1"}))
		}},
		{"GetLoyalty", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetLoyalty(c, &engagementv1.GetLoyaltyRequest{}))
		}},
		{"ListCollections", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListCollections(c, &engagementv1.ListCollectionsRequest{}))
		}},
		{"ListCollectionItems", gateRead, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListCollectionItems(c, &engagementv1.ListCollectionItemsRequest{CollectionId: "c-1", Page: page}))
		}},

		{"RecordView", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.RecordView(c, &engagementv1.RecordViewRequest{ListingId: "l-1"}))
		}},
		{"GetRecentlyViewed", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetRecentlyViewed(c, &engagementv1.GetRecentlyViewedRequest{Page: page}))
		}},
		{"GetListingStats", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetListingStats(c, &engagementv1.GetListingStatsRequest{ListingId: "l-1"}))
		}},
		{"ListReviews", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListReviews(c, &engagementv1.ListReviewsRequest{ListingId: "l-1"}))
		}},
		{"GetListingRatingSummary", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetListingRatingSummary(c, &engagementv1.GetListingRatingSummaryRequest{ListingId: "l-1"}))
		}},
		{"GetShopRatingSummary", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.GetShopRatingSummary(c, &engagementv1.GetShopRatingSummaryRequest{SellerId: "s-1"}))
		}},
		{"ListQuestionsByListing", gatePublic, func(h *rpc.Handler, c context.Context) error {
			return e(h.ListQuestionsByListing(c, &engagementv1.ListQuestionsByListingRequest{ListingId: "l-1"}))
		}},
	}
}

func callsOf(g gate) []rpcCall {
	var out []rpcCall
	for _, c := range allCalls() {
		if c.gate == g {
			out = append(out, c)
		}
	}
	return out
}

type gateFixture struct {
	h        *rpc.Handler
	repo     *storetest.FakeRepository
	disputes *countingDisputes
	audit    *auditSpy
	orders   *countingOrders
}

func newGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	repo := storetest.NewFakeRepository()
	disputes := &countingDisputes{FakeDisputeRepository: storetest.NewFakeDisputeRepository()}
	audit := &auditSpy{}
	gt := auditx.NewGate(audit, nil)
	orders := &countingOrders{orders: fakeOrders{"order-1": {buyer: "u-1", seller: "seller-1"}}}
	h := newHandler(t, deps{
		repo:     repo,
		disputes: engagement.NewDisputeService(disputes, gt, nil),
		reviews:  engagement.NewReviewService(storetest.NewFakeReviewRepository(), &reviewVerifier{orders: map[string]string{}}, gt, nil),
		reports:  engagement.NewReportService(storetest.NewFakeReportRepository(), gt, nil),
		orders:   orders,
	})
	return &gateFixture{h: h, repo: repo, disputes: disputes, audit: audit, orders: orders}
}

func (f *gateFixture) assertUntouched(t *testing.T, who string) {
	t.Helper()
	ctx := context.Background()
	if fav, _ := f.repo.IsFavorite(ctx, "u-1", "l-1"); fav {
		t.Fatalf("%s: a favourite was stored", who)
	}
	if fol, _ := f.repo.IsFollowing(ctx, "u-1", "s-1"); fol {
		t.Fatalf("%s: a follow was stored", who)
	}
	if l, _ := f.repo.GetLoyalty(ctx, "u-1"); l != (store.Loyalty{}) {
		t.Fatalf("%s: loyalty changed: %+v", who, l)
	}
	if f.disputes.creates != 0 || f.orders.calls != 0 {
		t.Fatalf("%s: disputes=%d order lookups=%d", who, f.disputes.creates, f.orders.calls)
	}
	if len(f.audit.events) != 0 || len(f.audit.generic) != 0 {
		t.Fatalf("%s: audit events written", who)
	}
}

var anonymousCallers = map[string]func() context.Context{
	"no principal":        context.Background,
	"anonymous principal": func() context.Context { return principalCtx("anonymous", auth.TypeAnonymous, "listing.read") },
}

// Scenario: Engagement SDK: Anonymous caller is unauthenticated on admin RPCs
func TestAdminRpcsRejectAnonymous(t *testing.T) {
	if n := len(callsOf(gateAdmin)); n != 5 {
		t.Fatalf("admin table has %d RPCs, want 5", n)
	}
	for who, mk := range anonymousCallers {
		for _, c := range callsOf(gateAdmin) {
			t.Run(who+"/"+c.name, func(t *testing.T) {
				f := newGateFixture(t)
				if got := status.Code(c.call(f.h, mk())); got != codes.Unauthenticated {
					t.Fatalf("code = %v, want Unauthenticated", got)
				}
				f.assertUntouched(t, c.name)
			})
		}
	}
}

// Scenario: Engagement SDK: Non-admin is denied on admin RPCs
func TestAdminRpcsDenyNonAdminBeforeValidation(t *testing.T) {
	ctx := principalCtx("u-1", auth.TypeUser, "user")
	invalid := map[string]func(h *rpc.Handler) error{
		"HideReview":     func(h *rpc.Handler) error { return e(h.HideReview(ctx, &engagementv1.HideReviewRequest{})) },
		"ResolveDispute": func(h *rpc.Handler) error { return e(h.ResolveDispute(ctx, &engagementv1.ResolveDisputeRequest{})) },
		"ListDisputes": func(h *rpc.Handler) error {
			return e(h.ListDisputes(ctx, &engagementv1.ListDisputesRequest{Page: &commonv1.PageRequest{Cursor: "%%%"}}))
		},
		"ListReports": func(h *rpc.Handler) error {
			return e(h.ListReports(ctx, &engagementv1.ListReportsRequest{Page: &commonv1.PageRequest{Cursor: "abc"}}))
		},
		"ResolveReport": func(h *rpc.Handler) error { return e(h.ResolveReport(ctx, &engagementv1.ResolveReportRequest{})) },
	}
	if len(invalid) != 5 {
		t.Fatalf("want 5 admin RPCs, have %d", len(invalid))
	}
	for name, call := range invalid {
		t.Run(name, func(t *testing.T) {
			f := newGateFixture(t)
			if got := status.Code(call(f.h)); got != codes.PermissionDenied {
				t.Fatalf("code = %v, want PermissionDenied", got)
			}
			f.assertUntouched(t, name)
		})
	}
}

// Scenario: Engagement SDK: Read scope alone cannot write
func TestEngagementScopesAreEnforced(t *testing.T) {
	if len(callsOf(gateWrite)) != 14 || len(callsOf(gateRead)) != 10 {
		t.Fatalf("table has %d write and %d read RPCs, want 14 and 10", len(callsOf(gateWrite)), len(callsOf(gateRead)))
	}
	cases := []struct {
		gate gate
		held string
	}{{gateWrite, "engagement:read"}, {gateRead, "engagement:write"}}
	for _, k := range cases {
		for _, c := range callsOf(k.gate) {
			t.Run(fmt.Sprintf("%s holding only %s/%s", k.gate, k.held, c.name), func(t *testing.T) {
				f := newGateFixture(t)
				if got := status.Code(c.call(f.h, principalCtx("u-1", auth.TypeUser, k.held))); got != codes.PermissionDenied {
					t.Fatalf("code = %v, want PermissionDenied", got)
				}
				f.assertUntouched(t, c.name)
			})
		}
	}
}

// Scenario: Engagement SDK: Anonymous caller is unauthenticated on scoped RPCs
func TestScopedRpcsRejectAnonymous(t *testing.T) {
	for who, mk := range anonymousCallers {
		for _, g := range []gate{gateWrite, gateRead} {
			for _, c := range callsOf(g) {
				t.Run(who+"/"+c.name, func(t *testing.T) {
					f := newGateFixture(t)
					if got := status.Code(c.call(f.h, mk())); got != codes.Unauthenticated {
						t.Fatalf("code = %v, want Unauthenticated", got)
					}
					f.assertUntouched(t, c.name)
				})
			}
		}
	}
}

// Scenario: Engagement SDK: Public RPCs serve anonymous callers
func TestPublicRpcsServeAnonymousCallers(t *testing.T) {
	if n := len(callsOf(gatePublic)); n != 7 {
		t.Fatalf("public table has %d RPCs, want 7", n)
	}
	callers := map[string]func() context.Context{
		"no principal":        context.Background,
		"anonymous principal": func() context.Context { return principalCtx("anonymous", auth.TypeAnonymous) },
		"user without scopes": func() context.Context { return principalCtx("u-9", auth.TypeUser, "listing.read") },
	}
	for who, mk := range callers {
		for _, c := range callsOf(gatePublic) {
			t.Run(who+"/"+c.name, func(t *testing.T) {
				f := newGateFixture(t)
				if err := c.call(f.h, mk()); err != nil {
					t.Fatalf("want OK, got %v", err)
				}
			})
		}
	}
	f := newGateFixture(t)
	res, err := f.h.GetRecentlyViewed(context.Background(), &engagementv1.GetRecentlyViewedRequest{})
	if err != nil || len(res.GetListingIds()) != 0 {
		t.Fatalf("anonymous GetRecentlyViewed = %v, %v; want empty list", res.GetListingIds(), err)
	}
}

// Scenario: Engagement SDK: Every RPC has a classified gate
func TestEveryRpcHasItsGate(t *testing.T) {
	methods := engagementv1.EngagementService_ServiceDesc.Methods
	if len(methods) != 36 {
		t.Fatalf("descriptor has %d methods, want 36", len(methods))
	}
	classified := map[string]gate{}
	for _, c := range allCalls() {
		if _, dup := classified[c.name]; dup {
			t.Fatalf("%s is classified twice", c.name)
		}
		classified[c.name] = c.gate
	}
	counts := map[gate]int{}
	f := newGateFixture(t)
	byName := map[string]rpcCall{}
	for _, c := range allCalls() {
		byName[c.name] = c
	}
	for _, m := range methods {
		g, ok := classified[m.MethodName]
		if !ok {
			t.Fatalf("%s has no classified gate", m.MethodName)
		}
		counts[g]++
		err := byName[m.MethodName].call(f.h, context.Background())
		if g == gatePublic {
			if err != nil {
				t.Fatalf("%s is public but an anonymous call returned %v", m.MethodName, err)
			}
			continue
		}
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s is gated (%s) but an anonymous call returned %v", m.MethodName, g, err)
		}
	}
	if counts[gateAdmin] != 5 || counts[gateWrite] != 14 || counts[gateRead] != 10 || counts[gatePublic] != 7 {
		t.Fatalf("gate counts = %v, want 5/14/10/7", counts)
	}
}

// Scenario: Engagement SDK: The feed is self-scoped and needs the read scope
func TestListFollowedListingsIsSelfScoped(t *testing.T) {
	repo := storetest.NewFakeRepository()
	h := newHandler(t, deps{repo: repo})
	ctx := context.Background()
	if _, err := repo.Follow(ctx, "u1", "seller-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.IndexSellerListing(ctx, "seller-1", "listing-1"); err != nil {
		t.Fatal(err)
	}
	req := &engagementv1.ListFollowedListingsRequest{Page: &commonv1.PageRequest{PageSize: 10}}

	own, err := h.ListFollowedListings(userCtx("u1"), req)
	if err != nil || len(own.GetListingIds()) != 1 {
		t.Fatalf("control: u1 feed = %v, %v; want 1 listing", own.GetListingIds(), err)
	}
	other, err := h.ListFollowedListings(userCtx("u2"), req)
	if err != nil || len(other.GetListingIds()) != 0 {
		t.Fatalf("u2 feed = %v, %v; want empty", other.GetListingIds(), err)
	}
	_, err = h.ListFollowedListings(principalCtx("u1", auth.TypeUser, "engagement:write"), req)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("write-only caller: want PermissionDenied, got %v", err)
	}
}

// Scenario: Engagement SDK: A malformed feed cursor is InvalidArgument
func TestListFollowedListingsMalformedCursor(t *testing.T) {
	h := newHandler(t, deps{})
	_, err := h.ListFollowedListings(userCtx("u1"), &engagementv1.ListFollowedListingsRequest{Page: &commonv1.PageRequest{Cursor: "garbage"}})
	st := status.Convert(err)
	if st.Code() != codes.InvalidArgument || st.Message() != "invalid cursor" {
		t.Fatalf("got %v %q, want InvalidArgument \"invalid cursor\"", st.Code(), st.Message())
	}
}
