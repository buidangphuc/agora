package rpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/subscription"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

const (
	scopeAdmin        = "admin"
	scopeListingWrite = "listing.write"
	scopePromoReserve = "promotion.reserve"
)

var errTable = mustErrorTable()

func mustErrorTable() grpcx.ErrorTable {
	t, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: voucher.ErrInvalidVoucher, Code: codes.InvalidArgument, Message: "invalid voucher"},
		grpcx.Rule{Target: store.ErrVoucherNotFound, Code: codes.NotFound, Message: "voucher not found"},
		grpcx.Rule{Target: flashsale.ErrInvalidCampaign, Code: codes.InvalidArgument, Message: "invalid campaign"},
		grpcx.Rule{Target: store.ErrCampaignNotFound, Code: codes.NotFound, Message: "campaign not found"},
		grpcx.Rule{Target: sponsored.ErrInvalidAdCampaign, Code: codes.InvalidArgument, Message: "invalid ad campaign"},
		grpcx.Rule{Target: subscription.ErrInvalidSubscription, Code: codes.InvalidArgument, Message: "invalid subscription"},
		grpcx.Rule{Target: store.ErrPlanNotFound, Code: codes.NotFound, Message: "plan not found"},
		grpcx.Rule{Target: listing.ErrNotFound, Code: codes.InvalidArgument},
		grpcx.Rule{Target: listing.ErrNotOwner, Code: codes.PermissionDenied},
		grpcx.Rule{Target: listing.ErrLookupFailed, Code: codes.Unavailable},
	)
	if err != nil {
		panic(err)
	}
	return t
}

func fail(ctx context.Context, logger *slog.Logger, err error) error {
	return grpcx.Status(ctx, logger, err, errTable)
}

func orDefault(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func requireAdmin(ctx context.Context) (auth.Principal, error) {
	return auth.RequireScopes(ctx, scopeAdmin)
}

func requireSeller(ctx context.Context) (auth.Principal, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return auth.Principal{}, err
	}
	if !p.HasScope(scopeAdmin) && !p.HasScope(scopeListingWrite) {
		return auth.Principal{}, status.Error(codes.PermissionDenied, "seller or admin role required")
	}
	return p, nil
}

func requireService(ctx context.Context, scope string) (auth.Principal, error) {
	p, err := auth.RequireScopes(ctx, scope)
	if err != nil {
		return auth.Principal{}, err
	}
	if p.Type != auth.TypeService {
		return auth.Principal{}, status.Error(codes.PermissionDenied, "service principal required")
	}
	return p, nil
}

func pageResponse(next string) *commonv1.PageResponse {
	return &commonv1.PageResponse{NextCursor: next}
}

func protoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func timeFromProto(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

func voucherToProto(v store.Voucher) *promotionv1.Voucher {
	return &promotionv1.Voucher{
		Id:            v.ID,
		Code:          v.Code,
		Scope:         promotionv1.VoucherScope(v.Scope),
		SellerId:      v.SellerID,
		DiscountType:  promotionv1.DiscountType(v.DiscountType),
		DiscountValue: v.DiscountValue,
		MinSpend:      v.MinSpend,
		MaxDiscount:   v.MaxDiscount,
		Quota:         v.Quota,
		Used:          v.Used,
		StartsAt:      protoTime(v.StartsAt),
		EndsAt:        protoTime(v.EndsAt),
	}
}

func campaignToProto(c store.FlashSaleCampaign) *promotionv1.FlashSaleCampaign {
	return &promotionv1.FlashSaleCampaign{
		Id:        c.ID,
		ListingId: c.ListingID,
		VariantId: c.VariantID,
		SalePrice: c.SalePrice,
		StockCap:  c.StockCap,
		StockSold: c.StockSold,
		StartsAt:  protoTime(c.StartsAt),
		EndsAt:    protoTime(c.EndsAt),
	}
}

func adCampaignToProto(c store.AdCampaign) *promotionv1.AdCampaign {
	return &promotionv1.AdCampaign{
		Id:        c.ID,
		SellerId:  c.SellerID,
		ListingId: c.ListingID,
		Budget:    c.Budget,
		Bid:       c.Bid,
		Status:    promotionv1.AdCampaignStatus(c.Status),
		CreatedAt: protoTime(c.CreatedAt),
	}
}

func planToProto(p store.Plan) *promotionv1.Plan {
	return &promotionv1.Plan{
		Id:       p.ID,
		Name:     promotionv1.PlanTier(p.Tier),
		Price:    p.Price,
		Features: p.Features,
	}
}

func subscriptionToProto(s store.SellerSubscription) *promotionv1.SellerSubscription {
	return &promotionv1.SellerSubscription{
		Id:       s.ID,
		SellerId: s.SellerID,
		PlanId:   s.PlanID,
		Tier:     promotionv1.PlanTier(s.Tier),
		Status:   subscriptionStatusToProto(s.Status),
		StartsAt: protoTime(s.CreatedAt),
	}
}

func subscriptionStatusToProto(s string) promotionv1.SubscriptionStatus {
	switch s {
	case store.SubscriptionStatusActive:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_ACTIVE
	case store.SubscriptionStatusExpired:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_EXPIRED
	case store.SubscriptionStatusCancelled:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_CANCELLED
	default:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_UNSPECIFIED
	}
}

func Register(srv *grpc.Server, v *VoucherHandler, f *FlashSaleHandler, s *SubscriptionHandler, a *SponsoredHandler) {
	promotionv1.RegisterVoucherServiceServer(srv, v)
	promotionv1.RegisterFlashSaleServiceServer(srv, f)
	promotionv1.RegisterSubscriptionServiceServer(srv, s)
	promotionv1.RegisterSponsoredServiceServer(srv, a)
}
