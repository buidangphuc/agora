package logx

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"
)

// Options mirrors the fields of config.Runtime that logx reads.
type Options struct {
	Level string
	JSON  bool
}

// New returns a logger writing to stdout with a service attribute.
func New(o Options, service string) *slog.Logger {
	return newWithWriter(os.Stdout, o, service)
}

func newWithWriter(w io.Writer, o Options, service string) *slog.Logger {
	ho := &slog.HandlerOptions{Level: parseLevel(o.Level)}
	var h slog.Handler
	if o.JSON {
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
	if id := reqctx.RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	if id := reqctx.PrincipalID(ctx); id != "" {
		r.AddAttrs(slog.String("principal_id", id))
	}
	if sc := trace.SpanContextFromContext(ctx); sc.HasTraceID() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()))
	}
	return h.inner.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ctxHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{inner: h.inner.WithGroup(name)}
}
