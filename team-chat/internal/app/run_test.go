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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/app"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/migrations"
)

const healthWindow = healthx.DefaultInterval + 1*time.Second

type switchPublisher struct {
	mu        sync.Mutex
	down      bool
	permanent map[string]bool
	records   []*kgo.Record
	calls     int
}

func (p *switchPublisher) PublishRecord(_ context.Context, r *kgo.Record) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.down {
		return errors.New("broker down")
	}
	if p.permanent[string(r.Key)] {
		return outbox.Permanent(errors.New("record rejected"))
	}
	p.records = append(p.records, r)
	return nil
}

func (p *switchPublisher) setDown(v bool) {
	p.mu.Lock()
	p.down = v
	p.mu.Unlock()
}

func (p *switchPublisher) recorded() []*kgo.Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*kgo.Record(nil), p.records...)
}

func (p *switchPublisher) factory() func(sdkconfig.Kafka) (outbox.Publisher, error) {
	return func(sdkconfig.Kafka) (outbox.Publisher, error) { return p, nil }
}

type running struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	done   chan struct{}
	err    error
	port   int
}

func chatSettings(port int, url string) config.Settings {
	return config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url},
		Kafka:    sdkconfig.Kafka{Brokers: "127.0.0.1:1"},
		Outbox:   sdkconfig.Outbox{PollInterval: 50 * time.Millisecond, BatchSize: 100, ClaimLock: 60 * time.Second},
		Events:   config.Events{ChatTopic: "chat.events"},
		OTel:     config.OTel{ServiceName: "team-chat"},
	}
}

func migratedURL(t *testing.T) string {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return url
}

func startWith(t *testing.T, s config.Settings, o app.Options) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan struct{}), port: s.Port}
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
		case <-time.After(20 * time.Second):
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
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
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

func asUser(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-principal-id", id, "x-principal-type", "user")
}

func newThread(t *testing.T, r *running, seller, listing string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	res, err := chatv1.NewChatServiceClient(r.conn).GetOrCreateThread(asUser(ctx, "buyer-1"),
		&chatv1.GetOrCreateThreadRequest{SellerId: seller, ListingId: listing}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("GetOrCreateThread: %v", err)
	}
	return res.GetThread().GetId()
}

func send(t *testing.T, r *running, thread, content string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := chatv1.NewChatServiceClient(r.conn).SendMessage(asUser(ctx, "buyer-1"),
		&chatv1.SendMessageRequest{ThreadId: thread, Content: content}); err != nil {
		t.Fatalf("SendMessage %q: %v", content, err)
	}
}

func contentOf(t *testing.T, rec *kgo.Record) string {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var msg chatv1.ChatMessage
	if err := proto.Unmarshal(env.GetPayload(), &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	return msg.GetContent()
}

func countRows(t *testing.T, url, where string) int {
	t.Helper()
	var n int
	if err := dial(t, url).QueryRow(context.Background(), `SELECT count(*) FROM chat_outbox_events WHERE `+where).Scan(&n); err != nil {
		t.Fatalf("count outbox %q: %v", where, err)
	}
	return n
}

func assertBootFails(t *testing.T, s config.Settings, o app.Options) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.RunWithOptions(ctx, s, o) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunWithOptions returned nil, want a boot error")
		}
		if !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("boot error %q does not name postgres as the failed dependency", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("RunWithOptions did not fail; it is serving or hanging")
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.Port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", s.Port, err)
	}
	_ = l.Close()
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: An unreachable database fails boot in a local ENV
	rec := &switchPublisher{}
	built := false
	factory := func(sdkconfig.Kafka) (outbox.Publisher, error) { built = true; return rec, nil }
	assertBootFails(t, chatSettings(testkit.FreePort(t), closedDatabaseURL), app.Options{NewPublisher: factory})
	if built {
		t.Error("producer was built although Postgres is unreachable; the build order is Postgres before producer")
	}
}

