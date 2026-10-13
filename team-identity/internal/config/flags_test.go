package config_test

import (
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-identity/internal/config"
)

// Every deploy runs the database, Kafka, the outbox relayer and gRPC reflection,
// so none of them has an enable flag.
func TestNoEnableFlagsDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, removed := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if slices.Contains(keys, removed) {
			t.Errorf("Settings still declares %s", removed)
		}
	}
}
