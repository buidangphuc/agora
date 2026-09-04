// Package handler is the gRPC transport adapter for EngagementService.
package handler

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
)

// EngagementHandler implements engagementv1.EngagementServiceServer.
type EngagementHandler struct {
	engagementv1.UnimplementedEngagementServiceServer
	repo          repository.Repository
	reviewSvc     *service.ReviewService
	qaSvc         *service.QAService
	disputeSvc    *service.DisputeService
	collectionSvc *service.CollectionService
}

func NewEngagementHandler(
	repo repository.Repository,
	reviewSvc *service.ReviewService,
	qaSvc *service.QAService,
	disputeSvc *service.DisputeService,
	collectionSvc *service.CollectionService,
) *EngagementHandler {
	return &EngagementHandler{
		repo:          repo,
		reviewSvc:     reviewSvc,
		qaSvc:         qaSvc,
		disputeSvc:    disputeSvc,
		collectionSvc: collectionSvc,
	}
}

func userID(ctx context.Context) string {
	if p, ok := interceptor.PrincipalFromContext(ctx); ok {
		return p.GetId()
	}
	return ""
}

func (h *EngagementHandler) AddFavorite(
	ctx context.Context, req *engagementv1.AddFavoriteRequest,
) (*engagementv1.AddFavoriteResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if _, err := h.repo.AddFavorite(ctx, userID(ctx), req.GetListingId()); err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.AddFavoriteResponse{}, nil
}

func (h *EngagementHandler) RemoveFavorite(
	ctx context.Context, req *engagementv1.RemoveFavoriteRequest,
) (*engagementv1.RemoveFavoriteResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if _, err := h.repo.RemoveFavorite(ctx, userID(ctx), req.GetListingId()); err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.RemoveFavoriteResponse{}, nil
}

func (h *EngagementHandler) IsFavorite(
	ctx context.Context, req *engagementv1.IsFavoriteRequest,
) (*engagementv1.IsFavoriteResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	fav, err := h.repo.IsFavorite(ctx, userID(ctx), req.GetListingId())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.IsFavoriteResponse{Favorite: fav}, nil
}

