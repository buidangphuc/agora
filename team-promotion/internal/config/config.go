package config

import (
	"errors"
	"strings"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Upstream struct {
	DomainAddr string `env:"UPSTREAM_DOMAIN_ADDR"`
}

func (u Upstream) Validate() error {
	if strings.TrimSpace(u.DomainAddr) == "" {
		return errors.New("UPSTREAM_DOMAIN_ADDR is required")
	}
	return nil
}

type Ads struct {
	MaxBid    int64 `env:"MAX_AD_BID" default:"1000000"`
	MaxBudget int64 `env:"MAX_AD_BUDGET" default:"1000000000"`
}

func (a Ads) Validate() error {
	var errs []error
	if a.MaxBid <= 0 {
		errs = append(errs, errors.New("MAX_AD_BID must be > 0"))
	}
	if a.MaxBudget <= 0 {
		errs = append(errs, errors.New("MAX_AD_BUDGET must be > 0"))
	}
	return errors.Join(errs...)
}

type Events struct {
	Topic string `env:"PROMOTION_EVENTS_TOPIC" default:"promotion.events"`
}

func (e Events) Validate() error {
	if strings.TrimSpace(e.Topic) == "" {
		return errors.New("PROMOTION_EVENTS_TOPIC is required")
	}
	return nil
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	Flags    sdkconfig.Flags
	Audit    sdkconfig.Audit
	Upstream Upstream
	Ads      Ads
	Events   Events
}

func (s Settings) Validate() error {
	var errs []error
	if strings.TrimSpace(s.Postgres.URL) == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if strings.TrimSpace(s.Audit.Addr) == "" {
		errs = append(errs, errors.New("UPSTREAM_AUDIT_ADDR is required"))
	}
	return errors.Join(errs...)
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func (s MigrateSettings) Validate() error {
	if strings.TrimSpace(s.Postgres.URL) == "" {
		return errors.New("DATABASE_URL is required")
	}
	return s.Runtime.Validate()
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
