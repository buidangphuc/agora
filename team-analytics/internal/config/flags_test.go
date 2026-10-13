package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-analytics/internal/config"
)

// Scenario: team-analytics declares no always-on enable flags.
func TestNoEnableFlagsDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, flag := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		assert.NotContains(t, keys, flag)
	}
	// The capabilities those flags used to gate are still configured.
	assert.Contains(t, keys, "KAFKA_BROKERS")
	assert.Contains(t, keys, "GRPC_PORT")
}
