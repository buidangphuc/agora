package consume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// BatchHandler processes every record of one fetch at once, for example one
// bulk write. nil commits the batch. An error wrapped by Permanent dead-letters
// the whole batch and commits. Any other error retries the same batch after
// backoff, without committing, up to BatchOptions.MaxAttempts, then the whole
// batch is dead-lettered; the handler must be idempotent.
//
// The ctx carries a "consume batch" span linked to the span of every record
// (a batch has no single parent). When all records share one valid x-request-id it is
// also set on ctx; otherwise read it from Record.Headers.
type BatchHandler func(ctx context.Context, records []Record) error

// BatchOptions tunes the batch loop. Zero fields take the defaults noted.
type BatchOptions struct {
	DLQTopic string // required; NewBatchConsumer fails when empty
	// MaxPollRecords caps the records of one poll, which is one batch (default
	// 500). Rebalances are blocked while a batch is handled, so an unbounded
	// batch could outlast the group's rebalance timeout.
	MaxPollRecords int
	MaxAttempts    int           // handler attempts per batch before it is dead-lettered (default 5)
	BaseBackoff    time.Duration // delay after the first failure, doubling (default 100ms)
	MaxBackoff     time.Duration // backoff ceiling (default 30s)
	Logger         *slog.Logger  // default slog.Default()
}

// BatchConsumer dispatches each fetched batch from a consumer-group client.
type BatchConsumer struct {
	client  groupClient
	group   string
	handler BatchHandler
	dlq     DLQProducer
	opts    BatchOptions
}

// NewBatchConsumer binds client, which must have auto-commit disabled (as
// kafkax.NewConsumerClient does), to h and dlq. It fails when client, h or dlq
// is nil, BatchOptions.DLQTopic is empty or client has no consumer group (its
// commits would fail).
func NewBatchConsumer(client *kgo.Client, h BatchHandler, dlq DLQProducer, o BatchOptions) (*BatchConsumer, error) {
	group, err := clientGroup(client)
	if err != nil {
		return nil, err
	}
	return newBatchConsumer(client, group, h, dlq, o)
}

func newBatchConsumer(client groupClient, group string, h BatchHandler, dlq DLQProducer, o BatchOptions) (*BatchConsumer, error) {
	switch {
	case h == nil:
		return nil, errors.New("consume: handler is nil")
	case dlq == nil:
		return nil, errors.New("consume: dead-letter producer is nil")
	case o.DLQTopic == "":
		return nil, errors.New("consume: BatchOptions.DLQTopic is required")
	}
	if o.MaxPollRecords <= 0 {
		o.MaxPollRecords = DefaultMaxPollRecords
	}
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 5
	}
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 100 * time.Millisecond
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 30 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &BatchConsumer{client: client, group: group, handler: h, dlq: dlq, opts: o}, nil
}

// Run polls until ctx is cancelled (it then returns nil) or a batch cannot be
// dead-lettered or committed. In that case Run returns the error without
// committing, so a restart in the same group delivers the batch again.
//
// The offsets of a fetch are committed once, after the whole batch was handled
// or dead-lettered, and rebalances are blocked until then; see Consumer.Run.
func (c *BatchConsumer) Run(ctx context.Context) error {
	defer c.client.AllowRebalance()
	for {
		fetches := c.client.PollRecords(ctx, c.opts.MaxPollRecords)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return nil
		}
		if err := c.fetch(ctx, fetches); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func (c *BatchConsumer) fetch(ctx context.Context, fetches kgo.Fetches) error {
	defer c.client.AllowRebalance()
	for _, fe := range fetches.Errors() {
		c.opts.Logger.WarnContext(ctx, "consume: fetch error", "topic", fe.Topic, "partition", fe.Partition, "err", fe.Err)
	}
	var recs []*kgo.Record
	fetches.EachRecord(func(r *kgo.Record) { recs = append(recs, r) })
	if len(recs) == 0 {
		return nil
	}
	if err := c.process(ctx, recs); err != nil {
		return err
	}
	done := handled{}
	for _, r := range recs {
		done.add(r)
	}
	if err := commitFetch(ctx, c.opts.Logger, done, c.client); err != nil {
		return fmt.Errorf("consume: commit batch of %d: %w", len(recs), err)
	}
	return nil
}

// process returns nil once the batch is handled or dead-lettered, or an error
// when ctx ends or the dead-letter write fails.
func (c *BatchConsumer) process(ctx context.Context, recs []*kgo.Record) error {
	batch := make([]Record, len(recs))
	for i, r := range recs {
		batch[i] = toRecord(r)
	}
	ctx, span := batchContext(ctx, recs)
	defer span.End()
	for attempt := 1; ; attempt++ {
		err := c.handler(ctx, batch)
		if err == nil {
			return nil
		}
		var perm permanentError
		if errors.As(err, &perm) || attempt >= c.opts.MaxAttempts {
			return c.deadLetterAll(ctx, recs, err)
		}
		c.opts.Logger.WarnContext(ctx, "consume: batch handler failed; will retry", "records", len(recs), "attempt", attempt, "err", err)
		if !sleep(ctx, backoff(c.opts.BaseBackoff, c.opts.MaxBackoff, attempt)) {
			return ctx.Err()
		}
	}
}

func (c *BatchConsumer) deadLetterAll(ctx context.Context, recs []*kgo.Record, cause error) error {
	for _, rec := range recs {
		if err := c.dlq.PublishRecord(ctx, deadLetter(c.opts.DLQTopic, c.group, rec, cause)); err != nil {
			return fmt.Errorf("consume: dead-letter %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
		}
	}
	c.opts.Logger.WarnContext(ctx, "consume: batch dead-lettered", "records", len(recs), "dlq", c.opts.DLQTopic, "err", cause)
	return nil
}

// batchContext starts a consumer span that links to the span of each record and
// sets the request id when the records agree on one.
func batchContext(ctx context.Context, recs []*kgo.Record) (context.Context, trace.Span) {
	prop := otel.GetTextMapPropagator()
	links := make([]trace.Link, 0, len(recs))
	ids := map[string]struct{}{}
	for _, rec := range recs {
		rc := prop.Extract(context.Background(), kotel.NewRecordCarrier(rec))
		if sc := trace.SpanContextFromContext(rc); sc.IsValid() {
			links = append(links, trace.Link{SpanContext: sc})
		}
		id := headerValue(rec, kafkax.HeaderRequestID)
		if !reqctx.ValidRequestID(id) {
			id = ""
		}
		ids[id] = struct{}{}
	}
	if len(ids) == 1 {
		for id := range ids {
			if id != "" {
				ctx = reqctx.WithRequestID(ctx, id)
			}
		}
	}
	return otel.Tracer("github.com/buidangphuc/platform-core/packages/go-sdk/pkg/consume").Start(ctx, "consume batch",
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithLinks(links...),
		trace.WithAttributes(attribute.Int("messaging.batch.message_count", len(recs))))
}
