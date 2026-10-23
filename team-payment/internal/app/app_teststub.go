package app

import (
	"context"
	"errors"

	"github.com/buidangphuc/team-payment/internal/config"
)

func Run(_ context.Context, _ config.Settings) error {
	return errors.New("TEST-STUB: app.Run not implemented")
}

func Migrate(_ context.Context) error { return errors.New("TEST-STUB: app.Migrate not implemented") }
