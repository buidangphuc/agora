package config

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
)

type Listener struct {
	Host              string        `env:"GRPC_HOST"`
	Port              int           `env:"GRPC_PORT,required"`
	ShutdownGrace     time.Duration `env:"SHUTDOWN_GRACE" default:"10s"`
	DrainDelay        time.Duration `env:"SHUTDOWN_DRAIN_DELAY" default:"0s"`
	ReflectionEnabled bool          `env:"GRPC_REFLECTION_ENABLED" default:"false"`
}

func (l *Listener) ResolveFor(rt sdkconfig.Runtime) error {
	if l.ReflectionEnabled && !rt.IsLocal() {
		return errors.New("GRPC_REFLECTION_ENABLED is only allowed when ENV is local or test")
	}
	return nil
}

func (l Listener) Validate() error {
	if l.Port <= 0 {
		return errors.New("GRPC_PORT must be a positive port number")
	}
	return nil
}

func (l Listener) Addr() string { return net.JoinHostPort(l.Host, strconv.Itoa(l.Port)) }

type Audit struct {
	Addr string `env:"UPSTREAM_AUDIT_ADDR"`
}

func (a Audit) Validate() error {
	if strings.TrimSpace(a.Addr) == "" {
		return errors.New("UPSTREAM_AUDIT_ADDR is required")
	}
	return nil
}

type Storage struct {
	Endpoint  string `env:"STORAGE_ENDPOINT"`
	Bucket    string `env:"STORAGE_BUCKET"`
	AccessKey string `env:"STORAGE_ACCESS_KEY"`
	SecretKey string `env:"STORAGE_SECRET_KEY"`
	Region    string `env:"STORAGE_REGION" default:"us-east-1"`
	UseSSL    bool   `env:"STORAGE_USE_SSL" default:"false"`
}

func (s Storage) ResolveFor(rt sdkconfig.Runtime) error {
	var errs []error
	for _, f := range []struct{ key, val string }{
		{"STORAGE_ENDPOINT", s.Endpoint},
		{"STORAGE_BUCKET", s.Bucket},
		{"STORAGE_ACCESS_KEY", s.AccessKey},
		{"STORAGE_SECRET_KEY", s.SecretKey},
	} {
		if strings.TrimSpace(f.val) == "" {
			errs = append(errs, errors.New(f.key+" is required"))
		}
	}
	if rt.IsStrict() {
		if s.AccessKey == "minioadmin" {
			errs = append(errs, errors.New("STORAGE_ACCESS_KEY must not be the minioadmin default when ENV is staging or prod"))
		}
		if s.SecretKey == "minioadmin" {
			errs = append(errs, errors.New("STORAGE_SECRET_KEY must not be the minioadmin default when ENV is staging or prod"))
		}
	}
	return errors.Join(errs...)
}

type Events struct {
	IdentityTopic    string `env:"KAFKA_IDENTITY_TOPIC" default:"identity.events"`
	IdentityDLQTopic string `env:"KAFKA_IDENTITY_DLQ_TOPIC" default:"identity.events.team-verification.dlq"`
	IdentityGroup    string `env:"KAFKA_IDENTITY_GROUP" default:"team-verification-identity"`
}

type Settings struct {
	sdkconfig.Runtime
	Listener
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Audit    Audit
	MockData sdkconfig.MockData
	Kafka    sdkconfig.Kafka
	Storage  Storage
	Events   Events
}

func (s Settings) Validate() error {
	if !s.IsLocal() && !s.IsStrict() {
		return errors.New("ENV must be one of local, test, staging or prod")
	}
	return nil
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
