package audit

import (
	"context"
	"errors"
	"time"
)

var (
	ErrEmptyAction      = errors.New("action is required")
	ErrInvalidTimeRange = errors.New("created_after must not be after created_before")
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

type Service struct{}

func NewService(repo Repository) *Service { panic("not implemented") }

func (s *Service) Write(ctx context.Context, actorID, action, targetType, targetID string, metadata map[string]string) (Event, error) {
	panic("not implemented")
}

func (s *Service) Query(ctx context.Context, f Filter) ([]Event, int64, error) {
	panic("not implemented")
}
