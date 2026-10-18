package repository

// TEST-STUB (task 2, red phase): compile-only placeholders so session_purge*_test.go build.
// The implementer DELETES this file when adding the real methods to session.go.

import (
	"context"
	"time"
)

func (r *PostgresSessionRepository) PurgeSessions(ctx context.Context, now time.Time, retention time.Duration, limit int) (int, error) {
	return 0, nil
}

func (r *PostgresSessionRepository) PurgeLoginHistory(ctx context.Context, before time.Time, limit int) (int, error) {
	return 0, nil
}

func (r *InMemorySessionRepository) PurgeSessions(ctx context.Context, now time.Time, retention time.Duration, limit int) (int, error) {
	return 0, nil
}

func (r *InMemorySessionRepository) PurgeLoginHistory(ctx context.Context, before time.Time, limit int) (int, error) {
	return 0, nil
}
