package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/propagation"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// Publisher sends one built record and returns once the broker acknowledged it.
// kafkax.Producer satisfies it, as does testkit.Recorder; the method set is the
// same as consume.DLQProducer.
type Publisher interface {
	PublishRecord(ctx context.Context, r *kgo.Record) error
}

// RelayerOptions configures a Relayer. Zero values fall back to the Outbox
// config defaults, a 1s BaseBackoff, a 5m MaxBackoff and a 10s MarkTimeout;
// negative values are rejected by NewRelayer. Topic is the default topic for
// rows without their own; it may be empty only if every row names one.
//
// Config.MaxAttempts is not consulted: transient failures retry forever and a
// non-retriable failure parks the row at once (see Relayer.RelayOnce).
type RelayerOptions struct {
	Topic                   string
	Config                  config.Outbox
	BaseBackoff, MaxBackoff time.Duration
	// MarkTimeout bounds each database write that records a publish outcome
	// (published, failed or released). Each write gets its own timeout, so a slow
	// publish never eats into the budget of the marks that follow it.
	MarkTimeout time.Duration
	Logger      *slog.Logger
}

// Report summarises one RelayOnce pass.
type Report struct{ Claimed, Published, Retried, Parked int }

// Relayer publishes outbox rows. It is a lifecycle component.
type Relayer struct {
	store *Store
	pub   Publisher
	opt   RelayerOptions
}

// NewRelayer builds a relayer over store and publisher. It returns an error for
// a nil store or publisher, a negative option, or a MaxBackoff below BaseBackoff.
func NewRelayer(s *Store, p Publisher, o RelayerOptions) (*Relayer, error) {
	switch {
	case s == nil:
		return nil, errors.New("outbox: nil store")
	case p == nil:
		return nil, errors.New("outbox: nil publisher")
	}
	for name, v := range map[string]time.Duration{
		"PollInterval": o.Config.PollInterval, "ClaimLock": o.Config.ClaimLock,
		"BaseBackoff": o.BaseBackoff, "MaxBackoff": o.MaxBackoff, "MarkTimeout": o.MarkTimeout,
	} {
		if v < 0 {
			return nil, fmt.Errorf("outbox: %s must not be negative, got %v", name, v)
		}
	}
	if o.Config.BatchSize < 0 {
		return nil, fmt.Errorf("outbox: BatchSize must not be negative, got %d", o.Config.BatchSize)
	}
	if o.Config.PollInterval == 0 {
		o.Config.PollInterval = time.Second
	}
	if o.Config.BatchSize == 0 {
		o.Config.BatchSize = 100
	}
	if o.Config.ClaimLock == 0 {
		o.Config.ClaimLock = time.Minute
	}
	if o.BaseBackoff == 0 {
		o.BaseBackoff = time.Second
	}
	if o.MaxBackoff == 0 {
		o.MaxBackoff = max(5*time.Minute, o.BaseBackoff)
	}
	if o.MarkTimeout == 0 {
		o.MarkTimeout = 10 * time.Second
	}
	if o.MaxBackoff < o.BaseBackoff {
		return nil, fmt.Errorf("outbox: MaxBackoff %v is below BaseBackoff %v", o.MaxBackoff, o.BaseBackoff)
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &Relayer{store: s, pub: p, opt: o}, nil
}

// Run polls every PollInterval until ctx is cancelled, then returns nil. A pass
// that published something is followed at once by another, so a backlog drains
// without waiting for the ticker. A failed pass is logged and retried on the next tick.
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
// aggregate: only an aggregate's oldest unpublished row is claimed, together
// with the claimable run right behind it, so a pass publishes up to BatchSize
// consecutive rows of one aggregate. At the first failure the aggregate stops
// and its remaining claimed rows are released unattempted; other aggregates go
// on. An aggregate whose head backs off or is parked is not scanned at all.
//
// A publish error is classified. A transient one (a broker or timeout error the
// client marks retriable, anything unrecognised) counts an attempt and delays
// the row by a capped exponential backoff; it never parks, so a long outage
// resolves by itself. A non-retriable one (record too large, unknown topic,
// authorization, ...) parks the row as failed at once; it blocks its aggregate
// until Store.Requeue or deletion, because order is the contract. Parking is
// logged at ERROR. A row with no topic (neither its own nor the relayer's) is
// a configuration error: it is logged at ERROR and released without counting
// an attempt.
//
// Publishing is bounded by half the claim lock: when that budget runs out the
// remaining rows are released unattempted, so a broker outage cannot outlive the
// lease and cause a second relayer to publish the same rows. Every mark (published,
// failed, released) has its own MarkTimeout and survives ctx cancellation.
func (r *Relayer) RelayOnce(ctx context.Context) (Report, error) {
	rows, err := r.store.claim(ctx, r.opt.Config.BatchSize, r.opt.Config.ClaimLock)
	if err != nil {
		return Report{}, err
	}
	rep := Report{Claimed: len(rows)}
	budget := r.opt.Config.ClaimLock / 2
	pubCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var release []pending
	var stopped aggKey // aggregate that already failed in this pass
	haveStopped := false
	for i, row := range rows {
		key := aggKey{row.aggregateType, row.aggregateID}
		if haveStopped && key == stopped {
			release = append(release, row)
			continue
		}
		topic := row.topic
		if topic == "" {
			topic = r.opt.Topic
		}
		if topic == "" {
			r.opt.Logger.ErrorContext(ctx, "outbox row has no topic: neither the row nor the relayer names one; not attempted",
				"event_id", row.eventID, "aggregate_id", row.aggregateID)
			stopped, haveStopped = key, true
			release = append(release, row)
			continue
		}
		if err := r.publish(pubCtx, topic, row); err != nil {
			if ctx.Err() != nil {
				// Shutdown, not a delivery failure: do not count an attempt.
				r.releaseFrom(ctx, append(release, rows[i:]...))
				return rep, nil
			}
			r.fail(ctx, row, err, &rep)
			stopped, haveStopped = key, true
			if pubCtx.Err() != nil {
				r.opt.Logger.WarnContext(ctx, "outbox batch budget exhausted", "budget", budget, "released", len(rows)-i-1)
				r.releaseFrom(ctx, append(release, rows[i+1:]...))
				return rep, nil
			}
			continue
		}
		markCtx, markCancel := r.markCtx(ctx)
		err := r.store.markPublished(markCtx, row.eventID)
		markCancel()
		if err != nil {
			// Published but not marked: the lease expires and the row is
			// delivered again, which at-least-once allows. The rest of the
			// aggregate must not overtake it.
			r.opt.Logger.WarnContext(ctx, "outbox mark published failed", "event_id", row.eventID, "err", err)
			stopped, haveStopped = key, true
			continue
		}
		rep.Published++
	}
	r.releaseFrom(ctx, release)
	return rep, nil
}

// aggKey identifies an aggregate.
type aggKey struct{ typ, id string }

// markCtx returns a context for one mark: it outlives ctx's cancellation (a
// delivered row must be recorded, or it would be delivered again) and has its
// own timeout.
func (r *Relayer) markCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), r.opt.MarkTimeout)
}

