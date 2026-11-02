package edge

import (
	"context"
	"log/slog"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
)

func (e *Edge) principalLogAttrs(p resolvedPrincipal) []any {
	kind := p.ptype
	switch kind {
	case "user", "service":
	default:
		kind = "anonymous"
	}
	if kind == "anonymous" || p.id == "" || e.fingerprinter == nil {
		return []any{slog.String("principal_kind", kind)}
	}
	return []any{
		slog.String("principal_fp", e.fingerprinter.Of([]byte(p.id))),
		slog.String("principal_kind", kind),
	}
}

type principalScrubHandler struct{ inner slog.Handler }

func NewPrincipalScrubHandler(inner slog.Handler) slog.Handler {
	return principalScrubHandler{inner: inner}
}

func (h principalScrubHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h principalScrubHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(reqctx.WithPrincipalID(ctx, ""), r)
}

func (h principalScrubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return principalScrubHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h principalScrubHandler) WithGroup(name string) slog.Handler {
	return principalScrubHandler{inner: h.inner.WithGroup(name)}
}
