package config_test

import (
	"slices"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

type messagingSettings struct {
	Kafka  config.Kafka
	Outbox config.Outbox
	Flags  config.Flags
}

func TestMessagingGroupDefaults(t *testing.T) {
	s, err := config.Load[messagingSettings]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s.Kafka.BrokerList(); !slices.Equal(got, []string{"localhost:9092"}) {
		t.Errorf("BrokerList = %v", got)
	}
	want := config.Outbox{PollInterval: time.Second, BatchSize: 100, ClaimLock: time.Minute, MaxAttempts: 10}
	if s.Outbox != want {
		t.Errorf("Outbox = %+v, want %+v", s.Outbox, want)
	}
	wantFlags := config.Flags{Enabled: true, FliptAddr: "localhost:9000", EvalTimeout: 500 * time.Millisecond}
	if s.Flags != wantFlags {
		t.Errorf("Flags = %+v, want %+v", s.Flags, wantFlags)
	}
}

func TestMessagingGroupOverrides(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", " a:1, ,b:2 ")
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("OUTBOX_MAX_ATTEMPTS", "3")
	t.Setenv("FEATURE_FLAGS_ENABLED", "false")
	t.Setenv("OUTBOX_CLAIM_LOCK", "90s")
	t.Setenv("FEATURE_FLAGS_EVAL_TIMEOUT", "250ms")

	s, err := config.Load[messagingSettings]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := s.Kafka.BrokerList(); !slices.Equal(got, []string{"a:1", "b:2"}) {
		t.Errorf("BrokerList = %v", got)
	}
	if s.Outbox.PollInterval != 250*time.Millisecond || s.Outbox.MaxAttempts != 3 {
		t.Errorf("Outbox = %+v", s.Outbox)
	}
	if s.Flags.Enabled {
		t.Error("Flags.Enabled = true, want false")
	}
	if s.Outbox.ClaimLock != 90*time.Second || s.Flags.EvalTimeout != 250*time.Millisecond {
		t.Errorf("durations = %v, %v", s.Outbox.ClaimLock, s.Flags.EvalTimeout)
	}
}
