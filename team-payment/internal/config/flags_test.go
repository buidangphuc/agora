package config_test

import (
	"slices"
	"testing"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-payment/internal/config"
)

// Every deploy runs the database, Kafka, the outbox relayer, the settlement consumer
// and gRPC reflection, so none of them has an enable flag.
func TestNoEnableFlagsDeclared(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range []string{
		"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED",
		"PAYMENT_SETTLEMENT_CONSUMER_ENABLED",
	} {
		if slices.Contains(keys, k) {
			t.Errorf("Settings still declares %s", k)
		}
	}
	// The SDK Outbox group is in: its keys now reach the relayer.
	if !slices.Contains(keys, "OUTBOX_MAX_ATTEMPTS") {
		t.Error("Settings does not declare OUTBOX_MAX_ATTEMPTS")
	}
}
