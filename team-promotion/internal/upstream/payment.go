package upstream

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	paymentv1 "github.com/buidangphuc/team-promotion/generated/platform/payment/v1"
)

const (
	paymentServicePrincipalID    = "service-team-promotion"
	paymentServicePrincipalScope = "payment:write"
)

type PaymentClient struct {
	conn *grpc.ClientConn
	svc  paymentv1.PaymentServiceClient
}

func NewPaymentClient(addr string, timeout time.Duration) (*PaymentClient, error) {
	conn, err := sdkupstream.NewClient(addr,
		sdkupstream.AsService(paymentServicePrincipalID, paymentServicePrincipalScope),
		sdkupstream.WithTimeout(timeout),
	)
	if err != nil {
		return nil, fmt.Errorf("dial team-payment at %s: %w", addr, err)
	}
	return &PaymentClient{conn: conn, svc: paymentv1.NewPaymentServiceClient(conn)}, nil
}

func (c *PaymentClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *PaymentClient) ChargeSellerFee(ctx context.Context, sellerID string, amount int64, referenceID string) (bool, error) {
	if c == nil || c.svc == nil {
		return false, status.Error(codes.Unavailable, "payment client not configured")
	}
	resp, err := c.svc.ChargeSellerFee(ctx, &paymentv1.ChargeSellerFeeRequest{
		SellerId:    sellerID,
		Amount:      amount,
		Kind:        paymentv1.SellerFeeKind_SELLER_FEE_KIND_SUBSCRIPTION,
		ReferenceId: referenceID,
	})
	if err != nil {
		code := status.Code(err)
		if code == codes.FailedPrecondition {
			return false, status.Error(codes.FailedPrecondition, "insufficient withdrawable balance")
		}
		slog.WarnContext(ctx, "charge seller fee failed", "grpc_code", code.String())
		return false, status.Error(codes.Unavailable, "payment service call failed")
	}
	return resp.GetCreated(), nil
}