func TestRunFailsWhenPublisherCannotBeBuilt(t *testing.T) {
	// scenario: A Kafka client that cannot be built fails boot
	url := migratedURL(t)
	s := chatSettings(testkit.FreePort(t), url)
	want := errors.New("kafka client cannot be built")
	o := app.Options{NewPublisher: func(sdkconfig.Kafka) (outbox.Publisher, error) { return nil, want }}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.RunWithOptions(ctx, s, o) }()
	select {
	case err := <-done:
		if !errors.Is(err, want) {
			t.Fatalf("RunWithOptions = %v, want an error wrapping the publisher error", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("RunWithOptions did not fail when the publisher cannot be built")
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.Port))
	if err != nil {
		t.Fatalf("port bound by failed boot: %v", err)
	}
	_ = l.Close()
}

func TestMessageSentWhileKafkaDownIsDeliveredAfterRecovery(t *testing.T) {
	// scenario: A message sent while Kafka is down is delivered after recovery
	url := migratedURL(t)
	pub := &switchPublisher{down: true}
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: pub.factory()})
	waitServing(t, r)

	thread := newThread(t, r, "seller-1", "listing-1")
	send(t, r, thread, "while-down")

	testkit.Eventually(t, 15*time.Second, func() bool {
		pub.mu.Lock()
		defer pub.mu.Unlock()
		return pub.calls > 0
	})
	if got := len(pub.recorded()); got != 0 {
		t.Fatalf("%d records published while the broker is down", got)
	}
	if n := countRows(t, url, "status = 'pending'"); n != 1 {
		t.Fatalf("pending outbox rows = %d, want 1", n)
	}
	if n := countRows(t, url, "status = 'failed'"); n != 0 {
		t.Fatalf("a transient failure parked %d rows", n)
	}

	pub.setDown(false)
	testkit.Eventually(t, 30*time.Second, func() bool { return len(pub.recorded()) == 1 })
	rec := pub.recorded()[0]
	if rec.Topic != "chat.events" {
		t.Errorf("topic = %q, want chat.events", rec.Topic)
	}
	if string(rec.Key) != thread {
		t.Errorf("key = %q, want thread id %q", rec.Key, thread)
	}
	if got := contentOf(t, rec); got != "while-down" {
		t.Errorf("delivered content = %q", got)
	}
	testkit.Eventually(t, 10*time.Second, func() bool { return countRows(t, url, "status = 'published'") == 1 })
}

func TestRelayerHasNoAttemptLimit(t *testing.T) {
	// scenario: A message sent while Kafka is down is delivered after recovery
	url := migratedURL(t)
	if _, err := dial(t, url).Exec(context.Background(), `INSERT INTO chat_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, error)
		VALUES ('evt-old', 'ChatThread', 'thread-old', 'platform.chat.v1.ChatMessage', '\x00', 'pending', 50, 'broker down')`); err != nil {
		t.Fatal(err)
	}
	pub := &switchPublisher{}
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: pub.factory()})
	waitServing(t, r)

	testkit.Eventually(t, 30*time.Second, func() bool { return len(pub.recorded()) == 1 })
	if n := countRows(t, url, "status = 'failed'"); n != 0 {
		t.Fatalf("row with 50 attempts was parked: %d failed rows", n)
	}
	testkit.Eventually(t, 10*time.Second, func() bool {
		return countRows(t, url, "event_id = 'evt-old' AND status = 'published'") == 1
	})
}

func TestPermanentErrorParksOneRowOnly(t *testing.T) {
	// scenario: A permanent record error parks the row only
	url := migratedURL(t)
	pub := &switchPublisher{permanent: map[string]bool{}}
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: pub.factory()})
	waitServing(t, r)

	poison := newThread(t, r, "seller-poison", "listing-1")
	good := newThread(t, r, "seller-good", "listing-2")
	pub.mu.Lock()
	pub.permanent[poison] = true
	pub.mu.Unlock()

	send(t, r, poison, "poison-1")
	send(t, r, good, "good-1")
	send(t, r, good, "good-2")

	testkit.Eventually(t, 30*time.Second, func() bool { return len(pub.recorded()) == 2 })
	testkit.Eventually(t, 30*time.Second, func() bool { return countRows(t, url, "status = 'failed'") == 1 })
	if n := countRows(t, url, "status = 'failed' AND aggregate_id = '"+poison+"'"); n != 1 {
		t.Errorf("parked rows for the poisoned thread = %d, want 1", n)
	}
	if n := countRows(t, url, "status = 'published'"); n != 2 {
		t.Errorf("published rows = %d, want the 2 of the other thread", n)
	}
	for _, rec := range pub.recorded() {
		if string(rec.Key) != good {
			t.Errorf("record for %q published; only the healthy thread may publish", rec.Key)
		}
	}
}

