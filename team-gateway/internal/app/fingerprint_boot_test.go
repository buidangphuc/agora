package app

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-gateway/internal/config"
)

func loadLocalSettings(t *testing.T) config.Settings {
	t.Helper()
	for k, v := range map[string]string{
		"ENV":           "local",
		"GRPC_HOST":     "127.0.0.1",
		"GRPC_PORT":     strconv.Itoa(testkit.FreePort(t)),
		"JWKS_URL":      "http://" + closedAddr(t) + "/jwks",
		"KAFKA_BROKERS": closedAddr(t),
	} {
		t.Setenv(k, v)
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return s
}

func TestGWFP9_RunFailsWhenFingerprinterCannotBeBuilt(t *testing.T) {
	s := loadLocalSettings(t)
	s.LogKeyFP.Secret = ""

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := Run(ctx, s)
	if err == nil {
		t.Fatal("Run returned nil with an empty log-key fingerprint secret; want a boot error")
	}
	if !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("Run error = %q; want it to name the fingerprinter failure", err)
	}
}
