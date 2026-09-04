package service

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/repository"
)

// VoucherToProto maps a persistence-layer voucher to the wire type. Exported so
// both the handler (responses) and the service (events) share one mapping.
func VoucherToProto(v repository.Voucher) *promotionv1.Voucher {
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

// CampaignToProto maps a persistence-layer campaign to the wire type.
func CampaignToProto(c repository.FlashSaleCampaign) *promotionv1.FlashSaleCampaign {
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

// AdCampaignToProto maps a persistence-layer sponsored campaign to the wire type.
func AdCampaignToProto(c repository.AdCampaign) *promotionv1.AdCampaign {
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

// PlanToProto maps a persistence-layer plan to the wire type. Limits are surfaced
// separately by GetEntitlements, not on the Plan message.
func PlanToProto(p repository.Plan) *promotionv1.Plan {
	return &promotionv1.Plan{
		Id:       p.ID,
		Name:     promotionv1.PlanTier(p.Tier),
		Price:    p.Price,
		Features: p.Features,
	}
}

// SubscriptionToProto maps a persistence-layer seller subscription to the wire
// type. created_at is surfaced as starts_at; ends_at is left nil (mock, no expiry).
func SubscriptionToProto(s repository.SellerSubscription) *promotionv1.SellerSubscription {
	return &promotionv1.SellerSubscription{
		Id:       s.ID,
		SellerId: s.SellerID,
		PlanId:   s.PlanID,
		Tier:     promotionv1.PlanTier(s.Tier),
		Status:   subscriptionStatusToProto(s.Status),
		StartsAt: protoTime(s.CreatedAt),
	}
}

// subscriptionStatusToProto maps the stored status text to the wire enum.
func subscriptionStatusToProto(status string) promotionv1.SubscriptionStatus {
	switch status {
	case repository.SubscriptionStatusActive:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_ACTIVE
	case repository.SubscriptionStatusExpired:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_EXPIRED
	case repository.SubscriptionStatusCancelled:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_CANCELLED
	default:
		return promotionv1.SubscriptionStatus_SUBSCRIPTION_STATUS_UNSPECIFIED
	}
}

// protoTime maps a Go time to a protobuf timestamp, leaving a zero time as nil.
func protoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// TimeFromProto maps a protobuf timestamp to a Go time; a nil timestamp becomes
// the zero time (NOT the Unix epoch — AsTime() on nil would yield 1970, which the
// time-window checks must not treat as a real bound).
func TimeFromProto(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
