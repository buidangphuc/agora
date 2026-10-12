package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

type svcSettings struct {
	config.Runtime
	config.GRPC
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "50065")
	t.Setenv("LOG_LEVEL", "debug")

	s, err := config.Load[svcSettings]()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 50065 {
		t.Errorf("Port = %d, want 50065", s.Port)
	}
	if s.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", s.LogLevel)
	}
	if s.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", s.Host)
	}
	if got := s.ShutdownGrace(); got != 10*time.Second {
		t.Errorf("ShutdownGrace = %v, want 10s", got)
	}
	if got := s.Addr(); got != "0.0.0.0:50065" {
		t.Errorf("Addr = %q", got)
	}
	if !s.IsLocal() {
		t.Error("IsLocal = false for default ENV=local")
	}
}

func TestLoadReportsBadVariable(t *testing.T) {
	t.Setenv("GRPC_PORT", "abc")
	t.Setenv("LOG_JSON", "maybe")

	_, err := config.Load[svcSettings]()
	if err == nil {
		t.Fatal("Load returned nil error")
	}
	for _, name := range []string{"GRPC_PORT", "LOG_JSON"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %s", err, name)
		}
	}
}

func TestLoadRequiredUnset(t *testing.T) {
	_, err := config.Load[svcSettings]()
	if err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("err = %v, want one naming GRPC_PORT", err)
	}
}

func TestCheckEnvExampleReportsBothDirections(t *testing.T) {
	type decl struct {
		A string `env:"A"`
		B string `env:"B"`
	}
	path := filepath.Join(t.TempDir(), ".env.example")
	body := "# comment\n\nB=1\nC=2\nlower=3\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	err := config.CheckEnvExample(path, decl{})
	if err == nil {
		t.Fatal("expected drift error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing from .env.example: A") {
		t.Errorf("missing A not reported: %s", msg)
	}
	if !strings.Contains(msg, "stale keys in .env.example: C") {
		t.Errorf("stale C not reported: %s", msg)
	}
	if strings.Contains(msg, "lower") {
		t.Errorf("lowercase line must be ignored: %s", msg)
	}

	if err := os.WriteFile(path, []byte("A=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := config.CheckEnvExample(path, &decl{}); err != nil {
		t.Errorf("in-sync file reported drift: %v", err)
	}
	if err := config.CheckEnvExample(filepath.Join(t.TempDir(), "nope"), decl{}); err == nil {
		t.Error("missing file must error")
	}
}

func TestDeclaredEnvKeysIncludesNested(t *testing.T) {
	type extra struct {
		Z string `env:"Z,required"`
		N string // no env tag
	}
	type all struct {
		config.Runtime
		config.OTel
		Extra extra
		Ptr   *extra
		skip  string `env:"UNEXPORTED"`
	}
	_ = all{}.skip

	got := config.DeclaredEnvKeys(all{})
	want := []string{"ENV", "LOG_JSON", "LOG_LEVEL", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "Z"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("keys = %v, want %v", got, want)
	}
}

type validatedSettings struct {
	config.GRPC
	Postgres config.Postgres
}

func (s validatedSettings) Validate() error { return s.Postgres.Validate() }

func TestLoadRunsValidate(t *testing.T) {
	t.Setenv("GRPC_PORT", "1")

	_, err := config.Load[validatedSettings]() // DATABASE_URL unset
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}

	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	if _, err := config.Load[validatedSettings](); err != nil {
		t.Fatalf("with URL: %v", err)
	}
}

// The database has no enable flag any more: a leftover DATABASE_ENABLED=false
// in an old environment must not let a missing URL through.
func TestDatabaseURLRequiredEvenWithLegacyDisableFlag(t *testing.T) {
	t.Setenv("GRPC_PORT", "1")
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")

	_, err := config.Load[validatedSettings]()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestDrainDelay(t *testing.T) {
	t.Setenv("GRPC_PORT", "1")
	t.Setenv("SHUTDOWN_DRAIN_DELAY_SECONDS", "1.5")
	s, err := config.Load[svcSettings]()
	if err != nil || s.DrainDelay() != 1500*time.Millisecond {
		t.Fatalf("DrainDelay = %v, %v", s.DrainDelay(), err)
	}
}

// An empty GRPC_PORT= parses as 0 and would otherwise pass "required".
func TestEmptyGRPCPortFailsValidation(t *testing.T) {
	t.Setenv("GRPC_PORT", "")

	_, err := config.Load[svcSettings]()
	if err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("err = %v, want one naming GRPC_PORT", err)
	}
}
