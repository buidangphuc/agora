package edge_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

type reqctxHandler struct{ inner slog.Handler }

func (h reqctxHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h reqctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := reqctx.PrincipalID(ctx); id != "" {
		r.AddAttrs(slog.String(strings.Join([]string{"principal", "id"}, "_"), id))
	}
	return h.inner.Handle(ctx, r)
}

func (h reqctxHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return reqctxHandler{inner: h.inner.WithAttrs(a)}
}

func (h reqctxHandler) WithGroup(n string) slog.Handler {
	return reqctxHandler{inner: h.inner.WithGroup(n)}
}

func TestGWFP7_GWFP11_ScrubHandlerDropsContextPrincipalID(t *testing.T) {
	ctx := reqctx.WithPrincipalID(context.Background(), "user-1")

	var control bytes.Buffer
	slog.New(reqctxHandler{slog.NewJSONHandler(&control, nil)}).InfoContext(ctx, "edge.request")
	if !strings.Contains(control.String(), "user-1") {
		t.Fatalf("control handler must leak the id for this test to be meaningful: %s", control.String())
	}

	var buf bytes.Buffer
	base := edge.NewPrincipalScrubHandler(reqctxHandler{slog.NewJSONHandler(&buf, nil)})
	l := slog.New(base).With("k", "v").WithGroup("g")
	l.InfoContext(ctx, "edge.request", "path", "/x")
	l.ErrorContext(ctx, "edge.upstream_error", "path", "/x")
	l.WarnContext(ctx, "edge.rate_limited")

	out := buf.String()
	if strings.Contains(out, "user-1") || strings.Contains(out, "principal_id") {
		t.Fatalf("raw principal id leaked: %s", out)
	}
	if strings.Count(out, "\n") != 3 {
		t.Fatalf("expected 3 log lines, got: %s", out)
	}
}
