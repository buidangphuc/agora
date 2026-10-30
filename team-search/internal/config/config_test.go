package config_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-search/internal/config"
)

var managedKeys = []string{
	"ENV", "LOG_LEVEL", "LOG_JSON", "GRPC_HOST", "GRPC_PORT", "SHUTDOWN_GRACE", "SHUTDOWN_DRAIN_DELAY",
	"GRPC_REFLECTION_ENABLED", "DATABASE_URL", "DB_MAX_CONNS", "DATABASE_ENABLED", "KAFKA_ENABLED",
	"KAFKA_BROKERS", "KAFKA_CONSUMER_GROUP", "KAFKA_LISTING_TOPIC", "SHUTDOWN_GRACE_SECONDS",
	"UPSTREAM_AI_ADDR", "LOG_KEY_FP_SECRET", "OTEL_ENABLED", "OTEL_EXPORTER_OTLP_ENDPOINT",
	"OTEL_SERVICE_NAME", "OPENSEARCH_URL", "OPENSEARCH_INDEX", "MODEL_SERVER_URL", "EMBEDDING_DIM",
	"ENABLE_HYBRID_SEARCH", "ENABLE_RERANKER", "HYBRID_FUSION_WINDOW", "HYBRID_RRF_K",
	"HYBRID_LEXICAL_WEIGHT", "HYBRID_SEMANTIC_WEIGHT", "HYBRID_SEMANTIC_MIN_SCORE",
	"TOMBSTONE_TTL", "TOMBSTONE_PURGE_INTERVAL",
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

func serverEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("GRPC_PORT", "50052")
	t.Setenv("DATABASE_URL", "postgres://x/y")
}

func indexerEnv(t *testing.T) {
	t.Helper()
	cleanEnv(t)
	t.Setenv("ENV", "local")
	t.Setenv("UPSTREAM_AI_ADDR", "ai:50051")
}

func strictIndexerEnv(t *testing.T, env string) {
	t.Helper()
	indexerEnv(t)
	t.Setenv("ENV", env)
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("LOG_KEY_FP_SECRET", "s3cret")
}

func mustErrNaming(t *testing.T, err error, keys ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("load succeeded, want an error naming %v", keys)
	}
	for _, k := range keys {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error %q does not name %s", err.Error(), k)
		}
	}
}

func TestLoadRequiresEnv(t *testing.T) {
	serverEnv(t)
	t.Setenv("UPSTREAM_AI_ADDR", "ai:50051")
	if err := os.Unsetenv("ENV"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "ENV")
	_, err = config.LoadIndexer()
	mustErrNaming(t, err, "ENV")
}

func TestServerLoadRequiresGRPCPort(t *testing.T) {
	serverEnv(t)
	if err := os.Unsetenv("GRPC_PORT"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_PORT")
}

func TestNoDependencyEnableFlags(t *testing.T) {
	both := struct {
		S config.Settings
		I config.IndexerSettings
	}{}
	for _, k := range sdkconfig.DeclaredEnvKeys(both) {
		switch k {
		case "DATABASE_ENABLED", "KAFKA_ENABLED", "SHUTDOWN_GRACE_SECONDS":
			t.Errorf("retired key %s is still declared", k)
		}
		if strings.HasSuffix(k, "_ENABLED") && k != "OTEL_ENABLED" && k != "GRPC_REFLECTION_ENABLED" {
			t.Errorf("enable flag %s is declared", k)
		}
	}
}

func TestEnvExampleInSync(t *testing.T) {
	all := struct {
		S config.Settings
		I config.IndexerSettings
		M config.MigrateSettings
	}{}
	if err := sdkconfig.CheckEnvExample("../../.env.example", all); err != nil {
		t.Fatal(err)
	}
}

func TestReflectionDefaultsOff(t *testing.T) {
	serverEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.ReflectionEnabled {
		t.Error("ReflectionEnabled = true by default, want false")
	}
}

func TestLoadRefusesReflectionInStrictEnv(t *testing.T) {
	serverEnv(t)
	t.Setenv("ENV", "dev")
	t.Setenv("GRPC_REFLECTION_ENABLED", "true")
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "GRPC_REFLECTION_ENABLED")
	if strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error %q names DATABASE_URL, the reflection refusal must come first", err.Error())
	}
}

