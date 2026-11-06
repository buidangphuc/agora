package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Referral struct {
	RewardXU            int64         `env:"REFERRAL_REWARD_XU" default:"10000"`
	DailyCap            int           `env:"REFERRAL_DAILY_CAP" default:"5"`
	LifetimeCap         int           `env:"REFERRAL_LIFETIME_CAP" default:"50"`
	RedeemMaxAccountAge time.Duration `env:"REFERRAL_REDEEM_MAX_ACCOUNT_AGE" default:"168h"`
	SweepInterval       time.Duration `env:"REFERRAL_RELEASE_SWEEP_INTERVAL" default:"1m"`
	CapTimezone         string        `env:"REFERRAL_CAP_TIMEZONE" default:"Asia/Ho_Chi_Minh"`
	IdentityTopic       string        `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
	OrderTopic          string        `env:"ORDER_EVENTS_TOPIC" default:"order.events"`
	EventsTopic         string        `env:"REFERRAL_EVENTS_TOPIC" default:"referral.events"`
	IdentityGroup       string        `env:"REFERRAL_IDENTITY_CONSUMER_GROUP" default:"team-referral-identity"`
	OrderGroup          string        `env:"REFERRAL_ORDER_CONSUMER_GROUP" default:"team-referral-order"`
	IdentityDLQTopic    string        `env:"REFERRAL_IDENTITY_DLQ_TOPIC" default:"referral.identity.dlq"`
	OrderDLQTopic       string        `env:"REFERRAL_ORDER_DLQ_TOPIC" default:"referral.order.dlq"`
}

func (r Referral) Location() (*time.Location, error) {
	return time.LoadLocation(r.CapTimezone)
}

func (r Referral) Validate() error {
	var errs []error
	if r.RewardXU <= 0 {
		errs = append(errs, errors.New("REFERRAL_REWARD_XU must be > 0"))
	}
	if r.DailyCap <= 0 {
		errs = append(errs, errors.New("REFERRAL_DAILY_CAP must be > 0"))
	}
	if r.LifetimeCap <= 0 {
		errs = append(errs, errors.New("REFERRAL_LIFETIME_CAP must be > 0"))
	}
	if r.RedeemMaxAccountAge <= 0 {
		errs = append(errs, errors.New("REFERRAL_REDEEM_MAX_ACCOUNT_AGE must be > 0"))
	}
	if r.SweepInterval <= 0 {
		errs = append(errs, errors.New("REFERRAL_RELEASE_SWEEP_INTERVAL must be > 0"))
	}
	if _, err := r.Location(); err != nil {
		errs = append(errs, fmt.Errorf("REFERRAL_CAP_TIMEZONE: %w", err))
	}
	for key, val := range map[string]string{
		"IDENTITY_EVENTS_TOPIC":            r.IdentityTopic,
		"ORDER_EVENTS_TOPIC":               r.OrderTopic,
		"REFERRAL_EVENTS_TOPIC":            r.EventsTopic,
		"REFERRAL_IDENTITY_CONSUMER_GROUP": r.IdentityGroup,
		"REFERRAL_ORDER_CONSUMER_GROUP":    r.OrderGroup,
		"REFERRAL_IDENTITY_DLQ_TOPIC":      r.IdentityDLQTopic,
		"REFERRAL_ORDER_DLQ_TOPIC":         r.OrderDLQTopic,
	} {
		if strings.TrimSpace(val) == "" {
			errs = append(errs, fmt.Errorf("%s is required", key))
		}
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
	Referral Referral
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
