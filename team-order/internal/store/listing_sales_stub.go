package store

import (
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

type ListingSalesChange struct {
	ListingID     string
	SoldCount     int64
	ChangedAt     time.Time
	SourceEventID string
}

type ListingSalesOutboxBuilder func(change ListingSalesChange) (outbox.Row, error)

func WithListingSalesOutbox(b ListingSalesOutboxBuilder) OrderRepoOption {
	return func(*orderRepoConfig) {}
}

func WithReturnListingSalesOutbox(b ListingSalesOutboxBuilder) ReturnRepoOption {
	return func(*returnRepoConfig) {}
}
