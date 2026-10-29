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

	"github.com/buidangphuc/team-order/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "DATABASE_ENABLED", "KAFKA_ENABLED",
	"OUTBOX_ENABLED", "FEATURE_FLAGS_ENABLED", "KAFKA_BROKERS", "SHUTDOWN_GRACE_SECONDS",
	"UPSTREAM_CALL_TIMEOUT_SECONDS", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR",
	"UPSTREAM_PROMOTION_ADDR", "UPSTREAM_AUDIT_ADDR", "LOG_KEY_FP_SECRET", "OTEL_ENABLED",
	"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME", "FLIPT_ADDR", "FEATURE_FLAGS_EVAL_TIMEOUT",
	"FEATURE_FLAGS_EVAL_TIMEOUT_MS", "PAYMENT_EVENTS_TOPIC", "ORDER_PAYMENT_CONSUMER_GROUP",
	"PAYMENT_EVENTS_DLQ_TOPIC", "ORDER_EVENTS_TOPIC", "ENGAGEMENT_EVENTS_TOPIC",
	"ORDER_DISPUTE_CONSUMER_GROUP", "ENGAGEMENT_EVENTS_DLQ_TOPIC", "OUTBOX_POLL_INTERVAL",
	"OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK", "OUTBOX_CLAIM_LOCK_SECONDS", "OUTBOX_MAX_ATTEMPTS",
	"RESERVATION_TTL", "RESERVATION_SWEEP_INTERVAL", "PAYMENT_DEADLINE", "PAYMENT_REMINDER_BEFORE",
	"LIFECYCLE_SWEEP_INTERVAL", "AUTO_COMPLETE_AFTER", "RETURN_WINDOW", "RETURN_AUTO_APPROVE_AFTER",
	"SHIP_BY_AFTER", "MOCK_DATA",
}

var removedKeys = []string{
	"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "FEATURE_FLAGS_ENABLED",
	"SHUTDOWN_GRACE_SECONDS", "UPSTREAM_CALL_TIMEOUT_SECONDS", "OUTBOX_CLAIM_LOCK_SECONDS",
	"OUTBOX_MAX_ATTEMPTS", "FEATURE_FLAGS_EVAL_TIMEOUT_MS", "MOCK_DATA",
}

var upstreamKeys = []string{
	"UPSTREAM_AUDIT_ADDR", "UPSTREAM_DOMAIN_ADDR", "UPSTREAM_IDENTITY_ADDR", "UPSTREAM_PROMOTION_ADDR",
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
	t.Setenv("GRPC_PORT", "50055")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	t.Setenv("UPSTREAM_AUDIT_ADDR", "audit:50066")
	t.Setenv("UPSTREAM_DOMAIN_ADDR", "domain:50051")
	t.Setenv("UPSTREAM_IDENTITY_ADDR", "identity:50053")
	t.Setenv("UPSTREAM_PROMOTION_ADDR", "promotion:50061")
}

func strictEnv(t *testing.T, env string) {
	t.Helper()
	localEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
}

func toPtr(t *testing.T, v any) *config.Settings {
	t.Helper()
	switch s := v.(type) {
	case config.Settings:
		return &s
	case *config.Settings:
		return s
	}
	t.Fatalf("config.Load returned unexpected type %T", v)
	return nil
}

func mustLoad(t *testing.T) *config.Settings {
	t.Helper()
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return toPtr(t, s)
}

func str(t *testing.T, s *config.Settings, path ...string) string {
	t.Helper()
	v := reflect.ValueOf(s).Elem()
	for _, name := range path {
		v = reflect.Indirect(v).FieldByName(name)
		if !v.IsValid() {
			t.Fatalf("Settings.%s is not declared", strings.Join(path, "."))
		}
	}
	if v.Kind() != reflect.String {
		t.Fatalf("Settings.%s is %s, want string", strings.Join(path, "."), v.Kind())
	}
	return v.String()
}

func declaredKeys(t reflect.Type, out map[string]struct{}) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if tag, ok := f.Tag.Lookup("env"); ok {
			out[strings.Split(tag, ",")[0]] = struct{}{}
			continue
		}
		declaredKeys(f.Type, out)
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
	for _, env := range []string{"local", "test", "staging", "production"} {
		t.Run(env, func(t *testing.T) {
			strictEnv(t, env)
			t.Setenv("DATABASE_URL", "")
			_, err := config.Load()
			mustErrNaming(t, err, "DATABASE_URL")
		})
	}
}

func TestRemovedKeysAreNotDeclared(t *testing.T) {
	// scenario: Removed keys are not declared
	keys := declared()
	for _, k := range removedKeys {
		if _, ok := keys[k]; ok {
			t.Errorf("Settings declares removed key %s", k)
		}
	}
	for _, k := range []string{
		"OTEL_ENABLED", "GRPC_REFLECTION_ENABLED", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
		"OUTBOX_CLAIM_LOCK", "FEATURE_FLAGS_EVAL_TIMEOUT", "LOG_KEY_FP_SECRET", "KAFKA_BROKERS",
		"DATABASE_URL", "FLIPT_ADDR", "UPSTREAM_AUDIT_ADDR", "UPSTREAM_DOMAIN_ADDR",
		"UPSTREAM_IDENTITY_ADDR", "UPSTREAM_PROMOTION_ADDR",
	} {
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

func TestStrictEnvRequiresBrokersAndKeySecret(t *testing.T) {
	// scenario: A strict environment requires brokers and key secret
	localEnv(t)
	t.Setenv("ENV", "staging")
	_, err := config.Load()
	mustErrNaming(t, err, "KAFKA_BROKERS", "LOG_KEY_FP_SECRET")
}

func TestUpstreamAddressesRequiredInEveryEnv(t *testing.T) {
	// scenario: A missing upstream address refuses to start in every environment
	// scenario: Order audit: Config requires the audit address in every environment
	for _, env := range []string{"local", "production"} {
		t.Run(env+" all four unset", func(t *testing.T) {
			strictEnv(t, env)
			for _, k := range upstreamKeys {
				if err := os.Unsetenv(k); err != nil {
					t.Fatal(err)
				}
			}
			_, err := config.Load()
			mustErrNaming(t, err, upstreamKeys...)
		})
	}
	for _, k := range upstreamKeys {
		t.Run("only "+k+" unset", func(t *testing.T) {
			localEnv(t)
			if err := os.Unsetenv(k); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load()
			mustErrNaming(t, err, k)
		})
	}
}

func TestUpstreamAuditConfig(t *testing.T) {
	// scenario: Order audit: Config requires the audit address in every environment
	localEnv(t)
	if err := os.Unsetenv("UPSTREAM_AUDIT_ADDR"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "UPSTREAM_AUDIT_ADDR")

	t.Setenv("UPSTREAM_AUDIT_ADDR", "team-audit-svc:50066")
	s := mustLoad(t)
	if got := str(t, s, "Audit", "Addr"); got != "team-audit-svc:50066" {
		t.Fatalf("Audit.Addr = %q", got)
	}
}

func readExampleValue(t *testing.T, key string) string {
	t.Helper()
	f, err := os.Open(exampleePath(t))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), key+"="); ok {
			return v
		}
	}
	t.Fatalf(".env.example has no %s= line", key)
	return ""
}

func exampleePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", ".env.example")
}

func TestLocalEnvDefaults(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	localEnv(t)
	s := mustLoad(t)
	if got := str(t, s, "Kafka", "Brokers"); got != "localhost:9092" {
		t.Errorf("Kafka.Brokers = %q, want localhost:9092", got)
	}
	got := str(t, s, "LogKeyFP", "Secret")
	if got == "" {
		t.Fatal("LogKeyFP.Secret is empty in a local ENV")
	}
	if want := readExampleValue(t, "LOG_KEY_FP_SECRET"); got != want {
		t.Errorf("LogKeyFP.Secret = %q, .env.example documents %q", got, want)
	}
}

