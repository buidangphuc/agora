package app_test

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-engagement/generated/platform/listing/v1"
	"github.com/buidangphuc/team-engagement/internal/app"
	"github.com/buidangphuc/team-engagement/internal/config"
	"github.com/buidangphuc/team-engagement/migrations"
)

const (
	closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"
	deadBroker        = "127.0.0.1:1"
	healthWindow      = healthx.DefaultInterval + 1*time.Second
	retentionDays     = 730
)

func serviceRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func freshURL(t *testing.T) string {
	t.Helper()
	return testkit.Schema(t, testkit.SharedPostgres(t))
}

func migratedURL(t *testing.T) string {
	t.Helper()
	url := freshURL(t)
	if err := migrate.Up(context.Background(), url, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
}

func migrateSettings(url string) config.MigrateSettings {
	return config.MigrateSettings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 2},
	}
}

func engSettings(port int, url, brokers string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url, MaxConns: 5},
		Kafka:    sdkconfig.Kafka{Brokers: brokers},
		Outbox:   sdkconfig.Outbox{PollInterval: 200 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		LogKeyFP: sdkconfig.LogKeyFP{Secret: "test-log-key-fp-secret"},
		Events: config.Events{
			ListingTopic:  "listing.events",
			ConsumerGroup: "team-engagement-feed",
			EventsTopic:   "engagement.events",
		},
		Upstream: config.Upstream{OrderAddr: "127.0.0.1:1", AuditAddr: "127.0.0.1:1"},
		Reports:  config.Reports{RetentionDays: retentionDays, SweepInterval: time.Hour},
	}
}

func dial(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	c, err := pgx.Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

func tableExists(t *testing.T, url, table string) bool {
	t.Helper()
	var ok bool
	err := dial(t, url).QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&ok)
	if err != nil {
		t.Fatalf("query table %s: %v", table, err)
	}
	return ok
}

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
	files, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("embedded migrations: %v (n=%d)", err, len(files))
	}
	return len(files)
}

var engagementTables = []string{
	"checkins", "collection_items", "collections", "disputes", "engagement_outbox_events", "favorites", "follows",
	"listing_stats", "loyalty_accounts", "outbox", "product_answers", "product_questions", "reports",
	"review_helpful_votes", "reviews", "seller_listings", "view_history",
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func start(t *testing.T, s config.Settings, o app.Options) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{})}
	go func() { r.err = app.RunWithOptions(ctx, s, o); close(r.done) }()
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
		case <-time.After(30 * time.Second):
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
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
}

func waitLiveness(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true)); err != nil {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
		t.Fatalf("server not reachable: %v", err)
	}
}

func waitServing(t *testing.T, r *running) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{}, grpc.WaitForReady(true))
	if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		select {
		case <-r.done:
			t.Fatalf("app exited before serving: %v", r.err)
		default:
		}
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

func asBuyer(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx,
		"x-principal-id", id, "x-principal-type", "user", "x-principal-scopes", "engagement:write,engagement:read")
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

func assertPortFree(t *testing.T, port int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

func buildServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "./cmd/server")
	build.Dir = serviceRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", err, out)
	}
	return bin
}

type failingPublisher struct {
	mu     sync.Mutex
	failOn string
	topics []string
}

func (p *failingPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.mu.Lock()
	p.topics = append(p.topics, r.Topic)
	p.mu.Unlock()
	if p.failOn == "" || strings.HasSuffix(r.Topic, p.failOn) {
		return errors.New("publish refused by test")
	}
	return nil
}

func downPublisher(sdkconfig.Kafka) (outbox.Publisher, error) { return &failingPublisher{}, nil }

func TestMigrateUpAppliesEmbeddedMigrations(t *testing.T) {
	// scenario: Engagement SDK: migrate up applies the embedded migrations
	url := freshURL(t)
	if err := app.Migrate(context.Background(), migrateSettings(url)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	for _, table := range engagementTables {
		if !tableExists(t, url, table) {
			t.Errorf("table %s missing after migrate", table)
		}
	}
	v, dirty := schemaVersion(t, url)
	if want := embeddedMigrationCount(t); dirty || v != want || v != 14 {
		t.Fatalf("schema version = %d dirty=%v, want 14 clean (embedded files: %d)", v, dirty, want)
	}
}

func TestMigrateUpTwiceIsNoOp(t *testing.T) {
	// scenario: Engagement SDK: migrate up with nothing pending succeeds
	url := freshURL(t)
	ms := migrateSettings(url)
	if err := app.Migrate(context.Background(), ms); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := app.Migrate(context.Background(), ms); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if v, dirty := schemaVersion(t, url); dirty || v != 14 {
		t.Fatalf("version after rerun = %d dirty=%v, want 14 clean", v, dirty)
	}
}

func TestMigrateUpUnreachableDatabaseFails(t *testing.T) {
	// scenario: Engagement SDK: migrate up fails loudly
	done := make(chan error, 1)
	go func() { done <- app.Migrate(context.Background(), migrateSettings(closedDatabaseURL)) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Migrate on an unreachable database returned nil")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Migrate did not fail fast on an unreachable database")
	}

	bin := buildServer(t)
	url := freshURL(t)
	good := exec.Command(bin, "migrate", "up")
	good.Env = append(os.Environ(), "ENV=local", "DATABASE_URL="+url)
	if out, err := good.CombinedOutput(); err != nil {
		t.Fatalf("migrate up: %v\n%s", err, out)
	}
	if !tableExists(t, url, "engagement_outbox_events") {
		t.Error("engagement_outbox_events missing after `server migrate up`")
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
	// scenario: Engagement SDK: The service never migrates at boot
	url := freshURL(t)
	r := start(t, engSettings(testkit.FreePort(t), url, deadBroker), app.Options{})
	waitLiveness(t, r)
	r.stop(t)
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
	}
	for _, table := range append([]string{"schema_migrations"}, engagementTables...) {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after Run returned; the service must not migrate", table)
		}
	}
}