func TestThreadEventsPublishInSendOrder(t *testing.T) {
	// scenario: Messages of one thread publish in send order
	url := migratedURL(t)
	pub := &switchPublisher{down: true}
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: pub.factory()})
	waitServing(t, r)

	thread := newThread(t, r, "seller-1", "listing-1")
	want := []string{"m0", "m1", "m2", "m3", "m4", "m5"}
	for _, c := range want {
		send(t, r, thread, c)
	}
	pub.setDown(false)
	testkit.Eventually(t, 30*time.Second, func() bool { return len(pub.recorded()) == len(want) })
	for i, rec := range pub.recorded() {
		if got := contentOf(t, rec); got != want[i] {
			t.Errorf("record %d content = %q, want %q", i, got, want[i])
		}
	}
}

func listServices(r *running) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err != nil {
		return err
	}
	if err := stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}}); err != nil {
		if _, rerr := stream.Recv(); rerr != nil {
			return rerr
		}
		return err
	}
	_, err = stream.Recv()
	return err
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Reflection is not served by default
	url := migratedURL(t)
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: (&switchPublisher{}).factory()})
	waitServing(t, r)
	if err := listServices(r); status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}

func TestKafkaOutageKeepsReadiness(t *testing.T) {
	// scenario: A Kafka outage keeps the service ready
	url := migratedURL(t)
	s := chatSettings(testkit.FreePort(t), url)
	s.Kafka.Brokers = "127.0.0.1:1"
	r := startWith(t, s, app.Options{})
	waitServing(t, r)

	thread := newThread(t, r, "seller-1", "listing-1")
	send(t, r, thread, "stored-while-kafka-is-unreachable")
	if n := countRows(t, url, "status = 'pending'"); n != 1 {
		t.Fatalf("pending rows = %d, want the message stored for later delivery", n)
	}

	time.Sleep(healthWindow + time.Second)
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING", got)
	}
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

func TestPostgresOutageFlipsReadinessNotLiveness(t *testing.T) {
	// scenario: A Postgres outage flips readiness but not liveness
	url := testkit.Postgres(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatal(err)
	}
	r := startWith(t, chatSettings(testkit.FreePort(t), url), app.Options{NewPublisher: (&switchPublisher{}).factory()})
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while Postgres is down, want SERVING", got)
	}

	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestShutdownOrder(t *testing.T) {
	// scenario: Shutdown drains health before stopping gRPC
	url := migratedURL(t)
	s := chatSettings(testkit.FreePort(t), url)
	s.DrainDelay = 1500 * time.Millisecond
	r := startWith(t, s, app.Options{NewPublisher: (&switchPublisher{}).factory()})
	waitServing(t, r)

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("first watch = %v, %v, want SERVING", first.GetStatus(), err)
	}

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
		t.Fatal("Run returned before the drain delay elapsed")
	default:
	}
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("check during drain = %v, want NOT_SERVING from a still-serving gRPC server", got)
	}
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
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
	t.Cleanup(c.finish)
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

