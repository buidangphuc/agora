// Package config assembles team-engagement's flat Settings from per-capability
// struct groups (same reflection loader + .env.example drift gate as the other
// Go services).
package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

type Settings struct {
	Runtime       Runtime
	Server        Server
	Database      Database
	Observability Observability
	Upstream      Upstream
}

type Runtime struct {
	Env      string `env:"ENV" default:"local"`
	LogLevel string `env:"LOG_LEVEL" default:"info"`
	LogJSON  bool   `env:"LOG_JSON" default:"true"`
}

type Server struct {
	Host              string  `env:"GRPC_HOST" default:"0.0.0.0"`
	Port              int     `env:"GRPC_PORT" default:"50054"`
	ReflectionEnabled bool    `env:"GRPC_REFLECTION_ENABLED" default:"true"`
	ShutdownGrace     float64 `env:"SHUTDOWN_GRACE_SECONDS" default:"10"`
}

// Database is this service's OWN Postgres (engagement_db). Rule 3.
type Database struct {
	Enabled  bool   `env:"DATABASE_ENABLED" default:"true"`
	URL      string `env:"DATABASE_URL" default:""`
	MaxConns int32  `env:"DB_MAX_CONNS" default:"10"`
}

type Observability struct {
	Enabled      bool   `env:"OTEL_ENABLED" default:"false"`
	OTLPEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" default:""`
	ServiceName  string `env:"OTEL_SERVICE_NAME" default:"team-engagement"`
}

// Upstream holds addresses of sibling services consulted over gRPC (never their
// DBs, Rule 3). Empty OrderAddr disables verified-purchase enrichment.
type Upstream struct {
	OrderAddr string `env:"UPSTREAM_ORDER_ADDR" default:""`
}

func LoadSettings() (*Settings, error) {
	s := &Settings{}
	if err := bindGroups(reflect.ValueOf(s).Elem()); err != nil {
		return nil, err
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Settings) Validate() error {
	if s.Database.Enabled && strings.TrimSpace(s.Database.URL) == "" {
		return errors.New("DATABASE_URL is required when DATABASE_ENABLED=true")
	}
	if s.Server.Port <= 0 || s.Server.Port > 65535 {
		return fmt.Errorf("GRPC_PORT out of range: %d", s.Server.Port)
	}
	return nil
}

func (s *Settings) IsProd() bool {
	e := strings.ToLower(strings.TrimSpace(s.Runtime.Env))
	return e == "prod" || e == "production"
}

func DeclaredEnvKeys() []string {
	var keys []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		gt := t.Field(i).Type
		for j := 0; j < gt.NumField(); j++ {
			if k := gt.Field(j).Tag.Get("env"); k != "" {
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func bindGroups(v reflect.Value) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		group := v.Field(i)
		gt := group.Type()
		for j := 0; j < gt.NumField(); j++ {
			f := gt.Field(j)
			key := f.Tag.Get("env")
			if key == "" {
				continue
			}
			raw := f.Tag.Get("default")
			if val, ok := os.LookupEnv(key); ok {
				raw = val
			}
			if err := setField(group.Field(j), raw); err != nil {
				return fmt.Errorf("config %s: %w", key, err)
			}
		}
	}
	return nil
}

func setField(fv reflect.Value, raw string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case reflect.Int, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return err
		}
		fv.SetInt(n)
	case reflect.Float32, reflect.Float64:
		x, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return err
		}
		fv.SetFloat(x)
	default:
		return fmt.Errorf("unsupported config field kind %s", fv.Kind())
	}
	return nil
}
