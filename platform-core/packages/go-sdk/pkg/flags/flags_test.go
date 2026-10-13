package flags_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/flags"
)

var quiet = slog.New(slog.DiscardHandler)

func TestUnreachableFliptReturnsDefault(t *testing.T) {
	cfg := config.Flags{Enabled: true, FliptAddr: "127.0.0.1:1", EvalTimeout: 300 * time.Millisecond}
	c, err := flags.New(context.Background(), cfg, quiet)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	start := time.Now()
	if got := c.BooleanEnabled(context.Background(), "checkout-enabled", true); !got {
		t.Error("BooleanEnabled = false, want the default true")
	}
	if got := c.BooleanEnabled(context.Background(), "checkout-enabled", false); got {
		t.Error("BooleanEnabled = true, want the default false")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("two evaluations took %v, want within the eval timeout", d)
	}
}

func TestDisabledReturnsDefault(t *testing.T) {
	if !flags.Disabled().BooleanEnabled(context.Background(), "x", true) {
		t.Error("Disabled() = false, want default true")
	}
	c, err := flags.New(context.Background(), config.Flags{Enabled: false}, quiet)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.BooleanEnabled(context.Background(), "x", false) {
		t.Error("disabled client = true, want default false")
	}
	if err := c.Close(context.Background()); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestEnabledRequiresAddr(t *testing.T) {
	if _, err := flags.New(context.Background(), config.Flags{Enabled: true}, quiet); err == nil {
		t.Fatal("New with empty FLIPT_ADDR: want error")
	}
}

func TestNewWithProviderEvaluates(t *testing.T) {
	p := memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{
		"checkout-enabled": {
			State:          memprovider.Enabled,
			DefaultVariant: "off",
			Variants:       map[string]any{"on": true, "off": false},
		},
	})
	c, err := flags.NewWithProvider(p, time.Second)
	if err != nil {
		t.Fatalf("NewWithProvider: %v", err)
	}
	if got := c.BooleanEnabled(context.Background(), "checkout-enabled", true); got {
		t.Error("checkout-enabled = true, want false from the provider")
	}
	if got := c.BooleanEnabled(context.Background(), "unknown-flag", true); !got {
		t.Error("unknown flag = false, want its default true")
	}
	if got := c.BooleanEnabled(context.Background(), "unknown-flag", false); got {
		t.Error("unknown flag = true, want its default false")
	}
}

func TestNewOrDisabledDegrades(t *testing.T) {
	ev, closeFn := flags.NewOrDisabled(context.Background(), config.Flags{Enabled: true}, quiet)
	if ev == nil || closeFn == nil {
		t.Fatal("NewOrDisabled returned a nil evaluator or closer")
	}
	if !ev.BooleanEnabled(context.Background(), "x", true) {
		t.Error("degraded evaluator = false, want caller default true")
	}
	if ev.BooleanEnabled(context.Background(), "x", false) {
		t.Error("degraded evaluator = true, want caller default false")
	}
	if err := closeFn(context.Background()); err != nil {
		t.Errorf("closer = %v, want nil", err)
	}
}

// recorder is a provider that remembers the flattened evaluation context.
type recorder struct {
	openfeature.NoopProvider
	mu  sync.Mutex
	got openfeature.FlattenedContext
	err bool
}

func (r *recorder) BooleanEvaluation(_ context.Context, _ string, def bool, fc openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = fc
	d := openfeature.BoolResolutionDetail{Value: def}
	if r.err {
		d.ResolutionError = openfeature.NewGeneralResolutionError("boom")
		d.Reason = openfeature.ErrorReason
	}
	return d
}

func (r *recorder) context() openfeature.FlattenedContext {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got
}

func TestTargetingKeyFromPrincipal(t *testing.T) {
	r := &recorder{}
	c, err := flags.NewWithProvider(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "user-42", Type: "user"})
	c.BooleanEnabled(ctx, "f", false)
	if got := r.context()[openfeature.TargetingKey]; got != "user-42" {
		t.Errorf("targeting key = %v, want user-42", got)
	}

	c.BooleanEnabled(context.Background(), "f", false)
	if got := r.context()[openfeature.TargetingKey]; got != string(auth.TypeAnonymous) {
		t.Errorf("targeting key without principal = %v, want %q", got, string(auth.TypeAnonymous))
	}
}

func TestEvalOptionsOverrideKeyAndAddAttributes(t *testing.T) {
	r := &recorder{}
	c, err := flags.NewWithProvider(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "user-42"})
	c.BooleanEnabled(ctx, "f", false, flags.WithTargetingKey("tenant-7"), flags.WithAttributes(map[string]any{"plan": "pro"}))
	fc := r.context()
	if fc[openfeature.TargetingKey] != "tenant-7" || fc["plan"] != "pro" {
		t.Errorf("evaluation context = %v, want key tenant-7 and plan=pro", fc)
	}
}

func TestCloseKeepsOtherClientWorking(t *testing.T) {
	on := func() *flags.Client {
		c, err := flags.NewWithProvider(memprovider.NewInMemoryProvider(map[string]memprovider.InMemoryFlag{
			"f": {State: memprovider.Enabled, DefaultVariant: "on", Variants: map[string]any{"on": true, "off": false}},
		}), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b := on(), on()
	t.Cleanup(func() { _ = b.Close(context.Background()) })
	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !b.BooleanEnabled(context.Background(), "f", false) {
		t.Error("client b lost its provider after client a was closed")
	}
}

func TestEvaluationWarningIsRateLimited(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	h := slog.NewTextHandler(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}), nil)
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	cl, err := flags.NewWithProvider(&recorder{err: true}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })
	for range 50 {
		cl.BooleanEnabled(context.Background(), "noisy", true)
	}
	cl.BooleanEnabled(context.Background(), "other", true)
	mu.Lock()
	defer mu.Unlock()
	if n := strings.Count(buf.String(), "flag=noisy"); n != 1 {
		t.Errorf("warnings for noisy = %d, want 1", n)
	}
	if n := strings.Count(buf.String(), "flag=other"); n != 1 {
		t.Errorf("warnings for other = %d, want 1 (limit is per flag)", n)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
