package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Events struct {
	IdentityTopic string `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
}

type OutboxPurge struct {
	Retention time.Duration `env:"OUTBOX_PURGE_RETENTION" default:"168h"`
	Interval  time.Duration `env:"OUTBOX_PURGE_INTERVAL" default:"1h"`
}

func (o OutboxPurge) Validate() error {
	var errs []error
	if o.Retention <= 0 {
		errs = append(errs, errors.New("OUTBOX_PURGE_RETENTION must be > 0"))
	}
	if o.Interval <= 0 {
		errs = append(errs, errors.New("OUTBOX_PURGE_INTERVAL must be > 0"))
	}
	return errors.Join(errs...)
}

type JWT struct {
	PrivateKey   string `env:"JWT_PRIVATE_KEY"`
	KID          string `env:"JWT_KID"`
	JWKSHTTPPort int    `env:"JWKS_HTTP_PORT" default:"50063"`
	TTLSeconds   int    `env:"JWT_TTL_SECONDS" default:"3600"`
}

func (j JWT) Validate() error {
	var errs []error
	if strings.TrimSpace(j.PrivateKey) == "" {
		errs = append(errs, errors.New("JWT_PRIVATE_KEY is required (PEM-encoded RSA private key)"))
	}
	if strings.TrimSpace(j.KID) == "" {
		errs = append(errs, errors.New("JWT_KID is required"))
	}
	if j.JWKSHTTPPort <= 0 || j.JWKSHTTPPort > 65535 {
		errs = append(errs, fmt.Errorf("JWKS_HTTP_PORT out of range: %d", j.JWKSHTTPPort))
	}
	if j.TTLSeconds <= 0 {
		errs = append(errs, fmt.Errorf("JWT_TTL_SECONDS must be > 0: %d", j.TTLSeconds))
	}
	return errors.Join(errs...)
}

func (j JWT) ResolveFor(rt sdkconfig.Runtime) error {
	if !rt.IsStrict() {
		return nil
	}
	pub := publicKeyOf(j.PrivateKey)
	return requireNonDevSigningKey(rt, j.KID, pub)
}

const MinSeedAdminPasswordLen = 12

type SeedAdmin struct {
	Enabled  bool   `env:"SEED_ADMIN_ENABLED" default:"false"`
	Username string `env:"SEED_ADMIN_USERNAME" default:"admin"`
	Password string `env:"SEED_ADMIN_PASSWORD"`
}

func (s SeedAdmin) Validate() error {
	if !s.Enabled {
		return nil
	}
	var errs []error
	if strings.TrimSpace(s.Username) == "" {
		errs = append(errs, errors.New("SEED_ADMIN_USERNAME must not be empty when SEED_ADMIN_ENABLED=true"))
	}
	if len(s.Password) < MinSeedAdminPasswordLen {
		errs = append(errs, fmt.Errorf("SEED_ADMIN_PASSWORD is required and must be at least %d characters when SEED_ADMIN_ENABLED=true", MinSeedAdminPasswordLen))
	}
	return errors.Join(errs...)
}

type PasswordReset struct {
	ExposeToken bool `env:"PASSWORD_RESET_EXPOSE_TOKEN" default:"false"`
}

func (p PasswordReset) ResolveFor(rt sdkconfig.Runtime) error {
	if p.ExposeToken && rt.IsStrict() {
		return fmt.Errorf("PASSWORD_RESET_EXPOSE_TOKEN must not be enabled when ENV is %q (staging or production)", rt.Env)
	}
	return nil
}

type Upstream struct {
	AuditAddr string `env:"UPSTREAM_AUDIT_ADDR"`
}

func (u Upstream) Validate() error {
	if strings.TrimSpace(u.AuditAddr) == "" {
		return errors.New("UPSTREAM_AUDIT_ADDR is required")
	}
	return nil
}

type Retention struct {
	Session        time.Duration `env:"SESSION_RETENTION" default:"720h"`
	LoginHistory   time.Duration `env:"LOGIN_HISTORY_RETENTION" default:"4320h"`
	SweepInterval  time.Duration `env:"SESSION_SWEEP_INTERVAL" default:"1h"`
	SweepBatchSize int           `env:"SESSION_SWEEP_BATCH_SIZE" default:"500"`
}

func (r Retention) Validate() error {
	var errs []error
	for _, c := range []struct {
		key string
		ok  bool
	}{
		{"SESSION_RETENTION", r.Session > 0},
		{"LOGIN_HISTORY_RETENTION", r.LoginHistory > 0},
		{"SESSION_SWEEP_INTERVAL", r.SweepInterval > 0},
		{"SESSION_SWEEP_BATCH_SIZE", r.SweepBatchSize > 0},
	} {
		if !c.ok {
			errs = append(errs, fmt.Errorf("%s must be > 0", c.key))
		}
	}
	return errors.Join(errs...)
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres      sdkconfig.Postgres
	Kafka         sdkconfig.Kafka
	LogKeyFP      sdkconfig.LogKeyFP
	OTel          sdkconfig.OTel
	Outbox        sdkconfig.Outbox
	OutboxPurge   OutboxPurge
	Events        Events
	JWT           JWT
	SeedAdmin     SeedAdmin
	PasswordReset PasswordReset
	Upstream      Upstream
	Retention     Retention
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
