package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Publisher sends one record. kafkax.Producer satisfies it.
type Publisher interface {
	Publish(ctx context.Context, topic string, key, value []byte) error
}

// RelayerOptions configures a Relayer. Zero values fall back to the Outbox
// config defaults, a 1s BaseBackoff and a 5m MaxBackoff.
type RelayerOptions struct {
	Topic                   string
	Config                  config.Outbox
	BaseBackoff, MaxBackoff time.Duration
	Logger                  *slog.Logger
}

// Report summarises one RelayOnce pass.
type Report struct{ Claimed, Published, Retried, Parked int }

// Relayer publishes outbox rows. It is a lifecycle component.
type Relayer struct {
	store *Store
	pub   Publisher
	opt   RelayerOptions
}

// NewRelayer builds a relayer over store and publisher.
func NewRelayer(s *Store, p Publisher, o RelayerOptions) *Relayer {
	if o.Config.PollInterval <= 0 {
		o.Config.PollInterval = time.Second
	}
	if o.Config.BatchSize <= 0 {
		o.Config.BatchSize = 100
	}
	if o.Config.ClaimLockSeconds <= 0 {
		o.Config.ClaimLockSeconds = 60
	}
	if o.Config.MaxAttempts <= 0 {
		o.Config.MaxAttempts = 10
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = time.Second
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 5 * time.Minute
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Relayer{store: s, pub: p, opt: o}
}

// Run polls every PollInterval until ctx is cancelled, then returns nil. A failed
// pass is logged and retried on the next tick.
func (r *Relayer) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.opt.Config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := r.RelayOnce(ctx); err != nil && ctx.Err() == nil {
				r.opt.Logger.WarnContext(ctx, "outbox relay pass failed", "err", err)
			}
		}
	}
}

// RelayOnce claims one batch and publishes it. A row that fails does not block
// the others: it is retried behind a backoff gate, or parked once its attempts
// reach MaxAttempts.
func (r *Relayer) RelayOnce(ctx context.Context) (Report, error) {
	rows, err := r.store.claim(ctx, r.opt.Config.BatchSize, r.opt.Config.ClaimLockSeconds)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Claimed: len(rows)}
	for _, row := range rows {
		if err := r.pub.Publish(ctx, r.opt.Topic, []byte(row.aggregateID), row.payload); err != nil {
			r.fail(ctx, row, err, &rep)
			continue
		}
		if err := r.store.markPublished(ctx, row.eventID); err != nil {
			// Published but not marked: the lease expires and the row is
			// delivered again, which at-least-once allows.
			r.opt.Logger.WarnContext(ctx, "outbox mark published failed", "event_id", row.eventID, "err", err)
			continue
		}
		rep.Published++
	}
	return rep, nil
}

func (r *Relayer) fail(ctx context.Context, row pending, pubErr error, rep *Report) {
	attempt := row.attempts + 1
	if attempt >= r.opt.Config.MaxAttempts {
		if err := r.store.markFailed(ctx, row.eventID, pubErr.Error(), nil); err != nil {
			r.opt.Logger.WarnContext(ctx, "outbox park failed", "event_id", row.eventID, "err", err)
			return
		}
		rep.Parked++
		r.opt.Logger.WarnContext(ctx, "outbox row parked", "event_id", row.eventID, "attempts", attempt, "err", pubErr)
		return
	}
	next := time.Now().Add(r.backoff(attempt))
	if err := r.store.markFailed(ctx, row.eventID, pubErr.Error(), &next); err != nil {
		r.opt.Logger.WarnContext(ctx, "outbox retry mark failed", "event_id", row.eventID, "err", err)
		return
	}
	rep.Retried++
}

// backoff is min(BaseBackoff·2^(attempt-1), MaxBackoff) for a 1-based attempt.
func (r *Relayer) backoff(attempt int) time.Duration {
	d := r.opt.BaseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= r.opt.MaxBackoff {
			return r.opt.MaxBackoff
		}
	}
	return min(d, r.opt.MaxBackoff)
}
