package subscription

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/team-promotion/internal/store"
)

type PaymentCharger interface {
	ChargeSellerFee(ctx context.Context, sellerID string, amount int64, referenceID string) (created bool, err error)
}

type Repository interface {
	store.SubscriptionRepository
	WithSellerLock(ctx context.Context, sellerID string, fn func(ctx context.Context, tx store.SubscriptionTx) error) error
	ListDueForRenewal(ctx context.Context, now time.Time, limit int) ([]store.SellerSubscription, error)
}

var (
	ErrInsufficientBalance = errors.New("insufficient withdrawable balance")
	ErrPaymentUnavailable  = errors.New("payment service unavailable")
	ErrNoPaidPlan          = errors.New("seller has no paid plan")
	errT2NotImplemented    = errors.New("T2 stub: not implemented")
)

func (s *Service) Cancel(_ context.Context, _ string) (store.SellerSubscription, error) {
	return store.SellerSubscription{}, errT2NotImplemented
}

func (s *Service) GetMySubscription(_ context.Context, _ string) (store.SellerSubscription, error) {
	return store.SellerSubscription{}, errT2NotImplemented
}

func (s *Service) WithPayment(p PaymentCharger) *Service {
	s.payment = p
	return s
}
