package app

import (
	"context"

	"github.com/buidangphuc/team-sharing/internal/config"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

// TEST-STUB: compile-only placeholder for task 1.6. The implementer deletes this
// file and provides Run and RunWithRepository in app.go.
//
// RunWithRepository is identical to Run (postgres is still opened, health still
// probes it) except that the share-link repository is the given one instead of
// store.NewPostgres(pool). It is the seam TestPanicIsContained needs.

func Run(ctx context.Context, _ config.Settings) error {
	<-ctx.Done()
	return nil
}

func RunWithRepository(ctx context.Context, _ config.Settings, _ sharelink.Repository) error {
	<-ctx.Done()
	return nil
}
