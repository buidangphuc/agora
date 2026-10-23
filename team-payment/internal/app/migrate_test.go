package app_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/app"
	"github.com/buidangphuc/team-payment/migrations"
)

var paymentTables = []string{"payment_transactions", "seller_wallets", "payout_requests", "wallet_transactions", "wallet_ledger", "payment_outbox_events"}

func schemaVersion(t *testing.T, url string) (int, bool) {
	t.Helper()
	var v int
	var dirty bool
	if err := dial(t, url).QueryRow(context.Background(), `SELECT version, dirty FROM schema_migrations`).Scan(&v, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return v, dirty
}

func embeddedMigrationCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrations.Files, "*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("embedded migrations: %v (n=%d)", err, len(files))
	}
	return len(files)
}

func TestMigrateUpAppliesEveryEmbeddedMigration(t *testing.T) {
	// scenario: The migrate subcommand applies every embedded migration
	url := freshURL(t)
	migrateEnv(t, url)

	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	for _, table := range paymentTables {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after migrate", table)
		}
	}
	v, dirty := schemaVersion(t, url)
	if want := embeddedMigrationCount(t); dirty || v != want {
		t.Fatalf("schema version = %d dirty=%v, want %d clean", v, dirty, want)
	}
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v2, d2 := schemaVersion(t, url); v2 != v || d2 {
		t.Errorf("version after rerun = %d dirty=%v, want %d clean", v2, d2, v)
	}
}

func TestMigrateUpNeedsOnlyEnvAndDatabaseURL(t *testing.T) {
	// scenario: The migrate subcommand applies every embedded migration
	url := freshURL(t)
	migrateEnv(t, url)
	if err := app.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate with only ENV and DATABASE_URL: %v", err)
	}
}

func TestMigrateUpFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: The migrate subcommand fails on an unreachable database
	migrateEnv(t, closedDatabaseURL)
	done := make(chan error, 1)
	go func() { done <- app.Migrate(context.Background()) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Migrate on an unreachable database returned nil")
		}
		if !strings.Contains(err.Error(), "127.0.0.1:1") {
			t.Errorf("error %q does not name the database host", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate did not fail fast on an unreachable database")
	}
}

func TestMigrateSubcommandRunsMigrationsAndFailsNonZero(t *testing.T) {
	// scenario: The migrate subcommand fails on an unreachable database
	bin := buildServer(t)

	url := freshURL(t)
	good := exec.Command(bin, "migrate", "up")
	good.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+url)
	if out, err := good.CombinedOutput(); err != nil {
		t.Fatalf("migrate up: %v\n%s", err, out)
	}
	for _, table := range paymentTables {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after `server migrate up`", table)
		}
	}

	bad := exec.Command(bin, "migrate", "up")
	bad.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+closedDatabaseURL)
	err := bad.Run()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() == 0 {
		t.Fatalf("migrate up on an unreachable database: %v, want a non-zero exit", err)
	}
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Booting the service never migrates
	url := freshURL(t)
	s := paySettings(testkit.FreePort(t), url, deadBroker)
	r := start(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := waitLiveness(ctx, r); err != nil {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("server not reachable: %v", err)
	}
	for _, table := range append([]string{"schema_migrations"}, paymentTables...) {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
	callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer callCancel()
	_, err := paymentv1.NewPaymentServiceClient(r.conn).GetPayment(asUser(callCtx, "buyer-1"),
		&paymentv1.GetPaymentRequest{Id: "00000000-0000-0000-0000-000000000001"}, grpc.WaitForReady(true))
	if st, ok := status.FromError(err); !ok || st.Code() != codes.Internal {
		t.Fatalf("GetPayment on unmigrated db = %v, want Internal", err)
	}
	if tableExists(t, url, "payment_transactions") {
		t.Error("an RPC created the payment_transactions table")
	}
}

func TestBootBinaryDoesNotMigrate(t *testing.T) {
	// scenario: Booting the service never migrates
	bin := buildServer(t)
	url := freshURL(t)
	bootBinary(t, bin, "DATABASE_URL="+url)
	for _, table := range append([]string{"schema_migrations"}, paymentTables...) {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}
