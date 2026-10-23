package config_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-chat/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE",
	"SHUTDOWN_DRAIN_DELAY", "GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS",
	"DATABASE_ENABLED", "KAFKA_ENABLED", "OUTBOX_ENABLED", "MOCK_DATA",
	"OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_SERVICE_NAME",
	"KAFKA_BROKERS", "KAFKA_CHAT_TOPIC",
	"OUTBOX_POLL_INTERVAL", "OUTBOX_BATCH_SIZE", "OUTBOX_CLAIM_LOCK",
	"OUTBOX_CLAIM_LOCK_SECONDS", "OUTBOX_MAX_ATTEMPTS",
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
	t.Setenv("GRPC_PORT", "50057")
	t.Setenv("DATABASE_URL", "postgres://x/y")
}

func TestRequiredVariables(t *testing.T) {
	// scenario: Required variables are enforced
	localEnv(t)
	if err := os.Unsetenv("GRPC_PORT"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "GRPC_PORT") {
		t.Fatalf("no port: err = %v, want an error naming GRPC_PORT", err)
	}

	localEnv(t)
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "ENV") {
		t.Fatalf("no ENV: err = %v, want an error naming ENV", err)
	}

	localEnv(t)
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("no database url: err = %v, want an error naming DATABASE_URL", err)
	}
}

func TestDefaults(t *testing.T) {
	// scenario: Defaults match the documented values
	localEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4317")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Port != 50057 {
		t.Errorf("Port = %d, want 50057", s.Port)
	}
	if s.Events.ChatTopic != "chat.events" {
		t.Errorf("Events.ChatTopic = %q, want chat.events", s.Events.ChatTopic)
	}
	if s.Outbox.PollInterval != time.Second {
		t.Errorf("Outbox.PollInterval = %v, want 1s", s.Outbox.PollInterval)
	}
	if s.Outbox.BatchSize != 100 {
		t.Errorf("Outbox.BatchSize = %d, want 100", s.Outbox.BatchSize)
	}
	if s.Outbox.ClaimLock != 60*time.Second {
		t.Errorf("Outbox.ClaimLock = %v, want 60s", s.Outbox.ClaimLock)
	}
	if s.ReflectionEnabled {
		t.Error("ReflectionEnabled = true, want false")
	}
	if s.Postgres.MaxConns != 10 {
		t.Errorf("Postgres.MaxConns = %d, want 10", s.Postgres.MaxConns)
	}
	if s.ShutdownGrace != 10*time.Second {
		t.Errorf("ShutdownGrace = %v, want 10s", s.ShutdownGrace)
	}
	if !s.LogJSON {
		t.Error("LogJSON = false, want true")
	}
	if s.OTel.Enabled {
		t.Error("OTel.Enabled = true with only an endpoint set, want false")
	}
}

func TestOutboxOverrides(t *testing.T) {
	// scenario: Defaults match the documented values
	localEnv(t)
	t.Setenv("OUTBOX_POLL_INTERVAL", "250ms")
	t.Setenv("OUTBOX_BATCH_SIZE", "7")
	t.Setenv("OUTBOX_CLAIM_LOCK", "5s")
	t.Setenv("KAFKA_CHAT_TOPIC", "chat.custom")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Outbox.PollInterval != 250*time.Millisecond || s.Outbox.BatchSize != 7 || s.Outbox.ClaimLock != 5*time.Second {
		t.Errorf("Outbox = %+v, want 250ms/7/5s", s.Outbox)
	}
	if s.Events.ChatTopic != "chat.custom" {
		t.Errorf("Events.ChatTopic = %q, want chat.custom", s.Events.ChatTopic)
	}
}

func TestStrictEnvRefusesReflectionFirst(t *testing.T) {
	// scenario: Strict ENV refuses reflection before other validation
	cleanEnv(t)
	t.Setenv("ENV", "prod")
	t.Setenv("GRPC_PORT", "50057")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	_, err := config.Load()
	if err == nil {
		t.Fatal("Load succeeded in prod with reflection and no DATABASE_URL")
	}
	if !strings.Contains(err.Error(), "GRPC_REFLECTION_ENABLED") {
		t.Errorf("err = %v, want the reflection refusal", err)
	}
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("err = %v, must not name DATABASE_URL", err)
	}
}

func TestStrictEnvRequiresKafkaBrokers(t *testing.T) {
	// scenario: Strict ENV requires Kafka brokers
	localEnv(t)
	t.Setenv("ENV", "staging")
	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "KAFKA_BROKERS") {
		t.Fatalf("err = %v, want an error naming KAFKA_BROKERS", err)
	}
	t.Setenv("KAFKA_BROKERS", "broker-1:9092,broker-2:9092")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load with brokers: %v", err)
	}
	if s.Kafka.Brokers != "broker-1:9092,broker-2:9092" {
		t.Errorf("Kafka.Brokers = %q", s.Kafka.Brokers)
	}
}