func TestOutboxAndKafkaDefaults(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	localEnv(t)
	s := mustLoad(t)
	if got := str(t, s, "Events", "OrderTopic"); got != "order.events" {
		t.Errorf("Events.OrderTopic = %q", got)
	}
	o := reflect.ValueOf(s).Elem().FieldByName("Outbox")
	if !o.IsValid() {
		t.Fatal("Settings.Outbox is not declared")
	}
	if got := o.FieldByName("PollInterval").Interface(); got != time.Second {
		t.Errorf("Outbox.PollInterval = %v, want 1s", got)
	}
	if got := o.FieldByName("BatchSize").Int(); got != 100 {
		t.Errorf("Outbox.BatchSize = %d, want 100", got)
	}
	if got := o.FieldByName("ClaimLock").Interface(); got != 60*time.Second {
		t.Errorf("Outbox.ClaimLock = %v, want 60s", got)
	}
}

func TestReservationDurations(t *testing.T) {
	// scenario: An unusable reservation duration still falls back with a warning
	cases := []struct {
		name             string
		ttl, interval    string
		wantTTL, wantInt time.Duration
		wantWarnTTL      bool
		wantWarnInterval bool
	}{
		{"defaults", "15m", "1m", 15 * time.Minute, time.Minute, false, false},
		{"override", "20s", "2s", 20 * time.Second, 2 * time.Second, false, false},
		{"invalid", "banana", "0s", 15 * time.Minute, time.Minute, true, true},
		{"negative and empty", "-1m", "", 15 * time.Minute, time.Minute, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &config.Settings{Reservation: config.Reservation{TTL: tc.ttl, SweepInterval: tc.interval}}
			ttl, w1 := s.ReservationTTL()
			iv, w2 := s.ReservationSweepInterval()
			if ttl != tc.wantTTL || iv != tc.wantInt {
				t.Fatalf("got ttl=%s interval=%s, want %s/%s", ttl, iv, tc.wantTTL, tc.wantInt)
			}
			if (w1 != "") != tc.wantWarnTTL || (w2 != "") != tc.wantWarnInterval {
				t.Fatalf("warnings ttl=%q interval=%q", w1, w2)
			}
			if tc.wantWarnTTL && !strings.Contains(w1, "RESERVATION_TTL") {
				t.Fatalf("ttl warning must name the variable: %q", w1)
			}
			if tc.wantWarnInterval && !strings.Contains(w2, "RESERVATION_SWEEP_INTERVAL") {
				t.Fatalf("interval warning must name the variable: %q", w2)
			}
		})
	}
}

func TestReservationDefaultsFromEnv(t *testing.T) {
	// scenario: An unusable reservation duration still falls back with a warning
	localEnv(t)
	t.Setenv("RESERVATION_TTL", "banana")
	t.Setenv("RESERVATION_SWEEP_INTERVAL", "0s")
	s := mustLoad(t)
	if d, w := s.ReservationTTL(); d != 15*time.Minute || !strings.Contains(w, "RESERVATION_TTL") {
		t.Fatalf("ttl=%s warn=%q", d, w)
	}
	if d, w := s.ReservationSweepInterval(); d != time.Minute || !strings.Contains(w, "RESERVATION_SWEEP_INTERVAL") {
		t.Fatalf("interval=%s warn=%q", d, w)
	}
	os.Unsetenv("RESERVATION_TTL")
	os.Unsetenv("RESERVATION_SWEEP_INTERVAL")
	s = mustLoad(t)
	if d, w := s.ReservationTTL(); d != 15*time.Minute || w != "" {
		t.Fatalf("unset ttl: %s warn=%q", d, w)
	}
	if d, w := s.ReservationSweepInterval(); d != time.Minute || w != "" {
		t.Fatalf("unset interval: %s warn=%q", d, w)
	}
}