func TestRunFailsFastWhenDatabaseUnreachable(t *testing.T) {
	// scenario: Engagement SDK: Unreachable database fails boot
	s := engSettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	began := time.Now()
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		if took := time.Since(began); took > 6*time.Second {
			t.Errorf("Run failed after %s, want within 6s", took)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving or hanging")
	}
	assertPortFree(t, s.Port)
}

func TestHealthLivenessSurvivesDatabaseOutage(t *testing.T) {
	// scenario: Engagement SDK: Database outage flips readiness but not liveness
	url := migratedURL(t)
	r := start(t, engSettings(testkit.FreePort(t), url, deadBroker), app.Options{})
	waitServing(t, r)

	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, 2*healthWindow+5*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while Postgres is down, want SERVING", got)
	}
}

func TestLivenessCheckLatency(t *testing.T) {
	// scenario: Engagement SDK: Liveness answers fast
	url := migratedURL(t)
	r := start(t, engSettings(testkit.FreePort(t), url, deadBroker), app.Options{})
	waitServing(t, r)

	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, 2*healthWindow+5*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})

	client := grpc_health_v1.NewHealthClient(r.conn)
	lat := make([]time.Duration, 0, 1000)
	for i := 0; i < 1000; i++ {
		began := time.Now()
		resp, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: "liveness"})
		lat = append(lat, time.Since(began))
		if err != nil || resp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("liveness call %d = %v, %v, want SERVING", i, resp.GetStatus(), err)
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	if p99 := lat[989]; p99 >= 5*time.Millisecond {
		t.Fatalf("liveness p99 = %s, want under 5ms", p99)
	}
}

