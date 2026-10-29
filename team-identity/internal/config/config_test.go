package config_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-identity/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET",
	"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK", "OUTBOX_PURGE_RETENTION", "OUTBOX_PURGE_INTERVAL",
	"UPSTREAM_AUDIT_ADDR", "IDENTITY_EVENTS_TOPIC", "JWT_PRIVATE_KEY", "JWT_KID", "JWKS_HTTP_PORT",
	"JWT_TTL_SECONDS", "SEED_ADMIN_ENABLED", "SEED_ADMIN_USERNAME", "SEED_ADMIN_PASSWORD",
	"PASSWORD_RESET_EXPOSE_TOKEN", "SESSION_RETENTION", "LOGIN_HISTORY_RETENTION", "SESSION_SWEEP_INTERVAL",
	"SESSION_SWEEP_BATCH_SIZE", "DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED",
	"OUTBOX_MAX_ATTEMPTS", "OUTBOX_CLAIM_LOCK_SECONDS", "SESSION_SWEEP_ENABLED",
	"UPSTREAM_CALL_TIMEOUT_SECONDS", "SHUTDOWN_GRACE_SECONDS", "MOCK_DATA",
}

var removedKeys = []string{
	"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "OUTBOX_MAX_ATTEMPTS", "OUTBOX_CLAIM_LOCK_SECONDS",
	"SESSION_SWEEP_ENABLED", "UPSTREAM_CALL_TIMEOUT_SECONDS", "SHUTDOWN_GRACE_SECONDS", "MOCK_DATA",
}

const devKID = "dev-2026"

func cleanEnv(t *testing.T) {
	t.Helper()
	for _, k := range managedKeys {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func unset(t *testing.T, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

func freshPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

var composeKey = regexp.MustCompile(`JWT_PRIVATE_KEY='(-----BEGIN [^']+)'`)

func committedDevPEM(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docker-compose.services.yaml"))
	if err != nil {
		t.Fatalf("compose file with the committed development key: %v", err)
	}
	m := composeKey.FindSubmatch(raw)
	if m == nil {
		t.Fatal("no JWT_PRIVATE_KEY in docker-compose.services.yaml")
	}
	pemText := strings.ReplaceAll(string(m[1]), `\n`, "\n")
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		t.Fatal("compose JWT_PRIVATE_KEY is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		t.Fatal("compose key is not RSA")
	}
	dev := devKey(t, devKID)
	if !rk.PublicKey.Equal(dev.pub) {
		t.Fatal("compose key does not match the committed development public key")
	}
	return pemText
}

func localEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "51053")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "audit:50061")
	t.Setenv("JWT_PRIVATE_KEY", freshPEM(t))
	t.Setenv("JWT_KID", "k1")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
}

func mustContain(t *testing.T, err error, names ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error naming %v, got nil", names)
	}
	for _, n := range names {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("error %q must name %s", err, n)
		}
	}
}

func valueByEnv(t *testing.T, s any, key string) any {
	t.Helper()
	var find func(reflect.Value) (any, bool)
	find = func(v reflect.Value) (any, bool) {
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if tag, ok := f.Tag.Lookup("env"); ok {
				if k, _, _ := strings.Cut(tag, ","); k == key {
					return v.Field(i).Interface(), true
				}
				continue
			}
			if v.Field(i).Kind() == reflect.Struct {
				if got, ok := find(v.Field(i)); ok {
					return got, true
				}
			}
		}
		return nil, false
	}
	got, ok := find(reflect.ValueOf(s))
	if !ok {
		t.Fatalf("no Settings field bound to %s", key)
	}
	return got
}

