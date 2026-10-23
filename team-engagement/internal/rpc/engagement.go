package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

type OrderPartiesReader interface {
	GetOrderParties(ctx context.Context, orderID string) (buyerID, sellerID string, err error)
}

type ReturnReader interface {
	GetReturn(ctx context.Context, returnID string) (orderID string, status orderv1.ReturnStatus, canEscalate bool, err error)
}

type Handler struct {
	engagementv1.UnimplementedEngagementServiceServer
	repo          store.Repository
	reviewSvc     *engagement.ReviewService
	qaSvc         *engagement.QAService
	disputeSvc    *engagement.DisputeService
	collectionSvc *engagement.CollectionService
	reportSvc     *engagement.ReportService
	orders        OrderPartiesReader
	returns       ReturnReader
	logger        *slog.Logger
	errs          grpcx.ErrorTable
}

func NewHandler(
	repo store.Repository,
	reviewSvc *engagement.ReviewService,
	qaSvc *engagement.QAService,
	disputeSvc *engagement.DisputeService,
	collectionSvc *engagement.CollectionService,
	reportSvc *engagement.ReportService,
	orders OrderPartiesReader,
	returns ReturnReader,
	logger *slog.Logger,
) (*Handler, error) {
	if repo == nil || reviewSvc == nil || qaSvc == nil || disputeSvc == nil || collectionSvc == nil ||
		reportSvc == nil || orders == nil || returns == nil || logger == nil {
		return nil, errors.New("rpc: every dependency is required")
	}
	errs, err := grpcx.NewErrorTable(errorRules()...)
	if err != nil {
		return nil, err
	}
	return &Handler{
		repo:          repo,
		reviewSvc:     reviewSvc,
		qaSvc:         qaSvc,
		disputeSvc:    disputeSvc,
		collectionSvc: collectionSvc,
		reportSvc:     reportSvc,
		orders:        orders,
		returns:       returns,
		logger:        logger,
		errs:          errs,
	}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	engagementv1.RegisterEngagementServiceServer(srv, h)
}

