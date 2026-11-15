package upstream

// STUB (test-author, task t2-obligations-lifecycle-rpcs): the implementer replaces this file
// with the real clients for OrderService.GetUserObligations and PaymentService.GetPayoutObligations.

import (
	"context"
	"errors"

	"google.golang.org/grpc"
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

type Obligations struct{}

func NewObligations(orderConn, paymentConn *grpc.ClientConn) *Obligations { return &Obligations{} }

func (o *Obligations) UserObligations(ctx context.Context, userID string) (UserObligations, error) {
	return UserObligations{}, errors.New("not implemented")
}

func (o *Obligations) PayoutObligations(ctx context.Context, userID string) (PayoutObligations, error) {
	return PayoutObligations{}, errors.New("not implemented")
}