func TestServerLoadRequiresDatabaseURL(t *testing.T) {
	serverEnv(t)
	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestIndexerLoadNeedsNoDatabase(t *testing.T) {
	indexerEnv(t)
	if _, err := config.LoadIndexer(); err != nil {
		t.Fatalf("LoadIndexer without DATABASE_URL and GRPC_PORT: %v", err)
	}
}

func TestIndexerLoadRequiresAIAddr(t *testing.T) {
	indexerEnv(t)
	if err := os.Unsetenv("UPSTREAM_AI_ADDR"); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadIndexer()
	mustErrNaming(t, err, "UPSTREAM_AI_ADDR")
}

func TestIndexerLoadRequiresBrokersInStrictEnv(t *testing.T) {
	strictIndexerEnv(t, "staging")
	if err := os.Unsetenv("KAFKA_BROKERS"); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadIndexer()
	mustErrNaming(t, err, "KAFKA_BROKERS")
}

func TestIndexerLoadRequiresLogKeyFPSecretInStrictEnv(t *testing.T) {
	strictIndexerEnv(t, "production")
	if err := os.Unsetenv("LOG_KEY_FP_SECRET"); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadIndexer()
	mustErrNaming(t, err, "LOG_KEY_FP_SECRET")
}

func TestTombstoneDurationsRefuseInvalid(t *testing.T) {
	for _, key := range []string{"TOMBSTONE_TTL", "TOMBSTONE_PURGE_INTERVAL"} {
		for _, bad := range []string{"soon", "0s", "-1h"} {
			t.Run(key+"="+bad, func(t *testing.T) {
				indexerEnv(t)
				t.Setenv(key, bad)
				_, err := config.LoadIndexer()
				mustErrNaming(t, err, key)
			})
		}
	}
}

func TestTombstoneDefaults(t *testing.T) {
	indexerEnv(t)
	s, err := config.LoadIndexer()
	if err != nil {
		t.Fatalf("LoadIndexer: %v", err)
	}
	if s.Tombstone.TTL != 336*time.Hour {
		t.Errorf("TTL = %s, want 336h", s.Tombstone.TTL)
	}
	if s.Tombstone.PurgeInterval != time.Hour {
		t.Errorf("PurgeInterval = %s, want 1h", s.Tombstone.PurgeInterval)
	}
}

func TestValidateRequiresOpenSearchURL(t *testing.T) {
	serverEnv(t)
	t.Setenv("OPENSEARCH_URL", "")
	_, err := config.Load()
	mustErrNaming(t, err, "OPENSEARCH_URL")

	indexerEnv(t)
	t.Setenv("OPENSEARCH_URL", "")
	_, err = config.LoadIndexer()
	mustErrNaming(t, err, "OPENSEARCH_URL")
}

func TestIndexerShutdownGraceDefaultsAndValidates(t *testing.T) {
	indexerEnv(t)
	t.Setenv("OPENSEARCH_URL", "http://os:9200")
	s, err := config.LoadIndexer()
	if err != nil {
		t.Fatalf("LoadIndexer: %v", err)
	}
	if s.ShutdownGrace != 10*time.Second {
		t.Errorf("ShutdownGrace = %s, want 10s", s.ShutdownGrace)
	}

	t.Setenv("SHUTDOWN_GRACE", "0s")
	_, err = config.LoadIndexer()
	mustErrNaming(t, err, "SHUTDOWN_GRACE")
}

func TestOTelServiceNameDefaultsPerBinary(t *testing.T) {
	serverEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.OTel.ServiceName != "team-search" {
		t.Errorf("server service name = %q, want team-search", s.OTel.ServiceName)
	}

	indexerEnv(t)
	i, err := config.LoadIndexer()
	if err != nil {
		t.Fatalf("LoadIndexer: %v", err)
	}
	if i.OTel.ServiceName != "team-search-indexer" {
		t.Errorf("indexer service name = %q, want team-search-indexer", i.OTel.ServiceName)
	}
}

func TestOTelServiceNameEnvOverrides(t *testing.T) {
	serverEnv(t)
	t.Setenv("OTEL_SERVICE_NAME", "custom")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.OTel.ServiceName != "custom" {
		t.Errorf("service name = %q, want custom", s.OTel.ServiceName)
	}
}

func TestLoadMigrateNeedsOnlyEnvAndDatabaseURL(t *testing.T) {
	cleanEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://x/y")
	s, err := config.LoadMigrate()
	if err != nil {
		t.Fatalf("LoadMigrate: %v", err)
	}
	if s.Postgres.URL != "postgres://x/y" {
		t.Errorf("Postgres.URL = %q", s.Postgres.URL)
	}

	if err := os.Unsetenv("DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	_, err = config.LoadMigrate()
	mustErrNaming(t, err, "DATABASE_URL")
}

func TestNoRuntimeInMemoryFallback(t *testing.T) {
	root := filepath.Join("..", "..")
	banned := []string{"InMemorySavedSearchRepository", "RequireDurableStorage", "DATABASE_ENABLED"}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "generated" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, w := range banned {
			if strings.Contains(string(b), w) {
				t.Errorf("%s contains %s", path, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSemanticMinScoreDefault(t *testing.T) {
	serverEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Retrieval.SemanticMinScore != 0.65 {
		t.Errorf("default HYBRID_SEMANTIC_MIN_SCORE = %v, want 0.65", s.Retrieval.SemanticMinScore)
	}
}
