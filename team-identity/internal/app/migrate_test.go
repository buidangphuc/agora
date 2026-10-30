package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
)

func migrateSettings(url string) config.MigrateSettings {
	s := config.MigrateSettings{}
	s.Env = "local"
	s.Postgres.URL = url
	return s
}

func TestMigrateUpAppliesAllMigrations(t *testing.T) {
	// scenario: An empty database is migrated by the subcommand
	url := freshURL(t)
	if err := Migrate(context.Background(), migrateSettings(url)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if v, dirty := schemaVersion(t, url); v != latestVersion || dirty {
		t.Fatalf("schema_migrations = %d dirty=%v, want %d clean", v, dirty, latestVersion)
	}

	s := newSettings(t, url, deadBroker)
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := identityv1.NewAuthServiceClient(r.conn)
	if _, err := c.Register(ctx, &identityv1.RegisterRequest{Username: "alice", Password: testPassword, Role: "buyer"}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("Register = %v, want OK", err)
	}
	if res, err := c.Login(ctx, &identityv1.LoginRequest{Username: "alice", Password: testPassword}); err != nil || res.GetResult().GetToken() == "" {
		t.Fatalf("Login = %v, %v, want a token", res, err)
	}
}

func TestMigrateUpgradesVersionEightDatabase(t *testing.T) {
	// scenario: A database already at version 8 is upgraded without data loss
	url := freshURL(t)
	migrateTo(t, url, 8)
	for _, id := range []string{"u-1", "u-2", "u-3"} {
		execSQL(t, url, `INSERT INTO users (id, username, password_hash, roles) VALUES ($1, $1, 'hash-'||$1, '{buyer}')`, id)
	}
	execSQL(t, url, `INSERT INTO sessions (id, user_id, device, ip) VALUES ('s-1', 'u-1', 'phone', '10.0.0.1'), ('s-2', 'u-2', 'laptop', '10.0.0.2')`)
	execSQL(t, url, `INSERT INTO identity_outbox_events (event_id, aggregate_type, aggregate_id, event_type, payload, created_at) VALUES
		('ev-z', 'Session', 'u-1', 'platform.identity.v1.SessionRevoked', 'x', now() - interval '4 hours'),
		('ev-b', 'Session', 'u-1', 'platform.identity.v1.SessionRevoked', 'x', now() - interval '1 hour'),
		('ev-a', 'User', 'u-1', 'platform.identity.v1.UserStatusChanged', 'x', now() - interval '1 hour'),
		('ev-m', 'Session', 'u-2', 'platform.identity.v1.SessionRevoked', 'x', now() - interval '30 minutes')`)
	snap := func(table string) string {
		return queryString(t, url, `SELECT coalesce(json_agg(t ORDER BY t.id)::text, '') FROM `+table+` t`)
	}
	usersBefore, sessionsBefore := snap("users"), snap("sessions")

	if err := Migrate(context.Background(), migrateSettings(url)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if got := snap("users"); got != usersBefore {
		t.Errorf("users changed by the upgrade:\n before %s\n after  %s", usersBefore, got)
	}
	if got := snap("sessions"); got != sessionsBefore {
		t.Errorf("sessions changed by the upgrade:\n before %s\n after  %s", sessionsBefore, got)
	}
	if n := queryInt(t, url, `SELECT count(*) FROM identity_outbox_events WHERE status = 'pending'`); n != 4 {
		t.Errorf("pending outbox rows = %d, want 4", n)
	}
	if n := queryInt(t, url, `SELECT count(DISTINCT seq) FROM identity_outbox_events WHERE seq IS NOT NULL`); n != 4 {
		t.Errorf("distinct seq values = %d, want 4", n)
	}
	bySeq := queryString(t, url, `SELECT string_agg(event_id, ',' ORDER BY seq) FROM identity_outbox_events`)
	if want := "ev-z,ev-a,ev-b,ev-m"; bySeq != want {
		t.Errorf("rows by seq = %s, want %s (created_at, event_id order)", bySeq, want)
	}
	if v, dirty := schemaVersion(t, url); v != latestVersion || dirty {
		t.Errorf("schema_migrations = %d dirty=%v, want %d clean", v, dirty, latestVersion)
	}
}

func TestMigrateFailsWhenStateIsDirty(t *testing.T) {
	// scenario: A dirty migration state fails the subcommand
	// scenario: A failed migration leaves no half-started server
	url := freshURL(t)
	migrateTo(t, url, 8)
	execSQL(t, url, `UPDATE schema_migrations SET dirty = true`)
	grpcPort, jwksPort := testkit.FreePort(t), testkit.FreePort(t)

	err := Migrate(context.Background(), migrateSettings(url))
	if err == nil {
		t.Fatal("Migrate on a dirty database returned nil")
	}
	if !strings.Contains(err.Error(), "8") || !strings.Contains(strings.ToLower(err.Error()), "dirty") {
		t.Errorf("error %q does not name the dirty version 8", err)
	}
	if v, dirty := schemaVersion(t, url); v != 8 || !dirty {
		t.Errorf("schema_migrations = %d dirty=%v, want 8 dirty untouched", v, dirty)
	}
	if n := queryInt(t, url, `SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'identity_outbox_events' AND column_name = 'seq'`); n != 0 {
		t.Error("a migration was applied on a dirty database")
	}
	assertPortFree(t, grpcPort)
	assertPortFree(t, jwksPort)
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: The service never migrates at boot
	url := freshURL(t)
	r := start(t, newSettings(t, url, deadBroker))
	waitLive(t, r.conn, r.done)
	r.stop(t)

	for _, table := range []string{"schema_migrations", "users", "sessions", "identity_outbox_events"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}
