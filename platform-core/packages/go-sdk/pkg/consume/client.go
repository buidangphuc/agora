package consume

import (
	"context"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// commitTimeout bounds the offset commit that follows a fetch. The commit uses
// a context detached from the run context so that a graceful shutdown still
// commits the records it already handled.
const commitTimeout = 10 * time.Second

// groupClient is the part of *kgo.Client the loops use; a test seam.
type groupClient interface {
	PollFetches(context.Context) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
}

// groupName returns the consumer group of client, "" when it has none.
func groupName(client *kgo.Client) string {
	g, _ := client.OptValue(kgo.ConsumerGroup).(string)
	return g
}

type partitionKey struct {
	topic     string
	partition int32
}

// handled tracks the highest handled record of each partition, so a commit
// never moves an offset backwards.
type handled map[partitionKey]*kgo.Record

func (h handled) add(rec *kgo.Record) {
	k := partitionKey{rec.Topic, rec.Partition}
	if cur, ok := h[k]; !ok || rec.Offset > cur.Offset {
		h[k] = rec
	}
}

// commit commits the handled records with one request. An empty set commits
// nothing.
func (h handled) commit(ctx context.Context, c groupClient) error {
	if len(h) == 0 {
		return nil
	}
	recs := make([]*kgo.Record, 0, len(h))
	for _, r := range h {
		recs = append(recs, r)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), commitTimeout)
	defer cancel()
	return c.CommitRecords(ctx, recs...)
}
