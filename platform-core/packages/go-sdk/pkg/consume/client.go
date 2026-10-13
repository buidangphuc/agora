package consume

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
)

// DefaultMaxPollRecords is the poll size when Options.MaxPollRecords or
// BatchOptions.MaxPollRecords is zero.
const DefaultMaxPollRecords = 500

// commitTimeout bounds the offset commit that follows a fetch. The commit uses
// a context detached from the run context so that a graceful shutdown still
// commits the records it already handled.
const commitTimeout = 10 * time.Second

// groupClient is the part of *kgo.Client the loops use; a test seam.
type groupClient interface {
	PollRecords(context.Context, int) kgo.Fetches
	CommitRecords(context.Context, ...*kgo.Record) error
	AllowRebalance()
}

// clientGroup returns the consumer group of client. It fails for a nil client
// and for a group-less one (kafkax.NewConsumerClient with an empty group):
// offsets can only be committed to a group.
func clientGroup(client *kgo.Client) (string, error) {
	if client == nil {
		return "", errors.New("consume: client is nil")
	}
	g, _ := client.OptValue(kgo.ConsumerGroup).(string)
	if g == "" {
		return "", errors.New("consume: client has no consumer group; build it with kafkax.NewConsumerClient and a non-empty group")
	}
	return g, nil
}

// lostGeneration reports whether err says the member no longer belongs to the
// group generation it committed under: a rebalance (or a session timeout)
// replaced it while the fetch was handled. The member-epoch and instance-id
// errors are the same condition under the KIP-848 protocol and static
// membership.
func lostGeneration(err error) bool {
	for _, k := range []*kerr.Error{
		kerr.UnknownMemberID, kerr.IllegalGeneration, kerr.RebalanceInProgress,
		kerr.StaleMemberEpoch, kerr.FencedMemberEpoch, kerr.FencedInstanceID,
	} {
		if errors.Is(err, k) {
			return true
		}
	}
	return false
}

// fetchLostGeneration logs the errors of fetches and reports whether one of
// them says the member lost its generation. The records of such a fetch belong
// to partitions the group has handed to another member, so the loops skip them
// instead of handling what the new owner will redeliver.
func fetchLostGeneration(ctx context.Context, log *slog.Logger, fetches kgo.Fetches) bool {
	lost := false
	for _, fe := range fetches.Errors() {
		log.WarnContext(ctx, "consume: fetch error", "topic", fe.Topic, "partition", fe.Partition, "err", fe.Err)
		lost = lost || lostGeneration(fe.Err)
	}
	return lost
}

// commitFetch commits done. A commit that fails because the member lost its
// generation is logged and swallowed: the group has already given the
// partitions to another member, which redelivers the uncommitted records, and
// the loop must go on to rejoin rather than die. Any other error is returned.
func commitFetch(ctx context.Context, log *slog.Logger, done handled, c groupClient) error {
	err := done.commit(ctx, c)
	if err != nil && lostGeneration(err) {
		log.WarnContext(ctx, "consume: commit lost its generation; records will be redelivered", "err", err)
		return nil
	}
	return err
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
