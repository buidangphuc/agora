package flashsale

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/flags"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type eventLog struct {
	mu        sync.Mutex
	campaigns []store.FlashSaleCampaign
	rows      []outbox.Row
	reqIDs    []string
	err       error
}

func (l *eventLog) build(ctx context.Context, c store.FlashSaleCampaign) (outbox.Row, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err != nil {
		return outbox.Row{}, l.err
	}
	row := outbox.Row{
		EventID:       "evt-" + c.ID,
		AggregateType: "flash_sale",
		AggregateID:   c.ID,
		EventType:     "platform.promotion.v1.FlashSaleChanged",
	}
	l.campaigns = append(l.campaigns, c)
	l.rows = append(l.rows, row)
	l.reqIDs = append(l.reqIDs, reqctx.RequestID(ctx))
	return row, nil
}

func (l *eventLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.rows)
}

type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(c string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *callLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

type recordingAuditWriter struct {
	log    *callLog
	err    error
	mu     sync.Mutex
	events []auditx.Event
}

func (w *recordingAuditWriter) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	if w.log != nil {
		w.log.add("audit")
	}
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
	return w.err
}

func (w *recordingAuditWriter) got() []auditx.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]auditx.Event(nil), w.events...)
}

var auditFailures = map[string]error{
	"upstream error": status.Error(codes.Internal, "audit db down"),
	"timeout":        context.DeadlineExceeded,
	"unavailable":    status.Error(codes.Unavailable, "team-audit down"),
	"plain error":    errors.New("dial tcp: refused"),
}

func assertAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
		t.Fatalf("err = %v, want Unavailable %q", err, auditx.Message)
	}
}

func flagProvider(enabled bool) openfeature.FeatureProvider {
	return memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{
		FlagFlashSaleEnabled: {
			Key:            FlagFlashSaleEnabled,
			State:          memprovider.Enabled,
			DefaultVariant: "value",
			Variants:       map[string]any{"value": enabled},
		},
	})
}

type erroringProvider struct {
	memprovider.InMemoryProvider
}

func (erroringProvider) BooleanEvaluation(_ context.Context, _ string, def bool, _ openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return openfeature.BoolResolutionDetail{
		Value: !def,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: openfeature.NewGeneralResolutionError("provider unreachable"),
			Reason:          openfeature.ErrorReason,
		},
	}
}

func newFlags(t *testing.T, p openfeature.FeatureProvider) flags.Evaluator {
	t.Helper()
	c, err := flags.NewWithProvider(t.Context(), p, time.Second, nil)
	if err != nil {
		t.Fatalf("flags.NewWithProvider: %v", err)
	}
	return c
}

func noEvent(store.FlashSaleCampaign) (outbox.Row, error) { return outbox.Row{}, nil }
