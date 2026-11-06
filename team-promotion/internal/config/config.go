package config

import (
	"errors"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Upstream struct {
	DomainAddr   string        `env:"UPSTREAM_DOMAIN_ADDR"`
	OrderAddr    string        `env:"UPSTREAM_ORDER_ADDR"`
	OrderTimeout time.Duration `env:"UPSTREAM_ORDER_TIMEOUT" default:"500ms"`
}

func (u Upstream) Validate() error {
	var errs []error
	if strings.TrimSpace(u.DomainAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_DOMAIN_ADDR is required"))
	}
	if u.OrderTimeout <= 0 {
		errs = append(errs, errors.New("UPSTREAM_ORDER_TIMEOUT must be > 0"))
	}
	return errors.Join(errs...)
}

type NewUser struct {
	WelcomeAmount  int64         `env:"NEW_USER_WELCOME_AMOUNT" default:"20000"`
	InvitedAmount  int64         `env:"NEW_USER_INVITED_AMOUNT" default:"30000"`
	MinSpend       int64         `env:"NEW_USER_MIN_SPEND" default:"100000"`
	VoucherTTL     time.Duration `env:"NEW_USER_VOUCHER_TTL" default:"336h"`
	VerdictTimeout time.Duration `env:"NEW_USER_VERDICT_TIMEOUT" default:"30m"`
	SweepInterval  time.Duration `env:"NEW_USER_VERDICT_SWEEP_INTERVAL" default:"1m"`
	ReservationTTL time.Duration `env:"VOUCHER_RESERVATION_TTL" default:"15m"`
}

func (n NewUser) Validate() error {
	var errs []error
	for key, val := range map[string]int64{
		"NEW_USER_WELCOME_AMOUNT": n.WelcomeAmount,
		"NEW_USER_INVITED_AMOUNT": n.InvitedAmount,
	} {
		if val <= 0 {
			errs = append(errs, errors.New(key+" must be > 0"))
		}
	}
	if n.MinSpend < 0 {
		errs = append(errs, errors.New("NEW_USER_MIN_SPEND must be >= 0"))
	}
	for key, val := range map[string]time.Duration{
		"NEW_USER_VOUCHER_TTL":            n.VoucherTTL,
		"NEW_USER_VERDICT_TIMEOUT":        n.VerdictTimeout,
		"NEW_USER_VERDICT_SWEEP_INTERVAL": n.SweepInterval,
		"VOUCHER_RESERVATION_TTL":         n.ReservationTTL,
	} {
		if val <= 0 {
			errs = append(errs, errors.New(key+" must be > 0"))
		}
	}
	return errors.Join(errs...)
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
	NewUser  NewUser
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
