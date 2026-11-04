package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

type handoffRedactHandler struct {
	inner slog.Handler
}

func redactHandoffPrincipal(l *slog.Logger) *slog.Logger {
	return slog.New(handoffRedactHandler{inner: l.Handler()})
}

func (h handoffRedactHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h handoffRedactHandler) Handle(ctx context.Context, r slog.Record) error {
	handoff := false
	r.Attrs(func(a slog.Attr) bool {
		if (a.Key == "grpc.method" || a.Key == "method") && strings.Contains(a.Value.String(), "HandoffCode") {
			handoff = true
			return false
		}
		return true
	})
	if !handoff {
		return h.inner.Handle(ctx, r)
	}
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key != "principal_id" {
			clean.AddAttrs(a)
		}
		return true
	})
	return h.inner.Handle(reqctx.WithPrincipalID(ctx, ""), clean)
}

func (h handoffRedactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handoffRedactHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h handoffRedactHandler) WithGroup(name string) slog.Handler {
	return handoffRedactHandler{inner: h.inner.WithGroup(name)}
}
