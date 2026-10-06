package revocation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
)

// sessionRevokedType is the EventEnvelope.Type identity stamps on revocations.
const sessionRevokedType = "platform.identity.v1.SessionRevoked"

// fallbackTTL bounds how long an event without an expiry is kept. Identity always
// sets one; this only guards a malformed event from living forever.
const fallbackTTL = 24 * time.Hour

// Apply decodes one `identity.events` record value (an EventEnvelope) and feeds the
// denylist. Other event types are ignored. Duplicates and replays are idempotent.
// It returns an error only for a record that cannot be decoded (poison), which the
// caller logs and skips.
func Apply(d *Denylist, value []byte) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(value, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if env.GetType() != sessionRevokedType {
		return nil
	}
	var ev identityv1.SessionRevoked
	if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
		return fmt.Errorf("decode SessionRevoked: %w", err)
	}
	exp := time.Now().Add(fallbackTTL)
	if ev.GetExpiresAt() != nil {
		exp = ev.GetExpiresAt().AsTime()
	}
	d.Add(ev.GetSessionId(), exp)
	return nil
}

// Stats is a point-in-time view of the consumer for the up/lag gauges.
type Stats struct {
	Up           bool  // a broker answered the last health probe
	Lag          int64 // records behind the high watermark, summed over partitions
	DenylistSize int
}

// Consumer feeds a Denylist from the `identity.events` topic. It deliberately does
// NOT join a consumer group: franz-go's direct (group-less) consuming assigns every
// partition of the topic to this client and starts at the earliest offset, so each
// gateway replica rebuilds the FULL denylist on start (replicas never split
// revocations between them). Kafka being down never blocks startup: Run keeps
// retrying in the background and the gateway fails open meanwhile.
type Consumer struct {
	brokers []string
	topic   string
	list    *Denylist
	logger  *slog.Logger

	up  atomic.Bool
	mu  sync.Mutex
	lag map[int32]int64
}

// NewConsumer builds a consumer (it does not connect until Run).
func NewConsumer(brokers []string, topic string, list *Denylist, logger *slog.Logger) *Consumer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Consumer{brokers: brokers, topic: topic, list: list, logger: logger, lag: map[int32]int64{}}
}

// Stats returns the current gauge values.
func (c *Consumer) Stats() Stats {
	c.mu.Lock()
	var lag int64
	for _, l := range c.lag {
		lag += l
	}
	c.mu.Unlock()
	return Stats{Up: c.up.Load(), Lag: lag, DenylistSize: c.list.Len()}
}

// Run consumes until ctx is cancelled. Safe to call in a goroutine from main.
func (c *Consumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := c.runOnce(ctx); err != nil && ctx.Err() == nil {
			c.up.Store(false)
			c.logger.Warn("revocation consumer stopped; revocations are NOT enforced until it recovers (fail open)",
				slog.String("topic", c.topic), slog.Any("err", err))
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

func (c *Consumer) runOnce(ctx context.Context) error {
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(c.brokers...),
		kgo.ConsumeTopics(c.topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		return fmt.Errorf("kafka client: %w", err)
	}
	defer cl.Close()

	probeCtx, stopProbe := context.WithCancel(ctx)
	defer stopProbe()
	go c.probeLoop(probeCtx, cl)
	go c.pruneLoop(probeCtx)

	c.logger.Info("revocation consumer started (no consumer group, from earliest offset)",
		slog.Any("brokers", c.brokers), slog.String("topic", c.topic))

	for {
		fetches := cl.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return ctx.Err()
		}
		fetches.EachError(func(topic string, part int32, err error) {
			c.logger.Warn("revocation consumer fetch error",
				slog.String("topic", topic), slog.Int("partition", int(part)), slog.Any("err", err))
		})
		fetches.EachRecord(func(r *kgo.Record) {
			if err := Apply(c.list, r.Value); err != nil {
				c.logger.Warn("revocation consumer skipped an undecodable record",
					slog.Int("partition", int(r.Partition)), slog.Int64("offset", r.Offset), slog.Any("err", err))
			}
		})
		c.mu.Lock()
		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			if n := len(p.Records); n > 0 {
				c.lag[p.Partition] = p.HighWatermark - p.Records[n-1].Offset - 1
			}
		})
		c.mu.Unlock()
	}
}

// probeLoop keeps the `up` flag honest: franz-go retries a dead broker silently, so
// a periodic ApiVersions ping is what tells us Kafka is unreachable.
func (c *Consumer) probeLoop(ctx context.Context, cl *kgo.Client) {
	probe := func() {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		ok := cl.Ping(pctx) == nil
		if was := c.up.Swap(ok); was != ok && ctx.Err() == nil {
			if ok {
				c.logger.Info("revocation consumer connected to Kafka")
			} else {
				c.logger.Warn("revocation consumer cannot reach Kafka; revocations are NOT enforced (fail open)")
			}
		}
	}
	probe()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			probe()
		}
	}
}

// pruneLoop drops denylist entries whose token expiry has passed.
func (c *Consumer) pruneLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.list.Prune()
		}
	}
}
