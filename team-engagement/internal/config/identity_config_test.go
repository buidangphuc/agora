package config_test

import (
	"testing"

	"github.com/buidangphuc/team-engagement/internal/config"
)

// Scenario: A malformed event goes to the DLQ (erasure consumer topic and group defaults)
func TestIdentityErasureConfigDefaults(t *testing.T) {
	localEnv(t)
	unset(t, "IDENTITY_EVENTS_TOPIC", "KAFKA_ERASURE_CONSUMER_GROUP")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := settingByEnvKey(t, s, "IDENTITY_EVENTS_TOPIC"); got != "identity.events" {
		t.Errorf("IDENTITY_EVENTS_TOPIC = %v, want identity.events", got)
	}
	if got := settingByEnvKey(t, s, "KAFKA_ERASURE_CONSUMER_GROUP"); got != "team-engagement-erasure" {
		t.Errorf("KAFKA_ERASURE_CONSUMER_GROUP = %v, want team-engagement-erasure", got)
	}
}

// Scenario: A malformed event goes to the DLQ (erasure settings are overridable)
func TestIdentityErasureConfigOverride(t *testing.T) {
	localEnv(t)
	t.Setenv("IDENTITY_EVENTS_TOPIC", "identity.events.test")
	t.Setenv("KAFKA_ERASURE_CONSUMER_GROUP", "erasure-g")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := settingByEnvKey(t, s, "IDENTITY_EVENTS_TOPIC"); got != "identity.events.test" {
		t.Errorf("IDENTITY_EVENTS_TOPIC = %v", got)
	}
	if got := settingByEnvKey(t, s, "KAFKA_ERASURE_CONSUMER_GROUP"); got != "erasure-g" {
		t.Errorf("KAFKA_ERASURE_CONSUMER_GROUP = %v", got)
	}
}

// Scenario: A malformed event goes to the DLQ (blank erasure settings are refused)
func TestIdentityErasureConfigRequiresTopicAndGroup(t *testing.T) {
	for _, k := range []string{"IDENTITY_EVENTS_TOPIC", "KAFKA_ERASURE_CONSUMER_GROUP"} {
		t.Run(k, func(t *testing.T) {
			localEnv(t)
			t.Setenv(k, " ")
			_, err := config.Load()
			mustErrNaming(t, err, k)
		})
	}
}
