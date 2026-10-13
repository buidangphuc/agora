// Package rpc is the gRPC transport adapter: it implements the generated
// service servers, authorizes each call, maps between wire (protobuf) and domain
// types, and turns errors into gRPC statuses. Business rules live in
// internal/promotion.
package rpc

import (
	"google.golang.org/grpc"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
)

// Register attaches the four promotion services to srv.
func Register(srv grpc.ServiceRegistrar, v *VoucherHandler, f *FlashSaleHandler, s *SubscriptionHandler, a *SponsoredHandler) {
	promotionv1.RegisterVoucherServiceServer(srv, v)
	promotionv1.RegisterFlashSaleServiceServer(srv, f)
	promotionv1.RegisterSubscriptionServiceServer(srv, s)
	promotionv1.RegisterSponsoredServiceServer(srv, a)
}
