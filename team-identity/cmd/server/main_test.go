package main_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

func runServer(t *testing.T, bin, url string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"),
		"ENV=local", "DATABASE_URL=" + url,
		"GRPC_PORT=" + strconv.Itoa(testkit.FreePort(t)),
	}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("server %v did not exit: it started a listener or hangs\n%s", args, out)
	}
	if err == nil {
		return string(out), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	}
	t.Fatalf("run server %v: %v", args, err)
	return "", -1
}

func schemaState(t *testing.T, url string) (version int, dirty, exists bool) {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close(context.Background())
	if err := conn.QueryRow(context.Background(), `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		return 0, false, false
	}
	if err := conn.QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	return version, dirty, true
}

func TestMigrateSubcommandDispatch(t *testing.T) {
	// scenario: The binary dispatches migrate up
	bin := buildServer(t)

	t.Run("migrate up", func(t *testing.T) {
		url := testkit.Schema(t, testkit.SharedPostgres(t))
		out, code := runServer(t, bin, url, "migrate", "up")
		if code != 0 {
			t.Fatalf("migrate up exit = %d, want 0\n%s", code, out)
		}
		if v, dirty, ok := schemaState(t, url); !ok || v != 19 || dirty {
			t.Fatalf("schema_migrations = %d dirty=%v exists=%v, want 19 clean", v, dirty, ok)
		}
	})

	for _, args := range [][]string{{"migrate"}, {"migrate", "down"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			url := testkit.Schema(t, testkit.SharedPostgres(t))
			out, code := runServer(t, bin, url, args...)
			if code == 0 {
				t.Fatalf("%v exit = 0, want non-zero\n%s", args, out)
			}
			if !strings.Contains(strings.ToLower(out), "usage") {
				t.Errorf("%v printed no usage line:\n%s", args, out)
			}
			if _, _, ok := schemaState(t, url); ok {
				t.Errorf("%v changed the database", args)
			}
		})
	}

	t.Run("migrate up on an unreachable database", func(t *testing.T) {
		if out, code := runServer(t, bin, closedDatabaseURL, "migrate", "up"); code == 0 {
			t.Fatalf("exit = 0, want non-zero\n%s", out)
		}
	})
}
