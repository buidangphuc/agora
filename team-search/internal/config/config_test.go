package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestEnvExampleInSync is the env-drift gate (`make check-env`).
func TestEnvExampleInSync(t *testing.T) {
	const path = "../../.env.example"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf(".env.example not found at %s: %v", path, err)
	}
	if err := CheckEnvExample(path); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDefaults(t *testing.T) {
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Server.Port != 50052 {
		t.Errorf("default GRPC_PORT = %d, want 50052", s.Server.Port)
	}
	if s.OpenSearch.Index != "listings" {
		t.Errorf("default OPENSEARCH_INDEX = %q, want listings", s.OpenSearch.Index)
	}
}

func TestValidateRequiresOpenSearchURL(t *testing.T) {
	s := &Settings{}
	s.Server.Port = 50052
	s.OpenSearch.URL = ""
	if err := s.Validate(); err == nil {
		t.Fatal("expected error: OPENSEARCH_URL required")
	}
}

func TestDatabaseConfig(t *testing.T) {
	t.Setenv("DATABASE_ENABLED", "true")
	t.Setenv("DATABASE_URL", "")
	if _, err := LoadSettings(); err == nil {
		t.Fatal("enabled without DATABASE_URL must fail validation")
	}
	t.Setenv("DATABASE_URL", "postgres://u:p@h/db")
	s, err := LoadSettings()
	if err != nil || !s.Database.Enabled || s.Database.URL != "postgres://u:p@h/db" {
		t.Fatalf("got %+v, %v", s.Database, err)
	}
	t.Setenv("DATABASE_ENABLED", "false")
	t.Setenv("DATABASE_URL", "")
	if s, err = LoadSettings(); err != nil || s.Database.Enabled {
		t.Fatalf("disabled default: %+v, %v", s.Database, err)
	}
}

func TestRequireDurableStorage(t *testing.T) {
	for _, env := range []string{"staging", "stage", "prod", "production", " Production ", "STAGING"} {
		s := &Settings{Runtime: Runtime{Env: env}}
		if err := s.RequireDurableStorage(); err == nil {
			t.Errorf("ENV=%q with DATABASE_ENABLED=false must refuse to boot", env)
		}
		s.Database.Enabled = true
		if err := s.RequireDurableStorage(); err != nil {
			t.Errorf("ENV=%q with the database enabled must pass: %v", env, err)
		}
	}
	for _, env := range []string{"", "local", "test", "dev", "unknown"} {
		s := &Settings{Runtime: Runtime{Env: env}}
		if err := s.RequireDurableStorage(); err != nil {
			t.Errorf("ENV=%q is non-strict and may use memory: %v", env, err)
		}
	}
}

func TestTombstoneDurations(t *testing.T) {
	cases := []struct {
		name               string
		ttl, interval      string
		wantTTL, wantInt   time.Duration
		warnTTL, warnInter bool
	}{
		{"defaults", "336h", "1h", 336 * time.Hour, time.Hour, false, false},
		{"override", "30s", "2s", 30 * time.Second, 2 * time.Second, false, false},
		{"invalid", "banana", "0s", 336 * time.Hour, time.Hour, true, true},
		{"negative and empty", "-1h", "", 336 * time.Hour, time.Hour, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Settings{Tombstone: Tombstone{TTL: tc.ttl, PurgeInterval: tc.interval}}
			ttl, w1 := s.TombstoneTTL()
			iv, w2 := s.TombstonePurgeInterval()
			if ttl != tc.wantTTL || iv != tc.wantInt {
				t.Fatalf("got ttl=%s interval=%s, want %s/%s", ttl, iv, tc.wantTTL, tc.wantInt)
			}
			if (w1 != "") != tc.warnTTL || (w2 != "") != tc.warnInter {
				t.Fatalf("warnings ttl=%q interval=%q", w1, w2)
			}
			if tc.warnTTL && !strings.Contains(w1, "TOMBSTONE_TTL") {
				t.Errorf("ttl warning must name the variable: %q", w1)
			}
			if tc.warnInter && !strings.Contains(w2, "TOMBSTONE_PURGE_INTERVAL") {
				t.Errorf("interval warning must name the variable: %q", w2)
			}
		})
	}
}

func TestTombstoneDefaultsFromEnv(t *testing.T) {
	t.Setenv("TOMBSTONE_TTL", "banana")
	os.Unsetenv("TOMBSTONE_PURGE_INTERVAL")
	cfg, err := LoadSettings()
	if err != nil {
		t.Fatalf("an invalid TOMBSTONE_TTL must not fail config loading: %v", err)
	}
	if d, w := cfg.TombstoneTTL(); d != 336*time.Hour || w == "" {
		t.Fatalf("ttl=%s warn=%q", d, w)
	}
	if d, w := cfg.TombstonePurgeInterval(); d != time.Hour || w != "" {
		t.Fatalf("unset interval=%s warn=%q", d, w)
	}
}

// semantic-floor-calibration: 0.65 is above all but one unrelated pair and below every related pair for
// bge-small-en-v1.5 (scripts/semantic_floor_probe.py); 0.6 let about one unrelated pair in ten through.
func TestSemanticMinScoreDefault(t *testing.T) {
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings with defaults: %v", err)
	}
	if s.Retrieval.SemanticMinScore != 0.65 {
		t.Errorf("default HYBRID_SEMANTIC_MIN_SCORE = %v, want 0.65", s.Retrieval.SemanticMinScore)
	}
}
