package rpc_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
)

func TestEngagementHandler(t *testing.T) {
	repo := storetest.NewFakeRepository()
	qaSvc := engagement.NewQAService(storetest.NewFakeQARepository(), nil)

	orders := fakeOrders{"order-1": {buyer: "buyer_test", seller: "seller-1"}}
	h := newHandler(t, deps{repo: repo, qa: qaSvc, orders: orders})

	ctx := principalCtx("buyer_test", auth.TypeUser, "engagement:read", "engagement:write")

	t.Run("Add and Check Favorite", func(t *testing.T) {
		_, err := h.AddFavorite(ctx, &engagementv1.AddFavoriteRequest{
			ListingId: "item_123",
		})
		if err != nil {
			t.Fatalf("unexpected error adding favorite: %v", err)
		}

		isFavRes, err := h.IsFavorite(ctx, &engagementv1.IsFavoriteRequest{
			ListingId: "item_123",
		})
		if err != nil {
			t.Fatalf("unexpected error checking favorite: %v", err)
		}
		if !isFavRes.GetFavorite() {
			t.Errorf("expected is_favorite to be true")
		}

		listFavRes, err := h.ListFavorites(ctx, &engagementv1.ListFavoritesRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error listing favorites: %v", err)
		}
		if len(listFavRes.ListingIds) != 1 || listFavRes.ListingIds[0] != "item_123" {
			t.Errorf("expected [item_123], got %v", listFavRes.ListingIds)
		}

		_, err = h.RemoveFavorite(ctx, &engagementv1.RemoveFavoriteRequest{
			ListingId: "item_123",
		})
		if err != nil {
			t.Fatalf("unexpected error removing favorite: %v", err)
		}

		isFavAfter, _ := h.IsFavorite(ctx, &engagementv1.IsFavoriteRequest{ListingId: "item_123"})
		if isFavAfter.GetFavorite() {
			t.Errorf("expected is_favorite to be false after removal")
		}
	})

	t.Run("Follow Seller feed roundtrip", func(t *testing.T) {
		// Seed the seller→listing index the feed resolves against.
		if err := repo.IndexSellerListing(ctx, "shop_9", "listing_f1"); err != nil {
			t.Fatalf("index seller listing: %v", err)
		}

		if _, err := h.FollowSeller(ctx, &engagementv1.FollowSellerRequest{SellerId: "shop_9"}); err != nil {
			t.Fatalf("unexpected error following seller: %v", err)
		}

		isRes, err := h.IsFollowing(ctx, &engagementv1.IsFollowingRequest{SellerId: "shop_9"})
		if err != nil {
			t.Fatalf("unexpected error checking follow: %v", err)
		}
		if !isRes.GetFollowing() {
			t.Errorf("expected following=true")
		}

		listRes, err := h.ListFollowedSellers(ctx, &engagementv1.ListFollowedSellersRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error listing followed sellers: %v", err)
		}
		if len(listRes.GetSellerIds()) != 1 || listRes.GetSellerIds()[0] != "shop_9" {
			t.Errorf("expected [shop_9], got %v", listRes.GetSellerIds())
		}

		feedRes, err := h.ListFollowedListings(ctx, &engagementv1.ListFollowedListingsRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error listing followed listings: %v", err)
		}
		if len(feedRes.GetListingIds()) != 1 || feedRes.GetListingIds()[0] != "listing_f1" {
			t.Errorf("expected feed [listing_f1], got %v", feedRes.GetListingIds())
		}

		// Empty seller_id is rejected.
		_, err = h.FollowSeller(ctx, &engagementv1.FollowSellerRequest{SellerId: ""})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument for empty seller_id, got %v", err)
		}

		// Anonymous callers are denied (auth-scoped write).
		anonCtx := context.Background()
		_, err = h.FollowSeller(anonCtx, &engagementv1.FollowSellerRequest{SellerId: "shop_9"})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated for anonymous follow, got %v", err)
		}

		if _, err := h.UnfollowSeller(ctx, &engagementv1.UnfollowSellerRequest{SellerId: "shop_9"}); err != nil {
			t.Fatalf("unexpected error unfollowing: %v", err)
		}
		afterRes, _ := h.IsFollowing(ctx, &engagementv1.IsFollowingRequest{SellerId: "shop_9"})
		if afterRes.GetFollowing() {
			t.Errorf("expected following=false after unfollow")
		}
	})

	t.Run("RecordView and GetListingStats", func(t *testing.T) {
		viewRes, err := h.RecordView(ctx, &engagementv1.RecordViewRequest{
			ListingId: "item_view_1",
		})
		if err != nil {
			t.Fatalf("unexpected error recording view: %v", err)
		}
		if viewRes.ViewCount != 1 {
			t.Errorf("expected 1 view, got %d", viewRes.ViewCount)
		}

		statsRes, err := h.GetListingStats(ctx, &engagementv1.GetListingStatsRequest{
			ListingId: "item_view_1",
		})
		if err != nil {
			t.Fatalf("unexpected error getting stats: %v", err)
		}
		if statsRes.ViewCount != 1 {
			t.Errorf("expected 1 view in stats, got %d", statsRes.ViewCount)
		}
	})

	t.Run("RecordView records recently-viewed history", func(t *testing.T) {
		for _, id := range []string{"rv_1", "rv_2", "rv_1"} {
			if _, err := h.RecordView(ctx, &engagementv1.RecordViewRequest{ListingId: id}); err != nil {
				t.Fatalf("record view %s: %v", id, err)
			}
		}
		res, err := h.GetRecentlyViewed(ctx, &engagementv1.GetRecentlyViewedRequest{
			Page: &commonv1.PageRequest{PageSize: 10},
		})
		if err != nil {
			t.Fatalf("unexpected error getting recently viewed: %v", err)
		}
		// rv_1 re-viewed last → newest-first, deduped. item_view_1 was recorded
		// by the earlier subtest under the same principal, so it trails.
		got := res.GetListingIds()
		if len(got) != 3 || got[0] != "rv_1" || got[1] != "rv_2" || got[2] != "item_view_1" {
			t.Fatalf("expected [rv_1 rv_2 item_view_1], got %v", got)
		}
		if res.GetPage().GetTotal() != 3 {
			t.Fatalf("expected total 3, got %d", res.GetPage().GetTotal())
		}
	})

	t.Run("GetRecentlyViewed anonymous returns empty", func(t *testing.T) {
		anonCtx := context.Background() // no principal forwarded
		if _, err := h.RecordView(anonCtx, &engagementv1.RecordViewRequest{ListingId: "anon_item"}); err != nil {
			t.Fatalf("anonymous record view: %v", err)
		}
		res, err := h.GetRecentlyViewed(anonCtx, &engagementv1.GetRecentlyViewedRequest{})
		if err != nil {
			t.Fatalf("anonymous get recently viewed should not error: %v", err)
		}
		if len(res.GetListingIds()) != 0 {
			t.Fatalf("anonymous expected empty list, got %v", res.GetListingIds())
		}
	})

	t.Run("Product Q&A", func(t *testing.T) {
		// Ask question without listing_id -> error
		_, err := h.AskQuestion(ctx, &engagementv1.AskQuestionRequest{
			ListingId:    "",
			QuestionText: "Có bảo hành không?",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}

		// Ask question
		qRes, err := h.AskQuestion(ctx, &engagementv1.AskQuestionRequest{
			ListingId:    "listing-qa-1",
			QuestionText: "Có bảo hành không?",
		})
		if err != nil {
			t.Fatalf("unexpected error asking question: %v", err)
		}
		if qRes.GetQuestion().GetQuestionText() != "Có bảo hành không?" {
			t.Fatalf("unexpected question text: %s", qRes.GetQuestion().GetQuestionText())
		}
		qID := qRes.GetQuestion().GetId()

		// seller_1 owns listing-qa-1 per the listing.events projection.
		if err := repo.IndexSellerListing(ctx, "seller_1", "listing-qa-1"); err != nil {
			t.Fatalf("index seller listing: %v", err)
		}
		// Answer question
		sellerCtx := principalCtx("seller_1", auth.TypeUser, "engagement:write", "listing.write")
		ansRes, err := h.AnswerQuestion(sellerCtx, &engagementv1.AnswerQuestionRequest{
			QuestionId:  qID,
			AnswerText:  "Bảo hành chính hãng 12 tháng bạn nhé.",
			IsShopReply: true,
		})
		if err != nil {
			t.Fatalf("unexpected error answering question: %v", err)
		}
		if ansRes.GetAnswer().GetAnswerText() != "Bảo hành chính hãng 12 tháng bạn nhé." {
			t.Fatalf("unexpected answer text: %s", ansRes.GetAnswer().GetAnswerText())
		}

		if !ansRes.GetAnswer().GetIsShopReply() {
			t.Fatalf("seller principal asking for shop reply should be honoured")
		}

		// A buyer cannot self-declare a shop reply: the client flag is ignored.
		buyerAns, err := h.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{
			QuestionId:  qID,
			AnswerText:  "Mình cũng đang hỏi.",
			IsShopReply: true,
		})
		if err != nil {
			t.Fatalf("unexpected error answering as buyer: %v", err)
		}
		if buyerAns.GetAnswer().GetIsShopReply() {
			t.Fatalf("buyer must not be able to mark an answer as a shop reply")
		}

		// Seller principal answering without the flag is not a shop reply
		plain, err := h.AnswerQuestion(sellerCtx, &engagementv1.AnswerQuestionRequest{
			QuestionId: qID,
			AnswerText: "Thêm thông tin.",
		})
		if err != nil || plain.GetAnswer().GetIsShopReply() {
			t.Fatalf("unflagged answer must not be a shop reply (err=%v)", err)
		}

		// Anonymous cannot answer
		if _, err = h.AnswerQuestion(context.Background(), &engagementv1.AnswerQuestionRequest{
			QuestionId: qID,
			AnswerText: "x",
		}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", err)
		}

		// Answer nonexistent question
		_, err = h.AnswerQuestion(sellerCtx, &engagementv1.AnswerQuestionRequest{
			QuestionId: "nonexistent-q",
			AnswerText: "Test",
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}

		// List questions
		listRes, err := h.ListQuestionsByListing(context.Background(), &engagementv1.ListQuestionsByListingRequest{
			ListingId: "listing-qa-1",
		})
		if err != nil {
			t.Fatalf("unexpected error listing questions: %v", err)
		}
		if len(listRes.GetQuestions()) != 1 {
			t.Fatalf("expected 1 question, got %d", len(listRes.GetQuestions()))
		}
		if len(listRes.GetQuestions()[0].GetAnswers()) != 3 {
			t.Fatalf("expected 3 answers in question, got %d", len(listRes.GetQuestions()[0].GetAnswers()))
		}
	})

	t.Run("Disputes", func(t *testing.T) {
		// Create dispute without reason -> error
		_, err := h.CreateDispute(ctx, &engagementv1.CreateDisputeRequest{
			OrderId:     "order-1",
			DefendantId: "seller-1",
			Reason:      "",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}

		// Create dispute
		dispRes, err := h.CreateDispute(ctx, &engagementv1.CreateDisputeRequest{
			OrderId:      "order-1",
			DefendantId:  "seller-1",
			Reason:       "Hàng không đúng mô tả",
			EvidenceUrls: []string{"https://img.example.com/1.jpg"},
		})
		if err != nil {
			t.Fatalf("unexpected error creating dispute: %v", err)
		}
		dispID := dispRes.GetDispute().GetId()
		if dispRes.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN {
			t.Fatalf("expected OPEN status, got %v", dispRes.GetDispute().GetStatus())
		}

		// Get dispute
		getRes, err := h.GetDispute(ctx, &engagementv1.GetDisputeRequest{
			DisputeId: dispID,
		})
		if err != nil {
			t.Fatalf("unexpected error getting dispute: %v", err)
		}
		if getRes.GetDispute().GetReason() != "Hàng không đúng mô tả" {
			t.Fatalf("unexpected reason: %s", getRes.GetDispute().GetReason())
		}

		// Get nonexistent dispute
		_, err = h.GetDispute(ctx, &engagementv1.GetDisputeRequest{
			DisputeId: "nonexistent-d",
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("expected NotFound, got %v", err)
		}

		// Resolve dispute
		adminCtx := principalCtx("admin_1", auth.TypeUser, "engagement:write", "admin")

		// Non-admin (buyer with engagement:write only) and anonymous cannot resolve
		if _, err = h.ResolveDispute(ctx, &engagementv1.ResolveDisputeRequest{
			DisputeId: dispID,
			Status:    engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		}); status.Code(err) != codes.PermissionDenied {
			t.Fatalf("expected PermissionDenied for non-admin, got %v", err)
		}
		if _, err = h.ResolveDispute(context.Background(), &engagementv1.ResolveDisputeRequest{
			DisputeId: dispID,
			Status:    engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated for anonymous, got %v", err)
		}
		if _, err = h.ResolveDispute(adminCtx, &engagementv1.ResolveDisputeRequest{
			DisputeId: dispID,
			Status:    engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING,
		}); err != nil {
			t.Fatalf("unexpected error investigating dispute: %v", err)
		}
		resRes, err := h.ResolveDispute(adminCtx, &engagementv1.ResolveDisputeRequest{
			DisputeId:  dispID,
			Status:     engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
			Resolution: "Chấp thuận trả hàng hoàn tiền",
		})
		if err != nil {
			t.Fatalf("unexpected error resolving dispute: %v", err)
		}
		if resRes.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED {
			t.Fatalf("expected RESOLVED status, got %v", resRes.GetDispute().GetStatus())
		}
		if resRes.GetDispute().GetResolution() != "Chấp thuận trả hàng hoàn tiền" {
			t.Fatalf("unexpected resolution: %s", resRes.GetDispute().GetResolution())
		}

		// Resolve already closed dispute -> FailedPrecondition
		_, err = h.ResolveDispute(adminCtx, &engagementv1.ResolveDisputeRequest{
			DisputeId:  dispID,
			Status:     engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED,
			Resolution: "Thử lại",
		})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})
}

// ── CreateReview (review integrity) ──

func createReviewReq(listing, order string, rating int32) *engagementv1.CreateReviewRequest {
	return &engagementv1.CreateReviewRequest{ListingId: listing, OrderId: order, Rating: rating, Comment: "ok"}
}

func TestCreateReviewVerifiedStored(t *testing.T) {
	f := newReviewFixture(t)
	f.verifier.orders["buyer-1|order-1|L-1"] = "seller-1"
	res, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5))
	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if !res.GetReview().GetVerifiedPurchase() || res.GetReview().GetOrderId() != "order-1" {
		t.Fatalf("unexpected review %v", res.GetReview())
	}
	if ids := f.listIDs(t, context.Background(), "L-1"); len(ids) != 1 {
		t.Fatalf("review not listed: %v", ids)
	}
}

func TestCreateReviewEmptyOrderInvalidArgument(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "", 5))
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if f.verifier.calls != 0 || f.count("L-1") != 0 {
		t.Fatalf("empty order must not reach the verifier or store a row (calls=%d)", f.verifier.calls)
	}
}

