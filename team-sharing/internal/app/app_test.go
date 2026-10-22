package app_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/app"
	"github.com/buidangphuc/team-sharing/internal/config"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
	"github.com/buidangphuc/team-sharing/migrations"
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

func migratedSchema(t *testing.T) string {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
}

func start(t *testing.T, s config.Settings, run func(context.Context, config.Settings) error) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), port: s.Port}
	go func() { r.err = run(ctx, s); close(r.done) }()

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

func startApp(t *testing.T, s config.Settings) *running { return start(t, s, app.Run) }

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
		t.Fatal("app did not stop")
	}
}

func principal(ctx context.Context, id, typ string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-principal-id", id, "x-principal-type", typ)
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

func TestBootFailsWhenDatabaseUnreachableInLocal(t *testing.T) {
	// scenario: Unreachable database fails boot locally
	assertBootFails(t, "local")
}

func TestBootFailsWhenDatabaseUnreachableOutsideLocal(t *testing.T) {
	// scenario: Unreachable database fails boot outside local
	assertBootFails(t, "staging")
}

func TestNoInMemoryStoreOrFallbackInProductionSource(t *testing.T) {
	// scenario: Production source has no in-memory store or local fallback
	root := serviceRoot(t)
	banned := []string{"InMemory", "OpenOrLocal", "DATABASE_ENABLED"}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "generated" || d.Name() == "third_party" || d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, w := range banned {
			if bytes.Contains(b, []byte(w)) {
				t.Errorf("%s contains %q", strings.TrimPrefix(path, root+string(filepath.Separator)), w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMainOnlyWiresConfigAndRun(t *testing.T) {
	// scenario: The service consumes the tagged SDK and the shared build files (main shape)
	b, err := os.ReadFile(filepath.Join(serviceRoot(t), "cmd", "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, want := range []string{"config.Load", "app.Run", "signal.NotifyContext"} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %s", want)
		}
	}
	for _, bad := range []string{"pgxpool", "internal/repository", "internal/service", "internal/handler", "internal/grpcserver"} {
		if strings.Contains(src, bad) {
			t.Errorf("main.go still references %s", bad)
		}
	}
	if n := strings.Count(src, "\n"); n > 40 {
		t.Errorf("main.go has %d lines, want the short ADR-0016 shape", n)
	}
}

func serviceRootDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func serviceRoot(t *testing.T) string {
	t.Helper()
	return serviceRootDir()
}

func TestHealthReportsDatabaseDown(t *testing.T) {
	// scenario: Database loss flips health to NOT_SERVING
	url := testkit.Postgres(t)
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)

	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING while the database is down", got)
	}
}

func TestHealthRecoversWhenDatabaseReturns(t *testing.T) {
	// scenario: Health recovers when the database returns
	url := testkit.Postgres(t)
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})

	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
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

func TestBootDoesNotApplyMigrations(t *testing.T) {
	// scenario: Boot does not apply migrations
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	for _, table := range []string{"share_links", "schema_migrations"} {
		var reg *string
		if err := conn.QueryRow(ctx, "SELECT to_regclass($1)::text", table).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg != nil {
			t.Errorf("table %s exists after boot; migrations must not run at boot", table)
		}
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	// scenario: Shutdown stops the server within the grace period
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	r.cancel()
	select {
	case <-r.done:
		if r.err != nil {
			t.Fatalf("Run = %v, want nil", r.err)
		}
	case <-time.After(5*time.Second + 2*time.Second):
		t.Fatal("Run did not return within SHUTDOWN_GRACE")
	}
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(r.port), 2*time.Second); err == nil {
		_ = c.Close()
		t.Fatal("listener still accepts connections after Run returned")
	}
}

type panicRepo struct{ good *sharelink.ShareLink }

func (p *panicRepo) Create(context.Context, *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	return nil, errors.New("unused")
}

func (p *panicRepo) Resolve(_ context.Context, code string) (*sharelink.ShareLink, error) {
	if code == "boom" {
		panic("store exploded")
	}
	return p.good, nil
}

func TestPanicIsContained(t *testing.T) {
	// scenario: A panicking handler returns Internal and the server keeps serving
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	repo := &panicRepo{good: &sharelink.ShareLink{ShortCode: "good123", TargetType: "listing", TargetID: "l1", ClickCount: 1}}
	r := start(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}), func(ctx context.Context, s config.Settings) error {
		return app.RunWithRepository(ctx, s, repo)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := sharingv1.NewSharingServiceClient(r.conn)

	_, err := client.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{ShortCode: "boom"}, grpc.WaitForReady(true))
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "internal error" {
		t.Fatalf("panicking call err = %v, want Internal \"internal error\"", err)
	}
	res, err := client.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{ShortCode: "good123"})
	if err != nil || res.GetTargetId() != "l1" {
		t.Fatalf("call after panic = %+v, %v", res, err)
	}
}

type logCapture struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
	w    *os.File
	old  *os.File
}

