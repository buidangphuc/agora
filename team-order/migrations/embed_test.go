package migrations_test

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/buidangphuc/team-order/migrations"
)

// Scenario: All migrations apply and reverse
func TestEmbeddedFSHoldsEverySQLFile(t *testing.T) {
	onDisk, err := filepath.Glob("*.sql")
	if err != nil || len(onDisk) == 0 {
		t.Fatalf("glob: %v (n=%d)", err, len(onDisk))
	}
	sort.Strings(onDisk)
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		t.Fatalf("read embedded FS: %v", err)
	}
	var embedded []string
	for _, e := range entries {
		embedded = append(embedded, e.Name())
	}
	sort.Strings(embedded)
	if strings.Join(embedded, ",") != strings.Join(onDisk, ",") {
		t.Fatalf("embedded files %v, want every *.sql on disk %v", embedded, onDisk)
	}
}

// Scenario: All migrations apply and reverse
func TestOutboxUpgradeMigrationsExistAsUpAndDownPairs(t *testing.T) {
	for _, name := range []string{
		"0011_processed_events_consumer_pk",
		"0012_order_outbox_v1_prepare",
		"0013_order_outbox_v1_backfill",
		"0014_order_outbox_v1_switch",
		"0015_order_outbox_v1_seq_check",
		"0016_order_outbox_v1_seq_validate",
		"0017_order_outbox_v1_seq_not_null",
		"0018_order_outbox_v1_status_idx",
		"0019_order_outbox_v1_agg_idx",
		"0020_order_outbox_v1_pending_idx",
		"0021_order_outbox_v1_drop_claim_idx",
		"0022_order_outbox_v1_requeue_failed",
	} {
		for _, suffix := range []string{".up.sql", ".down.sql"} {
			if _, err := fs.Stat(migrations.FS, name+suffix); err != nil {
				t.Errorf("embedded FS lacks %s%s: %v", name, suffix, err)
			}
		}
	}
}