func (c *logCapture) records(t *testing.T) (string, []map[string]any) {
	t.Helper()
	c.finish()
	c.mu.Lock()
	defer c.mu.Unlock()
	raw := c.buf.String()
	var recs []map[string]any
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

func TestLogsNeverContainMessageContent(t *testing.T) {
	// scenario: A sent message leaves no content in the logs
	url := migratedURL(t)
	logs := captureStdout(t)
	s := chatSettings(testkit.FreePort(t), url)
	s.LogLevel = "debug"
	pub := &switchPublisher{}
	r := startWith(t, s, app.Options{NewPublisher: pub.factory()})
	waitServing(t, r)

	thread := newThread(t, r, "seller-1", "listing-1")
	send(t, r, thread, "s3cret-needle")
	testkit.Eventually(t, 30*time.Second, func() bool { return len(pub.recorded()) == 1 })
	r.stop(t)

	raw, recs := logs.records(t)
	if len(recs) == 0 {
		t.Fatalf("no JSON log records captured; output:\n%s", raw)
	}
	if strings.Contains(raw, "s3cret-needle") {
		t.Errorf("log output contains the message content:\n%s", raw)
	}
	found := false
	for _, m := range recs {
		b, _ := json.Marshal(m)
		if strings.Contains(string(b), "SendMessage") {
			found = true
		}
	}
	if !found {
		t.Errorf("no log record names SendMessage; output:\n%s", raw)
	}
}

func insertPublished(t *testing.T, url, id string, age time.Duration) {
	t.Helper()
	_, err := dial(t, url).Exec(context.Background(), `INSERT INTO chat_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, status, published_at)
		VALUES ($1, 'ChatThread', $1, 'platform.chat.v1.ChatMessage', '\x00', 'published', now() - ($2 * interval '1 second'))`,
		id, age.Seconds())
	if err != nil {
		t.Fatal(err)
	}
}

func seedPurgeRows(t *testing.T, url string) {
	t.Helper()
	insertPublished(t, url, "pub-old", 8*24*time.Hour)
	insertPublished(t, url, "pub-recent", 24*time.Hour)
	_, err := dial(t, url).Exec(context.Background(), `INSERT INTO chat_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, status, created_at, error)
		VALUES ('parked-old', 'ChatThread', 'parked-old', 'platform.chat.v1.ChatMessage', '\x00', 'failed', now() - interval '30 days', 'rejected')`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublishedRowsOlderThanSevenDaysArePurged(t *testing.T) {
	// scenario: Published rows older than 7 days are purged on a schedule
	url := migratedURL(t)
	seedPurgeRows(t, url)
	r := startWith(t, chatSettings(testkit.FreePort(t), url),
		app.Options{NewPublisher: (&switchPublisher{}).factory(), PurgeInterval: 50 * time.Millisecond})
	waitServing(t, r)

	testkit.Eventually(t, 20*time.Second, func() bool { return countRows(t, url, "event_id = 'pub-old'") == 0 })
	for _, id := range []string{"pub-recent", "parked-old"} {
		if n := countRows(t, url, "event_id = '"+id+"'"); n != 1 {
			t.Errorf("row %s was purged; only published rows older than 7 days may be", id)
		}
	}
}

func TestPurgeFailureDoesNotStopService(t *testing.T) {
	// scenario: A purge failure does not stop the service
	url := migratedURL(t)
	c := dial(t, url)
	for _, q := range []string{
		`CREATE FUNCTION block_purge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'purge blocked by test'; END $$`,
		`CREATE TRIGGER block_purge BEFORE DELETE ON chat_outbox_events FOR EACH ROW EXECUTE FUNCTION block_purge()`,
	} {
		if _, err := c.Exec(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	seedPurgeRows(t, url)

	logs := captureStdout(t)
	s := chatSettings(testkit.FreePort(t), url)
	s.LogLevel = "info"
	r := startWith(t, s, app.Options{NewPublisher: (&switchPublisher{}).factory(), PurgeInterval: 50 * time.Millisecond})
	waitServing(t, r)

	time.Sleep(500 * time.Millisecond)
	select {
	case <-r.done:
		t.Fatalf("service stopped after a purge failure: %v", r.err)
	default:
	}
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v after a purge failure", got)
	}
	thread := newThread(t, r, "seller-1", "listing-1")
	send(t, r, thread, "still-serving")

	if _, err := c.Exec(context.Background(), `DROP TRIGGER block_purge ON chat_outbox_events`); err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, 20*time.Second, func() bool { return countRows(t, url, "event_id = 'pub-old'") == 0 })
	r.stop(t)

	raw, _ := logs.records(t)
	if !strings.Contains(raw, "purge blocked by test") {
		t.Errorf("purge failure cause not logged; output:\n%s", raw)
	}
}
