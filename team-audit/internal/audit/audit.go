package audit

import (
	"context"
	"errors"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
)

var (
	ErrEmptyAction      = errors.New("action is required")
	ErrInvalidTimeRange = errors.New("created_after must not be after created_before")
)

const (
	MaxPageSize     = 100
	DefaultPageSize = 20
)

type Event struct {
	ID         string
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]string
	CreatedAt  time.Time
}

type Filter struct {
	ActorID, TargetType, TargetID, Action string
	CreatedAfter, CreatedBefore           time.Time
	Limit, Offset                         int
}

type Repository interface {
	Append(ctx context.Context, ev Event) (Event, error)
	Query(ctx context.Context, f Filter) ([]Event, int64, error)
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Write(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) (Event, error) {
	if action == "" {
		return Event{}, ErrEmptyAction
	}
	return s.repo.Append(ctx, Event{
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Metadata:   metadata,
	})
}

func (s *Service) Query(ctx context.Context, f Filter) ([]Event, int64, error) {
	if !f.CreatedAfter.IsZero() && !f.CreatedBefore.IsZero() && f.CreatedAfter.After(f.CreatedBefore) {
		return nil, 0, ErrInvalidTimeRange
	}
	f.Limit = pagination.Clamp(f.Limit, DefaultPageSize, MaxPageSize)
	if f.Offset < 0 {
		f.Offset = 0
	}
	return s.repo.Query(ctx, f)
}
