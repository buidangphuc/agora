package bootstrap

import (
	"log/slog"

	"github.com/buidangphuc/team-identity/internal/config"
	"github.com/buidangphuc/team-identity/internal/repository"
)

// TEST-STUB (task 5): compile-only. The implementer deletes this file and
// defines startSessionSweeper in lifecycle.go with the same signature.
func (res *Resources) startSessionSweeper(_ *config.Settings, _ repository.SessionRepository, _ *slog.Logger) {
}
