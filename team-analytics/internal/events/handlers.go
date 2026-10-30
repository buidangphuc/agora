package events

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const engagementMaxPollRecords = 10

type BatchSpec struct {
	Topics          []string
	Group           string
	StartAtEarliest bool
	FetchMaxWait    time.Duration
	Options         consume.BatchOptions
}

type RecordSpec struct {
	Topics          []string
	Group           string
	StartAtEarliest bool
	Options         consume.Options
}

func MainOptions(s config.Settings) BatchSpec {
	return BatchSpec{
		Topics:       []string{s.Kafka.AnalyticsTopic, s.Kafka.OrderTopic},
		Group:        s.Kafka.ConsumerGroup,
		FetchMaxWait: time.Duration(s.Batch.FlushIntervalSeconds) * time.Second,
		Options: consume.BatchOptions{
			DLQTopic:       s.Kafka.DLQTopic,
			MaxPollRecords: s.Batch.MaxSize,
		},
	}
}

func ListingSpec(s config.Settings) BatchSpec {
	return BatchSpec{
		Topics:          []string{s.Kafka.ListingTopic},
		Group:           s.Kafka.ListingConsumerGroup,
		StartAtEarliest: true,
		Options:         consume.BatchOptions{DLQTopic: s.Kafka.ListingDLQTopic},
	}
}

func EngagementOptions(s config.Settings) RecordSpec {
	return RecordSpec{
		Topics: []string{s.Engagement.EventsTopic},
		Group:  s.Engagement.ConsumerGroup,
		Options: consume.Options{
			DLQTopic:       s.Engagement.DLQTopic,
			MaxPollRecords: engagementMaxPollRecords,
		},
	}
}

func NewBatchHandler(w warehouse.WarehouseWriter, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.BatchHandler {
	return func(ctx context.Context, records []consume.Record) error {
		var tracking []*warehouse.TrackingRecord
		var facts []*warehouse.OrderFactRecord
		var failures int64
		for _, rec := range records {
			tr, okTr, errTr := RecordFromEnvelope(rec.Value)
			if okTr {
				tracking = append(tracking, tr)
				continue
			}
			of, okOf, errOf := OrderFactsFromEnvelope(rec.Value)
			if okOf {
				facts = append(facts, of...)
				continue
			}
			if errTr != nil || errOf != nil {
				failures++
				logger.WarnContext(ctx, "decode record failed; skipping", "key_fp", fp.Of(rec.Key), "tracking_err", errTr, "order_err", errOf)
				continue
			}
			logger.DebugContext(ctx, "skipping unknown envelope", "key_fp", fp.Of(rec.Key))
		}
		if failures > 0 {
			if cw, ok := w.(warehouse.IngestCounterWriter); ok {
				if err := cw.RecordDecodeFailures(ctx, time.Now(), failures); err != nil {
					logger.WarnContext(ctx, "decode failure counter not updated", "err", err)
				}
			}
		}
		if len(tracking) > 0 {
			if err := w.Write(ctx, tracking); err != nil {
				return fmt.Errorf("write tracking events: %w", err)
			}
		}
		if len(facts) > 0 {
			if err := w.WriteOrderFacts(ctx, facts); err != nil {
				return fmt.Errorf("write order facts: %w", err)
			}
		}
		return nil
	}
}

func NewListingHandler(w warehouse.ListingSellerWriter, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.BatchHandler {
	return func(ctx context.Context, records []consume.Record) error {
		var batch []*warehouse.ListingSellerRecord
		for _, rec := range records {
			m, ok, err := ListingSellerFromEnvelope(rec.Value)
			if err != nil {
				logger.WarnContext(ctx, "decode listing record failed; skipping", "key_fp", fp.Of(rec.Key), "err", err)
				continue
			}
			if ok {
				batch = append(batch, m)
			}
		}
		if len(batch) == 0 {
			return nil
		}
		if err := w.UpsertListingSellers(ctx, batch); err != nil {
			return fmt.Errorf("upsert listing sellers: %w", err)
		}
		return nil
	}
}

func NewEngagementHandler(w warehouse.EngagementFactWriter, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.Handler {
	return func(ctx context.Context, rec consume.Record) error {
		row, err := EngagementFactFromEnvelope(rec.Value)
		if err != nil {
			logger.WarnContext(ctx, "engagement record dead-lettered", "key_fp", fp.Of(rec.Key), "err", err)
			return consume.Permanent(fmt.Errorf("%w: %w", ErrDeadLetter, err))
		}
		if err := w.WriteEngagementFacts(ctx, []*warehouse.EngagementFactRecord{row}); err != nil {
			return fmt.Errorf("write engagement fact: %w", err)
		}
		return nil
	}
}
