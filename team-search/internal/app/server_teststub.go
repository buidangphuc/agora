package app

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-search/internal/config"
)

var errNotImplemented = errors.New("app: not implemented (TEST-STUB, replaced by server.go in task 1.7.2)")

func Run(context.Context, config.Settings) error { return errNotImplemented }

func Migrate(context.Context, config.MigrateSettings) error { return errNotImplemented }
