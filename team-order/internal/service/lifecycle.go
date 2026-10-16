package service

import (
	"context"
	"errors"
	"time"

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
	s.remindUnpaid(ctx, now)
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
