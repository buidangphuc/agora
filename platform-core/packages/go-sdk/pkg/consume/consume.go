package consume

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/plugin/kotel"
	"go.opentelemetry.io/otel"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

// Header is one record header. A record may carry several with the same key.
type Header struct {
	Key   string
	Value []byte
}

// Record is one consumed message.
type Record struct {
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
	Key       []byte
	Value     []byte
	Headers   []Header
}

// Header returns the value of the first header named key.
func (r Record) Header(key string) ([]byte, bool) {
	for _, h := range r.Headers {
		if h.Key == key {
			return h.Value, true
		}
	}
	return nil, false
}

// Handler processes one record. Its ctx carries the W3C trace context and the
// request id the producer stamped on the record (see kafkax.Producer), so the
// handler's spans and logs join the producing request. A returned error is retried with backoff up to
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
	DLQTopic string // required; NewConsumer fails when empty
	// MaxPollRecords caps the records of one poll, and so of one commit
	// (default 500). Rebalances are blocked while a poll is handled, so
	// MaxPollRecords x MaxAttempts x handler time (plus backoff) must fit the
	// group's rebalance timeout; otherwise the member is kicked and the poll's
	// records are redelivered to another member.
	MaxPollRecords int
	MaxAttempts    int           // handler attempts per record (default 5)
	BaseBackoff    time.Duration // delay after the first failure, doubling (default 100ms)
	MaxBackoff     time.Duration // backoff ceiling (default 30s)
	Logger         *slog.Logger  // default slog.Default()
}

// DLQProducer publishes a built dead-letter record. *kafkax.Producer satisfies it.
type DLQProducer interface {
	PublishRecord(ctx context.Context, r *kgo.Record) error
}

// Consumer dispatches records from a consumer-group client.
type Consumer struct {
	client  groupClient
	group   string
	handler Handler
	dlq     DLQProducer
	opts    Options
}

// NewConsumer binds client, which must have auto-commit disabled (as
// kafkax.NewConsumerClient does), to h and dlq. It fails when client, h or dlq
// is nil, Options.DLQTopic is empty or client has no consumer group (its
// commits would fail).
func NewConsumer(client *kgo.Client, h Handler, dlq DLQProducer, o Options) (*Consumer, error) {
	group, err := clientGroup(client)
	if err != nil {
		return nil, err
	}
	return newConsumer(client, group, h, dlq, o)
}