func TestCreateReviewIneligibleOrderSingleMessage(t *testing.T) {
	f := newReviewFixture(t)
	// Only buyer-1 / order-ok / L-1 is eligible; each call below fails a different condition
	// (not completed, someone else's order, listing not in the order, unknown order),
	// all of which the verifier reports as (false, nil).
	f.verifier.orders["buyer-1|order-ok|L-1"] = "seller-1"
	calls := []struct{ buyer, order, listing string }{
		{"buyer-1", "order-pending", "L-1"},
		{"buyer-2", "order-ok", "L-1"},
		{"buyer-1", "order-ok", "L-other"},
		{"buyer-1", "order-404", "L-1"},
	}
	var msgs []string
	for _, c := range calls {
		_, err := f.h.CreateReview(userCtx(c.buyer), createReviewReq(c.listing, c.order, 4))
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("%+v: want FailedPrecondition, got %v", c, err)
		}
		msgs = append(msgs, status.Convert(err).Message())
	}
	for _, m := range msgs {
		if m == "" || m != msgs[0] {
			t.Fatalf("messages must be identical and non-empty, got %q", msgs)
		}
	}
	if f.count("L-1") != 0 || f.count("L-other") != 0 {
		t.Fatal("an ineligible review was stored")
	}
}

func TestCreateReviewAnonymousUnauthenticated(t *testing.T) {
	f := newReviewFixture(t)
	for name, ctx := range map[string]context.Context{"anonymous principal": anonCtx(), "no principal": context.Background()} {
		t.Run(name, func(t *testing.T) {
			_, err := f.h.CreateReview(ctx, createReviewReq("L-1", "order-1", 5))
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("want Unauthenticated, got %v", err)
			}
			if f.verifier.calls != 0 {
				t.Fatal("verifier called for an anonymous caller")
			}
		})
	}
}

