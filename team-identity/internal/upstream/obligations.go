package upstream

import (
	"context"

	"google.golang.org/grpc"

	orderv1 "github.com/buidangphuc/team-identity/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-identity/generated/platform/payment/v1"
)

type UserObligations struct {
	OpenBuyerOrders  int32
	OpenSellerOrders int32
	OpenReturns      int32
}

type PayoutObligations struct {
	AvailableBalance int64
	HeldBalance      int64
	PendingPayouts   int32
}

type Obligations struct {
	order   orderv1.OrderServiceClient
	payment paymentv1.PaymentServiceClient
}

func NewObligations(orderConn, paymentConn *grpc.ClientConn) *Obligations {
	return &Obligations{
		order:   orderv1.NewOrderServiceClient(orderConn),
		payment: paymentv1.NewPaymentServiceClient(paymentConn),
	}
}

func (o *Obligations) UserObligations(ctx context.Context, userID string) (UserObligations, error) {
	r, err := o.order.GetUserObligations(ctx, &orderv1.GetUserObligationsRequest{UserId: userID})
	if err != nil {
		return UserObligations{}, err
	}
	return UserObligations{OpenBuyerOrders: r.GetOpenBuyerOrders(), OpenSellerOrders: r.GetOpenSellerOrders(), OpenReturns: r.GetOpenReturns()}, nil
}

func (o *Obligations) PayoutObligations(ctx context.Context, userID string) (PayoutObligations, error) {
	r, err := o.payment.GetPayoutObligations(ctx, &paymentv1.GetPayoutObligationsRequest{UserId: userID})
	if err != nil {
		return PayoutObligations{}, err
	}
	return PayoutObligations{AvailableBalance: r.GetAvailableBalance(), HeldBalance: r.GetHeldBalance(), PendingPayouts: r.GetPendingPayouts()}, nil
}
