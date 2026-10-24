package app

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-promotion/internal/config"
)

var errNotImplemented = errors.New("app: not implemented")

func Execute(context.Context, []string) error { return errNotImplemented }

func Run(context.Context, config.Settings) error { return errNotImplemented }

func Migrate(context.Context) error { return errNotImplemented }