func (h *EngagementHandler) ListFavorites(
	ctx context.Context, req *engagementv1.ListFavoritesRequest,
) (*engagementv1.ListFavoritesResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}
	ids, next, total, err := h.repo.ListFavorites(ctx, userID(ctx), cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.ListFavoritesResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

// ── Seller Follow / Feed (F1) ──

func (h *EngagementHandler) FollowSeller(
	ctx context.Context, req *engagementv1.FollowSellerRequest,
) (*engagementv1.FollowSellerResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if req.GetSellerId() == userID(ctx) {
		return nil, status.Error(codes.InvalidArgument, "cannot follow yourself")
	}
	if _, err := h.repo.Follow(ctx, userID(ctx), req.GetSellerId()); err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.FollowSellerResponse{}, nil
}

func (h *EngagementHandler) UnfollowSeller(
	ctx context.Context, req *engagementv1.UnfollowSellerRequest,
) (*engagementv1.UnfollowSellerResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if _, err := h.repo.Unfollow(ctx, userID(ctx), req.GetSellerId()); err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.UnfollowSellerResponse{}, nil
}

func (h *EngagementHandler) IsFollowing(
	ctx context.Context, req *engagementv1.IsFollowingRequest,
) (*engagementv1.IsFollowingResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	following, err := h.repo.IsFollowing(ctx, userID(ctx), req.GetSellerId())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.IsFollowingResponse{Following: following}, nil
}

func (h *EngagementHandler) ListFollowedSellers(
	ctx context.Context, req *engagementv1.ListFollowedSellersRequest,
) (*engagementv1.ListFollowedSellersResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}
	ids, next, total, err := h.repo.ListFollowedSellers(ctx, userID(ctx), cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.ListFollowedSellersResponse{
		SellerIds: ids,
		Page:      &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

// ListFollowedListings is the follow feed: listings from sellers the caller
// follows, self-scoped to the forwarded principal and keyset-paginated.
func (h *EngagementHandler) ListFollowedListings(
	ctx context.Context, req *engagementv1.ListFollowedListingsRequest,
) (*engagementv1.ListFollowedListingsResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}
	ids, next, total, err := h.repo.ListFollowedListings(ctx, userID(ctx), cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.ListFollowedListingsResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

// RecordView is public (anonymous views count too).
func (h *EngagementHandler) RecordView(
	ctx context.Context, req *engagementv1.RecordViewRequest,
) (*engagementv1.RecordViewResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	count, err := h.repo.RecordView(ctx, userID(ctx), req.GetListingId())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.RecordViewResponse{ViewCount: count}, nil
}

// GetRecentlyViewed returns the caller's recently-viewed listing ids, newest
// first. It is self-scoped to the gateway-forwarded principal (never a
// request-supplied id); anonymous callers get an empty list with no error,
// mirroring RecordView being public.
func (h *EngagementHandler) GetRecentlyViewed(
	ctx context.Context, req *engagementv1.GetRecentlyViewedRequest,
) (*engagementv1.GetRecentlyViewedResponse, error) {
	uid := userID(ctx)
	if uid == "" {
		return &engagementv1.GetRecentlyViewedResponse{Page: &commonv1.PageResponse{}}, nil
	}
	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}
	ids, next, total, err := h.repo.GetRecentlyViewed(ctx, uid, cursor, pageSize)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.GetRecentlyViewedResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

// GetListingStats is public.
func (h *EngagementHandler) GetListingStats(
	ctx context.Context, req *engagementv1.GetListingStatsRequest,
) (*engagementv1.GetListingStatsResponse, error) {
	s, err := h.repo.GetStats(ctx, req.GetListingId())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.GetListingStatsResponse{ViewCount: s.ViewCount, FavoriteCount: s.FavoriteCount}, nil
}

// ── Reviews & Ratings ──

func (h *EngagementHandler) CreateReview(
	ctx context.Context, req *engagementv1.CreateReviewRequest,
) (*engagementv1.CreateReviewResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetRating() < 1 || req.GetRating() > 5 {
		return nil, status.Error(codes.InvalidArgument, "rating must be between 1 and 5")
	}

	uid := userID(ctx)
	userName := "Người mua"
	if len(uid) > 6 {
		userName = "User " + uid[:6]
	}

	if h.reviewSvc == nil {
		return nil, status.Error(codes.Unimplemented, "reviews service unavailable")
	}

	rev, err := h.reviewSvc.CreateReview(ctx, req.GetListingId(), uid, userName, req.GetOrderId(), req.GetRating(), req.GetComment(), req.GetMediaUrls())
	if err != nil {
		if errors.Is(err, service.ErrInvalidRating) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "create review: %v", err)
	}

	return &engagementv1.CreateReviewResponse{
		Review: toWireReview(rev),
	}, nil
}

func (h *EngagementHandler) ListReviews(
	ctx context.Context, req *engagementv1.ListReviewsRequest,
) (*engagementv1.ListReviewsResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if h.reviewSvc == nil {
		return &engagementv1.ListReviewsResponse{}, nil
	}

	pageSize := int(req.GetPage().GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}

	revs, total, err := h.reviewSvc.ListReviews(ctx, req.GetListingId(), req.GetRatingFilter(), 1, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list reviews: %v", err)
	}

	wireRevs := make([]*engagementv1.Review, 0, len(revs))
	for _, r := range revs {
		wireRevs = append(wireRevs, toWireReview(r))
	}

	return &engagementv1.ListReviewsResponse{
		Reviews: wireRevs,
		Page: &commonv1.PageResponse{
			Total: total,
		},
	}, nil
}

func (h *EngagementHandler) GetListingRatingSummary(
	ctx context.Context, req *engagementv1.GetListingRatingSummaryRequest,
) (*engagementv1.GetListingRatingSummaryResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if h.reviewSvc == nil {
		return &engagementv1.GetListingRatingSummaryResponse{ListingId: req.GetListingId()}, nil
	}

	summary, err := h.reviewSvc.GetRatingSummary(ctx, req.GetListingId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get rating summary: %v", err)
	}

	return &engagementv1.GetListingRatingSummaryResponse{
		ListingId:     summary.ListingID,
		AverageRating: summary.AverageRating,
		ReviewCount:   summary.ReviewCount,
		Breakdown: &engagementv1.RatingBreakdown{
			Star_1: summary.Breakdown.Star1,
			Star_2: summary.Breakdown.Star2,
			Star_3: summary.Breakdown.Star3,
			Star_4: summary.Breakdown.Star4,
			Star_5: summary.Breakdown.Star5,
		},
	}, nil
}

// ── Product Q&A ──

func (h *EngagementHandler) AskQuestion(
	ctx context.Context, req *engagementv1.AskQuestionRequest,
) (*engagementv1.AskQuestionResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuestionText() == "" {
		return nil, status.Error(codes.InvalidArgument, "question_text is required")
	}
	if h.qaSvc == nil {
		return nil, status.Error(codes.Unimplemented, "qa service unavailable")
	}

	q, err := h.qaSvc.AskQuestion(ctx, req.GetListingId(), userID(ctx), req.GetQuestionText())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "ask question: %v", err)
	}

	return &engagementv1.AskQuestionResponse{
		Question: toWireQuestion(q),
	}, nil
}

func (h *EngagementHandler) AnswerQuestion(
	ctx context.Context, req *engagementv1.AnswerQuestionRequest,
) (*engagementv1.AnswerQuestionResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetQuestionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "question_id is required")
	}
	if req.GetAnswerText() == "" {
		return nil, status.Error(codes.InvalidArgument, "answer_text is required")
	}
	if h.qaSvc == nil {
		return nil, status.Error(codes.Unimplemented, "qa service unavailable")
	}

	ans, err := h.qaSvc.AnswerQuestion(ctx, req.GetQuestionId(), userID(ctx), req.GetAnswerText(), req.GetIsShopReply())
	if err != nil {
		if errors.Is(err, repository.ErrQuestionNotFound) {
			return nil, status.Error(codes.NotFound, "question not found")
		}
		return nil, status.Errorf(codes.Internal, "answer question: %v", err)
	}

	return &engagementv1.AnswerQuestionResponse{
		Answer: toWireAnswer(ans),
	}, nil
}