func TestKafkaOutageKeepsReadinessAndQueuesFact(t *testing.T) {
	// scenario: Engagement SDK: Kafka outage keeps the service ready
	url := migratedURL(t)
	r := start(t, engSettings(testkit.FreePort(t), url, deadBroker), app.Options{NewPublisher: downPublisher})
	waitServing(t, r)

	time.Sleep(healthWindow + time.Second)
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka down, want SERVING", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := engagementv1.NewEngagementServiceClient(r.conn).AddFavorite(asBuyer(ctx, "buyer-1"),
		&engagementv1.AddFavoriteRequest{ListingId: "listing-1"}); err != nil {
		t.Fatalf("AddFavorite with Kafka down = %v, want OK", err)
	}
	var n int
	if err := dial(t, url).QueryRow(context.Background(),
		`SELECT count(*) FROM engagement_outbox_events WHERE status = 'pending' AND event_type LIKE '%FavoriteAdded'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pending FavoriteAdded rows = %d, want 1", n)
	}
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v after the call, want SERVING", got)
	}
}

func TestShutdownOrder(t *testing.T) {
	// scenario: Engagement SDK: Shutdown drains before workers stop
	url := migratedURL(t)
	s := engSettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s, app.Options{NewPublisher: downPublisher})
	waitServing(t, r)

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if first, err := stream.Recv(); err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first watch = %v, %v, want SERVING", first.GetStatus(), err)
	}

	lockConn := dial(t, url)
	tx, err := lockConn.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(context.Background(), `LOCK TABLE favorites IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	rpcDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := engagementv1.NewEngagementServiceClient(r.conn).AddFavorite(asBuyer(ctx, "buyer-1"),
			&engagementv1.AddFavoriteRequest{ListingId: "listing-in-flight"})
		rpcDone <- err
	}()
	testkit.Eventually(t, 10*time.Second, func() bool {
		var waiting int
		_ = dial(t, url).QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return waiting > 0
	})

	r.cancel()
	next, err := stream.Recv()
	if err != nil {
		t.Fatalf("watch ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("watch after cancel = %v, want NOT_SERVING", next.GetStatus())
	}
	select {
	case <-r.done:
		t.Fatal("Run returned while an RPC was still in flight")
	case err := <-rpcDone:
		t.Fatalf("in-flight RPC ended before its lock was released: %v", err)
	default:
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-rpcDone:
		if err != nil {
			t.Fatalf("in-flight AddFavorite = %v, want OK", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("in-flight RPC did not complete")
	}
	select {
	case <-r.done:
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
	}
	var n int
	if err := dial(t, url).QueryRow(context.Background(),
		`SELECT count(*) FROM engagement_outbox_events WHERE event_type LIKE '%FavoriteAdded'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("FavoriteAdded rows = %d, want 1 written by the in-flight RPC", n)
	}
}

func listingPublished(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: "listing-park", SellerId: "seller-park", Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: "evt-park", Type: "platform.listing.v1.ListingChanged", OccurredAt: timestamppb.Now(), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestConsumerFailureStopsService(t *testing.T) {
	// scenario: Engagement SDK: Unparkable record stops the service
	url := migratedURL(t)
	if _, err := dial(t, url).Exec(context.Background(), `ALTER TABLE seller_listings RENAME TO seller_listings_broken`); err != nil {
		t.Fatal(err)
	}
	brokers := testkit.Redpanda(t)
	s := engSettings(testkit.FreePort(t), url, brokers)
	s.Events.ConsumerGroup = "team-engagement-park-test"
	s.Events.ListingTopic = "listing.events.park"
	testkit.Produce(t, brokers, s.Events.ListingTopic, "listing-park", listingPublished(t))

	pub := &failingPublisher{failOn: ".dlq"}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- app.RunWithOptions(ctx, s, app.Options{NewPublisher: func(sdkconfig.Kafka) (outbox.Publisher, error) { return pub, nil }})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil although the record could be neither handled nor parked")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Run ended with the context error %v, want the consumer's error", err)
		}
	case <-time.After(80 * time.Second):
		t.Fatal("Run kept serving after an unparkable record")
	}
	pub.mu.Lock()
	sawDLQ := false
	for _, topic := range pub.topics {
		if topic == s.Events.ListingTopic+".dlq" {
			sawDLQ = true
		}
	}
	pub.mu.Unlock()
	if !sawDLQ {
		t.Errorf("the failing record was never offered to the DLQ topic; produced to %v", pub.topics)
	}

	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumerGroup(s.Events.ConsumerGroup),
		kgo.ConsumeTopics(s.Events.ListingTopic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	pctx, pcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer pcancel()
	got := false
	for !got && pctx.Err() == nil {
		cl.PollFetches(pctx).EachRecord(func(*kgo.Record) { got = true })
	}
	if !got {
		t.Fatal("the offset of the unparkable record was committed; it must stay uncommitted")
	}
}

type sweepProbe struct {
	mu      sync.Mutex
	cutoffs []time.Time
	failFst bool
}

func (p *sweepProbe) sweep(_ context.Context, cutoff time.Time) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cutoffs = append(p.cutoffs, cutoff)
	if p.failFst && len(p.cutoffs) == 1 {
		return 0, errors.New("sweep failed by test")
	}
	return 0, nil
}

func (p *sweepProbe) calls() []time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]time.Time(nil), p.cutoffs...)
}

var fixedNow = time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC)

func TestSweepRunsAtStartAndEveryInterval(t *testing.T) {
	// scenario: Engagement SDK: Sweep runs at start and on each tick
	url := migratedURL(t)
	probe := &sweepProbe{}
	s := engSettings(testkit.FreePort(t), url, deadBroker)
	s.Reports.SweepInterval = 150 * time.Millisecond
	r := start(t, s, app.Options{
		NewPublisher: downPublisher,
		Now:          func() time.Time { return fixedNow },
		SweepReports: probe.sweep,
	})
	waitServing(t, r)

	testkit.Eventually(t, 20*time.Second, func() bool { return len(probe.calls()) >= 3 })
	want := fixedNow.Add(-retentionDays * 24 * time.Hour)
	for i, got := range probe.calls() {
		if !got.Equal(want) {
			t.Errorf("sweep call %d cutoff = %s, want %s (now minus %d days)", i, got, want, retentionDays)
		}
	}
	r.stop(t)
	if r.err != nil {
		t.Errorf("Run = %v, want nil once the context ended", r.err)
	}
}

func TestSweepFailureDoesNotStopWorker(t *testing.T) {
	// scenario: Engagement SDK: A failing sweep does not stop the worker
	url := migratedURL(t)
	probe := &sweepProbe{failFst: true}
	s := engSettings(testkit.FreePort(t), url, deadBroker)
	s.Reports.SweepInterval = 150 * time.Millisecond
	r := start(t, s, app.Options{
		NewPublisher: downPublisher,
		Now:          func() time.Time { return fixedNow },
		SweepReports: probe.sweep,
	})
	waitServing(t, r)

	testkit.Eventually(t, 20*time.Second, func() bool { return len(probe.calls()) >= 2 })
	select {
	case <-r.done:
		t.Fatalf("Run returned after a failed sweep: %v", r.err)
	default:
	}
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v after a failed sweep, want SERVING", got)
	}
}
