package consume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// BatchHandler processes every record of one fetch at once, for example one
// bulk write. nil commits the batch. An error wrapped by Permanent dead-letters
// the whole batch and commits. Any other error retries the same batch after
// backoff, without committing, so the handler must be idempotent.
type BatchHandler func(ctx context.Context, records []Record) error

// BatchOptions tunes the batch loop. Zero fields take the defaults noted.
type BatchOptions struct {
	DLQTopic    string        // required
	BaseBackoff time.Duration // delay after the first failure, doubling (default 100ms)
	MaxBackoff  time.Duration // backoff ceiling (default 30s)
	Logger      *slog.Logger  // default slog.Default()
}

// BatchConsumer dispatches each fetched batch from a consumer-group client.
type BatchConsumer struct {
	poll    func(context.Context) kgo.Fetches // client.PollFetches; a test seam
	commit  func(context.Context) error       // client.CommitUncommittedOffsets; a test seam
	handler BatchHandler
	dlq     DLQProducer
	opts    BatchOptions
}

// NewBatchConsumer binds client, which must have auto-commit disabled (as
// kafkax.NewConsumerClient does), to h and dlq.
func NewBatchConsumer(client *kgo.Client, h BatchHandler, dlq DLQProducer, o BatchOptions) *BatchConsumer {
	if o.BaseBackoff <= 0 {
		o.BaseBackoff = 100 * time.Millisecond
	}
	if o.MaxBackoff <= 0 {
		o.MaxBackoff = 30 * time.Second
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return &BatchConsumer{poll: client.PollFetches, commit: client.CommitUncommittedOffsets, handler: h, dlq: dlq, opts: o}
}

// Run polls until ctx is cancelled (it then returns nil) or a permanent batch
// cannot be dead-lettered or committed. In that case Run returns the error
// without committing, so a restart in the same group delivers the batch again.
func (c *BatchConsumer) Run(ctx context.Context) error {
	for {
		fetches := c.poll(ctx)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return nil
		}
		for _, fe := range fetches.Errors() {
			c.opts.Logger.WarnContext(ctx, "consume: fetch error", "topic", fe.Topic, "partition", fe.Partition, "err", fe.Err)
		}
		var recs []*kgo.Record
		fetches.EachRecord(func(r *kgo.Record) { recs = append(recs, r) })
		if len(recs) == 0 {
			continue
		}
		if err := c.process(ctx, recs); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := c.commit(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("consume: commit batch of %d: %w", len(recs), err)
		}
	}
}

// process returns nil once the batch is handled or dead-lettered, or ctx ends.
func (c *BatchConsumer) process(ctx context.Context, recs []*kgo.Record) error {
	batch := make([]Record, len(recs))
	for i, r := range recs {
		batch[i] = toRecord(r)
	}
	for attempt := 1; ; attempt++ {
		err := c.handler(ctx, batch)
		if err == nil {
			return nil
		}
		var perm permanentError
		if errors.As(err, &perm) {
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
		if err := c.dlq.PublishRecord(ctx, deadLetter(c.opts.DLQTopic, rec, cause)); err != nil {
			return fmt.Errorf("consume: dead-letter %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
		}
	}
	c.opts.Logger.WarnContext(ctx, "consume: batch dead-lettered", "records", len(recs), "dlq", c.opts.DLQTopic, "err", cause)
	return nil
}
