package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"

	"github.com/buidangphuc/team-audit/internal/config"
)

// TEST-STUB: compile-only placeholder for task 1.6, replaced by app.go.

type built struct {
	pool     *pgxpool.Pool
	health   *healthx.Health
	srv      *grpc.Server
	closeAll func(context.Context) error
}

func Run(context.Context, config.Settings) error { return nil }

func Migrate(context.Context, config.Settings) error { return nil }

func build(context.Context, config.Settings) (*built, error) { return &built{}, nil }
