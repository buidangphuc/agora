package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	GRPCPort    int
	DatabaseURL string
	// AuditAddr is the team-audit gRPC address (UPSTREAM_AUDIT_ADDR); empty = no audit upstream.
	AuditAddr string
	// CallTimeout bounds each upstream call (UPSTREAM_CALL_TIMEOUT_SECONDS, float seconds).
	CallTimeout time.Duration
}

func Load() Config {
	port, _ := strconv.Atoi(getEnv("GRPC_PORT", "50064"))
	secs, err := strconv.ParseFloat(getEnv("UPSTREAM_CALL_TIMEOUT_SECONDS", "2"), 64)
	if err != nil {
		secs = 2
	}
	return Config{
		GRPCPort:    port,
		DatabaseURL: getEnv("DATABASE_URL", "postgres://verification_svc:verification_pass@localhost:5444/verification_db?sslmode=disable"),
		AuditAddr:   os.Getenv("UPSTREAM_AUDIT_ADDR"),
		CallTimeout: time.Duration(secs * float64(time.Second)),
	}
}

// Validate rejects an unusable configuration.
func (c Config) Validate() error {
	if c.CallTimeout <= 0 {
		return errors.New("UPSTREAM_CALL_TIMEOUT_SECONDS must be > 0")
	}
	return nil
}

func getEnv(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}