func mustLoad(t *testing.T) config.Settings {
	t.Helper()
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// Scenario: Load reads the environment
func TestLoadReadsEnvironment(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	want := map[string]any{
		"GRPC_PORT":              51053,
		"DATABASE_URL":           "postgres://x/y",
		"DB_MAX_CONNS":           int32(10),
		"OTEL_ENABLED":           false,
		"JWT_TTL_SECONDS":        3600,
		"JWKS_HTTP_PORT":         50063,
		"KAFKA_BROKERS":          "localhost:9092",
		"UPSTREAM_AUDIT_ADDR":    "audit:50061",
		"OUTBOX_PURGE_RETENTION": 168 * 3600 * 1e9,
		"OUTBOX_PURGE_INTERVAL":  3600 * 1e9,
	}
	for k, w := range want {
		got := valueByEnv(t, s, k)
		if gv := reflect.ValueOf(got); gv.CanInt() {
			if gv.Int() != reflect.ValueOf(w).Convert(gv.Type()).Int() {
				t.Errorf("%s = %v, want %v", k, got, w)
			}
			continue
		}
		if got != w {
			t.Errorf("%s = %v, want %v", k, got, w)
		}
	}
}

// Scenario: A missing database URL fails the load
func TestLoadRequiresDatabaseURL(t *testing.T) {
	localEnv(t)
	unset(t, "DATABASE_URL")
	_, err := config.Load()
	mustContain(t, err, "DATABASE_URL")
	t.Setenv("DATABASE_URL", "")
	_, err = config.Load()
	mustContain(t, err, "DATABASE_URL")
}

// Scenario: A missing port, environment, key or key id fails the load
func TestValidateRequiresPrivateKeyAndKID(t *testing.T) {
	for _, k := range []string{"GRPC_PORT", "ENV", "JWT_PRIVATE_KEY", "JWT_KID"} {
		t.Run(k, func(t *testing.T) {
			localEnv(t)
			t.Setenv(k, "")
			_, err := config.Load()
			mustContain(t, err, k)
		})
	}
}

// Scenario: No dependency enable flag is declared
func TestNoDependencyEnableFlags(t *testing.T) {
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, k := range removedKeys {
		if slices.Contains(keys, k) {
			t.Errorf("%s must not be declared", k)
		}
	}
	allowed := []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "SEED_ADMIN_ENABLED"}
	for _, k := range keys {
		if strings.HasSuffix(k, "_ENABLED") && !slices.Contains(allowed, k) {
			t.Errorf("unexpected enable flag %s", k)
		}
	}
	for _, k := range []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "SEED_ADMIN_ENABLED",
		"SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "OUTBOX_CLAIM_LOCK", "OUTBOX_PURGE_RETENTION",
		"OUTBOX_PURGE_INTERVAL", "LOG_KEY_FP_SECRET", "UPSTREAM_AUDIT_ADDR"} {
		if !slices.Contains(keys, k) {
			t.Errorf("%s must be declared", k)
		}
	}
}

// Scenario: Removed variables are ignored when set
func TestRemovedVariablesAreIgnored(t *testing.T) {
	localEnv(t)
	want := mustLoad(t)
	for _, k := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "SESSION_SWEEP_ENABLED"} {
		t.Setenv(k, "false")
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load with removed variables set: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("settings changed by removed variables:\n got %+v\nwant %+v", got, want)
	}
}

// Scenario: Strict ENV refuses reflection before validation
func TestStrictEnvRefusesReflectionBeforeValidation(t *testing.T) {
	strictEnv(t, "production")
	unset(t, "DATABASE_URL")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	mustContain(t, err, "GRPC_REFLECTION_ENABLED")
	if err != nil && strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("reflection refusal must come before group validation, got %q", err)
	}
}

// Scenario: Reflection is off by default and accepted locally
func TestReflectionDefaultsOffAndLocalAccepts(t *testing.T) {
	localEnv(t)
	if got := valueByEnv(t, mustLoad(t), "GRPC_REFLECTION_ENABLED"); got != false {
		t.Errorf("reflection default = %v, want false", got)
	}
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	if got := valueByEnv(t, mustLoad(t), "GRPC_REFLECTION_ENABLED"); got != true {
		t.Errorf("reflection = %v, want true", got)
	}
}

