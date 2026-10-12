package consume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	// HeaderError carries the last handler error on a dead-letter record.
	HeaderError = "error"
	// HeaderSourceTopic carries the topic a dead-letter record came from.
	HeaderSourceTopic = "source-topic"
)

// Record is one consumed message.
type Record struct {
	Topic   string
	Key     []byte
	Value   []byte
	Headers map[string]string
	Offset  int64
}

// Handler processes one record. A returned error is retried with backoff up to
// Options.MaxAttempts, then the record is dead-lettered. An error wrapped by
// Permanent skips the retries.
type Handler func(ctx context.Context, r Record) error

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying: the record goes straight to the
// dead-letter topic. Permanent(nil) is nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

// Options tunes the loop. Zero fields take the defaults noted.
type Options struct {
	DLQTopic    string        // required
	MaxAttempts int           // handler attempts per record (default 5)
	BaseBackoff time.Duration // delay after the first failure, doubling (default 100ms)
	MaxBackoff  time.Duration // backoff ceiling (default 30s)
	Logger      *slog.Logger  // default slog.Default()
}

// DLQProducer publishes a built dead-letter record. *kafkax.Producer satisfies it.
type DLQProducer interface {
	PublishRecord(ctx context.Context, r *kgo.Record) error
}

// Consumer dispatches records from a consumer-group client.
type Consumer struct {
	client  *kgo.Client
	handler Handler
	dlq     DLQProducer
	opts    Options
}

// NewConsumer binds client, which must have auto-commit disabled (as
// kafkax.NewConsumerClient does), to h and dlq.
func NewConsumer(client *kgo.Client, h Handler, dlq DLQProducer, o Options) *Consumer {
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
	return &Consumer{client: client, handler: h, dlq: dlq, opts: o}
}

// Run polls until ctx is cancelled (it then returns nil) or a record can be
// neither handled nor dead-lettered. In that case Run returns the error without
// committing the record, so a restart in the same group delivers it again.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil || fetches.IsClientClosed() {
			return nil
		}
		for _, fe := range fetches.Errors() {
			c.opts.Logger.WarnContext(ctx, "consume: fetch error", "topic", fe.Topic, "partition", fe.Partition, "err", fe.Err)
		}
		iter := fetches.RecordIter()
		for !iter.Done() {
			rec := iter.Next()
			if err := c.process(ctx, rec); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
			if err := c.client.CommitRecords(ctx, rec); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("consume: commit %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
			}
		}
	}
}

// process returns nil once rec is handled or dead-lettered.
func (c *Consumer) process(ctx context.Context, rec *kgo.Record) error {
	r := toRecord(rec)
	var last error
	for attempt := 1; attempt <= c.opts.MaxAttempts; attempt++ {
		last = c.handler(ctx, r)
		if last == nil {
			return nil
		}
		var perm permanentError
		if errors.As(last, &perm) {
			break
		}
		c.opts.Logger.WarnContext(ctx, "consume: handler failed", "topic", rec.Topic, "offset", rec.Offset, "attempt", attempt, "err", last)
		if attempt < c.opts.MaxAttempts && !sleep(ctx, c.backoff(attempt)) {
			return ctx.Err()
		}
	}
	if err := c.dlq.PublishRecord(ctx, deadLetter(c.opts.DLQTopic, rec, last)); err != nil {
		return fmt.Errorf("consume: dead-letter %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
	}
	c.opts.Logger.WarnContext(ctx, "consume: record dead-lettered", "topic", rec.Topic, "offset", rec.Offset, "dlq", c.opts.DLQTopic, "err", last)
	return nil
}

func (c *Consumer) backoff(attempt int) time.Duration {
	d := c.opts.BaseBackoff
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= c.opts.MaxBackoff {
			return c.opts.MaxBackoff
		}
	}
	return min(d, c.opts.MaxBackoff)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func toRecord(rec *kgo.Record) Record {
	h := make(map[string]string, len(rec.Headers))
	for _, x := range rec.Headers {
		h[x.Key] = string(x.Value)
	}
	return Record{Topic: rec.Topic, Key: rec.Key, Value: rec.Value, Headers: h, Offset: rec.Offset}
}

func deadLetter(topic string, rec *kgo.Record, cause error) *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(rec.Headers)+2)
	headers = append(headers, rec.Headers...)
	headers = append(headers,
		kgo.RecordHeader{Key: HeaderError, Value: []byte(cause.Error())},
		kgo.RecordHeader{Key: HeaderSourceTopic, Value: []byte(rec.Topic)},
	)
	return &kgo.Record{Topic: topic, Key: rec.Key, Value: rec.Value, Headers: headers}
}
