package config_test

import (
	"os"
	"testing"
	"time"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"
)

type TestAppConfig struct {
	AppPort int           `env:"TEST_APP_PORT" default:"8080"`
	AppName string        `env:"TEST_APP_NAME" default:"marketplace"`
	Debug   bool          `env:"TEST_DEBUG" default:"true"`
	Timeout time.Duration `env:"TEST_TIMEOUT" default:"5s"`
	Brokers []string      `env:"TEST_BROKERS" default:"localhost:9092, localhost:9093"`
}

func TestConfigLoad(t *testing.T) {
	os.Setenv("TEST_APP_PORT", "9999")
	os.Setenv("TEST_APP_NAME", "custom-app")
	defer func() {
		os.Unsetenv("TEST_APP_PORT")
		os.Unsetenv("TEST_APP_NAME")
	}()

	var cfg TestAppConfig
	if err := config.Load(&cfg); err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}

	if cfg.AppPort != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.AppPort)
	}
	if cfg.AppName != "custom-app" {
		t.Errorf("expected name custom-app, got %s", cfg.AppName)
	}
	if !cfg.Debug {
		t.Errorf("expected debug true, got false")
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", cfg.Timeout)
	}
	if len(cfg.Brokers) != 2 || cfg.Brokers[0] != "localhost:9092" {
		t.Errorf("expected 2 brokers, got %v", cfg.Brokers)
	}
}
