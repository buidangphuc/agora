package app

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/store"
)

func protoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func voucherProto(v store.Voucher) *promotionv1.Voucher {
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

func campaignProto(c store.FlashSaleCampaign) *promotionv1.FlashSaleCampaign {
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
