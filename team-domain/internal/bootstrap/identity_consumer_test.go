package bootstrap

import (
	"context"
	"testing"

	"github.com/buidangphuc/team-domain/internal/config"
)

func identitySettings(t *testing.T, enabled bool) *config.Settings {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	s.Events.IdentityConsumerEnabled = enabled
	return s
}

func findIdentityAddon() Addon {
	for _, a := range defaultAddons() {
		if a.Name() == "identity-consumer" {
			return a
		}
	}
	return nil
}

// Seller hold: Consumer disabled does nothing -- the addon is registered but
// disabled by default, so no Kafka consumer is started.
func TestIdentityConsumerAddonDisabled(t *testing.T) {
	a := findIdentityAddon()
	if a == nil {
		t.Fatal(`defaultAddons() has no addon named "identity-consumer"`)
	}
	if a.Enabled(identitySettings(t, false)) {
		t.Error("addon must be disabled when IDENTITY_CONSUMER_ENABLED=false")
	}
	if !a.Enabled(identitySettings(t, true)) {
		t.Error("addon must be enabled when IDENTITY_CONSUMER_ENABLED=true")
	}
}

// Seller hold: Consumer disabled does nothing -- closing an addon that never
// opened is a clean no-op (no consumer, no goroutine to drain).
func TestIdentityConsumerAddonCloseWithoutOpen(t *testing.T) {
	a := findIdentityAddon()
	if a == nil {
		t.Fatal(`defaultAddons() has no addon named "identity-consumer"`)
	}
	if err := a.Close(context.Background(), &Resources{}); err != nil {
		t.Errorf("Close without Open: %v", err)
	}
}

// Seller hold: Consumer disabled does nothing -- the addon is gated: it needs the
// database (it writes holds); with no pool Open must fail rather than panic.
func TestIdentityConsumerAddonOpenWithoutPoolFails(t *testing.T) {
	a := findIdentityAddon()
	if a == nil {
		t.Fatal(`defaultAddons() has no addon named "identity-consumer"`)
	}
	if err := a.Open(context.Background(), identitySettings(t, true), &Resources{}); err == nil {
		t.Error("Open without a DB pool must return an error")
	}
}
