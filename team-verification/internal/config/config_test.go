package config_test

import (
	"bufio"
	"os"
	"slices"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-verification/internal/config"
)

const (
	testDSN   = "postgres://test:pass@localhost:5444/db?sslmode=disable"
	auditAddr = "team-audit-svc:50066"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
	"UPSTREAM_AUDIT_ADDR", "UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_CALL_TIMEOUT", "MOCK_DATA",
}

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func baseEnv(t *testing.T, env string) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("GRPC_PORT", "50064")
	t.Setenv("DATABASE_URL", testDSN)
	t.Setenv("UPSTREAM_AUDIT_ADDR", auditAddr)
}

func wantErrNaming(t *testing.T, err error, key string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), key) {
		t.Fatalf("err = %v, want an error naming %s", err, key)
	}
}

func TestNoDependencyEnableFlags(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if len(keys) == 0 {
		t.Fatal("scenario Settings declare no dependency enable flags: no declared keys")
	}
	for _, k := range keys {
		if strings.HasSuffix(k, "_ENABLED") && k != "OTEL_ENABLED" && k != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("scenario Settings declare no dependency enable flags: declared %s", k)
		}
	}
	for _, gone := range []string{"DATABASE_ENABLED", "UPSTREAM_CALL_TIMEOUT_SECONDS"} {
		if slices.Contains(keys, gone) {
			t.Errorf("scenario Settings declare no dependency enable flags: %s is declared", gone)
		}
	}
	for _, want := range []string{"ENV", "GRPC_PORT", "DATABASE_URL", "UPSTREAM_AUDIT_ADDR", "MOCK_DATA", "OTEL_ENABLED", "GRPC_REFLECTION_ENABLED"} {
		if !slices.Contains(keys, want) {
			t.Errorf("declared keys miss %s", want)
		}
	}
}

func TestLoadRequiresEnv(t *testing.T) {
	t.Run("scenario ENV is required: unset", func(t *testing.T) {
		baseEnv(t, "local")
		os.Unsetenv("ENV")
		_, err := config.Load()
		wantErrNaming(t, err, "ENV")
	})
	t.Run("scenario ENV is required: dev rejected", func(t *testing.T) {
		baseEnv(t, "dev")
		_, err := config.Load()
		wantErrNaming(t, err, "ENV")
	})
	for _, c := range []struct {
		env    string
		strict bool
	}{{"local", false}, {"test", false}, {"staging", true}, {"prod", true}} {
		t.Run("scenario ENV is required: accepts "+c.env, func(t *testing.T) {
			baseEnv(t, c.env)
			s, err := config.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.Env != c.env {
				t.Errorf("Env = %q, want %q", s.Env, c.env)
			}
			if s.IsStrict() != c.strict {
				t.Errorf("IsStrict = %v, want %v", s.IsStrict(), c.strict)
			}
			if s.IsLocal() == c.strict {
				t.Errorf("IsLocal = %v, want %v", s.IsLocal(), !c.strict)
			}
		})
	}
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	const sc = "scenario Defaults and overrides"
	baseEnv(t, "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("%s: Load: %v", sc, err)
	}
	if s.Audit.Addr != auditAddr {
		t.Errorf("%s: Audit.Addr = %q", sc, s.Audit.Addr)
	}
	if s.Postgres.URL != testDSN {
		t.Errorf("%s: Postgres.URL = %q", sc, s.Postgres.URL)
	}
	if s.Postgres.MaxConns != 10 {
		t.Errorf("%s: MaxConns = %d, want 10", sc, s.Postgres.MaxConns)
	}
	if s.ReflectionEnabled {
		t.Errorf("%s: reflection on by default", sc)
	}
	if s.Host != "" {
		t.Errorf("%s: Host = %q, want empty", sc, s.Host)
	}
	if got := s.Addr(); got != ":50064" {
		t.Errorf("%s: Addr = %q, want :50064", sc, got)
	}
	if s.LogLevel != "info" || !s.LogJSON {
		t.Errorf("%s: LogLevel = %q LogJSON = %v", sc, s.LogLevel, s.LogJSON)
	}

	t.Setenv("GRPC_HOST", "127.0.0.1")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("%s: Load override: %v", sc, err)
	}
	if got := s.Addr(); got != "127.0.0.1:50064" {
		t.Errorf("%s: Addr = %q, want 127.0.0.1:50064", sc, got)
	}
}

