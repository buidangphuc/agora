package identity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-identity/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/store"
)

const UserErasedEventType = "platform.identity.v1.UserErased"

type ErasureRepository interface {
	EraseDue(ctx context.Context, now time.Time, limit int, enqueue store.ErasedEnqueueFn) (store.ErasureResult, error)
}

type ErasureSweeperConfig struct {
	Interval  time.Duration
	BatchSize int
}

type ErasureSweeper struct {
	repo   ErasureRepository
	cfg    ErasureSweeperConfig
	logger *slog.Logger
	now    func() time.Time
}

func NewErasureSweeper(repo ErasureRepository, cfg ErasureSweeperConfig, logger *slog.Logger) *ErasureSweeper {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultSweepBatchSize
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ErasureSweeper{repo: repo, cfg: cfg, logger: logger, now: time.Now}
}

func (s *ErasureSweeper) WithClock(now func() time.Time) *ErasureSweeper {
	s.now = now
	return s
}

func erasedEvent(e store.Erasure) (store.OutboxRow, error) {
	eventID := uuid.NewString()
	payload, err := proto.Marshal(&identityv1.UserErased{
		UserId:   e.UserID,
		Roles:    e.Roles,
		ErasedAt: timestamppb.New(e.ErasedAt),
	})
	if err != nil {
		return store.OutboxRow{}, fmt.Errorf("marshal UserErased: %w", err)
	}
	value, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId:    eventID,
		Type:       UserErasedEventType,
		OccurredAt: timestamppb.New(e.ErasedAt),
		Payload:    payload,
	})
	if err != nil {
		return store.OutboxRow{}, fmt.Errorf("marshal EventEnvelope: %w", err)
	}
	return store.OutboxRow{
		EventID:       eventID,
		AggregateType: "User",
		AggregateID:   e.UserID,
		EventType:     UserErasedEventType,
		Payload:       value,
	}, nil
}

func (s *ErasureSweeper) SweepOnce(ctx context.Context, now time.Time) (store.ErasureResult, error) {
	var total store.ErasureResult
	for {
		res, err := s.repo.EraseDue(ctx, now, s.cfg.BatchSize, erasedEvent)
		total.Erased += res.Erased
		total.Failed += res.Failed
		total.Skipped += res.Skipped
		if err != nil {
			return total, err
		}
		if res.Erased+res.Failed < s.cfg.BatchSize || res.Erased == 0 {
			return total, nil
		}
	}
}

func (s *ErasureSweeper) Tick(ctx context.Context) error {
	res, err := s.SweepOnce(ctx, s.now())
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil || res.Failed > 0:
		s.logger.WarnContext(ctx, "account erasure sweep had failures; will retry",
			"accounts_erased", res.Erased, "accounts_failed", res.Failed)
	case res.Erased > 0:
		s.logger.InfoContext(ctx, "account erasure sweep done", "accounts_erased", res.Erased)
	}
	return nil
}