// Scenario: Strict ENV requires the brokers and log fingerprint secret
func TestStrictEnvRequiresDependencyAddresses(t *testing.T) {
	for _, key := range []string{"KAFKA_BROKERS", "LOG_KEY_FP_SECRET"} {
		strictEnv(t, "staging")
		t.Setenv(key, "")
		_, err := config.Load()
		mustContain(t, err, key)

		strictEnv(t, "local")
		t.Setenv(key, "")
		if _, err := config.Load(); err != nil {
			t.Errorf("ENV=local with empty %s must load: %v", key, err)
		}
	}
}

// Scenario: The audit address is required in every ENV
// Scenario: User enforcement: No audit upstream configured
func TestAuditAddrRequiredInEveryEnv(t *testing.T) {
	for _, env := range []string{"local", "production"} {
		strictEnv(t, env)
		unset(t, "UPSTREAM_AUDIT_ADDR")
		_, err := config.Load()
		mustContain(t, err, "UPSTREAM_AUDIT_ADDR")

		t.Setenv("UPSTREAM_AUDIT_ADDR", "")
		_, err = config.Load()
		mustContain(t, err, "UPSTREAM_AUDIT_ADDR")
	}
}

// Scenario: Purge settings are validated
func TestOutboxPurgeSettingsValidated(t *testing.T) {
	for _, key := range []string{"OUTBOX_PURGE_RETENTION", "OUTBOX_PURGE_INTERVAL"} {
		for _, v := range []string{"0s", "-1h"} {
			localEnv(t)
			t.Setenv(key, v)
			_, err := config.Load()
			mustContain(t, err, key)
		}
	}
	localEnv(t)
	t.Setenv("OUTBOX_PURGE_RETENTION", "24h")
	t.Setenv("OUTBOX_PURGE_INTERVAL", "10m")
	mustLoad(t)
}

// Scenario: The migrate loader needs only the environment and the database
func TestLoadMigrateNeedsOnlyDatabase(t *testing.T) {
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.LoadMigrate()
	if err != nil {
		t.Fatalf("LoadMigrate with only ENV and DATABASE_URL: %v", err)
	}
	if got := valueByEnv(t, s, "DATABASE_URL"); got != "postgres://x/y" {
		t.Errorf("DATABASE_URL = %v", got)
	}
	declared := sdkconfig.DeclaredEnvKeys(s)
	for _, k := range []string{"JWT_PRIVATE_KEY", "JWT_KID", "KAFKA_BROKERS", "GRPC_PORT", "UPSTREAM_AUDIT_ADDR"} {
		if slices.Contains(declared, k) {
			t.Errorf("migrate settings must not declare %s", k)
		}
	}
	t.Setenv("DATABASE_URL", "")
	_, err = config.LoadMigrate()
	mustContain(t, err, "DATABASE_URL")
}

// Scenario: The reset token is never exposed in a deployed ENV
func TestPasswordResetExposeTokenDefaultsOffAndRefusedInProd(t *testing.T) {
	localEnv(t)
	if got := valueByEnv(t, mustLoad(t), "PASSWORD_RESET_EXPOSE_TOKEN"); got != false {
		t.Fatalf("PASSWORD_RESET_EXPOSE_TOKEN default = %v, want false", got)
	}
	for _, env := range []string{"staging", "stage", "prod", "production", "Production"} {
		strictEnv(t, env)
		t.Setenv("PASSWORD_RESET_EXPOSE_TOKEN", "true")
		_, err := config.Load()
		mustContain(t, err, "PASSWORD_RESET_EXPOSE_TOKEN")
	}
	for _, env := range []string{"local", "test"} {
		localEnv(t)
		t.Setenv("ENV", env)
		t.Setenv("PASSWORD_RESET_EXPOSE_TOKEN", "true")
		if got := valueByEnv(t, mustLoad(t), "PASSWORD_RESET_EXPOSE_TOKEN"); got != true {
			t.Errorf("ENV=%s flag = %v, want true", env, got)
		}
	}
}

