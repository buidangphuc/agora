package store

import (
	"context"
	"errors"
	"fmt"
)

type Eraser struct{ q Querier }

func NewEraser(q Querier) *Eraser { return &Eraser{q: q} }

func (e *Eraser) Erase(ctx context.Context, userID string) (int64, error) {
	if userID == "" {
		return 0, errors.New("erase: empty user id")
	}
	var total int64
	err := e.q.QueryRow(ctx, `WITH n AS (DELETE FROM notifications WHERE user_id = $1 RETURNING 1),
		a AS (DELETE FROM alert_subscriptions WHERE user_id = $1 RETURNING 1),
		p AS (DELETE FROM notification_prefs WHERE user_id = $1 RETURNING 1)
		SELECT (SELECT count(*) FROM n) + (SELECT count(*) FROM a) + (SELECT count(*) FROM p)`, userID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("erase notification data: %w", err)
	}
	return total, nil
}
