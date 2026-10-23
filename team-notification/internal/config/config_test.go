package config_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-notification/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "KAFKA_ENABLED", "KAFKA_BROKER", "KAFKA_BROKERS", "LOG_KEY_FP_SECRET",
	"SHUTDOWN_GRACE_SECONDS", "MOCK_DATA",
	"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
	"LOW_STOCK_THRESHOLD", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR",
	"LISTING_EVENTS_TOPIC", "LISTING_EVENTS_DLQ_TOPIC", "NOTIFICATION_LISTING_CONSUMER_GROUP",
	"CHAT_EVENTS_TOPIC", "CHAT_EVENTS_DLQ_TOPIC", "NOTIFICATION_CHAT_CONSUMER_GROUP",
	"ORDER_EVENTS_TOPIC", "ORDER_EVENTS_DLQ_TOPIC", "NOTIFICATION_ORDER_CONSUMER_GROUP",
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

func localEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50058")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "team-domain-svc:50051")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "team-identity-svc:50053")
}

func strictEnv(t *testing.T) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", "staging")
	t.Setenv("KAFKA_BROKERS", "broker-1:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret-for-tests")
}

func envValue(t *testing.T, v any, key string) any {
	t.Helper()
	var found any
	var walk func(rv reflect.Value)
	walk = func(rv reflect.Value) {
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Tag.Get("env") == key {
				found = rv.Field(i).Interface()
				return
			}
			if f.Type.Kind() == reflect.Struct && f.Type.PkgPath() != "time" {
				walk(rv.Field(i))
			}
		}
	}
	walk(reflect.ValueOf(v))
	if found == nil {
		t.Fatalf("Settings declares no field for %s", key)
	}
	return found
}

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	// scenario: A missing DATABASE_URL fails load
	localEnv(t)
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
	t.Setenv("DATABASE_URL", "postgres://x/y")
	if _, err := config.Load(); err != nil {
		t.Fatalf("Load with all required keys: %v", err)
	}
}

func TestLoadRequiresKafkaBrokersAndKeyFPSecretInStrictEnv(t *testing.T) {
	// scenario: Kafka brokers and the key fingerprint secret are required in a strict ENV
	strictEnv(t)
	if err := os.Unsetenv("KAFKA_BROKERS"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
		t.Fatalf("no brokers: err = %v, want an error naming KAFKA_BROKERS", err)
	}
	t.Setenv("KAFKA_BROKERS", "broker-1:9092")
	if err := os.Unsetenv("LOG_KEY_FP_SECRET"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "LOG_KEY_FP_SECRET") {
		t.Fatalf("no secret: err = %v, want an error naming LOG_KEY_FP_SECRET", err)
	}
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret-for-tests")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load with both: %v", err)
	}
	if got := envValue(t, s, "KAFKA_BROKERS"); got != "broker-1:9092" {
		t.Errorf("KAFKA_BROKERS = %v", got)
	}

	localEnv(t)
	s, err = config.Load()
	if err != nil {
		t.Fatalf("local Load without brokers or secret: %v", err)
	}
	if got := envValue(t, s, "KAFKA_BROKERS"); got != "localhost:9092" {
		t.Errorf("local KAFKA_BROKERS = %v, want the SDK default localhost:9092", got)
	}
}

func TestLoadRequiresUpstreamAddrsInEveryEnv(t *testing.T) {
	// scenario: Upstream addresses are required in every ENV
	for _, env := range []string{"prod", "local"} {
		for _, key := range []string{"UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR"} {
			strictEnv(t)
			t.Setenv("ENV", env)
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("ENV=%s without %s: err = %v, want an error naming it", env, key, err)
			}
		}
	}
	strictEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := envValue(t, s, "UPSTREAM_DOMAIN_ADDR"); got != "team-domain-svc:50051" {
		t.Errorf("UPSTREAM_DOMAIN_ADDR = %v", got)
	}
	if got := envValue(t, s, "UPSTREAM_IDENTITY_ADDR"); got != "team-identity-svc:50053" {
		t.Errorf("UPSTREAM_IDENTITY_ADDR = %v", got)
	}
}

func TestNoDependencyEnableFlags(t *testing.T) {
	// scenario: No dependency enable flag exists
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		if k == "DATABASE_ENABLED" || k == "KAFKA_ENABLED" || k == "SHUTDOWN_GRACE_SECONDS" {
			t.Errorf("Settings declares removed key %s", k)
		}
		if strings.HasSuffix(k, "_ENABLED") && k != "OTEL_ENABLED" && k != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("Settings declares enable flag %s", k)
		}
	}
	root := serviceRoot(t)
	enableRe := regexp.MustCompile(`[A-Z][A-Z0-9_]*_ENABLED|SHUTDOWN_GRACE_SECONDS`)
	for _, name := range []string{".env.example", "README.md"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range enableRe.FindAllString(string(b), -1) {
			if m != "OTEL_ENABLED" && m != "GRPC_REFLECTION_ENABLED" {
				t.Errorf("%s mentions %s", name, m)
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, "internal", "config", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range enableRe.FindAllString(string(b), -1) {
			if m != "OTEL_ENABLED" && m != "GRPC_REFLECTION_ENABLED" {
				t.Errorf("internal/config/%s mentions %s", e.Name(), m)
			}
		}
	}
}

func TestNoDependencyEnableFlagsInServiceSources(t *testing.T) {
	// scenario: No dependency enable flag exists
	root := serviceRoot(t)
	enableRe := regexp.MustCompile(`[A-Z][A-Z0-9_]*_ENABLED|SHUTDOWN_GRACE_SECONDS`)
	scanned := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "generated", ".git", "node_modules", "proto":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		scanned++
		rel, _ := filepath.Rel(root, p)
		for _, m := range enableRe.FindAllString(string(b), -1) {
			if m != "OTEL_ENABLED" && m != "GRPC_REFLECTION_ENABLED" {
				t.Errorf("%s mentions %s", rel, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no source files scanned")
	}
}

func TestLoadRefusesReflectionInStrictEnv(t *testing.T) {
	// scenario: Reflection is refused in a strict ENV and off by default
	cleanEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("GRPC_PORT", "50058")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded in prod with reflection and no other settings")
	}
	if !strings.Contains(err.Error(), "GRPC_REFLECTION_ENABLED") {
		t.Errorf("err = %v, want the reflection refusal", err)
	}
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("err = %v, must not name DATABASE_URL (refusal comes before group validation)", err)
	}

	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("local Load: %v", err)
	}
	if s.ReflectionEnabled {
		t.Error("ReflectionEnabled = true with the variable unset, want false")
	}
}

