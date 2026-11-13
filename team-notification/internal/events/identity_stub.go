package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

type UserEraser interface {
	Erase(ctx context.Context, userID string) (int64, error)
}

func IdentityEventHandler(_ UserEraser, _ *kafkax.Fingerprinter, _ *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errors.New("not implemented") }
}