func TestAuditConfigDefaultsAndOverrides(t *testing.T) {
	const sc = "scenario Audit config defaults and overrides"
	baseEnv(t, "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("%s: Load: %v", sc, err)
	}
	if s.Audit.Addr != "team-audit-svc:50066" {
		t.Errorf("%s: Audit.Addr = %q", sc, s.Audit.Addr)
	}
	t.Setenv("UPSTREAM_AUDIT_ADDR", "other-audit:1234")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("%s: Load override: %v", sc, err)
	}
	if s.Audit.Addr != "other-audit:1234" {
		t.Errorf("%s: override Audit.Addr = %q", sc, s.Audit.Addr)
	}
	t.Setenv("UPSTREAM_AUDIT_ADDR", "")
	_, err = config.Load()
	wantErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	baseEnv(t, "local")
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	wantErrNaming(t, err, "DATABASE_URL")

	os.Unsetenv("DATABASE_URL")
	_, err = config.Load()
	wantErrNaming(t, err, "DATABASE_URL")

	t.Setenv("DATABASE_ENABLED", "false")
	os.Unsetenv("DATABASE_URL")
	_, err = config.Load()
	wantErrNaming(t, err, "DATABASE_URL")
}

func TestLoadRequiresAuditAddr(t *testing.T) {
	for _, env := range []string{"local", "test", "staging", "prod"} {
		t.Run("scenario Audit address is required in every environment "+env, func(t *testing.T) {
			baseEnv(t, env)
			t.Setenv("UPSTREAM_AUDIT_ADDR", "")
			_, err := config.Load()
			wantErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")

			os.Unsetenv("UPSTREAM_AUDIT_ADDR")
			_, err = config.Load()
			wantErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")
		})
	}
}

func TestStrictEnvRefusesReflection(t *testing.T) {
	for _, env := range []string{"staging", "prod"} {
		t.Run("scenario Strict ENV refuses reflection "+env, func(t *testing.T) {
			baseEnv(t, env)
			t.Setenv("GRPC_REFLECTION_ENABLED", "true")
			_, err := config.Load()
			wantErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
		})
	}
	t.Run("scenario Strict ENV refuses reflection: local allows", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("GRPC_REFLECTION_ENABLED", "true")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if !s.ReflectionEnabled {
			t.Error("ReflectionEnabled = false, want true")
		}
	})
}

func TestStrictEnvRefusesMockDataFirst(t *testing.T) {
	const sc = "scenario Strict ENV refuses mock data first"
	for _, env := range []string{"staging", "prod"} {
		t.Run(sc+" "+env, func(t *testing.T) {
			baseEnv(t, env)
			os.Unsetenv("DATABASE_URL")
			os.Unsetenv("UPSTREAM_AUDIT_ADDR")
			t.Setenv("MOCK_DATA", "anything")
			_, err := config.Load()
			wantErrNaming(t, err, "MOCK_DATA")
			if strings.Contains(err.Error(), "DATABASE_URL") || strings.Contains(err.Error(), "UPSTREAM_AUDIT_ADDR") {
				t.Errorf("%s: err = %v, want the MOCK_DATA refusal before validation errors", sc, err)
			}
		})
	}
	t.Run(sc+" local allows value", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("MOCK_DATA", "anything")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.MockData.Adapters != "anything" {
			t.Errorf("MockData.Adapters = %q", s.MockData.Adapters)
		}
	})
}

func TestRetiredTimeoutKeyIgnored(t *testing.T) {
	const sc = "scenario Retired timeout key ignored"
	for _, key := range []string{"UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_CALL_TIMEOUT"} {
		for _, val := range []string{"5", "not-a-number"} {
			t.Run(sc+" "+key+"="+val, func(t *testing.T) {
				baseEnv(t, "local")
				t.Setenv(key, val)
				if _, err := config.Load(); err != nil {
					t.Fatalf("%s: Load: %v", sc, err)
				}
			})
		}
	}
	if slices.Contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "UPSTREAM_CALL_TIMEOUT_SECONDS") {
		t.Errorf("%s: key still declared", sc)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	const sc = "scenario Env example matches the settings"
	if err := sdkconfig.CheckEnvExample("../../.env.example", config.Settings{}); err != nil {
		t.Fatalf("%s: %v", sc, err)
	}
	cleanEnv(t)
	f, err := os.Open("../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc2 := bufio.NewScanner(f)
	for sc2.Scan() {
		line := strings.TrimSpace(sc2.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s: malformed line %q", sc, line)
		}
		t.Setenv(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("%s: Load of the example: %v", sc, err)
	}
	if s.Postgres.URL == "" || s.Audit.Addr != "team-audit-svc:50066" {
		t.Errorf("%s: URL = %q, audit = %q", sc, s.Postgres.URL, s.Audit.Addr)
	}
}