func TestLoadLowStockThreshold(t *testing.T) {
	// scenario: The low-stock threshold boundaries
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("default Load: %v", err)
	}
	if got := envValue(t, s, "LOW_STOCK_THRESHOLD"); got != int32(5) {
		t.Errorf("default threshold = %v (%T), want int32(5)", got, got)
	}
	for raw, want := range map[string]int32{"0": 0, "10": 10, "2147483647": 2147483647} {
		localEnv(t)
		t.Setenv("LOW_STOCK_THRESHOLD", raw)
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load(%q): %v", raw, err)
		}
		if got := envValue(t, s, "LOW_STOCK_THRESHOLD"); got != want {
			t.Errorf("threshold(%q) = %v, want %d", raw, got, want)
		}
	}
	for _, bad := range []string{"-1", "2147483648", "abc", "1.5"} {
		localEnv(t)
		t.Setenv("LOW_STOCK_THRESHOLD", bad)
		_, err := config.Load()
		if err == nil || !strings.Contains(err.Error(), "invalid LOW_STOCK_THRESHOLD") {
			t.Errorf("Load(%q): err = %v, want invalid LOW_STOCK_THRESHOLD", bad, err)
		}
	}
}

func TestConsumerSettingsDefaults(t *testing.T) {
	// scenario: Consumer settings keep their names and defaults
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"LISTING_EVENTS_TOPIC":                "listing.events",
		"LISTING_EVENTS_DLQ_TOPIC":            "listing.events.dlq",
		"NOTIFICATION_LISTING_CONSUMER_GROUP": "team-notification.listing",
		"CHAT_EVENTS_TOPIC":                   "chat.events",
		"CHAT_EVENTS_DLQ_TOPIC":               "chat.events.dlq",
		"NOTIFICATION_CHAT_CONSUMER_GROUP":    "team-notification.chat",
		"ORDER_EVENTS_TOPIC":                  "order.events",
		"ORDER_EVENTS_DLQ_TOPIC":              "order.events.dlq",
		"NOTIFICATION_ORDER_CONSUMER_GROUP":   "team-notification.order",
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if got := envValue(t, s, k); got != want[k] {
			t.Errorf("%s = %v, want %q", k, got, want[k])
		}
	}
	t.Setenv("CHAT_EVENTS_TOPIC", "chat.custom")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := envValue(t, s, "CHAT_EVENTS_TOPIC"); got != "chat.custom" {
		t.Errorf("CHAT_EVENTS_TOPIC override = %v", got)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	// scenario: .env.example matches Settings
	path := filepath.Join(serviceRoot(t), ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatalf("CheckEnvExample: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "SHUTDOWN_GRACE_SECONDS", "KAFKA_BROKER"} {
		if regexp.MustCompile(`(?m)^#?\s*` + name + `=`).Match(b) {
			t.Errorf(".env.example still lists removed key %s", name)
		}
	}
	if !regexp.MustCompile(`(?m)^GRPC_REFLECTION_ENABLED=false\s*$`).Match(b) {
		t.Error(".env.example must set GRPC_REFLECTION_ENABLED=false")
	}
}

func TestTask5ReadmeDocumentsOrderWorkEventsAndLowStockThreshold(t *testing.T) {
	// scenario: Notification rules keep their behaviour on the new wiring
	b, err := os.ReadFile(filepath.Join(serviceRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	readme := string(b)
	for _, want := range []string{
		"LOW_STOCK_THRESHOLD", "OrderPlaced", "OrderPaidEvent", "OrderPaymentReminder",
		"OrderCancelled", "OrderCompleted", "ReturnRequested", "ReturnRefunded",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md does not mention %s", want)
		}
	}
	for _, key := range []string{"ENV", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "LOG_KEY_FP_SECRET", "DB_MAX_CONNS", "KAFKA_BROKERS"} {
		if !strings.Contains(readme, "`"+key+"`") {
			t.Errorf("README env table does not list %s", key)
		}
	}
	for _, gone := range []string{"Unimplemented", "mock mode", "KAFKA_BROKER`"} {
		if strings.Contains(readme, gone) && gone != "KAFKA_BROKER`" {
			t.Errorf("README still contains %q", gone)
		}
	}
}
