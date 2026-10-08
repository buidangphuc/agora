package config

import (
	"os"
	"strings"
	"testing"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`): the repo-root
// .env.example must document exactly the env keys Settings declares.
func TestEnvExampleInSync(t *testing.T) {
	// Test runs with working dir = this package; .env.example is at the repo root.
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	// DATABASE_URL is required because DATABASE_ENABLED defaults to true; set it
	// so Validate passes, then assert the other struct defaults apply.
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50051 {
		t.Errorf("default GRPC_PORT = %d, want 50051", s.Server.Port)
	}
	if !s.Database.Enabled {
		t.Error("default DATABASE_ENABLED should be true")
	}
	if got := s.KafkaBrokers(); len(got) != 1 || got[0] != "localhost:9092" {
		t.Errorf("default KafkaBrokers = %v, want [localhost:9092]", got)
	}
}

func TestValidateDatabaseURLRequired(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50051
	s.Database.Enabled = true
	s.Database.URL = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: DATABASE_ENABLED=true requires DATABASE_URL")
	}
}

func strictSettings(env string) *Settings {
	s := &Settings{}
	s.Runtime.Env = env
	s.Server.Port = 50051
	s.Database.Enabled = true
	s.Database.URL = "postgresql://x/y"
	s.Events.KafkaEnabled = true
	s.Outbox.Enabled = true
	s.Storage.AccessKey = "ak"
	s.Storage.SecretKey = "sk"
	return s
}

func TestRequireSafeStrictConfig(t *testing.T) {
	strict := []string{"staging", "stage", "prod", "production", "PROD", "  Production ", "Stage"}
	for _, env := range strict {
		if err := strictSettings(env).Validate(); err != nil {
			t.Errorf("ENV=%q safe config must pass: %v", env, err)
		}
		cases := map[string]func(*Settings){
			"KAFKA_ENABLED":      func(s *Settings) { s.Events.KafkaEnabled = false },
			"OUTBOX_ENABLED":     func(s *Settings) { s.Outbox.Enabled = false },
			"STORAGE_ACCESS_KEY": func(s *Settings) { s.Storage.AccessKey = "minioadmin" },
			"STORAGE_SECRET_KEY": func(s *Settings) { s.Storage.SecretKey = "minioadmin" },
		}
		for want, mutate := range cases {
			s := strictSettings(env)
			mutate(s)
			err := s.Validate()
			if err == nil {
				t.Errorf("ENV=%q %s: must be refused", env, want)
				continue
			}
			for _, w := range []string{"ENV", want} {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("ENV=%q: error %q must name %s", env, err, w)
				}
			}
		}
	}
	for _, env := range []string{"local", "test", "", "dev", "development"} {
		s := strictSettings(env)
		s.Events.KafkaEnabled = false
		s.Outbox.Enabled = false
		s.Storage.AccessKey, s.Storage.SecretKey = "minioadmin", "minioadmin"
		if err := s.Validate(); err != nil {
			t.Errorf("ENV=%q must be unaffected: %v", env, err)
		}
	}
}

func TestRequireSafeStrictConfig_FromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://x/y")
	t.Setenv("ENV", "production")
	if _, err := LoadSettings(); err == nil {
		t.Fatal("production with default Kafka/storage settings must be refused by LoadSettings")
	}
	t.Setenv("ENV", "local")
	if _, err := LoadSettings(); err != nil {
		t.Fatalf("local defaults must load: %v", err)
	}
}
