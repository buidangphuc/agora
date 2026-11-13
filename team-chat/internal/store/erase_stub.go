package store

import (
	"context"
	"errors"
)

// RED-FIRST STUB: the implementer deletes this file and writes Erase in chat.go.
func (r *PostgresChatRepository) Erase(ctx context.Context, userID string) (int64, error) {
	return 0, errors.New("not implemented")
}
