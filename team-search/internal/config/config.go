package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

const (
	serverServiceName  = "team-search"
	indexerServiceName = "team-search-indexer"
)

type OpenSearch struct {
	URL   string `env:"OPENSEARCH_URL" default:"http://localhost:9200" keepempty:"true"`
	Index string `env:"OPENSEARCH_INDEX" default:"listings"`
}

func (o OpenSearch) Validate() error {
	if strings.TrimSpace(o.URL) == "" {
		return errors.New("OPENSEARCH_URL is required")
	}
	return nil
}

type Retrieval struct {
	ModelServerURL     string  `env:"MODEL_SERVER_URL" default:"http://localhost:8100"`
	EmbeddingDim       int     `env:"EMBEDDING_DIM" default:"384"`
	EnableHybridSearch bool    `env:"ENABLE_HYBRID_SEARCH" default:"true"`
	EnableReranker     bool    `env:"ENABLE_RERANKER" default:"false"`
	HybridFusionWindow int     `env:"HYBRID_FUSION_WINDOW" default:"200"`
	HybridRRFK         int     `env:"HYBRID_RRF_K" default:"60"`
	LexicalWeight      float64 `env:"HYBRID_LEXICAL_WEIGHT" default:"1.0"`
	SemanticWeight     float64 `env:"HYBRID_SEMANTIC_WEIGHT" default:"1.0"`
	SemanticMinScore   float64 `env:"HYBRID_SEMANTIC_MIN_SCORE" default:"0.65"`
}

type Taxonomy struct {
	AIAddr string `env:"UPSTREAM_AI_ADDR"`
}

func (t Taxonomy) Validate() error {
	if strings.TrimSpace(t.AIAddr) == "" {
		return errors.New("UPSTREAM_AI_ADDR is required")
	}
	return nil
}

type Events struct {
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-search-indexer"`
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
}

type Tombstone struct {
	TTL           time.Duration `env:"TOMBSTONE_TTL" default:"336h"`
	PurgeInterval time.Duration `env:"TOMBSTONE_PURGE_INTERVAL" default:"1h"`
}

func (t Tombstone) Validate() error {
	var errs []error
	if t.TTL <= 0 {
		errs = append(errs, fmt.Errorf("TOMBSTONE_TTL must be a positive duration: %s", t.TTL))
	}
	if t.PurgeInterval <= 0 {
		errs = append(errs, fmt.Errorf("TOMBSTONE_PURGE_INTERVAL must be a positive duration: %s", t.PurgeInterval))
	}
	return errors.Join(errs...)
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres   sdkconfig.Postgres
	OTel       sdkconfig.OTel
	OpenSearch OpenSearch
	Retrieval  Retrieval
}

func Load() (Settings, error) {
	s, err := sdkconfig.Load[Settings]()
	if err != nil {
		return s, err
	}
	if s.OTel.ServiceName == "" {
		s.OTel.ServiceName = serverServiceName
	}
	return s, nil
}

func (s Settings) Validate() error {
	var errs []error
	if s.ShutdownGrace < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_GRACE must be >= 0: %v", s.ShutdownGrace))
	}
	if s.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_DRAIN_DELAY must be >= 0: %v", s.DrainDelay))
	}
	return errors.Join(errs...)
}

type IndexerSettings struct {
	sdkconfig.Runtime
	OTel          sdkconfig.OTel
	OpenSearch    OpenSearch
	Retrieval     Retrieval
	Taxonomy      Taxonomy
	Kafka         sdkconfig.Kafka
	Events        Events
	LogKeyFP      sdkconfig.LogKeyFP
	Tombstone     Tombstone
	ShutdownGrace time.Duration `env:"SHUTDOWN_GRACE" default:"10s"`
}

func LoadIndexer() (IndexerSettings, error) {
	s, err := sdkconfig.Load[IndexerSettings]()
	if err != nil {
		return s, err
	}
	if s.OTel.ServiceName == "" {
		s.OTel.ServiceName = indexerServiceName
	}
	return s, nil
}

func (s IndexerSettings) Validate() error {
	if s.ShutdownGrace <= 0 {
		return fmt.Errorf("SHUTDOWN_GRACE must be a positive duration: %v", s.ShutdownGrace)
	}
	return nil
}

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
