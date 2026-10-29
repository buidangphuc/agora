package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-identity/internal/config"
)

// TEST-STUB: compile-only placeholder for task 1.9. The implementer deletes this
// file and writes app.go with the same unexported contract:
//
//	build(ctx, s, pub) assembles everything Run needs (publisher nil = kafkax producer),
//	starts no listener and runs nothing; run(ctx) is the lifecycle.Run call that Run
//	makes (it closes the stack itself); closeAll releases the stack when run is not used.
var errTestStub = errors.New("TEST-STUB: task 1.9 not implemented")

type built struct {
	pool      *pgxpool.Pool
	health    *healthx.Health
	relayer   *outbox.Relayer
	purgeTick func(context.Context) error
	run       func(context.Context) error
	closeAll  func(context.Context) error
}

func Migrate(context.Context, config.MigrateSettings) error { return nil }

func Run(context.Context, config.Settings) error { return nil }

func build(context.Context, config.Settings, outbox.Publisher) (*built, error) {
	return nil, errTestStub
}
