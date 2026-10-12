package config

import (
	"errors"
	"net"
	"strconv"
	"time"
)

// Runtime holds settings every service shares.
type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

// IsLocal reports whether the service runs in a developer or test environment.
func (r Runtime) IsLocal() bool { return r.Env == "local" || r.Env == "test" }

// GRPC holds the gRPC listener settings.
type GRPC struct {
	Host                 string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port                 int     `env:"GRPC_PORT,required"`
	ShutdownGraceSeconds float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
	// DrainDelaySeconds is how long to keep serving after health reports
	// NOT_SERVING, so probes and load balancers notice before the server stops.
	DrainDelaySeconds float64 `env:"SHUTDOWN_DRAIN_DELAY_SECONDS" default:"0"`
}

// Addr returns the host:port the server listens on.
func (g GRPC) Addr() string { return net.JoinHostPort(g.Host, strconv.Itoa(g.Port)) }

// ShutdownGrace returns the graceful shutdown deadline.
func (g GRPC) ShutdownGrace() time.Duration {
	return time.Duration(g.ShutdownGraceSeconds * float64(time.Second))
}

// DrainDelay returns the pause between draining health and stopping components.
func (g GRPC) DrainDelay() time.Duration {
	return time.Duration(g.DrainDelaySeconds * float64(time.Second))
}

// Postgres holds the database settings.
type Postgres struct {
	URL      string `env:"DATABASE_URL"`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

// Validate rejects an empty URL: the database is always on, with no enable flag.
func (p Postgres) Validate() error {
	if p.URL == "" {
		return errors.New("DATABASE_URL is required")
	}
	return nil
}

// OTel holds the telemetry settings.
type OTel struct {
	Enabled     bool   `env:"OTEL_ENABLED" default:"false"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName string `env:"OTEL_SERVICE_NAME"`
}