type lifecycleAccessor struct {
	env string
	def time.Duration
	get func(*config.Settings) (time.Duration, string)
}

func lifecycleAccessors() []lifecycleAccessor {
	return []lifecycleAccessor{
		{"PAYMENT_DEADLINE", 24 * time.Hour, (*config.Settings).PaymentDeadline},
		{"PAYMENT_REMINDER_BEFORE", 2 * time.Hour, (*config.Settings).PaymentReminderBefore},
		{"LIFECYCLE_SWEEP_INTERVAL", time.Minute, (*config.Settings).LifecycleSweepInterval},
	}
}

func TestLifecycleDefaults(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	localEnv(t)
	s := mustLoad(t)
	for _, a := range lifecycleAccessors() {
		d, warn := a.get(s)
		if d != a.def || warn != "" {
			t.Errorf("%s: got (%s, %q), want (%s, no warning)", a.env, d, warn, a.def)
		}
	}
}

func TestLifecycleUnusableDurationFallsBack(t *testing.T) {
	// scenario: An unusable reservation duration still falls back with a warning
	for _, a := range lifecycleAccessors() {
		for _, bad := range []string{"0", "-5m", "soon", "0s"} {
			t.Run(a.env+"="+bad, func(t *testing.T) {
				localEnv(t)
				t.Setenv(a.env, bad)
				s := mustLoad(t)
				d, warn := a.get(s)
				if d != a.def {
					t.Errorf("got %s, want default %s", d, a.def)
				}
				if !strings.Contains(warn, a.env) {
					t.Errorf("warning %q must name %s", warn, a.env)
				}
			})
		}
	}
}

func TestLifecycleValidDurationHonoured(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	vals := map[string]time.Duration{"PAYMENT_DEADLINE": 45 * time.Second, "PAYMENT_REMINDER_BEFORE": 30 * time.Second, "LIFECYCLE_SWEEP_INTERVAL": 2 * time.Second}
	localEnv(t)
	for k, v := range vals {
		t.Setenv(k, v.String())
	}
	s := mustLoad(t)
	for _, a := range lifecycleAccessors() {
		d, warn := a.get(s)
		if d != vals[a.env] || warn != "" {
			t.Errorf("%s: got (%s, %q), want (%s, no warning)", a.env, d, warn, vals[a.env])
		}
	}
}

func receiptDurationEnvs() []string {
	return []string{"AUTO_COMPLETE_AFTER", "RETURN_WINDOW", "RETURN_AUTO_APPROVE_AFTER", "SHIP_BY_AFTER"}
}

func TestLifecycleDurationDefaults(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	localEnv(t)
	s := mustLoad(t)
	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"AUTO_COMPLETE_AFTER", s.AutoCompleteAfter(), 168 * time.Hour},
		{"RETURN_WINDOW", s.ReturnWindow(), 360 * time.Hour},
		{"RETURN_AUTO_APPROVE_AFTER", s.ReturnAutoApproveAfter(), 72 * time.Hour},
		{"SHIP_BY_AFTER", s.ShipByAfter(), 48 * time.Hour},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, c.got, c.want)
		}
	}
}

func TestLifecycleDurationValidHonoured(t *testing.T) {
	// scenario: A local environment fills the documented defaults
	localEnv(t)
	t.Setenv("AUTO_COMPLETE_AFTER", "12s")
	t.Setenv("RETURN_WINDOW", "20s")
	t.Setenv("RETURN_AUTO_APPROVE_AFTER", "30s")
	t.Setenv("SHIP_BY_AFTER", "60s")
	s := mustLoad(t)
	if s.AutoCompleteAfter() != 12*time.Second || s.ReturnWindow() != 20*time.Second ||
		s.ReturnAutoApproveAfter() != 30*time.Second || s.ShipByAfter() != 60*time.Second {
		t.Errorf("values not honoured: %s %s %s %s", s.AutoCompleteAfter(), s.ReturnWindow(), s.ReturnAutoApproveAfter(), s.ShipByAfter())
	}
}

