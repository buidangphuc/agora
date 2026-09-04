package edge

import (
	"context"

	"connectrpc.com/connect"

	engagementv1 "github.com/buidangphuc/team-gateway/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/engagement/v1/engagementv1connect"
)

// EngagementForwarder forwards favorites + view/stats + review calls to team-engagement.
type EngagementForwarder struct {
	engagementv1connect.UnimplementedEngagementServiceHandler
	client engagementv1.EngagementServiceClient
	edge   *Edge
}

func NewEngagementForwarder(client engagementv1.EngagementServiceClient, edge *Edge) *EngagementForwarder {
	return &EngagementForwarder{client: client, edge: edge}
}

func (f *EngagementForwarder) AddFavorite(
	ctx context.Context, req *connect.Request[engagementv1.AddFavoriteRequest],
) (*connect.Response[engagementv1.AddFavoriteResponse], error) {
	var out *engagementv1.AddFavoriteResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.AddFavorite(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) RemoveFavorite(
	ctx context.Context, req *connect.Request[engagementv1.RemoveFavoriteRequest],
) (*connect.Response[engagementv1.RemoveFavoriteResponse], error) {
	var out *engagementv1.RemoveFavoriteResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RemoveFavorite(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) IsFavorite(
	ctx context.Context, req *connect.Request[engagementv1.IsFavoriteRequest],
) (*connect.Response[engagementv1.IsFavoriteResponse], error) {
	var out *engagementv1.IsFavoriteResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.IsFavorite(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListFavorites(
	ctx context.Context, req *connect.Request[engagementv1.ListFavoritesRequest],
) (*connect.Response[engagementv1.ListFavoritesResponse], error) {
	var out *engagementv1.ListFavoritesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListFavorites(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// RecordView increments a counter — treat as a write (no retry to avoid double count).
func (f *EngagementForwarder) RecordView(
	ctx context.Context, req *connect.Request[engagementv1.RecordViewRequest],
) (*connect.Response[engagementv1.RecordViewResponse], error) {
	var out *engagementv1.RecordViewResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RecordView(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetRecentlyViewed(
	ctx context.Context, req *connect.Request[engagementv1.GetRecentlyViewedRequest],
) (*connect.Response[engagementv1.GetRecentlyViewedResponse], error) {
	var out *engagementv1.GetRecentlyViewedResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetRecentlyViewed(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetListingStats(
	ctx context.Context, req *connect.Request[engagementv1.GetListingStatsRequest],
) (*connect.Response[engagementv1.GetListingStatsResponse], error) {
	var out *engagementv1.GetListingStatsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetListingStats(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Reviews & Ratings ──

func (f *EngagementForwarder) CreateReview(
	ctx context.Context, req *connect.Request[engagementv1.CreateReviewRequest],
) (*connect.Response[engagementv1.CreateReviewResponse], error) {
	var out *engagementv1.CreateReviewResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateReview(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListReviews(
	ctx context.Context, req *connect.Request[engagementv1.ListReviewsRequest],
) (*connect.Response[engagementv1.ListReviewsResponse], error) {
	var out *engagementv1.ListReviewsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListReviews(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetListingRatingSummary(
	ctx context.Context, req *connect.Request[engagementv1.GetListingRatingSummaryRequest],
) (*connect.Response[engagementv1.GetListingRatingSummaryResponse], error) {
	var out *engagementv1.GetListingRatingSummaryResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetListingRatingSummary(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) MarkReviewHelpful(
	ctx context.Context, req *connect.Request[engagementv1.MarkReviewHelpfulRequest],
) (*connect.Response[engagementv1.MarkReviewHelpfulResponse], error) {
	var out *engagementv1.MarkReviewHelpfulResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.MarkReviewHelpful(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetShopRatingSummary(
	ctx context.Context, req *connect.Request[engagementv1.GetShopRatingSummaryRequest],
) (*connect.Response[engagementv1.GetShopRatingSummaryResponse], error) {
	var out *engagementv1.GetShopRatingSummaryResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetShopRatingSummary(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Wishlist Collections ──

func (f *EngagementForwarder) CreateCollection(
	ctx context.Context, req *connect.Request[engagementv1.CreateCollectionRequest],
) (*connect.Response[engagementv1.CreateCollectionResponse], error) {
	var out *engagementv1.CreateCollectionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateCollection(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListCollections(
	ctx context.Context, req *connect.Request[engagementv1.ListCollectionsRequest],
) (*connect.Response[engagementv1.ListCollectionsResponse], error) {
	var out *engagementv1.ListCollectionsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListCollections(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) AddToCollection(
	ctx context.Context, req *connect.Request[engagementv1.AddToCollectionRequest],
) (*connect.Response[engagementv1.AddToCollectionResponse], error) {
	var out *engagementv1.AddToCollectionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.AddToCollection(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) RemoveFromCollection(
	ctx context.Context, req *connect.Request[engagementv1.RemoveFromCollectionRequest],
) (*connect.Response[engagementv1.RemoveFromCollectionResponse], error) {
	var out *engagementv1.RemoveFromCollectionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RemoveFromCollection(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListCollectionItems(
	ctx context.Context, req *connect.Request[engagementv1.ListCollectionItemsRequest],
) (*connect.Response[engagementv1.ListCollectionItemsResponse], error) {
	var out *engagementv1.ListCollectionItemsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListCollectionItems(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Product Q&A ──

func (f *EngagementForwarder) AskQuestion(
	ctx context.Context, req *connect.Request[engagementv1.AskQuestionRequest],
) (*connect.Response[engagementv1.AskQuestionResponse], error) {
	var out *engagementv1.AskQuestionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.AskQuestion(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) AnswerQuestion(
	ctx context.Context, req *connect.Request[engagementv1.AnswerQuestionRequest],
) (*connect.Response[engagementv1.AnswerQuestionResponse], error) {
	var out *engagementv1.AnswerQuestionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.AnswerQuestion(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListQuestionsByListing(
	ctx context.Context, req *connect.Request[engagementv1.ListQuestionsByListingRequest],
) (*connect.Response[engagementv1.ListQuestionsByListingResponse], error) {
	var out *engagementv1.ListQuestionsByListingResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListQuestionsByListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Disputes ──

func (f *EngagementForwarder) CreateDispute(
	ctx context.Context, req *connect.Request[engagementv1.CreateDisputeRequest],
) (*connect.Response[engagementv1.CreateDisputeResponse], error) {
	var out *engagementv1.CreateDisputeResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateDispute(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetDispute(
	ctx context.Context, req *connect.Request[engagementv1.GetDisputeRequest],
) (*connect.Response[engagementv1.GetDisputeResponse], error) {
	var out *engagementv1.GetDisputeResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetDispute(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ResolveDispute(
	ctx context.Context, req *connect.Request[engagementv1.ResolveDisputeRequest],
) (*connect.Response[engagementv1.ResolveDisputeResponse], error) {
	var out *engagementv1.ResolveDisputeResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ResolveDispute(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Seller Follows ──

func (f *EngagementForwarder) FollowSeller(
	ctx context.Context, req *connect.Request[engagementv1.FollowSellerRequest],
) (*connect.Response[engagementv1.FollowSellerResponse], error) {
	var out *engagementv1.FollowSellerResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.FollowSeller(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) UnfollowSeller(
	ctx context.Context, req *connect.Request[engagementv1.UnfollowSellerRequest],
) (*connect.Response[engagementv1.UnfollowSellerResponse], error) {
	var out *engagementv1.UnfollowSellerResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UnfollowSeller(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListFollowedSellers(
	ctx context.Context, req *connect.Request[engagementv1.ListFollowedSellersRequest],
) (*connect.Response[engagementv1.ListFollowedSellersResponse], error) {
	var out *engagementv1.ListFollowedSellersResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListFollowedSellers(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) IsFollowing(
	ctx context.Context, req *connect.Request[engagementv1.IsFollowingRequest],
) (*connect.Response[engagementv1.IsFollowingResponse], error) {
	var out *engagementv1.IsFollowingResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.IsFollowing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) ListFollowedListings(
	ctx context.Context, req *connect.Request[engagementv1.ListFollowedListingsRequest],
) (*connect.Response[engagementv1.ListFollowedListingsResponse], error) {
	var out *engagementv1.ListFollowedListingsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListFollowedListings(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Loyalty / Check-in ──

func (f *EngagementForwarder) CheckIn(
	ctx context.Context, req *connect.Request[engagementv1.CheckInRequest],
) (*connect.Response[engagementv1.CheckInResponse], error) {
	var out *engagementv1.CheckInResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CheckIn(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *EngagementForwarder) GetLoyalty(
	ctx context.Context, req *connect.Request[engagementv1.GetLoyaltyRequest],
) (*connect.Response[engagementv1.GetLoyaltyResponse], error) {
	var out *engagementv1.GetLoyaltyResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetLoyalty(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
