package revocation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
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

const (
	probeInterval = 5 * time.Second
	probeTimeout  = 3 * time.Second
	retryDelay    = 2 * time.Second
)

// Stats is a point-in-time view of the consumer for the up/lag gauges.
type Stats struct {
	Up           bool
	Lag          int64
	DenylistSize int
}

// Consumer feeds a Denylist from the identity events topic with a group-less
// client: every replica reads every partition from the earliest offset, so no
// consumer group exists and each replica rebuilds the full denylist on start.
// Kafka being down never blocks startup: Run keeps retrying and the gateway
// fails open meanwhile.
type Consumer struct {
	client *kgo.Client
	topic  string
	list   *Denylist
	logger *slog.Logger

	up       atomic.Bool
	caughtUp atomic.Bool
	mu       sync.Mutex
	lag      map[int32]int64
}

// NewConsumer builds a consumer; it does not connect until Run.
func NewConsumer(kafka config.Kafka, topic string, list *Denylist, logger *slog.Logger) (*Consumer, error) {
	if logger == nil {
		logger = slog.Default()
	}
	cl, err := kafkax.NewConsumerClient(kafka, "", []string{topic})
	if err != nil {
		return nil, fmt.Errorf("revocation consumer: %w", err)
	}
	return &Consumer{client: cl, topic: topic, list: list, logger: logger, lag: map[int32]int64{}}, nil
}

// CaughtUp reports whether the consumer has applied every record that existed
// when it first reached the broker. It is a latch: it never returns to false.
func (c *Consumer) CaughtUp() bool { return c.caughtUp.Load() }

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

// Run consumes until ctx is cancelled and then returns nil.
func (c *Consumer) Run(ctx context.Context) error {
	defer c.client.Close()

	go c.probeLoop(ctx)
	go c.pruneLoop(ctx)

	ends := c.captureEnds(ctx)
	if ctx.Err() != nil {
		return nil
	}
	c.logger.InfoContext(ctx, "revocation consumer started (no consumer group, from earliest offset)",
		"topic", c.topic)

	pending := map[int32]int64{}
	for p, end := range ends {
		if end > 0 {
			pending[p] = end
		}
	}
	if len(pending) == 0 {
		c.caughtUp.Store(true)
	}

	for {
		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			return nil
		}
		fetches.EachError(func(topic string, part int32, err error) {
			c.logger.WarnContext(ctx, "revocation consumer fetch error", "topic", topic, "partition", int(part), "err", err)
		})
		fetches.EachRecord(func(r *kgo.Record) {
			if err := Apply(c.list, r.Value); err != nil {
				c.logger.WarnContext(ctx, "revocation consumer skipped an undecodable record",
					"partition", int(r.Partition), "offset", r.Offset, "err", err)
			}
			if end, ok := pending[r.Partition]; ok && r.Offset+1 >= end {
				delete(pending, r.Partition)
			}
		})
		c.mu.Lock()
		fetches.EachPartition(func(p kgo.FetchTopicPartition) {
			if n := len(p.Records); n > 0 {
				c.lag[p.Partition] = p.HighWatermark - p.Records[n-1].Offset - 1
			}
		})
		c.mu.Unlock()
		if len(pending) == 0 {
			c.caughtUp.Store(true)
		}
	}
}

func (c *Consumer) captureEnds(ctx context.Context) map[int32]int64 {
	for ctx.Err() == nil {
		ends, err := c.endOffsets(ctx)
		if err == nil {
			return ends
		}
		c.logger.WarnContext(ctx, "revocation consumer cannot read end offsets; revocations are NOT enforced until it recovers (fail open)",
			"topic", c.topic, "err", err)
		select {
		case <-ctx.Done():
		case <-time.After(retryDelay):
		}
	}
	return nil
}

func (c *Consumer) endOffsets(ctx context.Context) (map[int32]int64, error) {
	rctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	mreq := kmsg.NewPtrMetadataRequest()
	mt := kmsg.NewMetadataRequestTopic()
	mt.Topic = kmsg.StringPtr(c.topic)
	mreq.Topics = append(mreq.Topics, mt)
	mresp, err := mreq.RequestWith(rctx, c.client)
	if err != nil {
		return nil, err
	}
	lreq := kmsg.NewPtrListOffsetsRequest()
	lreq.ReplicaID = -1
	lt := kmsg.NewListOffsetsRequestTopic()
	lt.Topic = c.topic
	for _, t := range mresp.Topics {
		if err := kerr.ErrorForCode(t.ErrorCode); err != nil {
			return nil, err
		}
		for _, p := range t.Partitions {
			lp := kmsg.NewListOffsetsRequestTopicPartition()
			lp.Partition = p.Partition
			lp.Timestamp = -1
			lp.CurrentLeaderEpoch = -1
			lt.Partitions = append(lt.Partitions, lp)
		}
	}
	lreq.Topics = append(lreq.Topics, lt)
	lresp, err := lreq.RequestWith(rctx, c.client)
	if err != nil {
		return nil, err
	}
	ends := map[int32]int64{}
	for _, t := range lresp.Topics {
		for _, p := range t.Partitions {
			if err := kerr.ErrorForCode(p.ErrorCode); err != nil {
				return nil, err
			}
			ends[p.Partition] = p.Offset
		}
	}
	return ends, nil
}

func (c *Consumer) probeLoop(ctx context.Context) {
	probe := func() {
		pctx, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()
		ok := c.client.Ping(pctx) == nil
		if was := c.up.Swap(ok); was != ok && ctx.Err() == nil {
			if ok {
				c.logger.InfoContext(ctx, "revocation consumer connected to Kafka")
			} else {
				c.logger.WarnContext(ctx, "revocation consumer cannot reach Kafka; revocations are NOT enforced (fail open)")
			}
		}
	}
	probe()
	t := time.NewTicker(probeInterval)
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
