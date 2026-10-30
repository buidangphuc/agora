package events

import (
	"context"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
)

const (
	aggregateIDKey = "aggregate_id"
	keyFingerprint = "key_fp"
)

type redactingHandler struct {
	inner slog.Handler
	fp    *kafkax.Fingerprinter
}

func RedactingHandler(inner slog.Handler, fp *kafkax.Fingerprinter) slog.Handler {
	return &redactingHandler{inner: inner, fp: fp}
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(h.redact(a))
		return true
	})
	return h.inner.Handle(ctx, out)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redact(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(redacted), fp: h.fp}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name), fp: h.fp}
}

func (h *redactingHandler) redact(a slog.Attr) slog.Attr {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		group := v.Group()
		redacted := make([]slog.Attr, len(group))
		for i, g := range group {
			redacted[i] = h.redact(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redacted...)}
	}
	if a.Key == aggregateIDKey {
		return slog.String(keyFingerprint, h.fp.Of([]byte(v.String())))
	}
	return a
}
