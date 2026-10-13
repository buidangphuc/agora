package config_test

import (
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-order/internal/config"
)

// Capabilities every deploy runs have no enable flag (ADR-0016). Only real
// toggles keep one: FEATURE_FLAGS_ENABLED (promotion runs with it off) and
// OTEL_ENABLED.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})

	for _, key := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(declared, key) {
			t.Errorf("%s is declared in Settings: always-on capabilities get no enable flag", key)
		}
	}
	for _, key := range []string{"FEATURE_FLAGS_ENABLED", "OTEL_ENABLED"} {
		if !slices.Contains(declared, key) {
			t.Errorf("%s must stay declared: it is a real toggle", key)
		}
	}
}
