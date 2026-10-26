package app_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-domain/internal/app"
	"github.com/buidangphuc/team-domain/migrations"
)

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(t.Context(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("embedded migrations: %v (n=%d)", err, len(files))
	}
	return len(files)
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Domain SDK: Boot does not migrate
	url := freshURL(t)
	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	for _, table := range []string{"schema_migrations", "outbox_events", "listings", "reservations", "processed_events"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}

func TestBootBinaryDoesNotMigrate(t *testing.T) {
	// scenario: Domain SDK: Boot does not migrate
	bin := buildServer(t)
	url := freshURL(t)
	bootBinary(t, bin, "DATABASE_URL="+url)
	for _, table := range []string{"schema_migrations", "outbox_events", "listings"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}

func TestMigrateUpSubcommandAppliesSchema(t *testing.T) {
	// scenario: Domain SDK: The migrate subcommand applies the schema
	bin := buildServer(t)
	url := freshURL(t)

	cmd := exec.CommandContext(t.Context(), bin, "migrate", "up")
	cmd.Env = []string{"ENV=local", "DATABASE_URL=" + url, "PATH=" + os.Getenv("PATH")}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("server migrate up with only ENV and DATABASE_URL: %v\n%s", err, out)
	}
	want := embeddedMigrationCount(t)
	if want != 16 {
		t.Fatalf("embedded migrations = %d, want 16", want)
	}
	if v, dirty := schemaVersion(t, url); dirty || v != want {
		t.Fatalf("schema version = %d dirty=%v, want %d clean", v, dirty, want)
	}
	for _, table := range []string{"listings", "outbox_events", "reservations"} {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after `server migrate up`", table)
		}
	}
}

func TestMigrateFailsWithoutDatabase(t *testing.T) {
	// scenario: Domain SDK: Migrate without a database fails
	migrateEnv(t, closedDatabaseURL)
	done := make(chan error, 1)
	go func() { done <- app.Migrate(t.Context()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Migrate on an unreachable database returned nil")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate did not fail fast on an unreachable database")
	}

	bin := buildServer(t)
	cmd := exec.CommandContext(t.Context(), bin, "migrate", "up")
	cmd.Env = []string{"ENV=local", "DATABASE_URL=" + closedDatabaseURL, "PATH=" + os.Getenv("PATH")}
	err := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() == 0 {
		t.Fatalf("migrate up on an unreachable database: %v, want a non-zero exit", err)
	}
}

func TestMigrateInProcessAppliesSchemaAndIsIdempotent(t *testing.T) {
	// scenario: Domain SDK: The migrate subcommand applies the schema
	url := freshURL(t)
	migrateEnv(t, url)
	if err := app.Migrate(t.Context()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	want := embeddedMigrationCount(t)
	if v, dirty := schemaVersion(t, url); dirty || v != want {
		t.Fatalf("schema version = %d dirty=%v, want %d clean", v, dirty, want)
	}
	if err := app.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v, dirty := schemaVersion(t, url); dirty || v != want {
		t.Fatalf("schema version after rerun = %d dirty=%v, want %d clean", v, dirty, want)
	}
}
