package config_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/buidangphuc/team-notification/internal/config"
)

func erasureSetting(t *testing.T, s any, key string) any {
	t.Helper()
	var walk func(v reflect.Value) (any, bool)
	walk = func(v reflect.Value) (any, bool) {
		v = reflect.Indirect(v)
		if v.Kind() != reflect.Struct {
			return nil, false
		}
		for i := 0; i < v.NumField(); i++ {
			ft := v.Type().Field(i)
			if !ft.IsExported() {
				continue
			}
			if tag, ok := ft.Tag.Lookup("env"); ok && strings.Split(tag, ",")[0] == key {
				return v.Field(i).Interface(), true
			}
			if got, ok := walk(v.Field(i)); ok {
				return got, true
			}
		}
		return nil, false
	}
	got, ok := walk(reflect.ValueOf(s))
	if !ok {
		t.Fatalf("no setting declared for %s", key)
	}
	return got
}

// Scenario: A malformed event goes to the DLQ (erasure consumer topic and group defaults)
func TestIdentityErasureConfigDefaults(t *testing.T) {
	localEnv(t)
	for _, k := range []string{"IDENTITY_EVENTS_TOPIC", "KAFKA_ERASURE_CONSUMER_GROUP"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := erasureSetting(t, s, "IDENTITY_EVENTS_TOPIC"); got != "identity.events" {
		t.Errorf("IDENTITY_EVENTS_TOPIC = %v, want identity.events", got)
	}
	if got := erasureSetting(t, s, "KAFKA_ERASURE_CONSUMER_GROUP"); got != "team-notification-erasure" {
		t.Errorf("KAFKA_ERASURE_CONSUMER_GROUP = %v, want team-notification-erasure", got)
	}
}

// Scenario: A malformed event goes to the DLQ (erasure settings are overridable)
func TestIdentityErasureConfigOverride(t *testing.T) {
	localEnv(t)
	t.Setenv("IDENTITY_EVENTS_TOPIC", "identity.events.test")
	t.Setenv("KAFKA_ERASURE_CONSUMER_GROUP", "erasure-g")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := erasureSetting(t, s, "IDENTITY_EVENTS_TOPIC"); got != "identity.events.test" {
		t.Errorf("IDENTITY_EVENTS_TOPIC = %v", got)
	}
	if got := erasureSetting(t, s, "KAFKA_ERASURE_CONSUMER_GROUP"); got != "erasure-g" {
		t.Errorf("KAFKA_ERASURE_CONSUMER_GROUP = %v", got)
	}
}
