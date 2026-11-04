package logx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
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

var (
	fpOnce sync.Once
	fp     *kafkax.Fingerprinter
)

func principalFingerprinter() *kafkax.Fingerprinter {
	fpOnce.Do(func() {
		k := config.LogKeyFP{Secret: os.Getenv("LOG_KEY_FP_SECRET")}
		if err := k.ResolveFor(config.Runtime{Env: os.Getenv("ENV")}); err != nil {
			b := make([]byte, 32)
			_, _ = rand.Read(b)
			k.Secret = hex.EncodeToString(b)
		}
		fp, _ = kafkax.NewFingerprinter(k.Secret)
	})
	return fp
}

// PrincipalFP returns the keyed fingerprint logged as principal_fp.
func PrincipalFP(id string) string {
	return principalFingerprinter().Of([]byte(id))
}

// NewHandler wraps inner so records gain request_id, principal_fp and trace_id
// when the record's context carries them. The raw principal id is never logged;
// principal_fp is a keyed fingerprint under LOG_KEY_FP_SECRET.
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
	if id := reqctx.PrincipalID(ctx); id != "" {
		add("principal_fp", PrincipalFP(id))
	}
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
