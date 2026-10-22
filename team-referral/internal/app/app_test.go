package app_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/app"
	"github.com/buidangphuc/team-referral/internal/config"
	"github.com/buidangphuc/team-referral/migrations"
)

const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

const healthWindow = healthx.DefaultInterval + 1*time.Second

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	port   int
}

func settings(port int, env string, db sdkconfig.Postgres) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: env, LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: db,
	}
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

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		t.Fatal("app did not stop")
	}
}

func principal(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-principal-id", id, "x-principal-type", "user")
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

func assertBootFails(t *testing.T, env string) {
	t.Helper()
	port := testkit.FreePort(t)
	s := settings(port, env, sdkconfig.Postgres{URL: closedDatabaseURL})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Run returned nil with an unreachable database in ENV=%s", env)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("Run did not fail in ENV=%s; it is serving or hanging without its database", env)
	}

	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func TestBootFailsWhenDatabaseUnreachableEvenLocally(t *testing.T) {
	// scenario: Unreachable database refuses to boot in the local environment
	assertBootFails(t, "local")
}

func TestBootFailsWhenDatabaseUnreachableOutsideLocal(t *testing.T) {
	// scenario: Unreachable database refuses to boot outside local
	assertBootFails(t, "production")
}

func TestServesFromPostgresAcrossRestart(t *testing.T) {
	// scenario: A reachable database serves from Postgres
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	ctx = principal(ctx, "alice")

	first := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	created, err := referralv1.NewReferralServiceClient(first.conn).CreateReferralCode(ctx, &referralv1.CreateReferralCodeRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("CreateReferralCode: %v", err)
	}
	if created.GetCode() == "" {
		t.Fatal("empty referral code")
	}
	first.stop(t)

	second := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	mine, err := referralv1.NewReferralServiceClient(second.conn).GetMyReferral(ctx, &referralv1.GetMyReferralRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("GetMyReferral after restart: %v", err)
	}
	if mine.GetCode() != created.GetCode() {
		t.Fatalf("code after restart = %q, want %q", mine.GetCode(), created.GetCode())
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

func adminConn(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	admin, err := pgx.Connect(context.Background(), strings.Replace(url, "/test?", "/postgres?", 1))
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	return admin
}

func TestHealthReportsDatabaseDown(t *testing.T) {
	// scenario: Health turns NOT_SERVING when the database is lost
	url := testkit.Postgres(t)
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	admin := adminConn(t, url)
	for _, q := range []string{
		"ALTER DATABASE test ALLOW_CONNECTIONS false",
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'test'",
	} {
		if _, err := admin.Exec(context.Background(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING while the database is down", got)
	}
}

func TestEarlyFailureClosesPool(t *testing.T) {
	// scenario: A failed start releases what was opened
	url := testkit.Postgres(t)
	port := testkit.FreePort(t)
	blocker, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, settings(port, "local", sdkconfig.Postgres{URL: url})) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil although the gRPC port was already bound")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail with the gRPC port already bound")
	}

	admin := adminConn(t, url)
	testkit.Eventually(t, 10*time.Second, func() bool {
		var n int
		if err := admin.QueryRow(context.Background(), "SELECT count(*) FROM pg_stat_activity WHERE datname = 'test'").Scan(&n); err != nil {
			return false
		}
		return n == 0
	})

	_ = blocker.Close()
	r := startApp(t, settings(port, "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)
}

func TestShutdownWithinGrace(t *testing.T) {
	// scenario: Context cancellation stops the server within the grace period
	url := testkit.Postgres(t)
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url})
	s.ShutdownGrace = 2 * time.Second
	r := startApp(t, s)
	waitServing(t, r)

	watchCtx, stopWatch := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopWatch()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(watchCtx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("watch first: %v", err)
	}

	r.cancel()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within 3s of cancellation with SHUTDOWN_GRACE=2s")
	}
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(r.port), 2*time.Second); err == nil {
		_ = c.Close()
		t.Fatal("listener still accepts connections after Run returned")
	}
}
