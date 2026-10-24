package app_test

import (
	"io/fs"
	"strconv"
	"strings"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-promotion/internal/app"
)

func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrationsFS(t), "*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("embedded migrations: %v (n=%d)", err, len(files))
	}
	return len(files)
}

func TestMigrateUpMigratesEmptyDatabase(t *testing.T) {
	// scenario: migrate up applies migrations to an empty database
	url := freshURL(t)
	migrateEnv(t, url)
	if err := app.Execute(t.Context(), []string{"migrate", "up"}); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	for _, table := range promotionTables {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after migrate up", table)
		}
	}
	v, dirty := schemaVersion(t, url)
	if dirty || v != 5 || v != embeddedMigrationCount(t) {
		t.Fatalf("schema version = %d dirty=%v, want 5 clean", v, dirty)
	}
}

func TestMigrateUpTwiceIsNoop(t *testing.T) {
	// scenario: migrate up twice is a no-op
	url := freshURL(t)
	migrateEnv(t, url)
	for i := 1; i <= 2; i++ {
		if err := app.Execute(t.Context(), []string{"migrate", "up"}); err != nil {
			t.Fatalf("migrate up run %d: %v", i, err)
		}
	}
	if v, dirty := schemaVersion(t, url); dirty || v != 5 {
		t.Fatalf("schema version = %d dirty=%v, want 5 clean", v, dirty)
	}
}

func TestMigrateUpFailsOnDirtyState(t *testing.T) {
	// scenario: A dirty migration state fails migrate up
	url := freshURL(t)
	if err := migrate.Up(t.Context(), url, migrationsFS(t)); err != nil {
		t.Fatalf("seed migrate: %v", err)
	}
	if _, err := dial(t, url).Exec(t.Context(), `UPDATE schema_migrations SET version = 3, dirty = true`); err != nil {
		t.Fatalf("mark dirty: %v", err)
	}
	migrateEnv(t, url)
	err := app.Execute(t.Context(), []string{"migrate", "up"})
	if err == nil {
		t.Fatal("migrate up on a dirty database returned nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "dirty") {
		t.Fatalf("migrate up error %q does not report the dirty state", err)
	}
	if v, dirty := schemaVersion(t, url); v != 3 || !dirty {
		t.Fatalf("schema_migrations = %d dirty=%v, want unchanged 3 dirty", v, dirty)
	}
}

func TestExecuteRejectsUnknownSubcommand(t *testing.T) {
	// scenario: An unknown subcommand is refused
	url := freshURL(t)
	port := testkit.FreePort(t)
	migrateEnv(t, url)
	t.Setenv("GRPC_PORT", strconv.Itoa(port))
	t.Setenv("UPSTREAM_DOMAIN_ADDR", deadBroker)
	t.Setenv("UPSTREAM_AUDIT_ADDR", deadBroker)
	for _, args := range [][]string{{"migrate", "down"}, {"serve"}} {
		err := app.Execute(t.Context(), args)
		if err == nil {
			t.Fatalf("Execute(%v) returned nil, want an error", args)
		}
		if !strings.Contains(err.Error(), args[len(args)-1]) {
			t.Errorf("Execute(%v) error %q does not name the subcommand", args, err)
		}
		assertPortFree(t, port)
		if tableExists(t, url, "schema_migrations") {
			t.Errorf("Execute(%v) ran a migration", args)
		}
	}
}
