package handler_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

func TestEngagementHandler(t *testing.T) {
	repo := repository.NewInMemoryRepository()
	reviewRepo := repository.NewInMemoryReviewRepository()
	reviewSvc := service.NewReviewService(reviewRepo, nil, nil)
	qaRepo := repository.NewInMemoryQARepository()
	qaSvc := service.NewQAService(qaRepo, nil)
	disputeRepo := repository.NewInMemoryDisputeRepository()
	disputeSvc := service.NewDisputeService(disputeRepo, nil)
	collectionRepo := repository.NewInMemoryCollectionRepository()
	collectionSvc := service.NewCollectionService(collectionRepo, nil)

	h := handler.NewEngagementHandler(repo, reviewSvc, qaSvc, disputeSvc, collectionSvc)

	principal := &commonv1.Principal{
		Id:     "buyer_test",
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"engagement:read", "engagement:write"},
	}
	ctx := interceptor.ContextWithPrincipal(context.Background(), principal)

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

		// Answer question
		sellerCtx := interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
			Id:     "seller_1",
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: []string{"engagement:write"},
		})
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
		if len(listRes.GetQuestions()[0].GetAnswers()) != 1 {
			t.Fatalf("expected 1 answer in question, got %d", len(listRes.GetQuestions()[0].GetAnswers()))
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
		adminCtx := interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{
			Id:     "admin_1",
			Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
			Scopes: []string{"engagement:write"},
		})
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
