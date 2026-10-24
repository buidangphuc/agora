package config

import (
	"errors"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Upstream struct {
	OrderAddr string `env:"UPSTREAM_ORDER_ADDR"`
	AuditAddr string `env:"UPSTREAM_AUDIT_ADDR"`
}

func (u Upstream) ResolveFor(sdkconfig.Runtime) error {
	var errs []error
	if strings.TrimSpace(u.OrderAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_ORDER_ADDR is required"))
	}
	if strings.TrimSpace(u.AuditAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_AUDIT_ADDR is required"))
	}
	return errors.Join(errs...)
}

type Events struct {
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
	ConsumerGroup string `env:"KAFKA_CONSUMER_GROUP" default:"team-engagement-feed"`
	EventsTopic   string `env:"ENGAGEMENT_EVENTS_TOPIC" default:"engagement.events"`
}

func (e Events) Validate() error {
	var errs []error
	if strings.TrimSpace(e.ListingTopic) == "" {
		errs = append(errs, errors.New("KAFKA_LISTING_TOPIC is required"))
	}
	if strings.TrimSpace(e.ConsumerGroup) == "" {
		errs = append(errs, errors.New("KAFKA_CONSUMER_GROUP is required"))
	}
	if strings.TrimSpace(e.EventsTopic) == "" {
		errs = append(errs, errors.New("ENGAGEMENT_EVENTS_TOPIC is required"))
	}
	return errors.Join(errs...)
}

type Reports struct {
	RetentionDays int           `env:"REPORT_RETENTION_DAYS" default:"730"`
	SweepInterval time.Duration `env:"REPORT_SWEEP_INTERVAL" default:"24h"`
}

func (r Reports) Validate() error {
	var errs []error
	if r.RetentionDays <= 0 {
		errs = append(errs, errors.New("REPORT_RETENTION_DAYS must be > 0"))
	}
	if r.SweepInterval <= 0 {
		errs = append(errs, errors.New("REPORT_SWEEP_INTERVAL must be > 0"))
	}
	return errors.Join(errs...)
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	LogKeyFP sdkconfig.LogKeyFP
	Events   Events
	Upstream Upstream
	Reports  Reports
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
