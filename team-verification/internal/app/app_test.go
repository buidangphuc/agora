package app_test

import (
	"context"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	auditv1 "github.com/buidangphuc/team-verification/generated/platform/audit/v1"
	"github.com/buidangphuc/team-verification/internal/app"
	"github.com/buidangphuc/team-verification/internal/config"
	"github.com/buidangphuc/team-verification/migrations"
)

const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

const healthWindow = healthx.DefaultInterval + 1*time.Second

type fakeAudit struct {
	auditv1.UnimplementedAuditServiceServer
}

func (fakeAudit) WriteAuditEvent(context.Context, *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	return &auditv1.WriteAuditEventResponse{}, nil
}

func startAudit(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	auditv1.RegisterAuditServiceServer(srv, fakeAudit{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func settings(t *testing.T, port int, dbURL string) config.Settings {
	t.Helper()
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		Listener: config.Listener{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: dbURL},
		Audit:    config.Audit{Addr: startAudit(t)},
	}
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	port   int
}

func startApp(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), port: s.Port}
	go func() { r.err = app.Run(ctx, s); close(r.done) }()

	var err error
	r.conn, err = grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = r.conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(15 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("health = %v, %v, want SERVING", resp.GetStatus(), err)
	}
}

func healthStatus(r *running, service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN
	}
	return resp.GetStatus()
}

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func runMustFail(t *testing.T, s config.Settings, within time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), within+5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		return err
	case <-time.After(within):
		t.Fatalf("Run did not fail within %s; it is serving or hanging", within)
	}
	return nil
}

func tableExists(t *testing.T, url, table string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	var reg *string
	if err := conn.QueryRow(ctx, "SELECT to_regclass($1)::text", table).Scan(&reg); err != nil {
		t.Fatal(err)
	}
	return reg != nil
}

func serviceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func TestMigrateUpSubcommandAppliesSchema(t *testing.T) {
	// scenario: Migrate up subcommand applies schema
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	root := serviceRoot(t)
	bin := filepath.Join(t.TempDir(), "server")

	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		cmd := exec.CommandContext(ctx, bin, "migrate", "up")
		cmd.Env = []string{"DATABASE_URL=" + url}
		out, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("migrate up run %d: %v\n%s", i+1, err, out)
		}
	}
	if !tableExists(t, url, "kyc_submissions") {
		t.Error("kyc_submissions missing after migrate up")
	}
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Run does not migrate
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(t, testkit.FreePort(t), url))
	waitServing(t, r)

	for _, table := range []string{"kyc_submissions", "schema_migrations"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; Run must not migrate", table)
		}
	}
}

func TestRunAfterMigrateServes(t *testing.T) {
	// scenario: Migrate up subcommand applies schema (library path used by the subcommand)
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := startApp(t, settings(t, testkit.FreePort(t), url))
	waitServing(t, r)
}

func TestRunRefusesEmptyAuditAddr(t *testing.T) {
	// scenario: Run refuses empty audit addr; Empty audit address yields no client
	for _, addr := range []string{"", "   "} {
		port := testkit.FreePort(t)
		s := settings(t, port, closedDatabaseURL)
		s.Audit = config.Audit{Addr: addr}
		err := runMustFail(t, s, 10*time.Second)
		if !strings.Contains(err.Error(), "UPSTREAM_AUDIT_ADDR") {
			t.Errorf("addr %q: error %q must name UPSTREAM_AUDIT_ADDR (checked before opening the database)", addr, err)
		}
		assertPortFree(t, port)
	}
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: An unreachable database fails boot locally
	port := testkit.FreePort(t)
	s := settings(t, port, closedDatabaseURL)
	runMustFail(t, s, 10*time.Second)
	assertPortFree(t, port)
}

func TestUnknownMockAdapterFailsBoot(t *testing.T) {
	// scenario: Unknown mock adapter fails boot
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	port := testkit.FreePort(t)
	s := settings(t, port, url)
	s.MockData = sdkconfig.MockData{Adapters: "kyc"}
	err := runMustFail(t, s, 10*time.Second)
	if !strings.Contains(err.Error(), "MOCK_DATA") || !strings.Contains(err.Error(), "kyc") {
		t.Errorf("error %q must name MOCK_DATA and the adapter kyc", err)
	}
	assertPortFree(t, port)
}

func TestHealthServingWhenDatabaseUp(t *testing.T) {
	// scenario: Health serving when database up
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(t, testkit.FreePort(t), url))
	waitServing(t, r)
	testkit.Eventually(t, healthWindow, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_SERVING &&
			healthStatus(r, "liveness") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func setDatabaseAvailable(t *testing.T, url string, available bool) {
	t.Helper()
	admin, err := pgx.Connect(context.Background(), strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	qs := []string{"ALTER DATABASE test ALLOW_CONNECTIONS " + strconv.FormatBool(available)}
	if !available {
		qs = append(qs, "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'")
	}
	for _, q := range qs {
		if _, err := admin.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

func TestReadinessFollowsDatabaseLivenessIndependent(t *testing.T) {
	// scenario: Readiness follows database, liveness independent
	url := testkit.Postgres(t)
	r := startApp(t, settings(t, testkit.FreePort(t), url))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)

	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING while the database is down", got)
	}
}

func TestRunShutsDownWithinGrace(t *testing.T) {
	// scenario: Run shuts down within grace
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	s := settings(t, testkit.FreePort(t), url)
	s.ShutdownGrace = 2 * time.Second
	r := startApp(t, s)
	waitServing(t, r)

	r.cancel()
	select {
	case <-r.done:
		if r.err != nil {
			t.Fatalf("Run = %v, want nil", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within SHUTDOWN_GRACE")
	}
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(r.port), 2*time.Second); err == nil {
		_ = c.Close()
		t.Fatal("listener still accepts connections after Run returned")
	}
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Reflection off by default
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(t, testkit.FreePort(t), url))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err == nil {
		_ = stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}})
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}

func TestRuntimeDoesNotImportStoreFake(t *testing.T) {
	// scenario: The runtime binary does not import the test fake
	cmd := exec.Command("go", "list", "-deps", "./cmd/server")
	cmd.Dir = serviceRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/server: %v", err)
	}
	deps := string(out)
	if !strings.Contains(deps, "team-verification/internal/app") {
		t.Fatal("cmd/server does not depend on internal/app")
	}
	for _, d := range strings.Fields(deps) {
		if strings.HasSuffix(d, "/storetest") || strings.Contains(d, "/storetest/") {
			t.Errorf("runtime binary imports %s", d)
		}
	}
}
