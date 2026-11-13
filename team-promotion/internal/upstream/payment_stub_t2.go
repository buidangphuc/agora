package upstream

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PaymentClient struct{}

func NewPaymentClient(_ string, _ time.Duration) (*PaymentClient, error) {
	return &PaymentClient{}, nil
}

func (c *PaymentClient) Close() error { return nil }

func (c *PaymentClient) ChargeSellerFee(_ context.Context, _ string, _ int64, _ string) (bool, error) {
	return false, status.Error(codes.Unimplemented, "T2 stub")
}
