package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/internal/grpcserver"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

func startServer(t *testing.T) engagementv1.EngagementServiceClient {
	t.Helper()
	s := &config.Settings{}
	s.Server.Port = 0
	s.Server.ReflectionEnabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reviewRepo := repository.NewInMemoryReviewRepository()
	reviewSvc := service.NewReviewService(reviewRepo, nil, logger)
	qaRepo := repository.NewInMemoryQARepository()
	qaSvc := service.NewQAService(qaRepo, logger)
	disputeRepo := repository.NewInMemoryDisputeRepository()
	disputeSvc := service.NewDisputeService(disputeRepo, logger)
	collectionRepo := repository.NewInMemoryCollectionRepository()
	collectionSvc := service.NewCollectionService(collectionRepo, logger)
	h := handler.NewEngagementHandler(repository.NewInMemoryRepository(), reviewSvc, qaSvc, disputeSvc, collectionSvc)
	srv := grpcserver.Build(s, h, nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(); srv.Stop() })
	return engagementv1.NewEngagementServiceClient(conn)
}

func userCtx(t *testing.T, id, scopes string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", "user", "x-principal-scopes", scopes)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func TestFavoriteLifecycle(t *testing.T) {
	client := startServer(t)
	ctx, cancel := userCtx(t, "u1", "engagement:read,engagement:write")
	defer cancel()

	if _, err := client.AddFavorite(ctx, &engagementv1.AddFavoriteRequest{ListingId: "L1"}); err != nil {
		t.Fatalf("AddFavorite: %v", err)
	}
	isFav, err := client.IsFavorite(ctx, &engagementv1.IsFavoriteRequest{ListingId: "L1"})
	if err != nil || !isFav.GetFavorite() {
		t.Fatalf("IsFavorite want true, got %v err=%v", isFav.GetFavorite(), err)
	}
	list, err := client.ListFavorites(ctx, &engagementv1.ListFavoritesRequest{})
	if err != nil || list.GetPage().GetTotal() != 1 || len(list.GetListingIds()) != 1 || list.GetListingIds()[0] != "L1" {
		t.Fatalf("ListFavorites unexpected: %+v err=%v", list, err)
	}
	if _, err := client.RemoveFavorite(ctx, &engagementv1.RemoveFavoriteRequest{ListingId: "L1"}); err != nil {
		t.Fatalf("RemoveFavorite: %v", err)
	}
	isFav, _ = client.IsFavorite(ctx, &engagementv1.IsFavoriteRequest{ListingId: "L1"})
	if isFav.GetFavorite() {
		t.Fatal("IsFavorite should be false after remove")
	}
}

