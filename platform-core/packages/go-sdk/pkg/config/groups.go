package config

import (
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
	ReflectionEnabled    bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGraceSeconds float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Addr returns the host:port the server listens on.
func (g GRPC) Addr() string { return net.JoinHostPort(g.Host, strconv.Itoa(g.Port)) }

// ShutdownGrace returns the graceful shutdown deadline.
func (g GRPC) ShutdownGrace() time.Duration {
	return time.Duration(g.ShutdownGraceSeconds * float64(time.Second))
}

// Postgres holds the database settings.
type Postgres struct {
	Enabled  bool   `env:"DATABASE_ENABLED" default:"true"`
	URL      string `env:"DATABASE_URL"`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

// OTel holds the telemetry settings.
type OTel struct {
	Enabled     bool   `env:"OTEL_ENABLED" default:"false"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName string `env:"OTEL_SERVICE_NAME"`
}
