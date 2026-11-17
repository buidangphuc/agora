package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-analytics/internal/config"
)

func fieldByEnv(v reflect.Value, key string) (reflect.Value, bool) {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("env") == key {
			return v.Field(i), true
		}
		if f.Type.Kind() == reflect.Struct {
			if r, ok := fieldByEnv(v.Field(i), key); ok {
				return r, true
			}
		}
	}
	return reflect.Value{}, false
}

func declared(key string) bool {
	for _, k := range sdkconfig.DeclaredEnvKeys(config.Settings{}) {
		if k == key {
			return true
		}
	}
	return false
}

// Scenario: An unset store address returns UNAVAILABLE (FORECAST_REDIS_URL optional, empty by default)
func TestForecastRedisURLOptionalAndEmpty_AnUnsetStoreAddressReturnsUnavailable(t *testing.T) {
	if !declared("FORECAST_REDIS_URL") {
		t.Fatal("FORECAST_REDIS_URL is not a declared setting")
	}
	for _, env := range []string{"local", "production"} {
		t.Run(env, func(t *testing.T) {
			if env == "local" {
				localEnv(t)
			} else {
				strictEnv(t, env)
			}
			s := mustLoad(t)
			f, ok := fieldByEnv(reflect.ValueOf(s), "FORECAST_REDIS_URL")
			if !ok || f.String() != "" {
				t.Fatalf("FORECAST_REDIS_URL found=%v value=%q, want empty", ok, f)
			}
		})
	}
}

// Scenario: A read stays within the latency budget (FORECAST_READ_TIMEOUT_MS defaults to 25)
func TestForecastReadTimeoutDefault25_AReadStaysWithinTheLatencyBudget(t *testing.T) {
	if !declared("FORECAST_READ_TIMEOUT_MS") {
		t.Fatal("FORECAST_READ_TIMEOUT_MS is not a declared setting")
	}
	localEnv(t)
	f, ok := fieldByEnv(reflect.ValueOf(mustLoad(t)), "FORECAST_READ_TIMEOUT_MS")
	if !ok || f.Int() != 25 {
		t.Fatalf("FORECAST_READ_TIMEOUT_MS found=%v value=%v, want 25", ok, f)
	}
}

// Scenario: A read stays within the latency budget (override via env)
func TestForecastSettingsReadFromEnv_AReadStaysWithinTheLatencyBudget(t *testing.T) {
	localEnv(t)
	t.Setenv("FORECAST_REDIS_URL", "redis://redis:6379/3")
	t.Setenv("FORECAST_READ_TIMEOUT_MS", "40")
	s := mustLoad(t)
	u, _ := fieldByEnv(reflect.ValueOf(s), "FORECAST_REDIS_URL")
	m, _ := fieldByEnv(reflect.ValueOf(s), "FORECAST_READ_TIMEOUT_MS")
	if !u.IsValid() || !m.IsValid() || u.String() != "redis://redis:6379/3" || m.Int() != 40 {
		t.Fatalf("url=%v timeout=%v", u, m)
	}
}

// Scenario: An unset store address returns UNAVAILABLE (documented in .env.example and README)
func TestForecastSettingsDocumented_AnUnsetStoreAddressReturnsUnavailable(t *testing.T) {
	for _, file := range []string{"../../.env.example", "../../README.md"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"FORECAST_REDIS_URL", "FORECAST_READ_TIMEOUT_MS"} {
			if !strings.Contains(string(b), k) {
				t.Errorf("%s does not document %s", file, k)
			}
		}
	}
}
