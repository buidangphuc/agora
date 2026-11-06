package store

import (
	"context"
	"embed"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
)

const outboxTable = "identity_outbox_events"

const outboxAggregateType = "User"

//go:embed migrations/*.sql
var migrationFiles embed.FS

func Migrations() fs.FS {
	sub, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

type OutboxRow = outbox.Row

type Registration struct {
	UserID       string
	ReferralCode string
	RegisteredAt time.Time
}

type RegisteredEnqueueFn func(Registration) (OutboxRow, error)

type EnqueueFn func(revoked Session) (OutboxRow, error)

func Outbox(pool *pgxpool.Pool) (*outbox.Store, error) {
	return outbox.NewStore(pool, outboxTable)
}

func enqueueTx(ctx context.Context, ob *outbox.Store, tx pgx.Tx, row OutboxRow) error {
	row.AggregateType = outboxAggregateType
	return ob.EnqueueTx(ctx, tx, row)
}
