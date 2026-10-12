// Package config holds team-search's settings: the go-sdk groups it uses
// (Runtime, GRPC, Postgres, OTel, Kafka) plus the service-specific groups below,
// bound from flat env keys with sdkconfig.Load. The `env`/`default` struct tags
// are the single source of truth for both loading and the .env.example drift
// test (config_test.go). Both binaries (server and indexer) load the same
// Settings.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

// Settings is the whole configuration surface. Runtime and GRPC are embedded SDK
// groups; the others are named because their field names collide or because
// they are service-specific. The server reads Postgres, OpenSearch and
// Retrieval; the indexer reads Kafka, Events, OpenSearch, Retrieval, Taxonomy and
// Tombstone.
type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres   sdkconfig.Postgres
	OTel       sdkconfig.OTel
	Kafka      sdkconfig.Kafka
	Events     Events
	OpenSearch OpenSearch
	Retrieval  Retrieval
	Taxonomy   Taxonomy
	Tombstone  Tombstone
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
	SemanticMinScore float64 `env:"HYBRID_SEMANTIC_MIN_SCORE" default:"0.65"`
}

// Taxonomy points the indexer at team-ai's gRPC AIService (ClassifyTags), which
// supplies the canonical SPU tags and per-variant attributes stored for dynamic
// facets. Empty disables classification: listings are indexed without tags.
type Taxonomy struct {
	AIAddr string `env:"UPSTREAM_AI_ADDR" default:""`
}

// Events names the topic and consumer group the indexer reads (ADR-0002). The
// brokers come from the SDK Kafka group (KAFKA_BROKERS). The dead-letter topic
// is the listing topic plus ".dlq".
type Events struct {
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
func (s Settings) TombstoneTTL() (time.Duration, string) {
	return positiveDuration("TOMBSTONE_TTL", s.Tombstone.TTL, DefaultTombstoneTTL)
}

// TombstonePurgeInterval parses TOMBSTONE_PURGE_INTERVAL with the same rules.
func (s Settings) TombstonePurgeInterval() (time.Duration, string) {
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

// Load reads Settings from the environment, applies defaults and runs Validate.
// It is the single entry point used by both binaries.
func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

// Validate runs after parsing; sdkconfig.Load calls it. It delegates to the SDK
// groups and adds this service's own checks.
func (s Settings) Validate() error {
	if err := s.GRPC.Validate(); err != nil {
		return err
	}
	if err := s.Postgres.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(s.OpenSearch.URL) == "" {
		return errors.New("OPENSEARCH_URL is required")
	}
	if s.ShutdownGraceSeconds < 0 {
		return fmt.Errorf("SHUTDOWN_GRACE_SECONDS must be >= 0: %v", s.ShutdownGraceSeconds)
	}
	return nil
}