func (r *Relayer) publish(ctx context.Context, topic string, row pending) error {
	hdr := []kgo.RecordHeader{
		{Key: kafkax.HeaderEventID, Value: []byte(row.eventID)},
		{Key: kafkax.HeaderEventType, Value: []byte(row.eventType)},
	}
	if row.requestID != "" {
		hdr = append(hdr, kgo.RecordHeader{Key: kafkax.HeaderRequestID, Value: []byte(row.requestID)})
		ctx = reqctx.WithRequestID(ctx, row.requestID)
	}
	if row.traceparent != "" {
		hdr = append(hdr, kgo.RecordHeader{Key: kafkax.HeaderTraceparent, Value: []byte(row.traceparent)})
		ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": row.traceparent})
	}
	return r.pub.PublishRecord(ctx, &kgo.Record{Topic: topic, Key: []byte(row.aggregateID), Value: row.payload, Headers: hdr})
}

// releaseFrom ends the lease of rows that were claimed but not attempted, with a
// mark timeout of its own.
func (r *Relayer) releaseFrom(ctx context.Context, rows []pending) {
	if len(rows) == 0 {
		return
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.eventID
	}
	markCtx, cancel := r.markCtx(ctx)
	defer cancel()
	if err := r.store.release(markCtx, ids); err != nil {
		r.opt.Logger.WarnContext(ctx, "outbox release failed", "rows", len(ids), "err", err)
	}
}

// retriable reports whether a publish error may succeed later. Errors the Kafka
// client reports as non-retriable (record too large, authorization, corrupt
// message, ...) are not; an unknown topic is not either, since the client only
// returns it after its own retries, so the topic is missing and auto-create is
// off. Everything else (timeouts, connection loss, retriable broker errors,
// errors that are not Kafka's) is retried.
func retriable(err error) bool {
	if errors.Is(err, kerr.UnknownTopicOrPartition) {
		return false
	}
	if ke, ok := errors.AsType[*kerr.Error](err); ok {
		return ke.Retriable
	}
	return true
}

func (r *Relayer) fail(ctx context.Context, row pending, pubErr error, rep *Report) {
	markCtx, cancel := r.markCtx(ctx)
	defer cancel()
	attempt := row.attempts + 1
	if !retriable(pubErr) {
		if err := r.store.markFailed(markCtx, row.eventID, pubErr.Error(), nil); err != nil {
			r.opt.Logger.WarnContext(ctx, "outbox park failed", "event_id", row.eventID, "err", err)
			return
		}
		rep.Parked++
		r.opt.Logger.ErrorContext(ctx, "outbox row parked by a non-retriable error; its aggregate is blocked until the row is requeued or removed",
			"event_id", row.eventID, "aggregate_id", row.aggregateID, "attempts", attempt, "err", pubErr)
		return
	}
	next := time.Now().Add(r.backoff(attempt))
	if err := r.store.markFailed(markCtx, row.eventID, pubErr.Error(), &next); err != nil {
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
