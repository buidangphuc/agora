// Package config assembles one flat Settings value from per-capability struct
// groups, populated from the environment with defaults — mirroring team-domain's
// config (and, one step back, team-ai's pydantic Settings). The `env`/`default`
// struct tags are the single source of truth for loading AND the .env.example
// drift gate (envcheck.go).
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Settings is the whole configuration surface, grouped by capability.
type Settings struct {
	Runtime       Runtime
	Server        Server
	OpenSearch    OpenSearch
	Retrieval     Retrieval
	Taxonomy      Taxonomy
	Kafka         Kafka
	Tombstone     Tombstone
	Database      Database
	Observability Observability
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50052"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// OpenSearch is this service's OWN read-model store (ADR-0005, Rule 3).
type OpenSearch struct {
	URL   string `env:"OPENSEARCH_URL" default:"http://localhost:9200"`
	Index string `env:"OPENSEARCH_INDEX" default:"listings"`
}

// Retrieval configures the multi-strategy hybrid retrieval platform.
type Retrieval struct {
	ModelServerURL     string  `env:"MODEL_SERVER_URL" default:"http://localhost:8100"`
	EmbeddingDim       int     `env:"EMBEDDING_DIM" default:"384"`
	EnableHybridSearch bool    `env:"ENABLE_HYBRID_SEARCH" default:"true"`
	EnableReranker     bool    `env:"ENABLE_RERANKER" default:"false"`
	HybridFusionWindow int     `env:"HYBRID_FUSION_WINDOW" default:"200"`
	HybridRRFK         int     `env:"HYBRID_RRF_K" default:"60"`
	LexicalWeight      float64 `env:"HYBRID_LEXICAL_WEIGHT" default:"1.0"`
	SemanticWeight     float64 `env:"HYBRID_SEMANTIC_WEIGHT" default:"1.0"`
	// SemanticMinScore is the minimum cosine similarity (-1..1) a semantic
	// candidate needs to be kept; a value <= -1 disables the floor.
	SemanticMinScore float64 `env:"HYBRID_SEMANTIC_MIN_SCORE" default:"0.6"`
}

// Taxonomy points the indexer at team-ai's gRPC AIService (ClassifyTags), which
// supplies the canonical SPU tags and per-variant attributes stored for dynamic
// facets. Empty disables classification: listings are indexed without tags.
type Taxonomy struct {
	AIAddr string `env:"UPSTREAM_AI_ADDR" default:""`
}

// Kafka configures the listing-events consumer (ADR-0002).
type Kafka struct {
	Enabled       bool   `env:"KAFKA_ENABLED" default:"false"`
	Brokers       string `env:"KAFKA_BROKERS" default:"localhost:9092"` // comma-separated
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-search-indexer"`
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

// Tombstone tunes the retention of deleted-listing tombstones in the read-model
// and the indexer's purge loop (D6). Both are Go duration strings; read them
// through TombstoneTTL and TombstonePurgeInterval, which never fail: an unusable
// value falls back to the default with a warning so a typo cannot stop the indexer.
type Tombstone struct {
	TTL           string `env:"TOMBSTONE_TTL" default:"336h"`
	PurgeInterval string `env:"TOMBSTONE_PURGE_INTERVAL" default:"1h"`
}

// DefaultTombstoneTTL (14 days) is how long a tombstone outlives its delete,
// measured from when the indexer applied it, before the purge removes it.
const DefaultTombstoneTTL = 336 * time.Hour

// DefaultTombstonePurgeInterval is how often the indexer purges expired tombstones.
const DefaultTombstonePurgeInterval = time.Hour

// TombstoneTTL parses TOMBSTONE_TTL. An empty, unparsable, zero or negative value
// yields the default and a non-empty warning naming the variable.
func (s *Settings) TombstoneTTL() (time.Duration, string) {
	return positiveDuration("TOMBSTONE_TTL", s.Tombstone.TTL, DefaultTombstoneTTL)
}

// TombstonePurgeInterval parses TOMBSTONE_PURGE_INTERVAL with the same rules.
func (s *Settings) TombstonePurgeInterval() (time.Duration, string) {
	return positiveDuration("TOMBSTONE_PURGE_INTERVAL", s.Tombstone.PurgeInterval, DefaultTombstonePurgeInterval)
}

func positiveDuration(key, raw string, def time.Duration) (time.Duration, string) {
	raw = strings.TrimSpace(raw)
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def, fmt.Sprintf("invalid %s %q (want a positive Go duration such as 30s or 336h); using default %s", key, raw, def)
	}
	return d, ""
}

// Database configures the Postgres store for saved searches (migrations/
// 0001_saved_searches). When disabled the service falls back to an in-memory
// store and saved searches do not survive a restart.
type Database struct {
	Enabled bool   `env:"DATABASE_ENABLED" default:"false"`
	URL     string `env:"DATABASE_URL" default:""`
}

// Observability configures OpenTelemetry (ADR-0004). Exporter swappable.
type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-search"`
}

