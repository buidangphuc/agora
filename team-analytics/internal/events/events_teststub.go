package events

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

var (
	ErrDeadLetter            = errors.New("dead letter")
	ErrUnknownEngagementType = errors.New("unknown engagement envelope type")
	errNotImplemented        = errors.New("not implemented")
)

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

type RedactingDLQ struct{}

func (*RedactingDLQ) PublishRecord(context.Context, *kgo.Record) error { return errNotImplemented }

func NewRedactingDLQ(consume.DLQProducer, *kafkax.Fingerprinter, *slog.Logger) *RedactingDLQ {
	return &RedactingDLQ{}
}

func NewBatchHandler(warehouse.WarehouseWriter, *kafkax.Fingerprinter, *slog.Logger) consume.BatchHandler {
	return func(context.Context, []consume.Record) error { return errNotImplemented }
}

func NewListingHandler(warehouse.ListingSellerWriter, *kafkax.Fingerprinter, *slog.Logger) consume.BatchHandler {
	return func(context.Context, []consume.Record) error { return errNotImplemented }
}

func NewEngagementHandler(warehouse.EngagementFactWriter, *kafkax.Fingerprinter, *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errNotImplemented }
}

func MainOptions(config.Settings) BatchSpec { return BatchSpec{} }

func ListingSpec(config.Settings) BatchSpec { return BatchSpec{} }

func EngagementOptions(config.Settings) RecordSpec { return RecordSpec{} }