func (h *EngagementHandler) ListQuestionsByListing(
	ctx context.Context, req *engagementv1.ListQuestionsByListingRequest,
) (*engagementv1.ListQuestionsByListingResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if h.qaSvc == nil {
		return &engagementv1.ListQuestionsByListingResponse{}, nil
	}

	pageSize := int(req.GetPage().GetPageSize())
	if pageSize <= 0 {
		pageSize = 20
	}

	questions, total, err := h.qaSvc.ListQuestionsByListing(ctx, req.GetListingId(), 1, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list questions: %v", err)
	}

	wireQuestions := make([]*engagementv1.ProductQuestion, 0, len(questions))
	for _, q := range questions {
		wireQuestions = append(wireQuestions, toWireQuestion(q))
	}

	return &engagementv1.ListQuestionsByListingResponse{
		Questions: wireQuestions,
		Page: &commonv1.PageResponse{
			Total: total,
		},
	}, nil
}

// ── Disputes ──

func (h *EngagementHandler) CreateDispute(
	ctx context.Context, req *engagementv1.CreateDisputeRequest,
) (*engagementv1.CreateDisputeResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetDefendantId() == "" {
		return nil, status.Error(codes.InvalidArgument, "defendant_id is required")
	}
	if req.GetReason() == "" {
		return nil, status.Error(codes.InvalidArgument, "reason is required")
	}
	if h.disputeSvc == nil {
		return nil, status.Error(codes.Unimplemented, "dispute service unavailable")
	}

	claimantID := userID(ctx)
	disp, err := h.disputeSvc.CreateDispute(ctx, req.GetOrderId(), claimantID, req.GetDefendantId(), req.GetReason(), req.GetEvidenceUrls())
	if err != nil {
		if errors.Is(err, service.ErrSameClaimantAndDef) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "create dispute: %v", err)
	}

	return &engagementv1.CreateDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

func (h *EngagementHandler) GetDispute(
	ctx context.Context, req *engagementv1.GetDisputeRequest,
) (*engagementv1.GetDisputeResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetDisputeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "dispute_id is required")
	}
	if h.disputeSvc == nil {
		return nil, status.Error(codes.Unimplemented, "dispute service unavailable")
	}

	disp, err := h.disputeSvc.GetDispute(ctx, req.GetDisputeId())
	if err != nil {
		if errors.Is(err, repository.ErrDisputeNotFound) {
			return nil, status.Error(codes.NotFound, "dispute not found")
		}
		return nil, status.Errorf(codes.Internal, "get dispute: %v", err)
	}

	return &engagementv1.GetDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

func (h *EngagementHandler) ResolveDispute(
	ctx context.Context, req *engagementv1.ResolveDisputeRequest,
) (*engagementv1.ResolveDisputeResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetDisputeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "dispute_id is required")
	}
	if h.disputeSvc == nil {
		return nil, status.Error(codes.Unimplemented, "dispute service unavailable")
	}

	statusDomain := toDomainDisputeStatus(req.GetStatus())
	if statusDomain == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid dispute status")
	}

	disp, err := h.disputeSvc.ResolveDispute(ctx, req.GetDisputeId(), statusDomain, req.GetResolution())
	if err != nil {
		if errors.Is(err, repository.ErrDisputeNotFound) {
			return nil, status.Error(codes.NotFound, "dispute not found")
		}
		if errors.Is(err, service.ErrDisputeClosed) {
			return nil, status.Error(codes.FailedPrecondition, "dispute is already closed")
		}
		if errors.Is(err, service.ErrInvalidDisputeStatus) {
			return nil, status.Error(codes.InvalidArgument, "invalid dispute status")
		}
		return nil, status.Errorf(codes.Internal, "resolve dispute: %v", err)
	}

	return &engagementv1.ResolveDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

