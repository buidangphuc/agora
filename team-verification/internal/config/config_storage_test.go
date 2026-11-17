package config_test

import (
	"os"
	"testing"

	"github.com/buidangphuc/team-verification/internal/config"
)

func TestLoadRequiresStorageSettings(t *testing.T) {
	const sc = "scenario Bucket is not publicly readable: boot fails without MinIO settings"
	for _, env := range []string{"local", "test", "staging", "prod"} {
		for _, key := range []string{"STORAGE_ENDPOINT", "STORAGE_BUCKET", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY"} {
			t.Run(sc+" "+env+" "+key, func(t *testing.T) {
				baseEnv(t, env)
				t.Setenv(key, "")
				_, err := config.Load()
				wantErrNaming(t, err, key)

				os.Unsetenv(key)
				_, err = config.Load()
				wantErrNaming(t, err, key)
			})
		}
	}
}

func TestLoadStorageValues(t *testing.T) {
	const sc = "scenario Bucket is not publicly readable: storage settings are loaded"
	baseEnv(t, "local")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("%s: Load: %v", sc, err)
	}
	if s.Storage.Endpoint != "minio:9000" || s.Storage.Bucket != "kyc-documents" ||
		s.Storage.AccessKey != "kyc-access" || s.Storage.SecretKey != "kyc-secret" {
		t.Errorf("%s: Storage = %+v", sc, s.Storage)
	}
	if s.Storage.UseSSL {
		t.Errorf("%s: UseSSL on by default", sc)
	}
	if s.Storage.Region != "us-east-1" {
		t.Errorf("%s: Region = %q, want us-east-1", sc, s.Storage.Region)
	}
	t.Setenv("STORAGE_USE_SSL", "true")
	s, err = config.Load()
	if err != nil {
		t.Fatalf("%s: Load override: %v", sc, err)
	}
	if !s.Storage.UseSSL {
		t.Errorf("%s: STORAGE_USE_SSL=true not honoured", sc)
	}
}

func TestStrictEnvRefusesMinioDefaultCredentials(t *testing.T) {
	const sc = "scenario Bucket is not publicly readable: strict ENV refuses the minioadmin default"
	for _, env := range []string{"staging", "prod"} {
		for _, key := range []string{"STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY"} {
			t.Run(sc+" "+env+" "+key, func(t *testing.T) {
				baseEnv(t, env)
				t.Setenv(key, "minioadmin")
				_, err := config.Load()
				wantErrNaming(t, err, key)
			})
		}
	}
	t.Run(sc+" local allows", func(t *testing.T) {
		baseEnv(t, "local")
		t.Setenv("STORAGE_ACCESS_KEY", "minioadmin")
		t.Setenv("STORAGE_SECRET_KEY", "minioadmin")
		if _, err := config.Load(); err != nil {
			t.Fatalf("Load: %v", err)
		}
	})
}

func TestLoadKafkaSettings(t *testing.T) {
	const sc = "scenario Storage outage sends the event to retry then DLQ: Kafka settings"
	for _, env := range []string{"staging", "prod"} {
		t.Run(sc+" "+env+" brokers required", func(t *testing.T) {
			baseEnv(t, env)
			os.Unsetenv("KAFKA_BROKERS")
			_, err := config.Load()
			wantErrNaming(t, err, "KAFKA_BROKERS")
		})
	}
	t.Run(sc+" local defaults brokers", func(t *testing.T) {
		baseEnv(t, "local")
		os.Unsetenv("KAFKA_BROKERS")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Kafka.Brokers == "" {
			t.Error("Kafka.Brokers empty in local")
		}
	})
	t.Run(sc+" topics and group", func(t *testing.T) {
		baseEnv(t, "local")
		s, err := config.Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if s.Kafka.Brokers != "kafka:9092" {
			t.Errorf("Brokers = %q", s.Kafka.Brokers)
		}
		if s.Events.IdentityTopic != "identity.events" {
			t.Errorf("IdentityTopic = %q, want identity.events", s.Events.IdentityTopic)
		}
		if s.Events.IdentityDLQTopic != "identity.events.team-verification.dlq" {
			t.Errorf("IdentityDLQTopic = %q, want identity.events.team-verification.dlq", s.Events.IdentityDLQTopic)
		}
		if s.Events.IdentityGroup == "" || s.Events.IdentityGroup == "team-domain-identity" {
			t.Errorf("IdentityGroup = %q, want an own consumer group", s.Events.IdentityGroup)
		}
	})
}