// LoadSettings reads the environment into Settings, applies defaults, validates.
func LoadSettings() (*Settings, error) {
	s := &Settings{}
	if err := bindGroups(reflect.ValueOf(s).Elem()); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Validate enforces cross-field invariants and prod-safety.
func (s *Settings) Validate() error {
	if strings.TrimSpace(s.OpenSearch.URL) == "" {
		return errors.New("OPENSEARCH_URL is required")
	}
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	if s.Server.ShutdownGrace < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.Server.ShutdownGrace)
	}
	if s.Database.Enabled && strings.TrimSpace(s.Database.URL) == "" {
		return errors.New("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	return nil
}

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

// durableStorageEnvs are the ENV values (normalised: trimmed, lowercase) in which
// the query server must never run on the in-memory saved-search repository.
// Anything else ("local", "test", unset, unknown) is non-strict.
var durableStorageEnvs = []string{"staging", "stage", "prod", "production"}

// RequiresDurableStorage reports whether ENV names an environment that must use
// the database (staging / production).
func (s *Settings) RequiresDurableStorage() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	for _, strict := range durableStorageEnvs {
		if e == strict {
			return true
		}
	}
	return false
}

// RequireDurableStorage is the boot guard against the silent in-memory fallback:
// in a strict ENV it fails when the database is disabled. Other environments
// always pass. (An enabled-but-unreachable database already fails the boot in
// bootstrap.OpenSavedSearchRepository.)
func (s *Settings) RequireDurableStorage() error {
	if !s.RequiresDurableStorage() || s.Database.Enabled {
		return nil
	}
	return fmt.Errorf(
		"refusing to start with in-memory saved-search storage: ENV=%q requires a database (strict for ENV in %s) but DATABASE_ENABLED=false; set DATABASE_ENABLED=true with a reachable DATABASE_URL",
		s.Runtime.Env, strings.Join(durableStorageEnvs, ", "))
}

// KafkaBrokers splits KAFKA_BROKERS into seed addresses.
func (s *Settings) KafkaBrokers() []string { return splitCSV(s.Kafka.Brokers) }

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// DeclaredEnvKeys returns every env key declared by Settings, in struct order.
func DeclaredEnvKeys() []string {
	var keys []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		gt := t.Field(i).Type
		for j := 0; j < gt.NumField(); j++ {
			if k := gt.Field(j).Tag.Get("env"); k != "" {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func bindGroups(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		group := v.Field(i)
		gt := group.Type()
		for j := 0; j < gt.NumField(); j++ {
			f := gt.Field(j)
			key := f.Tag.Get("env")
			if key == "" {
				continue
			}
			raw := f.Tag.Get("default")
			if val, ok := os.LookupEnv(key); ok {
				raw = val
			}
			if err := setField(group.Field(j), raw); err != nil {
				return fmt.Errorf("config %s: %w", key, err)
			}
		}
	}
	return nil
}

func setField(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		x, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return err
		}
		fv.SetFloat(x)
	default:
		return fmt.Errorf("unsupported config field kind %s", fv.Kind())
	}
	return nil
}