// ── Loyalty / Daily Check-in (F4) ──

// CheckIn records the caller's daily check-in (idempotent per calendar day),
// advancing the streak and awarding coins. Self-scoped to the forwarded
// principal; anonymous callers are denied by RequireScopes (no panic).
func (h *EngagementHandler) CheckIn(
	ctx context.Context, _ *engagementv1.CheckInRequest,
) (*engagementv1.CheckInResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	loy, earned, err := h.repo.CheckIn(ctx, userID(ctx), time.Now().UTC())
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &engagementv1.CheckInResponse{
		Streak:      loy.Streak,
		CoinsEarned: earned,
		CoinBalance: loy.CoinBalance,
	}, nil
}

// GetLoyalty returns the caller's loyalty snapshot (streak, coin balance, last
// check-in). Self-scoped to the forwarded principal.
func (h *EngagementHandler) GetLoyalty(
	ctx context.Context, _ *engagementv1.GetLoyaltyRequest,
) (*engagementv1.GetLoyaltyResponse, error) {
	if err := interceptor.RequireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	loy, err := h.repo.GetLoyalty(ctx, userID(ctx))
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	resp := &engagementv1.GetLoyaltyResponse{
		Streak:      loy.Streak,
		CoinBalance: loy.CoinBalance,
	}
	if !loy.LastCheckin.IsZero() {
		resp.LastCheckin = timestamppb.New(loy.LastCheckin)
	}
	return resp, nil
}

// ── Wire Mappers ──

func toWireReview(r repository.Review) *engagementv1.Review {
	return &engagementv1.Review{
		Id:               r.ID,
		ListingId:        r.ListingID,
		UserId:           r.UserID,
		UserName:         r.UserName,
		OrderId:          r.OrderID,
		Rating:           r.Rating,
		Comment:          r.Comment,
		MediaUrls:        r.MediaURLs,
		HelpfulCount:     r.HelpfulCount,
		VerifiedPurchase: r.VerifiedPurchase,
		CreatedAt:        timestamppb.New(r.CreatedAt),
	}
}

func toWireQuestion(q repository.ProductQuestion) *engagementv1.ProductQuestion {
	answers := make([]*engagementv1.ProductAnswer, 0, len(q.Answers))
	for _, a := range q.Answers {
		answers = append(answers, toWireAnswer(a))
	}
	return &engagementv1.ProductQuestion{
		Id:           q.ID,
		ListingId:    q.ListingID,
		UserId:       q.UserID,
		QuestionText: q.QuestionText,
		Answers:      answers,
		CreatedAt:    timestamppb.New(q.CreatedAt),
	}
}

func toWireAnswer(a repository.ProductAnswer) *engagementv1.ProductAnswer {
	return &engagementv1.ProductAnswer{
		Id:          a.ID,
		QuestionId:  a.QuestionID,
		UserId:      a.UserID,
		AnswerText:  a.AnswerText,
		IsShopReply: a.IsShopReply,
		CreatedAt:   timestamppb.New(a.CreatedAt),
	}
}

func toWireDispute(d repository.Dispute) *engagementv1.Dispute {
	return &engagementv1.Dispute{
		Id:           d.ID,
		OrderId:      d.OrderID,
		ClaimantId:   d.ClaimantID,
		DefendantId:  d.DefendantID,
		Reason:       d.Reason,
		EvidenceUrls: d.EvidenceURLs,
		Status:       toProtoDisputeStatus(d.Status),
		Resolution:   d.Resolution,
		CreatedAt:    timestamppb.New(d.CreatedAt),
		UpdatedAt:    timestamppb.New(d.UpdatedAt),
	}
}

func toDomainDisputeStatus(s engagementv1.DisputeStatus) repository.DisputeStatus {
	switch s {
	case engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN:
		return repository.DisputeStatusOpen
	case engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING:
		return repository.DisputeStatusInvestigating
	case engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED:
		return repository.DisputeStatusResolved
	case engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED:
		return repository.DisputeStatusRejected
	default:
		return ""
	}
}

func toProtoDisputeStatus(s repository.DisputeStatus) engagementv1.DisputeStatus {
	switch s {
	case repository.DisputeStatusOpen:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN
	case repository.DisputeStatusInvestigating:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING
	case repository.DisputeStatusResolved:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED
	case repository.DisputeStatusRejected:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED
	default:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_UNSPECIFIED
	}
}
