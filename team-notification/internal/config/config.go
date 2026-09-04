package config

import (
	"os"
	"strconv"
)

type Config struct {
	GRPCPort    int
	DatabaseURL string
	KafkaBroker string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("GRPC_PORT", "50058"))
	return Config{
		GRPCPort:    port,
		DatabaseURL: getEnv("DATABASE_URL", "postgres://notification_svc:notification_pass@localhost:5440/notification_db?sslmode=disable"),
		KafkaBroker: getEnv("KAFKA_BROKER", "localhost:19092"),
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
