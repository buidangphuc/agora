package identity

import (
	"context"
	"time"
)

type HandoffPurger interface {
	PurgeHandoffCodes(ctx context.Context, now time.Time, limit int) (int, error)
}

func (s *SessionSweeper) WithHandoffPurger(p HandoffPurger) *SessionSweeper { return s }
