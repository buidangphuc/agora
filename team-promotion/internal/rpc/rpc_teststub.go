package rpc

import (
	"log/slog"

	"google.golang.org/grpc"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
	"github.com/buidangphuc/team-promotion/internal/subscription"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

type VoucherHandler struct {
	promotionv1.UnimplementedVoucherServiceServer
}

type FlashSaleHandler struct {
	promotionv1.UnimplementedFlashSaleServiceServer
}

type SubscriptionHandler struct {
	promotionv1.UnimplementedSubscriptionServiceServer
}

type SponsoredHandler struct {
	promotionv1.UnimplementedSponsoredServiceServer
}

func NewVoucherHandler(*voucher.Service, *slog.Logger) *VoucherHandler { return &VoucherHandler{} }

func NewFlashSaleHandler(*flashsale.Service, *slog.Logger) *FlashSaleHandler {
	return &FlashSaleHandler{}
}

func NewSubscriptionHandler(*subscription.Service, *slog.Logger) *SubscriptionHandler {
	return &SubscriptionHandler{}
}

func NewSponsoredHandler(*sponsored.Service, *slog.Logger) *SponsoredHandler {
	return &SponsoredHandler{}
}

func Register(srv *grpc.Server, v *VoucherHandler, f *FlashSaleHandler, s *SubscriptionHandler, a *SponsoredHandler) {
	promotionv1.RegisterVoucherServiceServer(srv, v)
	promotionv1.RegisterFlashSaleServiceServer(srv, f)
	promotionv1.RegisterSubscriptionServiceServer(srv, s)
	promotionv1.RegisterSponsoredServiceServer(srv, a)
}