func TestFavorite_RequiresScope(t *testing.T) {
	client := startServer(t)
	ctx, cancel := userCtx(t, "u1", "listing.read") // no engagement scope
	defer cancel()
	_, err := client.AddFavorite(ctx, &engagementv1.AddFavoriteRequest{ListingId: "L1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
}

func TestRecordView_PublicAndCounts(t *testing.T) {
	client := startServer(t)
	// No principal at all — RecordView is public.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client.RecordView(ctx, &engagementv1.RecordViewRequest{ListingId: "L9"})
	r, err := client.RecordView(ctx, &engagementv1.RecordViewRequest{ListingId: "L9"})
	if err != nil || r.GetViewCount() != 2 {
		t.Fatalf("RecordView want count 2, got %d err=%v", r.GetViewCount(), err)
	}
	stats, err := client.GetListingStats(ctx, &engagementv1.GetListingStatsRequest{ListingId: "L9"})
	if err != nil || stats.GetViewCount() != 2 {
		t.Fatalf("GetListingStats want view 2, got %+v err=%v", stats, err)
	}
}

func TestReviewLifecycleAndRatingSummary(t *testing.T) {
	client := startServer(t)
	ctx, cancel := userCtx(t, "buyer-1", "engagement:read,engagement:write")
	defer cancel()

	// 1. Create a 5-star review
	rev1, err := client.CreateReview(ctx, &engagementv1.CreateReviewRequest{
		ListingId: "listing-100",
		Rating:    5,
		Comment:   "Sản phẩm rất tuyệt vời!",
	})
	if err != nil {
		t.Fatalf("CreateReview 1: %v", err)
	}
	if rev1.GetReview().GetRating() != 5 {
		t.Fatalf("want rating 5, got %d", rev1.GetReview().GetRating())
	}

	// 2. Create a 4-star review
	_, err = client.CreateReview(ctx, &engagementv1.CreateReviewRequest{
		ListingId: "listing-100",
		Rating:    4,
		Comment:   "Giao hàng hơi chậm nhưng hàng đẹp.",
	})
	if err != nil {
		t.Fatalf("CreateReview 2: %v", err)
	}

	// 3. List Reviews (public read)
	publicCtx, pubCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pubCancel()

	listResp, err := client.ListReviews(publicCtx, &engagementv1.ListReviewsRequest{
		ListingId: "listing-100",
	})
	if err != nil {
		t.Fatalf("ListReviews: %v", err)
	}
	if len(listResp.GetReviews()) != 2 {
		t.Fatalf("want 2 reviews, got %d", len(listResp.GetReviews()))
	}

	// 4. Rating Summary -> average (5 + 4) / 2 = 4.5
	summary, err := client.GetListingRatingSummary(publicCtx, &engagementv1.GetListingRatingSummaryRequest{
		ListingId: "listing-100",
	})
	if err != nil {
		t.Fatalf("GetListingRatingSummary: %v", err)
	}
	if summary.GetReviewCount() != 2 {
		t.Fatalf("want review count 2, got %d", summary.GetReviewCount())
	}
	if summary.GetAverageRating() != 4.5 {
		t.Fatalf("want avg rating 4.5, got %v", summary.GetAverageRating())
	}
	if summary.GetBreakdown().GetStar_5() != 1 || summary.GetBreakdown().GetStar_4() != 1 {
		t.Fatalf("unexpected breakdown: %+v", summary.GetBreakdown())
	}
}

func TestProductQALifecycle(t *testing.T) {
	client := startServer(t)
	buyerCtx, buyerCancel := userCtx(t, "buyer-qa", "engagement:read,engagement:write")
	defer buyerCancel()
	sellerCtx, sellerCancel := userCtx(t, "seller-qa", "engagement:read,engagement:write")
	defer sellerCancel()

	// 1. Ask a question
	qResp, err := client.AskQuestion(buyerCtx, &engagementv1.AskQuestionRequest{
		ListingId:    "item-qa-1",
		QuestionText: "Sản phẩm còn màu đen không shop?",
	})
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}
	qID := qResp.GetQuestion().GetId()
	if qID == "" {
		t.Fatal("expected non-empty question id")
	}
	if qResp.GetQuestion().GetQuestionText() != "Sản phẩm còn màu đen không shop?" {
		t.Fatalf("unexpected question text: %s", qResp.GetQuestion().GetQuestionText())
	}

	// 2. Seller answers the question
	ansResp, err := client.AnswerQuestion(sellerCtx, &engagementv1.AnswerQuestionRequest{
		QuestionId:  qID,
		AnswerText:  "Dạ shop còn sẵn màu đen bạn nhé!",
		IsShopReply: true,
	})
	if err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	if ansResp.GetAnswer().GetAnswerText() != "Dạ shop còn sẵn màu đen bạn nhé!" {
		t.Fatalf("unexpected answer text: %s", ansResp.GetAnswer().GetAnswerText())
	}
	if !ansResp.GetAnswer().GetIsShopReply() {
		t.Fatal("expected is_shop_reply to be true")
	}

	// 3. List questions (public)
	pubCtx, pubCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer pubCancel()

	listResp, err := client.ListQuestionsByListing(pubCtx, &engagementv1.ListQuestionsByListingRequest{
		ListingId: "item-qa-1",
	})
	if err != nil {
		t.Fatalf("ListQuestionsByListing: %v", err)
	}
	if len(listResp.GetQuestions()) != 1 {
		t.Fatalf("want 1 question, got %d", len(listResp.GetQuestions()))
	}
	if len(listResp.GetQuestions()[0].GetAnswers()) != 1 {
		t.Fatalf("want 1 answer in question, got %d", len(listResp.GetQuestions()[0].GetAnswers()))
	}
}

func TestDisputeLifecycle(t *testing.T) {
	client := startServer(t)
	buyerCtx, buyerCancel := userCtx(t, "buyer-disp", "engagement:read,engagement:write")
	defer buyerCancel()
	adminCtx, adminCancel := userCtx(t, "admin-user", "engagement:read,engagement:write,admin")
	defer adminCancel()

	// 1. Create dispute
	dispResp, err := client.CreateDispute(buyerCtx, &engagementv1.CreateDisputeRequest{
		OrderId:      "order-999",
		DefendantId:  "seller-123",
		Reason:       "Hàng vỡ hỏng khi nhận",
		EvidenceUrls: []string{"https://example.com/broken.jpg"},
	})
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	dispID := dispResp.GetDispute().GetId()
	if dispID == "" {
		t.Fatal("expected non-empty dispute ID")
	}
	if dispResp.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN {
		t.Fatalf("expected status OPEN, got %v", dispResp.GetDispute().GetStatus())
	}

	// 2. Get dispute
	getResp, err := client.GetDispute(buyerCtx, &engagementv1.GetDisputeRequest{
		DisputeId: dispID,
	})
	if err != nil {
		t.Fatalf("GetDispute: %v", err)
	}
	if getResp.GetDispute().GetReason() != "Hàng vỡ hỏng khi nhận" {
		t.Fatalf("unexpected reason: %s", getResp.GetDispute().GetReason())
	}

	// 3. Resolve dispute
	resResp, err := client.ResolveDispute(adminCtx, &engagementv1.ResolveDisputeRequest{
		DisputeId:  dispID,
		Status:     engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED,
		Resolution: "Hoàn tiền toàn bộ cho người mua",
	})
	if err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}
	if resResp.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED {
		t.Fatalf("expected status RESOLVED, got %v", resResp.GetDispute().GetStatus())
	}
	if resResp.GetDispute().GetResolution() != "Hoàn tiền toàn bộ cho người mua" {
		t.Fatalf("unexpected resolution: %s", resResp.GetDispute().GetResolution())
	}

	// 4. Resolving closed dispute should fail
	_, err = client.ResolveDispute(adminCtx, &engagementv1.ResolveDisputeRequest{
		DisputeId:  dispID,
		Status:     engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED,
		Resolution: "Thử từ chối tranh chấp đã đóng",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition when resolving closed dispute, got %v", err)
	}
}
