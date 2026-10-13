package config

import (
	"bytes"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`): the repo-root
// .env.example must document exactly the env keys Settings declares.
func TestEnvExampleInSync(t *testing.T) {
	// Test runs with working dir = this package; .env.example is at the repo root.
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := sdkconfig.CheckEnvExample(path, Settings{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	// DATABASE_URL is required; set it so Validate passes, then assert the other struct defaults apply.
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("ENV", "local")
	s, err := Load()
	if err != nil {
		t.Fatalf("Load with defaults: %v", err)
	}
	if s.Port != 50051 {
		t.Errorf("default GRPC_PORT = %d, want 50051", s.Port)
	}
	if got := s.Kafka.BrokerList(); len(got) != 1 || got[0] != "localhost:9092" {
		t.Errorf("default Kafka.BrokerList = %v, want [localhost:9092]", got)
	}
}

func TestValidateDatabaseURLRequired(t *testing.T) {
	// The SDK Postgres group rejects an empty URL; Load runs every group's Validate.
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("ENV", "local")
	if _, err := Load(); err == nil {
		t.Fatal("expected error: a missing DATABASE_URL")
	}
}

func strictSettings(env string) *Settings {
	s := &Settings{}
	s.Env = env
	s.Port = 50051
	s.Postgres.URL = "postgresql://x/y"
	s.Storage.AccessKey = "ak"
	s.Storage.SecretKey = "sk"
	return s
}

func TestRequireSafeStrictConfig(t *testing.T) {
	strict := []string{"staging", "stage", "prod", "production", "PROD", "  Production ", "Stage"}
	for _, env := range strict {
		if err := strictSettings(env).Validate(); err != nil {
			t.Errorf("ENV=%q safe config must pass: %v", env, err)
		}
		cases := map[string]func(*Settings){
			"STORAGE_ACCESS_KEY": func(s *Settings) { s.Storage.AccessKey = "minioadmin" },
			"STORAGE_SECRET_KEY": func(s *Settings) { s.Storage.SecretKey = "minioadmin" },
		}
		for want, mutate := range cases {
			s := strictSettings(env)
			mutate(s)
			err := s.Validate()
			if err == nil {
				t.Errorf("ENV=%q %s: must be refused", env, want)
				continue
			}
			for _, w := range []string{"ENV", want} {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("ENV=%q: error %q must name %s", env, err, w)
				}
			}
		}
	}
	for _, env := range []string{"local", "test", "", "dev", "development"} {
		s := strictSettings(env)
		s.Storage.AccessKey, s.Storage.SecretKey = "minioadmin", "minioadmin"
		if err := s.Validate(); err != nil {
			t.Errorf("ENV=%q must be unaffected: %v", env, err)
		}
	}
}

func TestRequireSafeStrictConfig_FromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("ENV", "production")
	t.Setenv("KAFKA_BROKERS", "")
	if _, err := Load(); err == nil {
		t.Fatal("production with default Kafka/storage settings must be refused by Load")
	}
	t.Setenv("ENV", "local")
	if _, err := Load(); err != nil {
		t.Fatalf("local defaults must load: %v", err)
	}
}

func reservationSettings(t *testing.T, env map[string]string) *Settings {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("GRPC_PORT", "50051")
	t.Setenv("ENV", "local")
	for k, v := range env {
		t.Setenv(k, v)
	}
	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return &s
}

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestReservationDefaults(t *testing.T) {
	s := reservationSettings(t, nil)
	logger, buf := captureLogger()
	if got := s.ReservationTTL(logger); got != 15*time.Minute {
		t.Errorf("default RESERVATION_TTL = %v, want 15m", got)
	}
	if got := s.ReservationSweepInterval(logger); got != time.Minute {
		t.Errorf("default RESERVATION_SWEEP_INTERVAL = %v, want 1m", got)
	}
	if buf.Len() != 0 {
		t.Errorf("defaults must not warn, got %q", buf.String())
	}
}

func TestReservationOverride(t *testing.T) {
	s := reservationSettings(t, map[string]string{"RESERVATION_TTL": "20s", "RESERVATION_SWEEP_INTERVAL": "2s"})
	logger, buf := captureLogger()
	if got := s.ReservationTTL(logger); got != 20*time.Second {
		t.Errorf("RESERVATION_TTL = %v, want 20s", got)
	}
	if got := s.ReservationSweepInterval(logger); got != 2*time.Second {
		t.Errorf("RESERVATION_SWEEP_INTERVAL = %v, want 2s", got)
	}
	if buf.Len() != 0 {
		t.Errorf("valid overrides must not warn, got %q", buf.String())
	}
}

// An invalid value never fails Load: it falls back to the default and
// logs a warning naming the variable.
func TestReservationInvalidFallsBackWithWarning(t *testing.T) {
	for _, bad := range []string{"banana", "0", "0s", "-5m", ""} {
		s := reservationSettings(t, map[string]string{"RESERVATION_TTL": bad, "RESERVATION_SWEEP_INTERVAL": bad})
		logger, buf := captureLogger()
		if got := s.ReservationTTL(logger); got != 15*time.Minute {
			t.Errorf("TTL %q -> %v, want default 15m", bad, got)
		}
		if got := s.ReservationSweepInterval(logger); got != time.Minute {
			t.Errorf("interval %q -> %v, want default 1m", bad, got)
		}
		out := buf.String()
		for _, want := range []string{"level=WARN", "RESERVATION_TTL", "RESERVATION_SWEEP_INTERVAL", "default=15m0s", "default=1m0s"} {
			if !strings.Contains(out, want) {
				t.Errorf("value %q: warning log %q lacks %q", bad, out, want)
			}
		}
	}
}

// TestNoEnableFlagsDeclared: capabilities every deploy runs have no enable flag.
func TestNoEnableFlagsDeclared(t *testing.T) {
	declared := sdkconfig.DeclaredEnvKeys(Settings{})
	has := func(key string) bool { return slices.Contains(declared, key) }

	for _, key := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if has(key) {
			t.Errorf("%s is declared; the database, the Kafka publisher, the outbox relayer and reflection are always on", key)
		}
	}
}
