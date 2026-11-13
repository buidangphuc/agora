package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/subscription"
)

const (
	renewalBatch           = 500
	defaultRenewalInterval = time.Minute
)

type RenewalConfig struct {
	RetryWindow time.Duration
	Interval    time.Duration
	Now         func() time.Time
	Logger      *slog.Logger
}

type RenewalSweeper struct {
	repo    subscription.Repository
	payment subscription.PaymentCharger
	cfg     RenewalConfig
	overdue metric.Int64Gauge
}

func NewRenewalSweeper(repo subscription.Repository, payment subscription.PaymentCharger, cfg RenewalConfig) *RenewalSweeper {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultRenewalInterval
	}
	g, err := otel.Meter("team-promotion/subscription").Int64Gauge("subscription_renewals_overdue",
		metric.WithDescription("Subscriptions past their period end that are still awaiting renewal"))
	if err != nil {
		cfg.Logger.Error("create renewals overdue gauge", "err", err)
	}
	return &RenewalSweeper{repo: repo, payment: payment, cfg: cfg, overdue: g}
}

func (s *RenewalSweeper) Sweep(ctx context.Context) (int, error) {
	now := s.cfg.Now()
	due, err := s.repo.ListDueForRenewal(ctx, now, renewalBatch)
	if err != nil {
		return 0, err
	}
	var overdue int64
	var errs []error
	processed := 0
	for _, d := range due {
		pending, err := s.renew(ctx, d.SellerID, now)
		if err != nil {
			errs = append(errs, err)
			overdue++
			continue
		}
		if pending {
			overdue++
		} else {
			processed++
		}
	}
	if s.overdue != nil {
		s.overdue.Record(ctx, overdue)
	}
	return processed, errors.Join(errs...)
}

func (s *RenewalSweeper) renew(ctx context.Context, sellerID string, now time.Time) (stillOverdue bool, err error) {
	err = s.repo.WithSellerLock(ctx, sellerID, func(ctx context.Context, tx store.SubscriptionTx) error {
		cur, err := tx.Get(ctx)
		if err != nil {
			return err
		}
		if cur.Status != store.SubscriptionStatusActive || cur.CurrentPeriodEnd.After(now) {
			return nil
		}
		period := store.SubscriptionPeriod{
			PlanID: cur.PlanID, Status: cur.Status, CurrentPeriodStart: cur.CurrentPeriodStart,
			CurrentPeriodEnd: cur.CurrentPeriodEnd, AutoRenew: cur.AutoRenew, PeriodSeq: cur.PeriodSeq,
		}
		if !cur.AutoRenew {
			period.Status = store.SubscriptionStatusCancelled
			_, err := tx.UpdatePeriod(ctx, period)
			return err
		}
		targetID := cur.PlanID
		if cur.PendingPlanID != "" {
			targetID = cur.PendingPlanID
		}
		target, err := s.repo.GetPlanByID(ctx, targetID)
		if err != nil {
			return err
		}
		period.PlanID = target.ID
		if target.Price <= 0 {
			period.Status = store.SubscriptionStatusExpired
			period.AutoRenew = false
			_, err := tx.UpdatePeriod(ctx, period)
			return err
		}
		seq := cur.PeriodSeq + 1
		cerr := subscription.Charge(ctx, s.payment, sellerID, target.Price, subscription.ChargeKey(cur.ID, seq, target.ID))
		switch {
		case cerr == nil:
			period = store.SubscriptionPeriod{
				PlanID: target.ID, Status: store.SubscriptionStatusActive, CurrentPeriodStart: now,
				CurrentPeriodEnd: now.Add(time.Duration(target.TermDays) * 24 * time.Hour),
				AutoRenew:        true, PeriodSeq: seq,
			}
		case errors.Is(cerr, subscription.ErrInsufficientBalance),
			now.Sub(cur.CurrentPeriodEnd) >= s.cfg.RetryWindow:
			s.cfg.Logger.WarnContext(ctx, "subscription renewal lapsed", "subscription_id", cur.ID, "err", cerr)
			period.PlanID = cur.PlanID
			period.Status = store.SubscriptionStatusExpired
			period.AutoRenew = false
			period.PendingPlanID = ""
		default:
			s.cfg.Logger.WarnContext(ctx, "subscription renewal charge failed, will retry", "subscription_id", cur.ID, "err", cerr)
			stillOverdue = true
			return nil
		}
		_, err = tx.UpdatePeriod(ctx, period)
		return err
	})
	return stillOverdue, err
}

func (s *RenewalSweeper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		n, err := s.Sweep(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			s.cfg.Logger.ErrorContext(ctx, "subscription renewal sweep failed", "err", err)
		case n > 0:
			s.cfg.Logger.InfoContext(ctx, "subscriptions processed", "count", n)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
