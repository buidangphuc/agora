package events

import (
	"context"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

const (
	TypeVoucherChanged   = "platform.promotion.v1.VoucherChanged"
	TypeFlashSaleChanged = "platform.promotion.v1.FlashSaleChanged"
)

func Voucher(_ context.Context, _ string, _ *promotionv1.Voucher) (outbox.Row, error) {
	return outbox.Row{}, nil
}

func FlashSale(_ context.Context, _ string, _ *promotionv1.FlashSaleCampaign) (outbox.Row, error) {
	return outbox.Row{}, nil
}