func TestLifecycleDurationMalformedRejected(t *testing.T) {
	// scenario: A malformed receipt duration still refuses to start and names its key
	for _, k := range receiptDurationEnvs() {
		for _, bad := range []string{"abc", "banana", "7", "1 day"} {
			t.Run(k+"="+bad, func(t *testing.T) {
				localEnv(t)
				t.Setenv(k, bad)
				_, err := config.Load()
				mustErrNaming(t, err, k)
			})
		}
	}
}

func TestLifecycleDurationNonPositiveRejected(t *testing.T) {
	// scenario: A malformed receipt duration still refuses to start and names its key
	for _, k := range receiptDurationEnvs() {
		for _, bad := range []string{"0s", "0", "-1h"} {
			t.Run(k+"="+bad, func(t *testing.T) {
				localEnv(t)
				t.Setenv(k, bad)
				_, err := config.Load()
				mustErrNaming(t, err, k)
			})
		}
	}
}

func TestKafkaEngagementConsumerSettings(t *testing.T) {
	// scenario: Group ids and DLQ names are unchanged by default
	localEnv(t)
	s := mustLoad(t)
	defaults := map[string]string{
		"PaymentTopic":    "payment.events",
		"PaymentGroup":    "team-order.payment",
		"PaymentDLQ":      "payment.events.dlq",
		"OrderTopic":      "order.events",
		"EngagementTopic": "engagement.events",
		"DisputeGroup":    "team-order.dispute",
		"EngagementDLQ":   "engagement.events.order.dlq",
	}
	for field, want := range defaults {
		if got := str(t, s, "Events", field); got != want {
			t.Errorf("default Events.%s = %q, want %q", field, got, want)
		}
	}

	overrides := map[string]string{
		"PAYMENT_EVENTS_TOPIC":         "pay.custom",
		"ORDER_PAYMENT_CONSUMER_GROUP": "payg.custom",
		"PAYMENT_EVENTS_DLQ_TOPIC":     "paydlq.custom",
		"ORDER_EVENTS_TOPIC":           "ord.custom",
		"ENGAGEMENT_EVENTS_TOPIC":      "eng.custom",
		"ORDER_DISPUTE_CONSUMER_GROUP": "grp.custom",
		"ENGAGEMENT_EVENTS_DLQ_TOPIC":  "dlq.custom",
	}
	for k, v := range overrides {
		t.Setenv(k, v)
	}
	s = mustLoad(t)
	set := map[string]string{
		"PaymentTopic":    "pay.custom",
		"PaymentGroup":    "payg.custom",
		"PaymentDLQ":      "paydlq.custom",
		"OrderTopic":      "ord.custom",
		"EngagementTopic": "eng.custom",
		"DisputeGroup":    "grp.custom",
		"EngagementDLQ":   "dlq.custom",
	}
	for field, want := range set {
		if got := str(t, s, "Events", field); got != want {
			t.Errorf("set Events.%s = %q, want %q", field, got, want)
		}
	}
}

var exampleKey = regexp.MustCompile(`^#?\s*([A-Z][A-Z0-9_]*)=`)

func TestEnvExampleInSync(t *testing.T) {
	// scenario: .env.example documents every key
	f, err := os.Open(exampleePath(t))
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
	for _, k := range removedKeys {
		if _, ok := got[k]; ok {
			t.Errorf(".env.example still lists removed key %s", k)
		}
	}
	for _, k := range []string{"SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY", "OUTBOX_CLAIM_LOCK", "FEATURE_FLAGS_EVAL_TIMEOUT", "LOG_KEY_FP_SECRET"} {
		if _, ok := got[k]; !ok {
			t.Errorf(".env.example lacks %s", k)
		}
	}
}
