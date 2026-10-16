package upstream

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	promotionv1 "github.com/buidangphuc/team-order/generated/platform/promotion/v1"
)

// PromotionClients wraps the gRPC connection to team-promotion and exposes the
// VoucherService client used for checkout redemption (ValidateAndReserve during
// the saga, CommitReservation on settle, ReleaseReservation on compensation). It
// mirrors the domain client wrapper (Dial/Close, shared principal-forwarding
// interceptor) so wiring stays uniform.
type PromotionClients struct {
	conn    *grpc.ClientConn
	Voucher promotionv1.VoucherServiceClient
	// Flash is the FlashSaleService client: stock hold RPCs are sent as the
	// service principal; GetActiveFlashSale keeps the forwarded principal.
	Flash promotionv1.FlashSaleServiceClient
}

// DialPromotion opens a lazy gRPC client to team-promotion at addr. An empty addr
// returns (nil, nil) so an unconfigured UPSTREAM_PROMOTION_ADDR degrades cleanly:
// checkout then runs its existing (no-voucher) path unchanged.
func DialPromotion(addr string) (*PromotionClients, error) {
	if addr == "" {
		return nil, nil
	}
	insec := grpc.WithTransportCredentials(insecure.NewCredentials())
	interceptor := grpc.WithUnaryInterceptor(forwardMetadataInterceptor())

	conn, err := grpc.NewClient(addr, insec, interceptor)
	if err != nil {
		return nil, fmt.Errorf("dial promotion %s: %w", addr, err)
	}
	return &PromotionClients{
		conn:    conn,
		Voucher: NewServicePromotionClient(promotionv1.NewVoucherServiceClient(conn)),
		Flash:   NewServiceFlashClient(promotionv1.NewFlashSaleServiceClient(conn)),
	}, nil
}

func (c *PromotionClients) Close() {
	if c != nil && c.conn != nil {
		_ = c.conn.Close()
	}
}

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
	return c.VoucherServiceClient.ValidateAndReserve(asServiceWithPromotion(ctx), req, opts...)
}

func (c servicePromotionClient) CommitReservation(ctx context.Context, req *promotionv1.CommitReservationRequest, opts ...grpc.CallOption) (*promotionv1.CommitReservationResponse, error) {
	return c.VoucherServiceClient.CommitReservation(asServiceWithPromotion(ctx), req, opts...)
}

func (c servicePromotionClient) ReleaseReservation(ctx context.Context, req *promotionv1.ReleaseReservationRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseReservationResponse, error) {
	return c.VoucherServiceClient.ReleaseReservation(asServiceWithPromotion(ctx), req, opts...)
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
	return c.FlashSaleServiceClient.ReserveFlashSaleStock(asServiceWithPromotion(ctx), req, opts...)
}

func (c serviceFlashClient) CommitFlashSaleStock(ctx context.Context, req *promotionv1.CommitFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.CommitFlashSaleStockResponse, error) {
	return c.FlashSaleServiceClient.CommitFlashSaleStock(asServiceWithPromotion(ctx), req, opts...)
}

func (c serviceFlashClient) ReleaseFlashSaleStock(ctx context.Context, req *promotionv1.ReleaseFlashSaleStockRequest, opts ...grpc.CallOption) (*promotionv1.ReleaseFlashSaleStockResponse, error) {
	return c.FlashSaleServiceClient.ReleaseFlashSaleStock(asServiceWithPromotion(ctx), req, opts...)
}