func newConsumer(client groupClient, group string, h Handler, dlq DLQProducer, o Options) (*Consumer, error) {
	switch {
	case h == nil:
		return nil, errors.New("consume: handler is nil")
	case dlq == nil:
		return nil, errors.New("consume: dead-letter producer is nil")
	case o.DLQTopic == "":
		return nil, errors.New("consume: Options.DLQTopic is required")
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
	return &Consumer{client: client, group: group, handler: h, dlq: dlq, opts: o}, nil
}

// Run polls until ctx is cancelled (it then returns nil) or a record can be
// neither handled nor dead-lettered. In that case Run returns the error without
// committing that record, so a restart in the same group delivers it again.
//
// Each poll returns at most Options.MaxPollRecords records. Offsets are
// committed once per fetch, after every record of it was handled or
// dead-lettered, never earlier. Run relies on kgo.BlockRebalanceOnPoll (set by
// kafkax.NewConsumerClient): rebalances wait until the fetch is committed and
// Run calls AllowRebalance, so a revoked partition is never committed after its
// new owner started, and no commit rewinds an offset.
func (c *Consumer) Run(ctx context.Context) error {
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

// fetch handles one polled fetch, commits what was handled and then allows
// rebalances again.
//
// Pattern from the franz-go docs: BlockRebalanceOnPoll "by blocking rebalancing
// after you poll until you call AllowRebalances, you can be sure that you commit
// records that your member currently owns" (kgo.BlockRebalanceOnPoll; see also
// kgo.DisableAutoCommit and Client.CommitRecords on committing across rebalances).
//
// The same docs warn that a blocked rebalance puts the member "at risk of
// waiting so long that the group member is kicked from the group because it
// exceeded the rebalance timeout", and recommend "PollRecords rather than
// PollFetches so that you can bound how many records you process at once":
// Run therefore polls at most Options.MaxPollRecords records. If the member is
// kicked anyway, the commit fails with a lost-generation error; that is logged
// and the loop goes on, since the new owner redelivers the uncommitted records.
// A fetch that reports a lost generation is skipped without handling, and a
// cancelled ctx stops the fetch before its next record (what was handled is
// still committed).
func (c *Consumer) fetch(ctx context.Context, fetches kgo.Fetches) error {
	defer c.client.AllowRebalance()
	if fetchLostGeneration(ctx, c.opts.Logger, fetches) {
		c.opts.Logger.WarnContext(ctx, "consume: generation lost; fetch skipped, records will be redelivered")
		return nil
	}
	done := handled{}
	var failure error
	iter := fetches.RecordIter()
	for !iter.Done() && ctx.Err() == nil {
		rec := iter.Next()
		if err := c.process(ctx, rec); err != nil {
			failure = err
			break
		}
		done.add(rec)
	}
	// Records handled before a failure are committed; the failed record never is.
	if err := commitFetch(ctx, c.opts.Logger, done, c.client); err != nil && failure == nil {
		return fmt.Errorf("consume: commit offsets: %w", err)
	}
	return failure
}

// process returns nil once rec is handled or dead-lettered.
func (c *Consumer) process(ctx context.Context, rec *kgo.Record) error {
	r := toRecord(rec)
	hctx := recordContext(ctx, rec)
	var last error
	for attempt := 1; attempt <= c.opts.MaxAttempts; attempt++ {
		last = c.handler(hctx, r)
		if last == nil {
			return nil
		}
		var perm permanentError
		if errors.As(last, &perm) {
			break
		}
		c.opts.Logger.WarnContext(hctx, "consume: handler failed", "topic", rec.Topic, "offset", rec.Offset, "attempt", attempt, "err", last)
		if attempt < c.opts.MaxAttempts && !sleep(ctx, c.backoff(attempt)) {
			return ctx.Err()
		}
	}
	if err := c.dlq.PublishRecord(hctx, deadLetter(c.opts.DLQTopic, c.group, rec, last)); err != nil {
		return fmt.Errorf("consume: dead-letter %s[%d]@%d: %w", rec.Topic, rec.Partition, rec.Offset, err)
	}
	c.opts.Logger.WarnContext(hctx, "consume: record dead-lettered", "topic", rec.Topic, "offset", rec.Offset, "dlq", c.opts.DLQTopic, "err", last)
	return nil
}

func (c *Consumer) backoff(attempt int) time.Duration {
	return backoff(c.opts.BaseBackoff, c.opts.MaxBackoff, attempt)
}

func backoff(base, ceiling time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= ceiling {
			return ceiling
		}
	}
	return min(d, ceiling)
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
	h := make([]Header, len(rec.Headers))
	for i, x := range rec.Headers {
		h[i] = Header{Key: x.Key, Value: x.Value}
	}
	return Record{
		Topic: rec.Topic, Partition: rec.Partition, Offset: rec.Offset, Timestamp: rec.Timestamp,
		Key: rec.Key, Value: rec.Value, Headers: h,
	}
}

// recordContext derives the handler context of rec from ctx: the W3C trace
// context from the global propagator (kafkax.Producer injects it on produce)
// and the request id from the x-request-id header when it is valid
// (reqctx.ValidRequestID); it ends up in logs, so a malformed one is dropped.
func recordContext(ctx context.Context, rec *kgo.Record) context.Context {
	ctx = otel.GetTextMapPropagator().Extract(ctx, kotel.NewRecordCarrier(rec))
	if id := headerValue(rec, kafkax.HeaderRequestID); reqctx.ValidRequestID(id) {
		ctx = reqctx.WithRequestID(ctx, id)
	}
	return ctx
}

func headerValue(rec *kgo.Record, key string) string {
	for _, h := range rec.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

// MaxDLQErrorBytes caps the dlq-error header value; a longer handler error is
// cut at a UTF-8 boundary so a huge error cannot exceed the broker's record
// size limit and make the dead-letter write itself fail.
const MaxDLQErrorBytes = 4096

func truncateError(msg string) string {
	if len(msg) <= MaxDLQErrorBytes {
		return msg
	}
	n := MaxDLQErrorBytes
	for n > 0 && !utf8.RuneStart(msg[n]) {
		n--
	}
	return msg[:n]
}

// deadLetter builds the dead-letter record of rec. The source headers are
// copied except dlq-*: a record that was replayed from a dead-letter topic
// carries the previous failure's, which the new ones replace.
func deadLetter(topic, group string, rec *kgo.Record, cause error) *kgo.Record {
	headers := make([]kgo.RecordHeader, 0, len(rec.Headers)+5)
	for _, h := range rec.Headers {
		if !strings.HasPrefix(h.Key, "dlq-") {
			headers = append(headers, h)
		}
	}
	headers = append(headers,
		kgo.RecordHeader{Key: kafkax.HeaderDLQError, Value: []byte(truncateError(cause.Error()))},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourceTopic, Value: []byte(rec.Topic)},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourcePartition, Value: []byte(strconv.FormatInt(int64(rec.Partition), 10))},
		kgo.RecordHeader{Key: kafkax.HeaderDLQSourceOffset, Value: []byte(strconv.FormatInt(rec.Offset, 10))},
	)
	if group != "" {
		headers = append(headers, kgo.RecordHeader{Key: kafkax.HeaderDLQSourceGroup, Value: []byte(group)})
	}
	return &kgo.Record{Topic: topic, Key: rec.Key, Value: rec.Value, Headers: headers}
}
