package config_test

import (
	"slices"
	"strings"
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
	want := config.Outbox{PollInterval: time.Second, BatchSize: 100, ClaimLock: time.Minute}
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
	if s.Outbox.PollInterval != 250*time.Millisecond {
		t.Errorf("Outbox = %+v", s.Outbox)
	}
	if s.Flags.Enabled {
		t.Error("Flags.Enabled = true, want false")
	}
	if s.Outbox.ClaimLock != 90*time.Second || s.Flags.EvalTimeout != 250*time.Millisecond {
		t.Errorf("durations = %v, %v", s.Outbox.ClaimLock, s.Flags.EvalTimeout)
	}
}

type deployedSettings struct {
	config.Runtime
	Kafka config.Kafka
	Flags config.Flags
}

func TestStrictEnvRequiresKafkaAndFlipt(t *testing.T) {
	for _, env := range []string{"staging", "prod", "production"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("ENV", env)
			_, err := config.Load[deployedSettings]()
			if err == nil {
				t.Fatal("Load: want an error for the unset KAFKA_BROKERS and FLIPT_ADDR")
			}
			for _, key := range []string{"KAFKA_BROKERS", "FLIPT_ADDR"} {
				if !strings.Contains(err.Error(), key) {
					t.Errorf("error %q does not name %s", err, key)
				}
			}
			t.Setenv("KAFKA_BROKERS", "k:9092")
			t.Setenv("FLIPT_ADDR", "flipt:9000")
			if _, err := config.Load[deployedSettings](); err != nil {
				t.Fatalf("Load with both set: %v", err)
			}
		})
	}
}

func TestStrictEnvIgnoresFliptWhenFlagsDisabled(t *testing.T) {
	t.Setenv("ENV", "prod")
	t.Setenv("KAFKA_BROKERS", "k:9092")
	t.Setenv("FEATURE_FLAGS_ENABLED", "false")
	if _, err := config.Load[deployedSettings](); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLocalEnvKeepsKafkaAndFliptDefaults(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		t.Setenv("ENV", env)
		s, err := config.Load[deployedSettings]()
		if err != nil {
			t.Fatalf("Load(%s): %v", env, err)
		}
		if s.Kafka.Brokers != "localhost:9092" || s.Flags.FliptAddr != "localhost:9000" {
			t.Errorf("%s: Brokers=%q FliptAddr=%q, want the localhost defaults", env, s.Kafka.Brokers, s.Flags.FliptAddr)
		}
	}
}

func TestStrictEnvRejectsEmptyKafkaBrokers(t *testing.T) {
	t.Setenv("ENV", "prod")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("FLIPT_ADDR", "flipt:9000")
	if _, err := config.Load[deployedSettings](); err == nil {
		t.Fatal("Load: want an error for an empty KAFKA_BROKERS")
	}
}
