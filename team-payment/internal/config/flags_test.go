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
	// The relayer has no attempt limit: the SDK Outbox group no longer carries the key, and
	// the renamed duration keys are the ones that reach the relayer.
	if slices.Contains(keys, "OUTBOX_MAX_ATTEMPTS") {
		t.Error("Settings still declares OUTBOX_MAX_ATTEMPTS")
	}
	for _, k := range []string{"OUTBOX_CLAIM_LOCK", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY"} {
		if !slices.Contains(keys, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}
	for _, k := range []string{"OUTBOX_CLAIM_LOCK_SECONDS", "SHUTDOWN_GRACE_SECONDS", "SHUTDOWN_DRAIN_DELAY_SECONDS"} {
		if slices.Contains(keys, k) {
			t.Errorf("Settings still declares %s", k)
		}
	}
}
