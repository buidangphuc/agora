package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	GRPCPort    int
	DatabaseURL string
	KafkaBroker string

	// Upstream gRPC addresses for chat sender names: team-domain (shop display
	// name via BatchGetStorefronts) and team-identity (GetPublicProfiles).
	DomainAddr   string
	IdentityAddr string

	// LowStockThreshold: stock at or below this raises a low-stock notification.
	LowStockThreshold int32
}

func Load() (Config, error) {
	thr, err := strconv.ParseInt(getEnv("LOW_STOCK_THRESHOLD", "5"), 10, 32)
	if err != nil || thr < 0 {
		return Config{}, fmt.Errorf("invalid LOW_STOCK_THRESHOLD %q: want integer in [0, 2147483647]", os.Getenv("LOW_STOCK_THRESHOLD"))
	}
	port, _ := strconv.Atoi(getEnv("GRPC_PORT", "50058"))
	return Config{
		LowStockThreshold: int32(thr),
		GRPCPort:          port,
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://notification_svc:notification_pass@localhost:5440/notification_db?sslmode=disable"),
		KafkaBroker:       getEnv("KAFKA_BROKER", "localhost:19092"),

		DomainAddr:   getEnv("UPSTREAM_DOMAIN_ADDR", "localhost:50051"),
		IdentityAddr: getEnv("UPSTREAM_IDENTITY_ADDR", "localhost:50053"),
	}, nil
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