func captureStdout(t *testing.T) *logCapture {
	t.Helper()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	c := &logCapture{done: make(chan struct{}), w: wr, old: os.Stdout}
	os.Stdout = wr
	go func() {
		_, _ = io.Copy(&lockedWriter{c}, rd)
		close(c.done)
	}()
	t.Cleanup(func() { c.finish() })
	return c
}

type lockedWriter struct{ c *logCapture }

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.c.mu.Lock()
	defer l.c.mu.Unlock()
	return l.c.buf.Write(p)
}

func (c *logCapture) finish() {
	if c.w == nil {
		return
	}
	os.Stdout = c.old
	_ = c.w.Close()
	<-c.done
	c.w = nil
}

func (c *logCapture) records(t *testing.T) (raw string, recs []map[string]any) {
	t.Helper()
	c.finish()
	c.mu.Lock()
	defer c.mu.Unlock()
	raw = c.buf.String()
	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			recs = append(recs, m)
		}
	}
	return raw, recs
}

func loggedSettings(port int, url string) config.Settings {
	s := settings(port, "local", sdkconfig.Postgres{URL: url})
	s.LogLevel = "info"
	return s
}

func recordsFor(recs []map[string]any, method string) []map[string]any {
	var out []map[string]any
	for _, m := range recs {
		b, _ := json.Marshal(m)
		if strings.Contains(string(b), method) {
			out = append(out, m)
		}
	}
	return out
}

func TestCallLogsCarryNoLinkData(t *testing.T) {
	// scenario: Call logs carry no link data
	url := migratedSchema(t)
	logs := captureStdout(t)
	r := startApp(t, loggedSettings(testkit.FreePort(t), url))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := sharingv1.NewSharingServiceClient(r.conn).CreateShareLink(principal(ctx, "user-1", "user"),
		&sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "listing-8f3a9c", Utm: map[string]string{"campaign": "Secret title"}},
		grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	r.stop(t)

	raw, recs := logs.records(t)
	named := recordsFor(recs, "CreateShareLink")
	if len(named) == 0 {
		t.Fatalf("no log record names CreateShareLink; output:\n%s", raw)
	}
	found := false
	for _, m := range named {
		if m["principal_id"] == "user-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("no CreateShareLink record carries principal_id user-1: %v", named)
	}
	for _, secret := range []string{"listing-8f3a9c", "Secret title", res.GetShortCode()} {
		if secret != "" && strings.Contains(raw, secret) {
			t.Errorf("log output contains %q", secret)
		}
	}
}

func TestAnonymousCallIsLoggedAndServed(t *testing.T) {
	// scenario: An anonymous call is logged without a principal id
	url := migratedSchema(t)
	logs := captureStdout(t)
	r := startApp(t, loggedSettings(testkit.FreePort(t), url))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := sharingv1.NewSharingServiceClient(r.conn)
	created, err := client.CreateShareLink(ctx, &sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "l1"}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	anon := principal(ctx, "anonymous", "anonymous")
	if _, err := client.ResolveShareLink(anon, &sharingv1.ResolveShareLinkRequest{ShortCode: created.GetShortCode()}); err != nil {
		t.Fatalf("anonymous ResolveShareLink: %v", err)
	}
	r.stop(t)

	raw, recs := logs.records(t)
	named := recordsFor(recs, "ResolveShareLink")
	if len(named) == 0 {
		t.Fatalf("no log record names ResolveShareLink; output:\n%s", raw)
	}
	for _, m := range named {
		if id, ok := m["principal_id"]; ok && id != "" {
			t.Errorf("anonymous call logged with principal_id %v: %v", id, m)
		}
	}
	b, _ := json.Marshal(named)
	if !strings.Contains(string(b), "OK") {
		t.Errorf("no record carries the OK code: %s", b)
	}
}

func listServices(t *testing.T, r *running) ([]string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}}); err != nil {
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, s := range resp.GetListServicesResponse().GetService() {
		names = append(names, s.GetName())
	}
	return names, nil
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Reflection is off by default
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	r := startApp(t, settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}))
	waitServing(t, r)

	_, err := listServices(t, r)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}

func TestReflectionEnabledLocally(t *testing.T) {
	// scenario: Reflection works when enabled locally
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url})
	s.ReflectionEnabled = true
	r := startApp(t, s)

	names, err := listServices(t, r)
	if err != nil {
		t.Fatalf("reflection: %v", err)
	}
	for _, n := range names {
		if n == "platform.sharing.v1.SharingService" {
			return
		}
	}
	t.Fatalf("services %v lack platform.sharing.v1.SharingService", names)
}

func TestBootWithoutCollectorWhenOTelOff(t *testing.T) {
	// scenario: Boot with telemetry off and no collector
	url := migratedSchema(t)
	s := settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url})
	s.OTel = sdkconfig.OTel{Enabled: false, Endpoint: "127.0.0.1:1", ServiceName: "team-sharing"}
	r := startApp(t, s)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := sharingv1.NewSharingServiceClient(r.conn).CreateShareLink(ctx,
		&sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "l1"}, grpc.WaitForReady(true)); err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	r.stop(t)
	if r.err != nil {
		t.Fatalf("Run = %v, want nil", r.err)
	}
}
