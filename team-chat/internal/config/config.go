package config

import sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

type OTel struct {
	Enabled     bool   `env:"OTEL_ENABLED" default:"false"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName string `env:"OTEL_SERVICE_NAME" default:"team-chat"`
}

type Events struct {
	ChatTopic string `env:"KAFKA_CHAT_TOPIC" default:"chat.events"`

	IdentityTopic        string `env:"IDENTITY_EVENTS_TOPIC" default:"identity.events"`
	ErasureConsumerGroup string `env:"KAFKA_ERASURE_CONSUMER_GROUP" default:"team-chat-erasure"`
	LogKeyFPSecret       string `env:"LOG_KEY_FP_SECRET"`
}

type Settings struct {
	sdkconfig.Runtime
	sdkconfig.GRPC
	Postgres sdkconfig.Postgres
	Kafka    sdkconfig.Kafka
	Outbox   sdkconfig.Outbox
	Events   Events
	OTel     OTel
}

func Load() (Settings, error) { return sdkconfig.Load[Settings]() }

type MigrateSettings struct {
	sdkconfig.Runtime
	Postgres sdkconfig.Postgres
}

func LoadMigrate() (MigrateSettings, error) { return sdkconfig.Load[MigrateSettings]() }
