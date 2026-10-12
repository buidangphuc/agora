package config_test

import (
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-engagement/internal/config"
)

// Capabilities every deploy runs have no enable flag (ADR-0016). Only OTEL_ENABLED
// stays: it is a real toggle.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})

	for _, key := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(declared, key) {
			t.Errorf("%s is declared in Settings: always-on capabilities get no enable flag", key)
		}
	}
	if !slices.Contains(declared, "OTEL_ENABLED") {
		t.Error("OTEL_ENABLED must stay declared: it is a real toggle")
	}
}
