package config_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
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
	wantFlags := config.Flags{FliptAddr: "localhost:9000", EvalTimeout: 500 * time.Millisecond}
	if s.Flags != wantFlags {
		t.Errorf("Flags = %+v, want %+v", s.Flags, wantFlags)
	}
}

func TestMessagingGroupOverrides(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", " a:1, ,b:2 ")
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("FLIPT_ADDR", "flipt:9000")
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
	if s.Flags.FliptAddr != "flipt:9000" {
		t.Errorf("FliptAddr = %q, want flipt:9000", s.Flags.FliptAddr)
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

func TestStrictEnvRequiresKafka(t *testing.T) {
	for _, env := range []string{"staging", "prod", "production"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("ENV", env)
			_, err := config.Load[deployedSettings]()
			if err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
				t.Fatalf("Load: err = %v, want one naming KAFKA_BROKERS", err)
			}
			if strings.Contains(err.Error(), "FLIPT_ADDR") {
				t.Errorf("error %q names FLIPT_ADDR; flags need no setting to boot", err)
			}
			t.Setenv("KAFKA_BROKERS", "k:9092")
			if _, err := config.Load[deployedSettings](); err != nil {
				t.Fatalf("Load with brokers set: %v", err)
			}
		})
	}
}

func TestFlagsAlwaysPointAtFlipt(t *testing.T) {
	for _, env := range []string{"local", "prod"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("ENV", env)
			t.Setenv("KAFKA_BROKERS", "k:9092")
			s, err := config.Load[deployedSettings]()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.Flags.FliptAddr != "localhost:9000" {
				t.Errorf("FliptAddr = %q, want the default localhost:9000", s.Flags.FliptAddr)
			}
		})
	}
}

func TestLocalEnvKeepsKafkaDefault(t *testing.T) {
	for _, env := range []string{"local", "test"} {
		t.Setenv("ENV", env)
		s, err := config.Load[deployedSettings]()
		if err != nil {
			t.Fatalf("Load(%s): %v", env, err)
		}
		if s.Kafka.Brokers != "localhost:9092" {
			t.Errorf("%s: Brokers=%q, want the localhost default", env, s.Kafka.Brokers)
		}
	}
}

func TestStrictEnvRejectsEmptyKafkaBrokers(t *testing.T) {
	t.Setenv("ENV", "prod")
	t.Setenv("KAFKA_BROKERS", "")
	if _, err := config.Load[deployedSettings](); err == nil {
		t.Fatal("Load: want an error for an empty KAFKA_BROKERS")
	}
}

type everyGroup struct {
	Runtime  config.Runtime
	GRPC     config.GRPC
	Postgres config.Postgres
	OTel     config.OTel
	Kafka    config.Kafka
	Outbox   config.Outbox
	Flags    config.Flags
}

// Scenario: No config group declares a dependency enable variable.
func TestNoDependencyEnableFlags(t *testing.T) {
	keys := config.DeclaredEnvKeys(everyGroup{})
	if len(keys) == 0 {
		t.Fatal("no env keys declared")
	}
	var enabled []string
	for _, k := range keys {
		if strings.HasSuffix(k, "_ENABLED") {
			enabled = append(enabled, k)
		}
	}
	if !slices.Equal(enabled, []string{"OTEL_ENABLED"}) {
		t.Errorf("_ENABLED keys = %v, want only [OTEL_ENABLED]", enabled)
	}
	for _, banned := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "FEATURE_FLAGS_ENABLED"} {
		if slices.Contains(keys, banned) {
			t.Errorf("%s must not be declared", banned)
		}
	}
}

type mockDataSettings struct {
	config.Runtime
	config.Postgres
	config.MockData
}

func unsetDatabaseURL(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "")
}

func TestLoadRefusesMockDataInStrictEnvBeforeValidation(t *testing.T) {
	unsetDatabaseURL(t)
	t.Setenv("ENV", "prod")
	t.Setenv("MOCK_DATA", "payment-provider")
	_, err := config.Load[mockDataSettings]()
	if err == nil {
		t.Fatal("Load succeeded, want an error naming MOCK_DATA")
	}
	if !strings.Contains(err.Error(), "MOCK_DATA") {
		t.Errorf("error %q does not name MOCK_DATA", err)
	}
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error %q names DATABASE_URL, validation masked MOCK_DATA", err)
	}
}

func TestMockDataRefusedForEveryStrictSpelling(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production", "PROD"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
			t.Setenv("ENV", env)
			t.Setenv("MOCK_DATA", "payment-provider")
			_, err := config.Load[mockDataSettings]()
			if err == nil || !strings.Contains(err.Error(), "MOCK_DATA") {
				t.Fatalf("ENV=%s: err = %v, want one naming MOCK_DATA", env, err)
			}
		})
	}
}

func TestMockDataAllowedWhenLocalOrEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost:5432/db")
	cases := []struct{ env, mock string }{
		{"local", "payment-provider"},
		{"test", "payment-provider"},
		{"prod", ""},
	}
	for _, c := range cases {
		t.Run(c.env+"/"+c.mock, func(t *testing.T) {
			t.Setenv("ENV", c.env)
			t.Setenv("MOCK_DATA", c.mock)
			if _, err := config.Load[mockDataSettings](); err != nil {
				t.Fatalf("Load: %v", err)
			}
		})
	}
	t.Run("prod/unset", func(t *testing.T) {
		t.Setenv("ENV", "prod")
		t.Setenv("MOCK_DATA", "")
		if _, err := config.Load[mockDataSettings](); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})
}

type auditSettings struct {
	config.Runtime
	config.Audit
}

func TestStrictEnvRequiresAuditAddr(t *testing.T) {
	for _, env := range []string{"prod", "staging"} {
		t.Run(env+"/unset-or-empty", func(t *testing.T) {
			t.Setenv("ENV", env)
			t.Setenv("UPSTREAM_AUDIT_ADDR", "")
			_, err := config.Load[auditSettings]()
			if err == nil || !strings.Contains(err.Error(), "UPSTREAM_AUDIT_ADDR") {
				t.Fatalf("ENV=%s: err = %v, want one naming UPSTREAM_AUDIT_ADDR", env, err)
			}
		})
	}
	t.Run("strict/set", func(t *testing.T) {
		t.Setenv("ENV", "prod")
		t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit:9090")
		s, err := config.Load[auditSettings]()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Addr != "team-audit:9090" {
			t.Errorf("Addr = %q, want team-audit:9090", s.Addr)
		}
	})
	t.Run("local", func(t *testing.T) {
		t.Setenv("ENV", "local")
		t.Setenv("UPSTREAM_AUDIT_ADDR", "")
		s, err := config.Load[auditSettings]()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Addr != "" {
			t.Errorf("Addr = %q, want empty", s.Addr)
		}
	})
}
