package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

// RED-FIRST STUB: the implementer deletes this file and writes identity.go.
type UserEraser interface {
	Erase(ctx context.Context, userID string) (int64, error)
}

func IdentityEventHandler(store UserEraser, fp *kafkax.Fingerprinter, logger *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errors.New("not implemented") }
}
