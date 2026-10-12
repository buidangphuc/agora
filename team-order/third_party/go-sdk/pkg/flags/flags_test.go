package flags_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/flags"
)

var quiet = slog.New(slog.DiscardHandler)

func TestUnreachableFliptReturnsDefault(t *testing.T) {
	cfg := config.Flags{Enabled: true, FliptAddr: "127.0.0.1:1", EvalTimeoutMS: 300}
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
