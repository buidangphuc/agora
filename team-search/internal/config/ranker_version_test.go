package config_test

import (
	"testing"

	"github.com/buidangphuc/team-search/internal/config"
)

func TestSearchRankerVersionDefaultsToV1(t *testing.T) {
	serverEnv(t)
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Retrieval.RankerVersion != "v1" {
		t.Errorf("default SEARCH_RANKER_VERSION = %q, want v1", s.Retrieval.RankerVersion)
	}
}

func TestSearchRankerVersionComesFromEnvironment(t *testing.T) {
	serverEnv(t)
	t.Setenv("SEARCH_RANKER_VERSION", "v2")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Retrieval.RankerVersion != "v2" {
		t.Errorf("SEARCH_RANKER_VERSION = %q, want v2", s.Retrieval.RankerVersion)
	}
}
