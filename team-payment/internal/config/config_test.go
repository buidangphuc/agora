package config_test

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-payment/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "DATABASE_ENABLED", "KAFKA_ENABLED",
	"KAFKA_BROKERS", "PAYMENT_SETTLEMENT_CONSUMER_ENABLED", "MOCK_PAYMENTS", "MOCK_DATA",
	"SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR",
	"LOG_KEY_FP_SECRET", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
	"PAYOUT_HOLD_DAYS", "PAYOUT_HOLD_WINDOW", "PAYMENT_EVENTS_TOPIC", "ORDER_EVENTS_TOPIC",
	"PAYMENT_SETTLEMENT_CONSUMER_GROUP", "PAYMENT_SETTLEMENT_DLQ_TOPIC",
	"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
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
	t.Setenv("GRPC_PORT", "50056")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_ORDER_ADDR", "order:50055")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "audit:50066")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
}

func declaredKeys(t reflect.Type, out map[string]struct{}) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if tag, ok := f.Tag.Lookup("env"); ok {
			out[strings.Split(tag, ",")[0]] = struct{}{}
		}
		if f.Type.Kind() == reflect.Struct {
			declaredKeys(f.Type, out)
		}
	}
}

func declared() map[string]struct{} {
	out := map[string]struct{}{}
	declaredKeys(reflect.TypeOf(config.Settings{}), out)
	return out
}

func mustErrNaming(t *testing.T, err error, keys ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Load succeeded, want an error naming %v", keys)
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err, k)
		}
	}
}

func TestLoadRefusesMissingEnv(t *testing.T) {
	// scenario: A missing ENV refuses to start
	localEnv(t)
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")
}

func TestLoadRefusesEmptyDatabaseURL(t *testing.T) {
	// scenario: An empty database URL refuses to start in every environment
	localEnv(t)
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestRemovedKeysAreNotDeclared(t *testing.T) {
	// scenario: Removed keys are not declared
	keys := declared()
	for _, k := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "PAYMENT_SETTLEMENT_CONSUMER_ENABLED", "MOCK_PAYMENTS", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS"} {
		if _, ok := keys[k]; ok {
			t.Errorf("Settings declares removed key %s", k)
		}
	}
	for _, k := range []string{"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "MOCK_DATA", "LOG_KEY_FP_SECRET", "KAFKA_BROKERS", "UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR", "SHUTDOWN_GRACE", "DATABASE_URL", "PAYOUT_HOLD_DAYS", "PAYOUT_HOLD_WINDOW"} {
		if _, ok := keys[k]; !ok {
			t.Errorf("Settings does not declare %s", k)
		}
	}
}

func TestReflectionRefusedInStrictEnv(t *testing.T) {
	// scenario: Reflection is refused in a strict environment
	strictEnv(t, "production")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
}

func TestStrictEnvRequiresBrokersAuditAndKeySecret(t *testing.T) {
	// scenario: A strict environment requires brokers, upstream addresses and key secret
	localEnv(t)
	t.Setenv("ENV", "staging")
	for _, k := range []string{"UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR"} {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	_, err := config.Load()
	mustErrNaming(t, err, "KAFKA_BROKERS", "UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET")
}

func TestUpstreamAddrsRequiredInEveryEnv(t *testing.T) {
	// scenario: Upstream addresses are required in every environment
	// scenario: Payment audit: no audit upstream configured
	localEnv(t)
	for _, k := range []string{"UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR"} {
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	_, err := config.Load()
	mustErrNaming(t, err, "UPSTREAM_ORDER_ADDR", "UPSTREAM_AUDIT_ADDR")

	localEnv(t)
	if err := os.Unsetenv("UPSTREAM_AUDIT_ADDR"); err != nil {
		t.Fatal(err)
	}
	_, err = config.Load()
	mustErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")

	strictEnv(t, "production")
	if err := os.Unsetenv("UPSTREAM_AUDIT_ADDR"); err != nil {
		t.Fatal(err)
	}
	_, err = config.Load()
	mustErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")

	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load with both addresses set: %v", err)
	}
	if got := reflect.ValueOf(s).FieldByName("Kafka").FieldByName("Brokers").String(); got != "localhost:9092" {
		t.Errorf("Kafka.Brokers = %q, want localhost:9092", got)
	}
}

func TestPayoutHoldWindow(t *testing.T) {
	// scenario: An invalid payout hold window still refuses to start and names its key
	cases := []struct {
		name, days, window string
		want               time.Duration
		errKey             string
	}{
		{name: "unset is 7 days", want: 7 * 24 * time.Hour},
		{name: "days", days: "3", want: 3 * 24 * time.Hour},
		{name: "0 disables", days: "0", want: 0},
		{name: "max days", days: "3650", want: 3650 * 24 * time.Hour},
		{name: "negative days", days: "-1", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "too many days", days: "3651", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "non-numeric days", days: "abc", errKey: "PAYOUT_HOLD_DAYS"},
		{name: "window overrides days", days: "7", window: "20s", want: 20 * time.Second},
		{name: "window 0 disables", window: "0s", want: 0},
		{name: "unparsable window", window: "abc", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "negative window", window: "-1s", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "window above 3650 days", window: "87601h", errKey: "PAYOUT_HOLD_WINDOW"},
		{name: "invalid days still refused under a valid window", days: "-1", window: "20s", errKey: "PAYOUT_HOLD_DAYS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			localEnv(t)
			if tc.days != "" {
				t.Setenv("PAYOUT_HOLD_DAYS", tc.days)
			}
			if tc.window != "" {
				t.Setenv("PAYOUT_HOLD_WINDOW", tc.window)
			}
			s, err := config.Load()
			if tc.errKey != "" {
				mustErrNaming(t, err, tc.errKey)
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if s.Payout.Window != tc.want {
				t.Fatalf("window = %s, want %s", s.Payout.Window, tc.want)
			}
		})
	}
}

var exampleKey = regexp.MustCompile(`^#?\s*([A-Z][A-Z0-9_]*)=`)

func TestEnvExampleInSync(t *testing.T) {
	// scenario: .env.example documents every key
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	f, err := os.Open(filepath.Join(filepath.Dir(file), "..", "..", ".env.example"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m := exampleKey.FindStringSubmatch(sc.Text()); m != nil {
			got[m[1]] = struct{}{}
		}
	}
	var missing []string
	for k := range declared() {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf(".env.example lacks declared keys: %v", missing)
	}
	for _, k := range []string{"DATABASE_ENABLED", "KAFKA_ENABLED", "PAYMENT_SETTLEMENT_CONSUMER_ENABLED", "MOCK_PAYMENTS", "SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS"} {
		if _, ok := got[k]; ok {
			t.Errorf(".env.example still lists removed key %s", k)
		}
	}
}

func TestMockDataRefusedBeforeValidation(t *testing.T) {
	// scenario: A strict environment refuses mock data before validating anything else
	cleanEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("MOCK_DATA", "payment-provider")
	t.Setenv("DATABASE_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "MOCK_DATA")
	if err != nil && strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error %q must not be the DATABASE_URL error", err)
	}
}