func errorRules() []grpcx.Rule {
	return []grpcx.Rule{
		{Target: engagement.ErrOrderNotEligible, Code: codes.FailedPrecondition, Message: "order is not eligible for review"},
		{Target: engagement.ErrOrderUnavailable, Code: codes.Unavailable, Message: "order verification unavailable"},
		{Target: store.ErrDuplicateReview, Code: codes.AlreadyExists},
		{Target: store.ErrOpenDisputeExists, Code: codes.AlreadyExists},
		{Target: store.ErrInvalidCursor, Code: codes.InvalidArgument, Message: "invalid cursor"},
		{Target: store.ErrReviewNotFound, Code: codes.NotFound},
		{Target: store.ErrQuestionNotFound, Code: codes.NotFound},
		{Target: store.ErrCollectionNotFound, Code: codes.NotFound},
		{Target: store.ErrReportNotFound, Code: codes.NotFound},
		{Target: store.ErrDisputeNotFound, Code: codes.NotFound},
		{Target: upstream.ErrOrderNotFound, Code: codes.NotFound},
		{Target: upstream.ErrReturnNotFound, Code: codes.NotFound},
		{Target: engagement.ErrDisputeClosed, Code: codes.FailedPrecondition},
		{Target: engagement.ErrInvalidTransition, Code: codes.FailedPrecondition},
		{Target: store.ErrDisputeNotInState, Code: codes.FailedPrecondition},
		{Target: engagement.ErrReportClosed, Code: codes.FailedPrecondition},
		{Target: engagement.ErrOwnListing, Code: codes.FailedPrecondition},
		{Target: engagement.ErrInvalidDisputeStatus, Code: codes.InvalidArgument},
		{Target: engagement.ErrRulingRequired, Code: codes.InvalidArgument},
		{Target: engagement.ErrRulingNotAllowed, Code: codes.InvalidArgument},
		{Target: engagement.ErrSameClaimantAndDef, Code: codes.InvalidArgument},
		{Target: engagement.ErrInvalidRating, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyListing, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyReviewID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptySeller, Code: codes.InvalidArgument},
		{Target: engagement.ErrReasonTooLong, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyQuestionText, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyAnswerText, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyQuestionID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyUserID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyDisputeID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyOrderID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyClaimantID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyDefendantID, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyReason, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyCollectionName, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyCollectionID, Code: codes.InvalidArgument},
		{Target: engagement.ErrInvalidReportTarget, Code: codes.InvalidArgument},
		{Target: engagement.ErrInvalidReportReason, Code: codes.InvalidArgument},
		{Target: engagement.ErrReportNoteTooLong, Code: codes.InvalidArgument},
		{Target: engagement.ErrEmptyReportID, Code: codes.InvalidArgument},
		{Target: engagement.ErrInvalidReportStatus, Code: codes.InvalidArgument},
		{Target: engagement.ErrResolutionNoteRequired, Code: codes.InvalidArgument},
		{Target: engagement.ErrResolutionNoteTooLong, Code: codes.InvalidArgument},
		{Target: engagement.ErrReportCapReached, Code: codes.ResourceExhausted},
	}
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func requireScopes(ctx context.Context, scopes ...string) error {
	_, err := auth.RequireScopes(ctx, scopes...)
	return err
}

func isAdmin(ctx context.Context) bool {
	p, err := auth.RequireScopes(ctx, "admin")
	return err == nil && p.ID != ""
}

func userID(ctx context.Context) string {
	if p, ok := auth.FromContext(ctx); ok {
		return p.ID
	}
	return ""
}

func (h *Handler) AddFavorite(
	ctx context.Context, req *engagementv1.AddFavoriteRequest,
) (*engagementv1.AddFavoriteResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if _, err := h.repo.AddFavorite(ctx, userID(ctx), req.GetListingId()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.AddFavoriteResponse{}, nil
}

func (h *Handler) RemoveFavorite(
	ctx context.Context, req *engagementv1.RemoveFavoriteRequest,
) (*engagementv1.RemoveFavoriteResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if _, err := h.repo.RemoveFavorite(ctx, userID(ctx), req.GetListingId()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.RemoveFavoriteResponse{}, nil
}

func (h *Handler) IsFavorite(
	ctx context.Context, req *engagementv1.IsFavoriteRequest,
) (*engagementv1.IsFavoriteResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	fav, err := h.repo.IsFavorite(ctx, userID(ctx), req.GetListingId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.IsFavoriteResponse{Favorite: fav}, nil
}

func (h *Handler) ListFavorites(
	ctx context.Context, req *engagementv1.ListFavoritesRequest,
) (*engagementv1.ListFavoritesResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
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
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.ListFavoritesResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func (h *Handler) FollowSeller(
	ctx context.Context, req *engagementv1.FollowSellerRequest,
) (*engagementv1.FollowSellerResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if req.GetSellerId() == userID(ctx) {
		return nil, status.Error(codes.InvalidArgument, "cannot follow yourself")
	}
	if _, err := h.repo.Follow(ctx, userID(ctx), req.GetSellerId()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.FollowSellerResponse{}, nil
}

func (h *Handler) UnfollowSeller(
	ctx context.Context, req *engagementv1.UnfollowSellerRequest,
) (*engagementv1.UnfollowSellerResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if _, err := h.repo.Unfollow(ctx, userID(ctx), req.GetSellerId()); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.UnfollowSellerResponse{}, nil
}

func (h *Handler) IsFollowing(
	ctx context.Context, req *engagementv1.IsFollowingRequest,
) (*engagementv1.IsFollowingResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	following, err := h.repo.IsFollowing(ctx, userID(ctx), req.GetSellerId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.IsFollowingResponse{Following: following}, nil
}

func (h *Handler) ListFollowedSellers(
	ctx context.Context, req *engagementv1.ListFollowedSellersRequest,
) (*engagementv1.ListFollowedSellersResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
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
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.ListFollowedSellersResponse{
		SellerIds: ids,
		Page:      &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func (h *Handler) ListFollowedListings(
	ctx context.Context, req *engagementv1.ListFollowedListingsRequest,
) (*engagementv1.ListFollowedListingsResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
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
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.ListFollowedListingsResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func (h *Handler) RecordView(
	ctx context.Context, req *engagementv1.RecordViewRequest,
) (*engagementv1.RecordViewResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	count, err := h.repo.RecordView(ctx, userID(ctx), req.GetListingId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.RecordViewResponse{ViewCount: count}, nil
}

func (h *Handler) GetRecentlyViewed(
	ctx context.Context, req *engagementv1.GetRecentlyViewedRequest,
) (*engagementv1.GetRecentlyViewedResponse, error) {
	p, ok := auth.FromContext(ctx)
	if !ok || p.Anonymous() {
		return &engagementv1.GetRecentlyViewedResponse{Page: &commonv1.PageResponse{}}, nil
	}
	var cursor string
	var pageSize int32
	if p := req.GetPage(); p != nil {
		cursor = p.GetCursor()
		pageSize = p.GetPageSize()
	}
	ids, next, total, err := h.repo.GetRecentlyViewed(ctx, p.ID, cursor, pageSize)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.GetRecentlyViewedResponse{
		ListingIds: ids,
		Page:       &commonv1.PageResponse{NextCursor: next, Total: total},
	}, nil
}

func (h *Handler) GetListingStats(
	ctx context.Context, req *engagementv1.GetListingStatsRequest,
) (*engagementv1.GetListingStatsResponse, error) {
	s, err := h.repo.GetStats(ctx, req.GetListingId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.GetListingStatsResponse{ViewCount: s.ViewCount, FavoriteCount: s.FavoriteCount}, nil
}

func (h *Handler) CreateReview(
	ctx context.Context, req *engagementv1.CreateReviewRequest,
) (*engagementv1.CreateReviewResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
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

	rev, err := h.reviewSvc.CreateReview(ctx, req.GetListingId(), uid, userName, req.GetOrderId(), req.GetRating(), req.GetComment(), req.GetMediaUrls())
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	return &engagementv1.CreateReviewResponse{
		Review: toWireReview(rev),
	}, nil
}

func (h *Handler) ListReviews(
	ctx context.Context, req *engagementv1.ListReviewsRequest,
) (*engagementv1.ListReviewsResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	offset := 0
	if c := req.GetPage().GetCursor(); c != "" {
		n, err := strconv.Atoi(c)
		if err != nil || n < 0 {
			return nil, status.Error(codes.InvalidArgument, "invalid page cursor")
		}
		offset = n
	}
	pageSize := pagination.Clamp(int(req.GetPage().GetPageSize()), engagement.DefaultReviewPageSize, engagement.MaxReviewPageSize)

	revs, total, err := h.reviewSvc.ListReviews(ctx, req.GetListingId(), req.GetRatingFilter(), offset, pageSize)
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	wireRevs := make([]*engagementv1.Review, 0, len(revs))
	for _, r := range revs {
		wireRevs = append(wireRevs, toWireReview(r))
	}

	nextCursor := ""
	if next := offset + len(revs); int64(next) < total && len(revs) > 0 {
		nextCursor = strconv.Itoa(next)
	}

	return &engagementv1.ListReviewsResponse{
		Reviews: wireRevs,
		Page: &commonv1.PageResponse{
			NextCursor: nextCursor,
			Total:      total,
		},
	}, nil
}

func (h *Handler) GetListingRatingSummary(
	ctx context.Context, req *engagementv1.GetListingRatingSummaryRequest,
) (*engagementv1.GetListingRatingSummaryResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	summary, err := h.reviewSvc.GetRatingSummary(ctx, req.GetListingId())
	if err != nil {
		return nil, h.fail(ctx, err)
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

func (h *Handler) AskQuestion(
	ctx context.Context, req *engagementv1.AskQuestionRequest,
) (*engagementv1.AskQuestionResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	if req.GetQuestionText() == "" {
		return nil, status.Error(codes.InvalidArgument, "question_text is required")
	}

	q, err := h.qaSvc.AskQuestion(ctx, req.GetListingId(), userID(ctx), req.GetQuestionText())
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	return &engagementv1.AskQuestionResponse{
		Question: toWireQuestion(q),
	}, nil
}

func (h *Handler) AnswerQuestion(
	ctx context.Context, req *engagementv1.AnswerQuestionRequest,
) (*engagementv1.AnswerQuestionResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	if req.GetQuestionId() == "" {
		return nil, status.Error(codes.InvalidArgument, "question_id is required")
	}
	if req.GetAnswerText() == "" {
		return nil, status.Error(codes.InvalidArgument, "answer_text is required")
	}

	isShopReply := false
	if req.GetIsShopReply() {
		q, err := h.qaSvc.GetQuestion(ctx, req.GetQuestionId())
		if err != nil {
			return nil, h.fail(ctx, err)
		}
		owner, found, err := h.repo.ListingSeller(ctx, q.ListingID)
		if err != nil {
			return nil, h.fail(ctx, err)
		}
		isShopReply = found && owner == userID(ctx)
	}

	ans, err := h.qaSvc.AnswerQuestion(ctx, req.GetQuestionId(), userID(ctx), req.GetAnswerText(), isShopReply)
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	return &engagementv1.AnswerQuestionResponse{
		Answer: toWireAnswer(ans),
	}, nil
}

func (h *Handler) ListQuestionsByListing(
	ctx context.Context, req *engagementv1.ListQuestionsByListingRequest,
) (*engagementv1.ListQuestionsByListingResponse, error) {
	if req.GetListingId() == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}

	pageSize := pagination.Clamp(int(req.GetPage().GetPageSize()), 20, 100)

	questions, total, err := h.qaSvc.ListQuestionsByListing(ctx, req.GetListingId(), 1, pageSize)
	if err != nil {
		return nil, h.fail(ctx, err)
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

func (h *Handler) CreateDispute(
	ctx context.Context, req *engagementv1.CreateDisputeRequest,
) (*engagementv1.CreateDisputeResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
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

	claimantID := userID(ctx)

	buyerID, sellerID, err := h.orders.GetOrderParties(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, upstream.ErrOrderNotFound) {
			return nil, status.Error(codes.NotFound, "order not found")
		}
		return nil, status.Error(codes.Unavailable, "order verification unavailable")
	}
	if buyerID == "" || buyerID != claimantID {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	if sellerID == "" || sellerID != req.GetDefendantId() {
		if req.GetReturnId() != "" {
			return nil, status.Error(codes.InvalidArgument, "defendant must be the order's seller")
		}
		return nil, status.Error(codes.PermissionDenied, "defendant must be the order's seller")
	}

	if req.GetReturnId() != "" {
		retOrderID, _, canEscalate, rerr := h.returns.GetReturn(ctx, req.GetReturnId())
		if rerr != nil {
			if errors.Is(rerr, upstream.ErrReturnNotFound) {
				return nil, status.Error(codes.NotFound, "return not found")
			}
			return nil, status.Error(codes.Unavailable, "return verification unavailable")
		}
		if retOrderID != req.GetOrderId() {
			return nil, status.Error(codes.NotFound, "return not found")
		}
		if !canEscalate {
			return nil, status.Error(codes.FailedPrecondition, "return cannot be escalated")
		}
	}

	disp, err := h.disputeSvc.CreateDispute(ctx, req.GetOrderId(), claimantID, req.GetDefendantId(), req.GetReturnId(), req.GetReason(), req.GetEvidenceUrls())
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	return &engagementv1.CreateDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

func (h *Handler) GetDispute(
	ctx context.Context, req *engagementv1.GetDisputeRequest,
) (*engagementv1.GetDisputeResponse, error) {
	caller, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetDisputeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "dispute_id is required")
	}

	disp, err := h.disputeSvc.GetDispute(ctx, req.GetDisputeId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	if caller.ID != disp.ClaimantID && caller.ID != disp.DefendantID && !isAdmin(ctx) {
		return nil, status.Error(codes.NotFound, "dispute not found")
	}

	return &engagementv1.GetDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

func (h *Handler) ResolveDispute(
	ctx context.Context, req *engagementv1.ResolveDisputeRequest,
) (*engagementv1.ResolveDisputeResponse, error) {
	if err := requireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	if req.GetDisputeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "dispute_id is required")
	}

	statusDomain := toDomainDisputeStatus(req.GetStatus())
	if statusDomain == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid dispute status")
	}

	disp, err := h.disputeSvc.ResolveDispute(ctx, userID(ctx), req.GetDisputeId(), statusDomain, req.GetRuling(), req.GetResolution())
	if err != nil {
		return nil, h.fail(ctx, err)
	}

	return &engagementv1.ResolveDisputeResponse{
		Dispute: toWireDispute(disp),
	}, nil
}

func (h *Handler) ListOrderDisputes(
	ctx context.Context, req *engagementv1.ListOrderDisputesRequest,
) (*engagementv1.ListOrderDisputesResponse, error) {
	caller, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if !isAdmin(ctx) {
		buyerID, sellerID, oerr := h.orders.GetOrderParties(ctx, req.GetOrderId())
		if oerr != nil && !errors.Is(oerr, upstream.ErrOrderNotFound) {
			return nil, status.Error(codes.Unavailable, "order verification unavailable")
		}
		if oerr != nil || caller.ID == "" || (caller.ID != buyerID && caller.ID != sellerID) {
			return nil, status.Error(codes.PermissionDenied, "not a party to this order")
		}
	}
	ds, err := h.disputeSvc.ListOrderDisputes(ctx, req.GetOrderId())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*engagementv1.Dispute, 0, len(ds))
	for _, d := range ds {
		out = append(out, toWireDispute(d))
	}
	return &engagementv1.ListOrderDisputesResponse{Disputes: out}, nil
}

func (h *Handler) ListDisputes(
	ctx context.Context, req *engagementv1.ListDisputesRequest,
) (*engagementv1.ListDisputesResponse, error) {
	if err := requireScopes(ctx, "admin"); err != nil {
		return nil, err
	}
	ds, next, err := h.disputeSvc.ListDisputes(ctx, toDomainDisputeStatus(req.GetStatus()), int(req.GetPage().GetPageSize()), req.GetPage().GetCursor())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*engagementv1.Dispute, 0, len(ds))
	for _, d := range ds {
		out = append(out, toWireDispute(d))
	}
	return &engagementv1.ListDisputesResponse{Disputes: out, Page: &commonv1.PageResponse{NextCursor: next}}, nil
}

func (h *Handler) CheckIn(
	ctx context.Context, _ *engagementv1.CheckInRequest,
) (*engagementv1.CheckInResponse, error) {
	if err := requireScopes(ctx, "engagement:write"); err != nil {
		return nil, err
	}
	loy, earned, err := h.repo.CheckIn(ctx, userID(ctx), time.Now().UTC())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &engagementv1.CheckInResponse{
		Streak:      loy.Streak,
		CoinsEarned: earned,
		CoinBalance: loy.CoinBalance,
	}, nil
}

func (h *Handler) GetLoyalty(
	ctx context.Context, _ *engagementv1.GetLoyaltyRequest,
) (*engagementv1.GetLoyaltyResponse, error) {
	if err := requireScopes(ctx, "engagement:read"); err != nil {
		return nil, err
	}
	loy, err := h.repo.GetLoyalty(ctx, userID(ctx))
	if err != nil {
		return nil, h.fail(ctx, err)
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

func toWireReview(r store.Review) *engagementv1.Review {
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

func toWireQuestion(q store.ProductQuestion) *engagementv1.ProductQuestion {
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

func toWireAnswer(a store.ProductAnswer) *engagementv1.ProductAnswer {
	return &engagementv1.ProductAnswer{
		Id:          a.ID,
		QuestionId:  a.QuestionID,
		UserId:      a.UserID,
		AnswerText:  a.AnswerText,
		IsShopReply: a.IsShopReply,
		CreatedAt:   timestamppb.New(a.CreatedAt),
	}
}

func toWireDispute(d store.Dispute) *engagementv1.Dispute {
	return &engagementv1.Dispute{
		Id:           d.ID,
		OrderId:      d.OrderID,
		ClaimantId:   d.ClaimantID,
		DefendantId:  d.DefendantID,
		Reason:       d.Reason,
		EvidenceUrls: d.EvidenceURLs,
		Status:       toProtoDisputeStatus(d.Status),
		Resolution:   d.Resolution,
		ReturnId:     d.ReturnID,
		Ruling:       d.Ruling,
		CreatedAt:    timestamppb.New(d.CreatedAt),
		UpdatedAt:    timestamppb.New(d.UpdatedAt),
	}
}

func toDomainDisputeStatus(s engagementv1.DisputeStatus) store.DisputeStatus {
	switch s {
	case engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN:
		return store.DisputeStatusOpen
	case engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING:
		return store.DisputeStatusInvestigating
	case engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED:
		return store.DisputeStatusResolved
	case engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED:
		return store.DisputeStatusRejected
	default:
		return ""
	}
}

func toProtoDisputeStatus(s store.DisputeStatus) engagementv1.DisputeStatus {
	switch s {
	case store.DisputeStatusOpen:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN
	case store.DisputeStatusInvestigating:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_INVESTIGATING
	case store.DisputeStatusResolved:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED
	case store.DisputeStatusRejected:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_REJECTED
	default:
		return engagementv1.DisputeStatus_DISPUTE_STATUS_UNSPECIFIED
	}
}
