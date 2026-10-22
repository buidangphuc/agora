package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

type Postgres struct{}

func NewPostgres(_ *pgxpool.Pool) *Postgres { panic("TEST-STUB: store.NewPostgres not implemented") }

func (*Postgres) Create(context.Context, *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	panic("TEST-STUB")
}

func (*Postgres) Resolve(context.Context, string) (*sharelink.ShareLink, error) {
	panic("TEST-STUB")
}
