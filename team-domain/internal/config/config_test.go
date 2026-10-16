package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`): the repo-root
// .env.example must document exactly the env keys Settings declares.
func TestEnvExampleInSync(t *testing.T) {
	// Test runs with working dir = this package; .env.example is at the repo root.
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	// DATABASE_URL is required because DATABASE_ENABLED defaults to true; set it
	// so Validate passes, then assert the other struct defaults apply.
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50051 {
		t.Errorf("default GRPC_PORT = %d, want 50051", s.Server.Port)
	}
	if !s.Database.Enabled {
		t.Error("default DATABASE_ENABLED should be true")
	}
	if got := s.KafkaBrokers(); len(got) != 1 || got[0] != "localhost:9092" {
		t.Errorf("default KafkaBrokers = %v, want [localhost:9092]", got)
	}
}

func TestValidateDatabaseURLRequired(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50051
	s.Database.Enabled = true
	s.Database.URL = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: DATABASE_ENABLED=true requires DATABASE_URL")
	}
}

func strictSettings(env string) *Settings {
	s := &Settings{}
	s.Runtime.Env = env
	s.Server.Port = 50051
	s.Database.Enabled = true
	s.Database.URL = "postgresql://x/y"
	s.Events.KafkaEnabled = true
	s.Outbox.Enabled = true
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
			"KAFKA_ENABLED":      func(s *Settings) { s.Events.KafkaEnabled = false },
			"OUTBOX_ENABLED":     func(s *Settings) { s.Outbox.Enabled = false },
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
		s.Events.KafkaEnabled = false
		s.Outbox.Enabled = false
		s.Storage.AccessKey, s.Storage.SecretKey = "minioadmin", "minioadmin"
		if err := s.Validate(); err != nil {
			t.Errorf("ENV=%q must be unaffected: %v", env, err)
		}
	}
}

func TestRequireSafeStrictConfig_FromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("ENV", "production")
	if _, err := LoadSettings(); err == nil {
		t.Fatal("production with default Kafka/storage settings must be refused by LoadSettings")
	}
	t.Setenv("ENV", "local")
	if _, err := LoadSettings(); err != nil {
		t.Fatalf("local defaults must load: %v", err)
	}
}

func reservationSettings(t *testing.T, env map[string]string) *Settings {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	for k, v := range env {
		t.Setenv(k, v)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	return s
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

// An invalid value never fails LoadSettings: it falls back to the default and
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

// Scenario: Listing audit: Config exposes the audit address.
func TestUpstreamAuditConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Upstream.AuditAddr != "" {
		t.Errorf("default UPSTREAM_AUDIT_ADDR = %q, want empty", s.Upstream.AuditAddr)
	}
	if s.Upstream.CallTimeoutSeconds != 2 {
		t.Errorf("default UPSTREAM_CALL_TIMEOUT_SECONDS = %v, want 2", s.Upstream.CallTimeoutSeconds)
	}
	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	s, err = LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Upstream.AuditAddr != "team-audit-svc:50066" {
		t.Errorf("UPSTREAM_AUDIT_ADDR = %q, want team-audit-svc:50066", s.Upstream.AuditAddr)
	}
	// The env drift gate must pass with both new keys documented.
	if err := CheckEnvExample("../../.env.example"); err != nil {
		t.Errorf("env drift gate: %v", err)
	}
	keys := strings.Join(DeclaredEnvKeys(), ",")
	for _, k := range []string{"UPSTREAM_AUDIT_ADDR", "UPSTREAM_CALL_TIMEOUT_SECONDS"} {
		if !strings.Contains(keys, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}
}

// Scenario: Auto-screen: Missing terms file stops start-up.
func TestScreenTermsFileMissingFails(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	missing := t.TempDir() + "/does-not-exist.txt"
	t.Setenv("SCREEN_TERMS_FILE", missing)
	_, err := LoadSettings()
	if err == nil {
		t.Fatal("expected start-up error for a missing SCREEN_TERMS_FILE")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q must name the file %q", err, missing)
	}
}

// Scenario: Auto-screen: Embedded term list is the default (unset = no file needed).
func TestScreenTermsFileDefaultEmpty(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Screen.TermsFile != "" {
		t.Errorf("default SCREEN_TERMS_FILE = %q, want empty", s.Screen.TermsFile)
	}
}

// Scenario: Auto-screen: Keyword list loads from the config file (existing file passes).
func TestScreenTermsFileExistingLoads(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	p := t.TempDir() + "/terms.txt"
	if err := os.WriteFile(p, []byte("a\nb\nc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCREEN_TERMS_FILE", p)
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.Screen.TermsFile != p {
		t.Errorf("TermsFile = %q, want %q", s.Screen.TermsFile, p)
	}
}
