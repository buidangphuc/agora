package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/status"

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

const failedPrecondition = "FailedPrecondition"

var (
	ErrInsufficientBalance = errors.New("insufficient withdrawable balance")
	ErrPaymentUnavailable  = errors.New("payment service unavailable")
	ErrNoPaidPlan          = errors.New("seller has no paid plan")
)

func (s *Service) WithPayment(p PaymentCharger) *Service {
	s.payment = p
	return s
}

func ChargeKey(subID string, seq int64, planID string) string {
	return fmt.Sprintf("sub:%s:%d:%s", subID, seq, planID)
}

func (s *Service) Charge(ctx context.Context, sellerID string, amount int64, key string) error {
	if s.payment == nil {
		return ErrPaymentUnavailable
	}
	return Charge(ctx, s.payment, sellerID, amount, key)
}

func Charge(ctx context.Context, p PaymentCharger, sellerID string, amount int64, key string) error {
	if _, err := p.ChargeSellerFee(ctx, sellerID, amount, key); err != nil {
		if status.Code(err).String() == failedPrecondition {
			return fmt.Errorf("%w: %s", ErrInsufficientBalance, err.Error())
		}
		return fmt.Errorf("%w: %s", ErrPaymentUnavailable, err.Error())
	}
	return nil
}

func (s *Service) Subscribe(ctx context.Context, sellerID, planID string) (store.SellerSubscription, error) {
	if strings.TrimSpace(sellerID) == "" || strings.TrimSpace(planID) == "" {
		return store.SellerSubscription{}, ErrInvalidSubscription
	}
	plan, err := s.repo.GetPlanByID(ctx, planID)
	if err != nil {
		return store.SellerSubscription{}, err
	}
	var out store.SellerSubscription
	err = s.repo.WithSellerLock(ctx, sellerID, func(ctx context.Context, tx store.SubscriptionTx) error {
		cur, err := tx.Get(ctx)
		if err != nil {
			return err
		}
		period := store.SubscriptionPeriod{
			PlanID: cur.PlanID, Status: cur.Status, CurrentPeriodStart: cur.CurrentPeriodStart,
			CurrentPeriodEnd: cur.CurrentPeriodEnd, AutoRenew: cur.AutoRenew,
			PendingPlanID: cur.PendingPlanID, PeriodSeq: cur.PeriodSeq,
		}
		active := cur.Status == store.SubscriptionStatusActive && cur.Tier != store.PlanTierFree
		switch {
		case active && cur.PlanID == plan.ID:
			period.AutoRenew = true
			period.PendingPlanID = ""
		case active && plan.Tier < cur.Tier:
			period.PendingPlanID = plan.ID
			period.AutoRenew = true
		case plan.Price <= 0:
			out = cur
			return nil
		default:
			seq := cur.PeriodSeq + 1
			if err := s.Charge(ctx, sellerID, plan.Price, ChargeKey(cur.ID, seq, plan.ID)); err != nil {
				return err
			}
			now := s.nowFn()
			period = store.SubscriptionPeriod{
				PlanID: plan.ID, Status: store.SubscriptionStatusActive, CurrentPeriodStart: now,
				CurrentPeriodEnd: now.Add(time.Duration(plan.TermDays) * 24 * time.Hour),
				AutoRenew:        true, PeriodSeq: seq,
			}
		}
		out, err = tx.UpdatePeriod(ctx, period)
		return err
	})
	if err != nil {
		return store.SellerSubscription{}, err
	}
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, sellerID string) (store.SellerSubscription, error) {
	var out store.SellerSubscription
	err := s.repo.WithSellerLock(ctx, sellerID, func(ctx context.Context, tx store.SubscriptionTx) error {
		cur, err := tx.Get(ctx)
		if err != nil {
			return err
		}
		if cur.Status != store.SubscriptionStatusActive || cur.Tier == store.PlanTierFree {
			return ErrNoPaidPlan
		}
		out, err = tx.UpdatePeriod(ctx, store.SubscriptionPeriod{
			PlanID: cur.PlanID, Status: cur.Status, CurrentPeriodStart: cur.CurrentPeriodStart,
			CurrentPeriodEnd: cur.CurrentPeriodEnd, AutoRenew: false, PeriodSeq: cur.PeriodSeq,
		})
		return err
	})
	if err != nil {
		return store.SellerSubscription{}, err
	}
	return out, nil
}

func (s *Service) GetMySubscription(ctx context.Context, sellerID string) (store.SellerSubscription, error) {
	return s.repo.GetActiveSubscriptionBySeller(ctx, sellerID)
}
