package config

import sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

type OTel struct {
	Enabled     bool   `env:"OTEL_ENABLED" default:"false"`
	Endpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	ServiceName string `env:"OTEL_SERVICE_NAME" default:"team-chat"`
}

type Events struct {
	ChatTopic string `env:"KAFKA_CHAT_TOPIC" default:"chat.events"`
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
