package store

import (
	"context"
	"embed"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

//go:embed migrations/*.sql
var Migrations embed.FS

var (
	ErrNotFound             = errors.New("notification not found")
	ErrPrefsNotFound        = errors.New("notification prefs not found")
	ErrSubscriptionNotFound = errors.New("alert subscription not found")
)

type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
