package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

// New returns a logger writing to stdout with a service attribute.
func New(r config.Runtime, service string) *slog.Logger {
	return NewTo(os.Stdout, r, service)
}

// NewTo is New writing to w, for tests (io.Discard keeps them quiet).
func NewTo(w io.Writer, r config.Runtime, service string) *slog.Logger {
	ho := &slog.HandlerOptions{Level: parseLevel(r.LogLevel)}
	var h slog.Handler
	if r.LogJSON {
		h = slog.NewJSONHandler(w, ho)
	} else {
		h = slog.NewTextHandler(w, ho)
	}
	return slog.New(NewHandler(h)).With("service", service)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewHandler wraps inner so records gain request_id, principal_id and trace_id
// when the record's context carries them.
func NewHandler(inner slog.Handler) slog.Handler {
	return ctxHandler{inner: inner}
}

type ctxHandler struct{ inner slog.Handler }

func (h ctxHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	// A record that already names a key (for example the gRPC logging
	// interceptor's request_id) keeps its value instead of getting a duplicate.
	have := map[string]bool{}
	r.Attrs(func(a slog.Attr) bool { have[a.Key] = true; return true })
	add := func(key, val string) {
		if val != "" && !have[key] {
			r.AddAttrs(slog.String(key, val))
		}
	}
	add("request_id", reqctx.RequestID(ctx))
	add("principal_id", reqctx.PrincipalID(ctx))
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		add("trace_id", sc.TraceID().String())
	}
	return h.inner.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{inner: h.inner.WithGroup(name)}
}
