package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

func buildServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

type result struct {
	code           int
	stdout, stderr string
}

func runServer(t *testing.T, bin string, env []string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}, env...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("server %v did not exit within 15s", args)
	}
	code := 0
	if err != nil {
		ee := &exec.ExitError{}
		ok := errors.As(err, &ee)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		code = ee.ExitCode()
	}
	return result{code, so.String(), se.String()}
}

func tableSet(t *testing.T, url string) map[string]bool {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close(context.Background()) }()
	rows, err := c.Query(context.Background(), `SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[n] = true
	}
	return out
}

func TestMigrateUpAppliesEmbeddedMigrations(t *testing.T) {
	// scenario: migrate up applies the embedded migrations and exits
	bin := buildServer(t)
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	env := []string{"ENV=local", "DATABASE_URL=" + url}

	r := runServer(t, bin, env, "migrate", "up")
	if r.code != 0 {
		t.Fatalf("migrate up exit = %d, stderr: %s", r.code, r.stderr)
	}
	first := tableSet(t, url)
	for _, table := range []string{
		"cart_items", "orders", "order_items", "order_returns", "shipments", "shipment_checkpoints",
		"order_outbox_events", "order_sagas", "order_reservations", "processed_events",
	} {
		if !first[table] {
			t.Errorf("table %s missing after migrate up", table)
		}
	}

	r = runServer(t, bin, env, "migrate", "up")
	if r.code != 0 {
		t.Fatalf("second migrate up exit = %d, stderr: %s", r.code, r.stderr)
	}
	second := tableSet(t, url)
	if len(second) != len(first) {
		t.Errorf("second run changed the table set: %d -> %d", len(first), len(second))
	}
}

func TestMigrateUpFailsWhenPostgresUnreachable(t *testing.T) {
	// scenario: migrate up against an unreachable Postgres fails
	bin := buildServer(t)
	r := runServer(t, bin, []string{"ENV=local", "DATABASE_URL=" + closedDatabaseURL}, "migrate", "up")
	if r.code == 0 {
		t.Fatal("migrate up exited 0 against an unreachable database")
	}
	low := strings.ToLower(r.stderr)
	if !strings.Contains(low, "database") && !strings.Contains(low, "postgres") && !strings.Contains(low, "127.0.0.1:1") {
		t.Fatalf("stderr does not name the database: %q", r.stderr)
	}
}

func TestUnknownSubcommandRefused(t *testing.T) {
	// scenario: An unknown subcommand is refused
	bin := buildServer(t)
	r := runServer(t, bin, []string{"ENV=local", "DATABASE_URL=" + closedDatabaseURL}, "migrate", "down")
	if r.code != 2 {
		t.Fatalf("exit = %d, want 2; stderr: %s", r.code, r.stderr)
	}
	out := r.stderr + r.stdout
	if !strings.Contains(strings.ToLower(out), "usage") || !strings.Contains(out, "migrate up") {
		t.Errorf("no usage message naming `migrate up`: %q", out)
	}
	if strings.Contains(out, "127.0.0.1:1") {
		t.Errorf("the refusal touched the database: %q", out)
	}
}
