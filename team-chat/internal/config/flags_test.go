package config_test

import (
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-chat/internal/config"
)

// Capabilities every deploy runs get no enable flag. The positive keys keep the
// check from passing on a Settings that declares nothing at all.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(config.Settings{})

	for _, want := range []string{
		"GRPC_PORT", "DATABASE_URL", "KAFKA_BROKERS", "KAFKA_CHAT_TOPIC",
		"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK_SECONDS", "OUTBOX_MAX_ATTEMPTS",
	} {
		if !slices.Contains(declared, want) {
			t.Errorf("Settings does not declare %s; declared = %v", want, declared)
		}
	}
	for _, flag := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(declared, flag) {
			t.Errorf("Settings declares the removed flag %s", flag)
		}
	}
}
