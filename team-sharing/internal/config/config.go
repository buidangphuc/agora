package config

import (
	"os"
	"strconv"
)

type Config struct {
	GRPCPort    int
	DatabaseURL string
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("GRPC_PORT", "50065"))
	return Config{
		GRPCPort:    port,
		DatabaseURL: getEnv("DATABASE_URL", "postgres://sharing_svc:sharing_pass@localhost:5447/sharing_db?sslmode=disable"),
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
