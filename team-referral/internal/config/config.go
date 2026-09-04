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
	port, _ := strconv.Atoi(getEnv("GRPC_PORT", "50062"))
	return Config{
		GRPCPort:    port,
		DatabaseURL: getEnv("DATABASE_URL", "postgres://referral_svc:referral_pass@localhost:5444/referral_db?sslmode=disable"),
	}
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
