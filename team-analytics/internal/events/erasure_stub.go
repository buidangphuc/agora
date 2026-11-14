package events

import (
	"context"
	"errors"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// RED-STUB (analytics-user-erasure/T1): compile-only; the implementer replaces it.

// UserErasedEventType is the EventEnvelope.Type of the erasure event.
const UserErasedEventType = "platform.identity.v1.UserErased"

// NewErasureHandler is not implemented yet.
func NewErasureHandler(warehouse.UserEraser, *kafkax.Fingerprinter, *slog.Logger) consume.Handler {
	return func(context.Context, consume.Record) error { return errors.New("not implemented") }
}
