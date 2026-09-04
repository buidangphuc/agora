package upstream

import (
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
		Voucher: promotionv1.NewVoucherServiceClient(conn),
	}, nil
}

func (c *PromotionClients) Close() {
	if c != nil && c.conn != nil {
		_ = c.conn.Close()
	}
}
