package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type ServerConfig struct {
	Host              string
	Port              int
	ShutdownGrace     float64
	ReflectionEnabled bool
}

type RuntimeConfig struct {
	LogLevel string
	LogJSON  bool
}

type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	SSLMode  string
}

type DatabaseConfig struct {
	Enabled  bool
	URL      string
	MaxConns int32
}

type ObservabilityConfig struct {
	Enabled      bool
	OTLPEndpoint string
	ServiceName  string
}

type EventsConfig struct {
	KafkaEnabled bool
	Brokers      string
	ChatTopic    string
}

type UpstreamConfig struct {
	ListingAddr string
}

type Settings struct {
	Server        ServerConfig
	Runtime       RuntimeConfig
	Postgres      PostgresConfig
	Database      DatabaseConfig
	Events        EventsConfig
	Observability ObservabilityConfig
	Upstream      UpstreamConfig
}

func LoadSettings() (*Settings, error) {
	port, _ := strconv.Atoi(getEnv("SERVER_PORT", "50057"))
	pgPort, _ := strconv.Atoi(getEnv("POSTGRES_PORT", "5432"))
	grace, _ := strconv.ParseFloat(getEnv("SERVER_SHUTDOWN_GRACE_SECONDS", "5.0"), 64)
	reflection, _ := strconv.ParseBool(getEnv("SERVER_REFLECTION_ENABLED", "true"))
	logJSON, _ := strconv.ParseBool(getEnv("RUNTIME_LOG_JSON", "false"))

	pgCfg := PostgresConfig{
		Host:     getEnv("POSTGRES_HOST", "postgres-chat"),
		Port:     pgPort,
		DB:       getEnv("POSTGRES_DB", "chat_db"),
		User:     getEnv("POSTGRES_USER", "chat_svc"),
		Password: getEnv("POSTGRES_PASSWORD", "chat_pass"),
		SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
	}

	otelEndpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	kafkaEnabled, _ := strconv.ParseBool(getEnv("KAFKA_ENABLED", "false"))

	return &Settings{
		Server: ServerConfig{
			Host:              getEnv("SERVER_HOST", "0.0.0.0"),
			Port:              port,
			ShutdownGrace:     grace,
			ReflectionEnabled: reflection,
		},
		Runtime: RuntimeConfig{
			LogLevel: getEnv("RUNTIME_LOG_LEVEL", "info"),
			LogJSON:  logJSON,
		},
		Postgres: pgCfg,
		Database: DatabaseConfig{
			Enabled:  true,
			URL:      pgCfg.DSN(),
			MaxConns: 20,
		},
		Events: EventsConfig{
			KafkaEnabled: kafkaEnabled,
			Brokers:      getEnv("KAFKA_BROKERS", "localhost:9092"),
			ChatTopic:    getEnv("KAFKA_CHAT_TOPIC", "chat.events"),
		},
		Observability: ObservabilityConfig{
			Enabled:      otelEndpoint != "",
			OTLPEndpoint: otelEndpoint,
			ServiceName:  getEnv("OTEL_SERVICE_NAME", "team-chat"),
		},
		Upstream: UpstreamConfig{
			ListingAddr: getEnv("UPSTREAM_LISTING_ADDR", "localhost:50051"),
		},
	}, nil
}

func (s *Settings) KafkaBrokers() []string {
	parts := strings.Split(s.Events.Brokers, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (p PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.DB, p.SSLMode)
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