// Scenario: Seed admin keeps its guard
func TestSeedAdminValidation(t *testing.T) {
	cases := []struct {
		name    string
		enabled string
		user    string
		pw      string
		wantErr string
	}{
		{"enabled without password", "true", "admin", "", "SEED_ADMIN_PASSWORD"},
		{"enabled 11 chars", "true", "admin", "elevenchars", "SEED_ADMIN_PASSWORD"},
		{"enabled blank username", "true", " ", "a-long-enough-password", "SEED_ADMIN_USERNAME"},
		{"enabled 12 chars", "true", "admin", "twelve-chars", ""},
		{"unset ignores password", "", "admin", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			localEnv(t)
			if c.enabled != "" {
				t.Setenv("SEED_ADMIN_ENABLED", c.enabled)
			}
			t.Setenv("SEED_ADMIN_USERNAME", c.user)
			t.Setenv("SEED_ADMIN_PASSWORD", c.pw)
			_, err := config.Load()
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			mustContain(t, err, c.wantErr)
		})
	}
}

// Scenario: Seed admin keeps its guard (defaults off)
func TestSeedAdminDefaultsOff(t *testing.T) {
	localEnv(t)
	s := mustLoad(t)
	if valueByEnv(t, s, "SEED_ADMIN_ENABLED") != false || valueByEnv(t, s, "SEED_ADMIN_PASSWORD") != "" ||
		valueByEnv(t, s, "SEED_ADMIN_USERNAME") != "admin" {
		t.Errorf("seed defaults must be disabled, empty password, username admin")
	}
}

// Scenario: Seed admin keeps its guard (load fails fast)
func TestSeedAdminLoadFailsFast(t *testing.T) {
	localEnv(t)
	t.Setenv("SEED_ADMIN_ENABLED", "true")
	_, err := config.Load()
	mustContain(t, err, "SEED_ADMIN_PASSWORD")
}

// Scenario: The development signing key is refused when settings load in a deployed ENV
func TestStrictEnvRefusesDevSigningKey(t *testing.T) {
	cases := []struct {
		name string
		pem  func(*testing.T) string
		kid  string
		want string
	}{
		{"dev kid", func(t *testing.T) string { return freshPEM(t) }, devKID, "JWT_KID"},
		{"dev key under another kid", committedDevPEM, "renamed-kid", "JWT_PRIVATE_KEY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			strictEnv(t, "production")
			pemText := c.pem(t)
			t.Setenv("JWT_PRIVATE_KEY", pemText)
			t.Setenv("JWT_KID", c.kid)
			t.Setenv("LOG_KEY_FP_SECRET", "")
			_, err := config.Load()
			mustContain(t, err, "refusing to start", c.want, "LOG_KEY_FP_SECRET")
			if err == nil {
				return
			}
			body := strings.Join(strings.Fields(strings.NewReplacer("-----BEGIN PRIVATE KEY-----", "", "-----END PRIVATE KEY-----", "").Replace(pemText)), "")
			if strings.Contains(err.Error(), "PRIVATE KEY-----") || strings.Contains(err.Error(), body[:40]) {
				t.Errorf("error must not contain key material: %v", err)
			}
		})
	}
}

// Scenario: Every strict refusal of one load is reported together
func TestStrictEnvReportsAllRefusals(t *testing.T) {
	strictEnv(t, "staging")
	t.Setenv("PASSWORD_RESET_EXPOSE_TOKEN", "true")
	t.Setenv("JWT_PRIVATE_KEY", committedDevPEM(t))
	t.Setenv("JWT_KID", devKID)
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("LOG_KEY_FP_SECRET", "")
	_, err := config.Load()
	mustContain(t, err, "PASSWORD_RESET_EXPOSE_TOKEN", "refusing to start", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET")
}

// Scenario: .env.example matches the declared keys
func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if !slices.Contains(sdkconfig.DeclaredEnvKeys(config.Settings{}), "LOG_KEY_FP_SECRET") {
		t.Fatal("Settings must declare the SDK groups (LOG_KEY_FP_SECRET missing)")
	}
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatal(err)
	}
}
