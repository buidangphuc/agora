package service

import (
	"context"
	"errors"
	"time"

	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/repository"
)

// lifecycleBatch bounds how many orders one pass handles per tick.
const lifecycleBatch = 100

// DefaultPaymentReminderBefore is how long before the deadline the reminder goes out.
const DefaultPaymentReminderBefore = 2 * time.Hour

// WithPaymentReminderBefore sets how long before an order's payment deadline its
// reminder is emitted.
func WithPaymentReminderBefore(d time.Duration) OrderServiceOption {
	return func(s *OrderService) { s.paymentReminderBefore = d }
}

// SweepLifecycle runs one lifecycle tick at now: the reminder pass. Each pass
// handles at most lifecycleBatch orders and is safe to repeat and to run on
// several replicas (the stamp is a compare-and-set).
func (s *OrderService) SweepLifecycle(ctx context.Context, now time.Time) {
	// Passes are independent: a failure in one is logged and never stops the next.
	// Expiry runs first so an order already past its deadline is never reminded.
	s.expireUnpaid(ctx, now)
	s.remindUnpaid(ctx, now)
	if _, err := s.AutoCompleteShipped(ctx, now); err != nil {
		s.logger.Error("auto-complete shipped orders", "error", err)
	}
	if _, err := s.AutoApproveReturns(ctx, now); err != nil {
		s.logger.Error("auto-approve returns", "error", err)
	}
}

// AutoCompleteShipped completes up to one batch of SHIPPED orders whose
// auto-complete deadline has passed and that no return blocks, as the system. It
// returns how many it completed; an order that lost a race (confirmed, completed
// or moved meanwhile) is skipped.
func (s *OrderService) AutoCompleteShipped(ctx context.Context, now time.Time) (int, error) {
	finder, ok := s.orderRepo.(repository.AutoCompleteFinder)
	if !ok {
		return 0, nil
	}
	due, err := finder.ListAutoCompletable(ctx, now, lifecycleBatch)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, o := range due {
		_, err := s.orderRepo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusCompleted,
			AllowedFrom(repository.OrderStatusCompleted, ActorSystem), "",
			repository.WithCompletedBy(int32(orderv1.CompletedBy_COMPLETED_BY_SYSTEM)))
		switch {
		case err == nil:
			done++
		case errors.Is(err, repository.ErrStatusConflict), errors.Is(err, repository.ErrOrderNotFound):
		default:
			s.logger.Error("auto-complete order", "order_id", o.ID, "error", err)
		}
	}
	return done, nil
}

// AutoApproveReturns approves up to one batch of PENDING returns whose answer
// window has passed. It returns how many it approved; a return the seller answered
// first is skipped. Refund still goes through UpdateReturnStatus.
func (s *OrderService) AutoApproveReturns(ctx context.Context, now time.Time) (int, error) {
	finder, ok := s.returnRepo.(repository.AutoApproveFinder)
	if !ok {
		return 0, nil
	}
	due, err := finder.ListAutoApprovable(ctx, now, lifecycleBatch)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, ret := range due {
		_, err := s.returnRepo.TransitionReturn(ctx, ret.ID, repository.ReturnStatusPending, repository.ReturnStatusApproved)
		switch {
		case err == nil:
			done++
		case errors.Is(err, repository.ErrReturnStatusConflict), errors.Is(err, repository.ErrReturnNotFound):
		default:
			s.logger.Error("auto-approve return", "return_id", ret.ID, "error", err)
		}
	}
	return done, nil
}

func (s *OrderService) expireUnpaid(ctx context.Context, now time.Time) {
	finder, ok := s.orderRepo.(repository.ExpiryFinder)
	if !ok {
		return
	}
	due, err := finder.FindExpired(ctx, now, lifecycleBatch)
	if err != nil {
		s.logger.Error("find expired orders", "error", err)
		return
	}
	for _, o := range due {
		if _, err := s.CancelOrderWithReason(ctx, o.ID, orderv1.CancelReason_CANCEL_REASON_PAYMENT_EXPIRED); err != nil {
			// Lost the race (paid, cancelled or expired by another replica): nothing to do.
			if !errors.Is(err, ErrInvalidStatus) && !errors.Is(err, repository.ErrOrderNotFound) {
				s.logger.Error("expire unpaid order", "order_id", o.ID, "error", err)
			}
		}
	}
}

func (s *OrderService) remindUnpaid(ctx context.Context, now time.Time) {
	before := s.paymentReminderBefore
	if before <= 0 {
		before = DefaultPaymentReminderBefore
	}
	due, err := s.orderRepo.FindReminderDue(ctx, now, before, lifecycleBatch)
	if err != nil {
		s.logger.Error("find orders due a payment reminder", "error", err)
		return
	}
	for _, o := range due {
		if _, err := s.orderRepo.MarkReminderSent(ctx, o.ID, now); err != nil {
			// Lost the race (another replica, paid or cancelled meanwhile): nothing to send.
			if !errors.Is(err, repository.ErrStatusConflict) && !errors.Is(err, repository.ErrOrderNotFound) {
				s.logger.Error("mark payment reminder sent", "order_id", o.ID, "error", err)
			}
		}
	}
}
