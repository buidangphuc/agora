package warehouse_test

import (
	"testing"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

// Scenario: AUE-13 The tombstone holds no clear id (schema: user_hmac, pseudonym, erased_at; no user id column)
func TestErasedUsersSchemaHasNoClearIDColumn(t *testing.T) {
	if warehouse.ErasedUsersTableName != "erased_users" {
		t.Errorf("ErasedUsersTableName = %q", warehouse.ErasedUsersTableName)
	}
	var names []string
	for _, c := range warehouse.ErasedUsersSchema {
		names = append(names, c.Name)
		if c.DuckDBType == "" || c.BigQueryType == "" {
			t.Errorf("column %q has an empty type", c.Name)
		}
	}
	want := []string{"user_hmac", "pseudonym", "erased_at"}
	if len(names) != len(want) {
		t.Fatalf("ErasedUsersSchema columns = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("column %d = %q, want %q", i, names[i], want[i])
		}
	}
	if c := warehouse.ErasedUsersSchema[2]; c.DuckDBType != "TIMESTAMP" {
		t.Errorf("erased_at DuckDB type = %q, want TIMESTAMP", c.DuckDBType)
	}
}

// Scenario: AUE-12 Pseudonym format constant
func TestErasedPseudonymPrefix(t *testing.T) {
	if warehouse.PseudonymPrefix != "erased:" {
		t.Errorf("PseudonymPrefix = %q, want erased:", warehouse.PseudonymPrefix)
	}
}
