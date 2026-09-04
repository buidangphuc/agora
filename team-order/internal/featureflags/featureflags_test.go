package featureflags_test

import (
	"context"
	"testing"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/memprovider"

	"github.com/buidangphuc/team-order/internal/featureflags"
)

// newMemClient backs the Evaluator with OpenFeature's in-memory (fake) provider,
// exercising the real OpenFeature evaluation path used in production — only the
// provider is swapped for Flipt.
func newMemClient(t *testing.T, flags map[string]memprovider.InMemoryFlag) *featureflags.Client {
	t.Helper()
	provider := memprovider.NewInMemoryProvider(flags)
	if err := openfeature.SetProviderAndWait(provider); err != nil {
		t.Fatalf("set in-memory provider: %v", err)
	}
	return featureflags.NewWithClient(openfeature.NewClient("test"), nil)
}

func boolFlag(value bool) memprovider.InMemoryFlag {
	variant := "on"
	if !value {
		variant = "off"
	}
	return memprovider.InMemoryFlag{
		State:          memprovider.Enabled,
		DefaultVariant: variant,
		Variants: map[string]any{
			"on":  true,
			"off": false,
		},
	}
}

func TestBooleanEnabled_FlagOn_Allowed(t *testing.T) {
	c := newMemClient(t, map[string]memprovider.InMemoryFlag{
		featureflags.FlagCheckoutEnabled: boolFlag(true),
	})
	if !c.BooleanEnabled(context.Background(), featureflags.FlagCheckoutEnabled, false) {
		t.Fatal("flag ON must evaluate true")
	}
}

func TestBooleanEnabled_FlagOff_Blocked(t *testing.T) {
	c := newMemClient(t, map[string]memprovider.InMemoryFlag{
		featureflags.FlagCheckoutEnabled: boolFlag(false),
	})
	if c.BooleanEnabled(context.Background(), featureflags.FlagCheckoutEnabled, true) {
		t.Fatal("flag OFF must evaluate false")
	}
}

func TestBooleanEnabled_ProviderError_FailsOpen(t *testing.T) {
	// The flag is absent from the provider → evaluation errors → the client must
	// fall back to the caller's default (fail-open). A flag-system outage must not
	// take checkout down.
	c := newMemClient(t, map[string]memprovider.InMemoryFlag{})
	if !c.BooleanEnabled(context.Background(), featureflags.FlagCheckoutEnabled, true) {
		t.Fatal("provider error must fail open to default true")
	}
}

func TestBooleanEnabled_DisabledClient_ReturnsDefault(t *testing.T) {
	c := featureflags.Disabled(nil)
	if !c.BooleanEnabled(context.Background(), featureflags.FlagCheckoutEnabled, true) {
		t.Fatal("disabled client must return the caller's default (true)")
	}
	if c.BooleanEnabled(context.Background(), featureflags.FlagCheckoutEnabled, false) {
		t.Fatal("disabled client must return the caller's default (false)")
	}
}
