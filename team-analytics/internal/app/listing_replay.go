package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/events"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const (
	listingReplayService = "team-analytics-listing-replay"
	replayRetryTimeout   = 15 * time.Second
	listOffsetsLatest    = -1
	listOffsetsEarliest  = -2
)

func RunListingReplay(ctx context.Context, s config.Settings) (err error) {
	logger := logx.New(s.Runtime, listingReplayService)
	slog.SetDefault(logger)

	fp, err := kafkax.NewFingerprinter(s.LogKeyFP.Secret)
	if err != nil {
		return fmt.Errorf("fingerprinter: %w", err)
	}

	var stack lifecycle.Stack
	defer func() {
		if cErr := stack.Close(context.WithoutCancel(ctx)); err == nil && cErr != nil {
			err = cErr
		}
	}()

	w, err := openWarehouse(ctx, s)
	if err != nil {
		return fmt.Errorf("open warehouse: %w", err)
	}
	stack.Push(func(context.Context) error { return w.Close() })
	lw, ok := w.(warehouse.ListingSellerWriter)
	if !ok {
		return fmt.Errorf("warehouse driver %q cannot store listing sellers", s.Warehouse.Driver)
	}

	client, err := newGroupClient(s, &stack, s.Kafka.ListingReplayGroup, []string{s.Kafka.ListingTopic},
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.RetryTimeout(replayRetryTimeout))
	if err != nil {
		return err
	}

	ends, err := listingReplayRanges(ctx, client, s.Kafka.ListingTopic)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "listing replay starting", "topic", s.Kafka.ListingTopic, "group", s.Kafka.ListingReplayGroup, "partitions", len(ends))

	handle := events.NewListingHandler(lw, fp, logger)
	var replayed int
	for len(ends) > 0 {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			return fmt.Errorf("kafka client closed")
		}
		if cErr := ctx.Err(); cErr != nil {
			client.AllowRebalance()
			return cErr
		}
		var fetchErr error
		fetches.EachError(func(topic string, p int32, e error) {
			if fetchErr == nil {
				fetchErr = fmt.Errorf("fetch %s/%d: %w", topic, p, e)
			}
		})
		if fetchErr != nil {
			client.AllowRebalance()
			return fetchErr
		}
		var batch []consume.Record
		fetches.EachRecord(func(r *kgo.Record) {
			end, tracked := ends[r.Partition]
			if !tracked || r.Offset >= end {
				return
			}
			batch = append(batch, consume.Record{
				Topic: r.Topic, Partition: r.Partition, Offset: r.Offset, Timestamp: r.Timestamp, Key: r.Key, Value: r.Value,
			})
			if r.Offset+1 >= end {
				delete(ends, r.Partition)
			}
		})
		if len(batch) > 0 {
			if hErr := handle(ctx, batch); hErr != nil {
				client.AllowRebalance()
				return hErr
			}
			replayed += len(batch)
		}
		client.AllowRebalance()
	}
	logger.InfoContext(ctx, "listing replay finished", "records", replayed)
	return nil
}

func listingReplayRanges(ctx context.Context, client *kgo.Client, topic string) (map[int32]int64, error) {
	meta := kmsg.NewPtrMetadataRequest()
	mt := kmsg.NewMetadataRequestTopic()
	mt.Topic = &topic
	meta.Topics = append(meta.Topics, mt)
	mresp, err := meta.RequestWith(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("metadata for %s: %w", topic, err)
	}
	var parts []int32
	for _, t := range mresp.Topics {
		if err := kerr.ErrorForCode(t.ErrorCode); err != nil {
			return nil, fmt.Errorf("metadata for %s: %w", topic, err)
		}
		for _, p := range t.Partitions {
			parts = append(parts, p.Partition)
		}
	}
	ends, err := listOffsets(ctx, client, topic, parts, listOffsetsLatest)
	if err != nil {
		return nil, err
	}
	starts, err := listOffsets(ctx, client, topic, parts, listOffsetsEarliest)
	if err != nil {
		return nil, err
	}
	out := make(map[int32]int64, len(ends))
	for p, end := range ends {
		if end > starts[p] {
			out[p] = end
		}
	}
	return out, nil
}

func listOffsets(ctx context.Context, client *kgo.Client, topic string, parts []int32, ts int64) (map[int32]int64, error) {
	req := kmsg.NewPtrListOffsetsRequest()
	req.ReplicaID = -1
	rt := kmsg.NewListOffsetsRequestTopic()
	rt.Topic = topic
	for _, p := range parts {
		rp := kmsg.NewListOffsetsRequestTopicPartition()
		rp.Partition = p
		rp.Timestamp = ts
		rt.Partitions = append(rt.Partitions, rp)
	}
	req.Topics = append(req.Topics, rt)
	resp, err := req.RequestWith(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("list offsets for %s: %w", topic, err)
	}
	out := make(map[int32]int64, len(parts))
	for _, t := range resp.Topics {
		for _, p := range t.Partitions {
			if err := kerr.ErrorForCode(p.ErrorCode); err != nil {
				return nil, fmt.Errorf("list offsets for %s/%d: %w", topic, p.Partition, err)
			}
			out[p.Partition] = p.Offset
		}
	}
	return out, nil
}
