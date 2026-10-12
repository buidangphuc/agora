package ga4relay

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse/duckdb"
)

func mappingContract(t *testing.T, mk func(t *testing.T, clk *fakeClock) MappingStore) {
	ctx := context.Background()
	start := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	t.Run("latest wins", func(t *testing.T) {
		s := mk(t, &fakeClock{t: start})
		_ = s.Put(ctx, "b1", "111.1")
		_ = s.Put(ctx, "b1", "222.2")
		if id, ok := mustGet(t, s, "b1"); !ok || id != "222.2" {
			t.Fatalf("got %q,%v", id, ok)
		}
		if _, ok := mustGet(t, s, "b2"); ok {
			t.Fatal("b2 must have no mapping")
		}
	})
	t.Run("delete", func(t *testing.T) {
		s := mk(t, &fakeClock{t: start})
		_ = s.Put(ctx, "b1", "222.2")
		if err := s.Delete(ctx, "b1"); err != nil {
			t.Fatal(err)
		}
		if _, ok := mustGet(t, s, "b1"); ok {
			t.Fatal("mapping still present")
		}
		if err := s.Delete(ctx, "nobody"); err != nil {
			t.Fatalf("deleting absent mapping: %v", err)
		}
	})
	t.Run("expires after 30 days", func(t *testing.T) {
		clk := &fakeClock{t: start}
		s := mk(t, clk)
		_ = s.Put(ctx, "b1", "222.2")
		clk.Advance(29 * 24 * time.Hour)
		if _, ok := mustGet(t, s, "b1"); !ok {
			t.Fatal("expired before 30 days")
		}
		clk.Advance(2 * 24 * time.Hour)
		if _, ok := mustGet(t, s, "b1"); ok {
			t.Fatal("still present after 30 days")
		}
	})
	t.Run("put refreshes the ttl", func(t *testing.T) {
		clk := &fakeClock{t: start}
		s := mk(t, clk)
		_ = s.Put(ctx, "b1", "222.2")
		clk.Advance(20 * 24 * time.Hour)
		_ = s.Put(ctx, "b1", "222.2")
		clk.Advance(20 * 24 * time.Hour)
		if _, ok := mustGet(t, s, "b1"); !ok {
			t.Fatal("ttl not refreshed by Put")
		}
	})
}

func TestMappingStoreContract_Memory(t *testing.T) { // Scenarios: Latest consented client id wins; Mapping expires
	mappingContract(t, func(_ *testing.T, clk *fakeClock) MappingStore { return NewMemoryMappingStore(clk.Now) })
}

func TestMappingStoreContract_DuckDB(t *testing.T) { // Scenarios: Latest consented client id wins; Denied consent deletes the mapping; Mapping expires
	mappingContract(t, func(t *testing.T, clk *fakeClock) MappingStore {
		ctx := context.Background()
		w, err := duckdb.Open(ctx, filepath.Join(t.TempDir(), "wh.duckdb"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = w.Close() })
		s, err := NewSQLMappingStore(ctx, w.DB(), clk.Now)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestMappingStoreDuckDB_SurvivesReopen(t *testing.T) { // Scenario: Latest consented client id wins (durable)
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wh.duckdb")
	clk := &fakeClock{t: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	w1, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	s1, err := NewSQLMappingStore(ctx, w1.DB(), clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	_ = s1.Put(ctx, "b1", "222.2")
	_ = w1.Close()
	w2, err := duckdb.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer w2.Close()
	s2, err := NewSQLMappingStore(ctx, w2.DB(), clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := mustGet(t, s2, "b1"); !ok || id != "222.2" {
		t.Fatalf("after reopen got %q,%v", id, ok)
	}
}
