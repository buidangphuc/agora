package upstream

import (
	"context"

	"google.golang.org/grpc"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

// servicePromotionClient marks exactly the three saga RPCs AsService with the
// scope promotion.reserve (team-promotion gates Commit/ReleaseReservation, and the
// service path of ValidateAndReserve, on it). Every other VoucherService RPC is
// left untouched so a forwarded buyer principal is never replaced.
type servicePromotionClient struct {
	promotionv1.VoucherServiceClient
}

// NewServicePromotionClient wraps inner so ValidateAndReserve, CommitReservation
// and ReleaseReservation are sent as team-order's service principal with scope
// promotion.reserve, in a buyer's request, the PaymentSettled consumer and
// compensation alike.
func NewServicePromotionClient(inner promotionv1.VoucherServiceClient) promotionv1.VoucherServiceClient {
	if inner == nil {
		return nil
	}
	if already, ok := inner.(servicePromotionClient); ok {
		return already
	}
	return servicePromotionClient{VoucherServiceClient: inner}
}

func (c servicePromotionClient) ValidateAndReserve(ctx context.Context, req *promotionv1.ValidateAndReserveRequest, opts ...grpc.CallOption) (*promotionv1.ValidateAndReserveResponse, error) {
	return c.VoucherServiceClient.ValidateAndReserve(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}

func (c servicePromotionClient) CommitReservation(ctx context.Context, req *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return c.VoucherServiceClient.CommitReservation(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}

func (c servicePromotionClient) ReleaseReservation(ctx context.Context, req *promotionv1.ReleaseReservationRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	return c.VoucherServiceClient.ReleaseReservation(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}

// serviceFlashClient marks the three flash stock-hold RPCs AsService with scope
// promotion.reserve. Reads such as GetActiveFlashSale keep the forwarded principal.
type serviceFlashClient struct {
	promotionv1.FlashSaleServiceClient
}

// NewServiceFlashClient wraps inner so ReserveFlashSaleStock, CommitFlashSaleStock
// and ReleaseFlashSaleStock are sent as team-order's service principal.
func NewServiceFlashClient(inner promotionv1.FlashSaleServiceClient) promotionv1.FlashSaleServiceClient {
	if inner == nil {
		return nil
	}
	if already, ok := inner.(serviceFlashClient); ok {
		return already
	}
	return serviceFlashClient{FlashSaleServiceClient: inner}
}

func (c serviceFlashClient) ReserveFlashSaleStock(ctx context.Context, req *promotionv1.ReserveFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.ReserveFlashSaleStockResponse, error) {
	return c.FlashSaleServiceClient.ReserveFlashSaleStock(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}

func (c serviceFlashClient) CommitFlashSaleStock(ctx context.Context, req *promotionv1.CommitFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	return c.FlashSaleServiceClient.CommitFlashSaleStock(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}

func (c serviceFlashClient) ReleaseFlashSaleStock(ctx context.Context, req *promotionv1.ReleaseFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	return c.FlashSaleServiceClient.ReleaseFlashSaleStock(sdkupstream.WithServicePrincipal(ctx, servicePrincipalID, promotionScope), req, opts...)
}