func TestCreateReviewVerifierDownUnavailable(t *testing.T) {
	f := newReviewFixture(t)
	f.verifier.err = errors.New("order down")
	_, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5))
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want Unavailable, got %v", err)
	}
	if f.count("L-1") != 0 {
		t.Fatal("review stored while verifier was down")
	}
}

func TestCreateReviewDuplicateAlreadyExists(t *testing.T) {
	f := newReviewFixture(t)
	f.verifier.orders["buyer-1|order-1|L-1"] = "seller-1"
	if _, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5)); err != nil {
		t.Fatal(err)
	}
	_, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 4))
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("want AlreadyExists, got %v", err)
	}
	if f.count("L-1") != 1 {
		t.Fatalf("want 1 row, got %d", f.count("L-1"))
	}
}

func TestListReviewsIncludesUnverifiedExcludesHidden(t *testing.T) {
	f := newReviewFixture(t)
	f.seed(t, "L-1", "s-1", 5, true)
	f.seed(t, "L-1", "s-1", 3, true)
	legacy := f.seed(t, "L-1", "s-1", 1, false)
	hidden := f.seed(t, "L-1", "s-1", 4, true)
	if _, err := f.hide(adminCtx(), hidden.ID, "spam"); err != nil {
		t.Fatal(err)
	}

	res, err := f.h.ListReviews(context.Background(), &engagementv1.ListReviewsRequest{ListingId: "L-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.GetReviews()) != 3 || res.GetPage().GetTotal() != 3 {
		t.Fatalf("want 3 reviews / total 3, got %d / %d", len(res.GetReviews()), res.GetPage().GetTotal())
	}
	var sawLegacy bool
	for _, r := range res.GetReviews() {
		if r.GetId() == hidden.ID {
			t.Fatal("hidden review listed")
		}
		if r.GetId() == legacy.ID {
			sawLegacy = true
			if r.GetVerifiedPurchase() {
				t.Fatal("legacy review must be verified_purchase=false")
			}
		}
	}
	if !sawLegacy {
		t.Fatal("legacy unverified review missing from the list")
	}
}
