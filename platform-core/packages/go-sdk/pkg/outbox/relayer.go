package outbox

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// Publisher sends one built record and returns once the broker acknowledged it.
// kafkax.Producer satisfies it, as does testkit.Recorder; the method set is the
// same as consume.DLQProducer.
type Publisher interface {
	PublishRecord(ctx context.Context, r *kgo.Record) error
}

// Header names set on every published record.
const (
	HeaderEventID     = "event_id"
	HeaderEventType   = "event_type"
	HeaderRequestID   = "x-request-id"
	HeaderTraceparent = "traceparent"
)

// RelayerOptions configures a Relayer. Zero values fall back to the Outbox
// config defaults, a 1s BaseBackoff and a 5m MaxBackoff. Topic is the default
// topic for rows without their own; it may be empty only if every row names one.
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

// Run polls every PollInterval until ctx is cancelled, then returns nil. A pass
// that published something is followed at once by another, so a backlog (or a
// long run of events of one aggregate, which go out one per pass) drains without
// waiting for the ticker. A failed pass is logged and retried on the next tick.
func (r *Relayer) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.opt.Config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for ctx.Err() == nil {
				rep, err := r.RelayOnce(ctx)
				if err != nil {
					if ctx.Err() == nil {
						r.opt.Logger.WarnContext(ctx, "outbox relay pass failed", "err", err)
					}
					break
				}
				if rep.Published == 0 {
					break
				}
			}
		}
	}
}

// RelayOnce claims one batch and publishes it in order. Ordering is per
// aggregate: a row is claimed only when no older row of its aggregate is
// unpublished, so a failing row (backing off, or parked after MaxAttempts) holds
// back only the newer rows of its own aggregate, and a pass publishes at most one
// row per aggregate. A parked row keeps blocking its aggregate until an operator
// sets it back to status 'pending' (or deletes it): order is the contract, so
// the relayer never skips past a row it could not deliver. Parking is logged at
// ERROR for that reason.
//
// Publishing is bounded by half the claim lock: when that budget runs out the
// remaining rows are released unattempted, so a broker outage cannot outlive the
// lease and cause a second relayer to publish the same rows. Marking a row done
// is not bound by the budget or by ctx cancellation.
func (r *Relayer) RelayOnce(ctx context.Context) (Report, error) {
	rows, err := r.store.claim(ctx, r.opt.Config.BatchSize, r.opt.Config.ClaimLockSeconds)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Claimed: len(rows)}
	budget := time.Duration(r.opt.Config.ClaimLockSeconds) * time.Second / 2
	pubCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	// Marks outlive both the budget and a shutdown, or a delivered row would be
	// delivered again.
	markCtx, markCancel := context.WithTimeout(context.WithoutCancel(ctx), markTimeout)
	defer markCancel()

	for i, row := range rows {
		if err := r.publish(pubCtx, row); err != nil {
			if ctx.Err() != nil {
				// Shutdown, not a delivery failure: do not count an attempt.
				r.releaseFrom(markCtx, rows[i:])
				return rep, nil
			}
			r.fail(markCtx, row, err, &rep)
			if pubCtx.Err() != nil {
				r.opt.Logger.WarnContext(ctx, "outbox batch budget exhausted", "budget", budget, "released", len(rows)-i-1)
				r.releaseFrom(markCtx, rows[i+1:])
				return rep, nil
			}
			continue
		}
		if err := r.store.markPublished(markCtx, row.eventID); err != nil {
			// Published but not marked: the lease expires and the row is
			// delivered again, which at-least-once allows.
			r.opt.Logger.WarnContext(ctx, "outbox mark published failed", "event_id", row.eventID, "err", err)
			continue
		}
		rep.Published++
	}
	return rep, nil
}

const markTimeout = 10 * time.Second

func (r *Relayer) publish(ctx context.Context, row pending) error {
	topic := row.topic
	if topic == "" {
		topic = r.opt.Topic
	}
	if topic == "" {
		return errors.New("no topic: neither the row nor the relayer names one")
	}
	hdr := []kgo.RecordHeader{
		{Key: HeaderEventID, Value: []byte(row.eventID)},
		{Key: HeaderEventType, Value: []byte(row.eventType)},
	}
	if row.requestID != "" {
		hdr = append(hdr, kgo.RecordHeader{Key: HeaderRequestID, Value: []byte(row.requestID)})
		ctx = reqctx.WithRequestID(ctx, row.requestID)
	}
	if row.traceparent != "" {
		hdr = append(hdr, kgo.RecordHeader{Key: HeaderTraceparent, Value: []byte(row.traceparent)})
		ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": row.traceparent})
	}
	return r.pub.PublishRecord(ctx, &kgo.Record{Topic: topic, Key: []byte(row.aggregateID), Value: row.payload, Headers: hdr})
}

func (r *Relayer) releaseFrom(ctx context.Context, rows []pending) {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.eventID
	}
	if err := r.store.release(ctx, ids); err != nil {
		r.opt.Logger.WarnContext(ctx, "outbox release failed", "rows", len(ids), "err", err)
	}
}

func (r *Relayer) fail(ctx context.Context, row pending, pubErr error, rep *Report) {
	attempt := row.attempts + 1
	if attempt >= r.opt.Config.MaxAttempts {
		if err := r.store.markFailed(ctx, row.eventID, pubErr.Error(), nil); err != nil {
			r.opt.Logger.WarnContext(ctx, "outbox park failed", "event_id", row.eventID, "err", err)
			return
		}
		rep.Parked++
		r.opt.Logger.ErrorContext(ctx, "outbox row parked; its aggregate is blocked until the row is requeued or removed",
			"event_id", row.eventID, "aggregate_id", row.aggregateID, "attempts", attempt, "err", pubErr)
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
