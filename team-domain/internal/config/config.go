package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Storage struct {
	Endpoint      string `env:"STORAGE_ENDPOINT" default:"localhost:9000"`
	Bucket        string `env:"STORAGE_BUCKET" default:"listing-images"`
	AccessKey     string `env:"STORAGE_ACCESS_KEY" default:"minioadmin"`
	SecretKey     string `env:"STORAGE_SECRET_KEY" default:"minioadmin"`
	Region        string `env:"STORAGE_REGION" default:"us-east-1"`
	UseSSL        bool   `env:"STORAGE_USE_SSL" default:"false"`
	PublicBaseURL string `env:"STORAGE_PUBLIC_BASE_URL" default:"http://localhost:9000/listing-images"`
}

func (s Storage) ResolveFor(rt sdkconfig.Runtime) error {
	if !rt.IsStrict() {
		return nil
	}
	var errs []error
	if s.AccessKey == "minioadmin" {
		errs = append(errs, errors.New("STORAGE_ACCESS_KEY must not be the minioadmin default when ENV is staging or prod"))
	}
	if s.SecretKey == "minioadmin" {
		errs = append(errs, errors.New("STORAGE_SECRET_KEY must not be the minioadmin default when ENV is staging or prod"))
	}
	return errors.Join(errs...)
}

type Events struct {
	ListingTopic  string `env:"KAFKA_LISTING_TOPIC" default:"listing.events"`
	IdentityTopic string `env:"KAFKA_IDENTITY_TOPIC" default:"identity.events"`
	IdentityGroup string `env:"KAFKA_IDENTITY_GROUP" default:"team-domain-identity"`
}

type Reservation struct {
	TTL           string `env:"RESERVATION_TTL" default:"15m" keepempty:"true"`
	SweepInterval string `env:"RESERVATION_SWEEP_INTERVAL" default:"1m" keepempty:"true"`
}

const (
	DefaultReservationTTL           = 15 * time.Minute
	DefaultReservationSweepInterval = time.Minute
)

type Upstream struct {
	AuditAddr        string  `env:"UPSTREAM_AUDIT_ADDR"`
	AIAddr           string  `env:"UPSTREAM_AI_ADDR"`
	AITimeoutSeconds float64 `env:"UPSTREAM_AI_TIMEOUT_SECONDS" default:"2"`
}

func (u Upstream) ResolveFor(sdkconfig.Runtime) error {
	var errs []error
	if strings.TrimSpace(u.AuditAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_AUDIT_ADDR is required"))
	}
	if strings.TrimSpace(u.AIAddr) == "" {
		errs = append(errs, errors.New("UPSTREAM_AI_ADDR is required"))
	}
	return errors.Join(errs...)
}

type Screen struct {
	TermsFile string `env:"SCREEN_TERMS_FILE"`
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres    sdkconfig.Postgres
	OTel        sdkconfig.OTel
	Kafka       sdkconfig.Kafka
	Outbox      sdkconfig.Outbox
	LogKeyFP    sdkconfig.LogKeyFP
	Storage     Storage
	Events      Events
	Reservation Reservation
	Upstream    Upstream
	Screen      Screen
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

func (s Settings) ResolveFor(sdkconfig.Runtime) error { return s.Postgres.Validate() }

func (s Settings) Validate() error {
	var errs []error
	if s.GRPC.ShutdownGrace < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_GRACE must be >= 0: %v", s.GRPC.ShutdownGrace))
	}
	if s.GRPC.DrainDelay < 0 {
		errs = append(errs, fmt.Errorf("SHUTDOWN_DRAIN_DELAY must be >= 0: %v", s.GRPC.DrainDelay))
	}
	if f := strings.TrimSpace(s.Screen.TermsFile); f != "" {
		if _, err := os.Stat(f); err != nil {
			errs = append(errs, fmt.Errorf("SCREEN_TERMS_FILE %s: %w", f, err))
		}
	}
	return errors.Join(errs...)
}

func (s Settings) ReservationTTL(logger *slog.Logger) time.Duration {
	return positiveDuration(logger, "RESERVATION_TTL", s.Reservation.TTL, DefaultReservationTTL)
}

func (s Settings) ReservationSweepInterval(logger *slog.Logger) time.Duration {
	return positiveDuration(logger, "RESERVATION_SWEEP_INTERVAL", s.Reservation.SweepInterval, DefaultReservationSweepInterval)
}

func positiveDuration(logger *slog.Logger, key, raw string, def time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err == nil && d > 0 {
		return d
	}
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("invalid "+key+"; using the default",
		slog.String("key", key), slog.String("value", raw), slog.String("default", def.String()))
	return def
}
