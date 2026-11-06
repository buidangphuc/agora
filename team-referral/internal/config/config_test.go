package config_test

import (
	"os"
	"path/filepath"
	"regexp"
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

var sdkTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

var lintToolRe = regexp.MustCompile(`(?m)^\s*(?:tool\s+)?github\.com/golangci/golangci-lint/v2/cmd/golangci-lint\s*$`)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
	"KAFKA_BROKERS", "OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
	"REFERRAL_REWARD_XU", "REFERRAL_DAILY_CAP", "REFERRAL_LIFETIME_CAP", "REFERRAL_REDEEM_MAX_ACCOUNT_AGE",
	"REFERRAL_RELEASE_SWEEP_INTERVAL", "REFERRAL_CAP_TIMEZONE", "IDENTITY_EVENTS_TOPIC", "ORDER_EVENTS_TOPIC",
	"REFERRAL_EVENTS_TOPIC", "REFERRAL_IDENTITY_CONSUMER_GROUP", "REFERRAL_ORDER_CONSUMER_GROUP",
	"REFERRAL_IDENTITY_DLQ_TOPIC", "REFERRAL_ORDER_DLQ_TOPIC",
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
	requires := 0
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
		if len(f) >= 2 && f[0] == sdkModule {
			requires++
			if !sdkTagRe.MatchString(f[1]) {
				t.Errorf("go-sdk must be a tagged release vX.Y.Z, got %q", f[1])
			}
		}
	}
	if requires != 1 {
		t.Errorf("go.mod has %d requires of %s, want exactly 1", requires, sdkModule)
	}
	if !lintToolRe.MatchString(readFile(t, filepath.Join(root, "go.mod"))) {
		t.Error("go.mod must pin golangci-lint as a tool")
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
	declared := strings.Join(sdkconfig.DeclaredEnvKeys(config.Settings{}), ",")
	for _, k := range []string{"KAFKA_BROKERS", "OUTBOX_POLL_INTERVAL", "REFERRAL_DAILY_CAP", "REFERRAL_EVENTS_TOPIC"} {
		if !strings.Contains(declared, k) {
			t.Errorf("Settings does not declare %s", k)
		}
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("declared enable flags = %v, want %v", got, want)
	}
	if len(sdkconfig.DeclaredEnvKeys(config.Settings{})) == 0 {
		t.Fatal("Settings declares no keys")
	}
}