func TestLocalEnvDefaultsKafkaBrokers(t *testing.T) {
	// scenario: A local ENV without brokers uses the SDK default broker
	localEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Kafka.Brokers != "localhost:9092" {
		t.Errorf("Kafka.Brokers = %q, want localhost:9092", s.Kafka.Brokers)
	}
}

func TestOTelNamesAndServiceNameDefault(t *testing.T) {
	// scenario: OTel names keep their variables and the service name default
	localEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://otel:4317")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.OTel.ServiceName != "team-chat" {
		t.Errorf("OTel.ServiceName = %q, want team-chat", s.OTel.ServiceName)
	}
	if s.OTel.Endpoint != "http://otel:4317" {
		t.Errorf("OTel.Endpoint = %q", s.OTel.Endpoint)
	}
	if s.OTel.Enabled {
		t.Error("OTel.Enabled = true without OTEL_ENABLED")
	}
	t.Setenv("OTEL_SERVICE_NAME", "chat-x")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.OTel.ServiceName != "chat-x" {
		t.Errorf("OTel.ServiceName = %q, want chat-x", s.OTel.ServiceName)
	}
	if s.OTel.Endpoint != "http://otel:4317" || s.OTel.Enabled {
		t.Errorf("OTel = %+v", s.OTel)
	}
}

func TestRemovedEnableFlagsAreIgnored(t *testing.T) {
	// scenario: Removed enable flags are ignored
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50057")
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("KAFKA_ENABLED", "false")
	t.Setenv("OUTBOX_ENABLED", "false")
	_, err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("err = %v, want an error naming DATABASE_URL", err)
	}
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Kafka.Brokers != "localhost:9092" || s.Outbox.BatchSize != 100 {
		t.Errorf("flags changed settings: kafka=%+v outbox=%+v", s.Kafka, s.Outbox)
	}
}

func TestEnvExampleInSync(t *testing.T) {
	// scenario: .env.example is in sync
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", ".env.example")
	if err := sdkconfig.CheckEnvExample(path, config.Settings{}); err != nil {
		t.Fatalf("CheckEnvExample: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"SERVER_PORT", "SERVER_HOST", "RUNTIME_LOG_LEVEL", "POSTGRES_HOST", "KAFKA_ENABLED",
		"OUTBOX_ENABLED", "OUTBOX_MAX_ATTEMPTS", "OUTBOX_CLAIM_LOCK_SECONDS", "UPSTREAM_LISTING_ADDR",
	} {
		if regexp.MustCompile(`(?m)^#?\s*` + name + `=`).Match(b) {
			t.Errorf(".env.example still lists removed name %s", name)
		}
	}
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	for _, want := range []string{"KAFKA_BROKERS", "KAFKA_CHAT_TOPIC", "OUTBOX_CLAIM_LOCK", "OTEL_SERVICE_NAME"} {
		found := false
		for _, k := range keys {
			found = found || k == want
		}
		if !found {
			t.Errorf("Settings does not declare %s", want)
		}
	}
}

func TestNoEnableFlagOrFallbackInSource(t *testing.T) {
	// scenario: No source reads an enable flag or builds a fallback
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")
	enableRe := regexp.MustCompile(`[A-Z][A-Z0-9_]*_ENABLED`)
	allowed := map[string]bool{"OTEL_ENABLED": true, "GRPC_REFLECTION_ENABLED": true}
	banned := []string{"NewInMemory", "Noop", "RequireDurableStorage"}
	fset := token.NewFileSet()
	scanned := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "generated" || d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		scanned++
		rel, _ := filepath.Rel(root, p)
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind != token.STRING {
					return true
				}
				s, uerr := strconv.Unquote(x.Value)
				if uerr != nil {
					s = x.Value
				}
				for _, m := range enableRe.FindAllString(s, -1) {
					if !allowed[m] {
						t.Errorf("%s: string literal names %s", rel, m)
					}
				}
			case *ast.Ident:
				for _, b := range banned {
					if strings.Contains(x.Name, b) {
						t.Errorf("%s: identifier %s contains %s", rel, x.Name, b)
					}
				}
				if enableRe.MatchString(x.Name) && !allowed[x.Name] {
					t.Errorf("%s: identifier %s", rel, x.Name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("no source files scanned")
	}
}

func TestDeclaredEnableFlagsAreOnlyOTelAndReflection(t *testing.T) {
	// scenario: No source reads an enable flag or builds a fallback
	keys := sdkconfig.DeclaredEnvKeys(config.Settings{})
	if len(keys) == 0 {
		t.Fatal("Settings declares no keys")
	}
	var got []string
	for _, k := range keys {
		if strings.HasSuffix(k, "_ENABLED") {
			got = append(got, k)
		}
	}
	sort.Strings(got)
	want := "GRPC_REFLECTION_ENABLED,OTEL_ENABLED"
	if strings.Join(got, ",") != want {
		t.Fatalf("declared enable flags = %v, want %s", got, want)
	}
}
