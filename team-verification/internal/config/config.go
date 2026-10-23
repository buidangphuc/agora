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

type Settings struct {
	sdkconfig.Runtime
	Listener
	Postgres sdkconfig.Postgres
	OTel     sdkconfig.OTel
	Audit    Audit
	MockData sdkconfig.MockData
}

func (s Settings) Validate() error {
	if !s.IsLocal() && !s.IsStrict() {
		return errors.New("ENV must be one of local, test, staging or prod")
	}
	return nil
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }
