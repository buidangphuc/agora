package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-referral/internal/config"
)

const (
	testDSN   = "postgres://test:pass@localhost:5432/db"
	sdkModule = "github.com/buidangphuc/agora/platform-core/packages/go-sdk"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
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

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestModuleRequiresSDKTagWithoutReplace(t *testing.T) {
	// scenario: The module requires the tag without a replace
	root := serviceRoot(t)
	required := false
	inRequire, inReplace := false, false
	for _, raw := range strings.Split(readFile(t, filepath.Join(root, "go.mod")), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "require ("):
			inRequire = true
			continue
		case strings.HasPrefix(line, "replace ("):
			inReplace = true
			continue
		case line == ")":
			inRequire, inReplace = false, false
			continue
		}
		if strings.HasPrefix(line, "replace ") || inReplace {
			t.Errorf("go.mod contains a replace: %q", line)
		}
		spec := line
		if strings.HasPrefix(line, "require ") {
			spec = strings.TrimSpace(strings.TrimPrefix(line, "require "))
		} else if !inRequire {
			continue
		}
		f := strings.Fields(spec)
		if len(f) >= 2 && f[0] == sdkModule && f[1] == "v1.0.2" {
			required = true
		}
	}
	if !required {
		t.Errorf("go.mod must require %s v1.0.2", sdkModule)
	}
	if _, err := os.Lstat(filepath.Join(root, "third_party")); err == nil {
		t.Error("third_party must not exist")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func baseEnv(t *testing.T, env string) {
	t.Helper()
	cleanEnv(t)
	if env != "" {
		t.Setenv("ENV", env)
	}
	t.Setenv("GRPC_PORT", "50062")
	t.Setenv("DATABASE_URL", testDSN)
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	// scenario: Missing database URL fails the load
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50062")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestLoadRequiresEnvAndPort(t *testing.T) {
	// scenario: Missing ENV or port fails the load
	cleanEnv(t)
	t.Setenv("DATABASE_URL", testDSN)
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded without ENV and GRPC_PORT")
	}
	for _, key := range []string{"ENV", "GRPC_PORT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("err = %v, want it to name %s", err, key)
		}
	}
}

func TestLoadIgnoresRemovedEnableFlags(t *testing.T) {
	// scenario: A removed enable flag is ignored, not honoured
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50062")
	t.Setenv("DATABASE_ENABLED", "false")
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
	baseEnv(t, "local")
	t.Setenv("DATABASE_ENABLED", "false")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load with DATABASE_ENABLED=false: %v", err)
	}
	if s.Postgres.URL != testDSN {
		t.Errorf("Postgres.URL = %q, want %q", s.Postgres.URL, testDSN)
	}
}

func TestNoEnableFlagsDeclared(t *testing.T) {
	// scenario: No enable flag is declared
	var got []string
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		if strings.HasSuffix(k, "_ENABLED") {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	want := []string{"GRPC_REFLECTION_ENABLED", "OTEL_ENABLED"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("declared enable flags = %v, want %v", got, want)
	}
	if len(sdkconfig.DeclaredEnvKeys(config.Settings{})) == 0 {
		t.Fatal("Settings declares no keys")
	}
}
