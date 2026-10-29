package app_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-analytics/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
	"github.com/buidangphuc/team-analytics/internal/config"
	"github.com/buidangphuc/team-analytics/internal/warehouse"
)

const (
	trackingType = "platform.analytics.v1.TrackingEvent"
	healthWindow = 1200 * time.Millisecond
)

func closedBroker(t *testing.T) string {
	t.Helper()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(testkit.FreePort(t)))
}

func baseSettings(t *testing.T, brokers string, extra map[string]string) config.Settings {
	t.Helper()
	vals := map[string]string{
		"ENV":                             "local",
		"GRPC_HOST":                       "127.0.0.1",
		"GRPC_PORT":                       strconv.Itoa(testkit.FreePort(t)),
		"SHUTDOWN_GRACE":                  "10s",
		"SHUTDOWN_DRAIN_DELAY":            "0s",
		"METRICS_ADDR":                    "",
		"DUCKDB_PATH":                     filepath.Join(t.TempDir(), "analytics.duckdb"),
		"KAFKA_BROKERS":                   brokers,
		"BATCH_FLUSH_INTERVAL_SECONDS":    "1",
		"PARQUET_EXPORT_INTERVAL_SECONDS": "0",
		"LOG_LEVEL":                       "error",
		"GA4_API_SECRET":                  "",
		"GA4_MEASUREMENT_ID":              "",
		"MOCK_DATA":                       "",
	}
	for k, v := range extra {
		vals[k] = v
	}
	for k, v := range vals {
		t.Setenv(k, v)
	}
	s, err := config.Load()
	require.NoError(t, err)
	return s
}

type proc struct {
	cancel   context.CancelFunc
	finished chan struct{}
	err      error
	addr     string
}

func startProc(t *testing.T, run func(context.Context, config.Settings) error, s config.Settings) *proc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	p := &proc{cancel: cancel, finished: make(chan struct{}), addr: s.Addr()}
	go func() {
		p.err = run(ctx, s)
		close(p.finished)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-p.finished:
		case <-time.After(30 * time.Second):
			t.Error("root did not return within 30s of cancel")
		}
	})
	return p
}

func (p *proc) result(t *testing.T, within time.Duration) error {
	t.Helper()
	select {
	case <-p.finished:
		return p.err
	case <-time.After(within):
		t.Fatalf("root did not return within %v", within)
		return nil
	}
}

func (p *proc) stopAndWait(t *testing.T) {
	t.Helper()
	p.cancel()
	require.NoError(t, p.result(t, 30*time.Second))
}

func dial(t *testing.T, addr string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func healthOf(conn *grpc.ClientConn, service string) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service})
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, err
	}
	return resp.GetStatus(), nil
}

func waitUp(t *testing.T, p *proc) *grpc.ClientConn {
	t.Helper()
	conn := dial(t, p.addr)
	testkit.Eventually(t, 20*time.Second, func() bool {
		st, err := healthOf(conn, "liveness")
		return err == nil && st == grpc_health_v1.HealthCheckResponse_SERVING
	})
	return conn
}

func requireStatusWithin(t *testing.T, conn *grpc.ClientConn, service string, want grpc_health_v1.HealthCheckResponse_ServingStatus, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var last grpc_health_v1.HealthCheckResponse_ServingStatus
	for time.Now().Before(deadline) {
		st, err := healthOf(conn, service)
		if err == nil {
			last = st
			if st == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("health %q did not reach %v within %v (last %v)", service, want, within, last)
}

func requireStatusHolds(t *testing.T, conn *grpc.ClientConn, service string, want grpc_health_v1.HealthCheckResponse_ServingStatus, window time.Duration) {
	t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		st, err := healthOf(conn, service)
		require.NoError(t, err)
		require.Equal(t, want, st, "health %q changed", service)
		time.Sleep(50 * time.Millisecond)
	}
}

func notListening(t *testing.T, addr string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		_ = c.Close()
		t.Fatalf("%s accepts connections, want nothing listening", addr)
	}
}

type recWriter struct {
	mu          sync.Mutex
	fail        atomic.Bool
	trackCalls  atomic.Int32
	upsertCalls atomic.Int32
	rows        []*warehouse.TrackingRecord
	listing     []*warehouse.ListingSellerRecord
	engagement  []*warehouse.EngagementFactRecord
	events      []string
	first, last time.Time
	gate        chan struct{}
	started     chan struct{}
	startOnce   sync.Once
}

var errWarehouseDown = errors.New("warehouse down")

func (w *recWriter) Write(_ context.Context, b []*warehouse.TrackingRecord) error {
	w.trackCalls.Add(1)
	if w.started != nil {
		w.startOnce.Do(func() { close(w.started) })
	}
	if w.gate != nil {
		<-w.gate
	}
	if w.fail.Load() {
		return errWarehouseDown
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	if w.first.IsZero() {
		w.first = now
	}
	w.last = now
	w.rows = append(w.rows, b...)
	w.events = append(w.events, "write")
	return nil
}

func (w *recWriter) WriteOrderFacts(_ context.Context, _ []*warehouse.OrderFactRecord) error {
	if w.fail.Load() {
		return errWarehouseDown
	}
	return nil
}

func (w *recWriter) UpsertListingSellers(_ context.Context, b []*warehouse.ListingSellerRecord) error {
	w.upsertCalls.Add(1)
	if w.fail.Load() {
		return errWarehouseDown
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.listing = append(w.listing, b...)
	return nil
}

func (w *recWriter) WriteEngagementFacts(_ context.Context, b []*warehouse.EngagementFactRecord) error {
	if w.fail.Load() {
		return errWarehouseDown
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.engagement = append(w.engagement, b...)
	return nil
}

func (w *recWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, "close")
	return nil
}

func (w *recWriter) rowCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.rows)
}

func (w *recWriter) engagementCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.engagement)
}

func (w *recWriter) eventLog() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.events...)
}

func marshal(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	require.NoError(t, err)
	return b
}

func envelope(t *testing.T, id, typ, principal string, payload []byte) []byte {
	t.Helper()
	return marshal(t, &eventsv1.EventEnvelope{
		EventId:    id,
		Type:       typ,
		Payload:    payload,
		OccurredAt: timestamppb.New(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)),
		Principal:  &commonv1.Principal{Id: principal, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER},
	})
}

func trackingValue(t *testing.T, id, principal, gaClientID string, analytics bool) []byte {
	t.Helper()
	te := marshal(t, &analyticsv1.TrackingEvent{
		EventType:  analyticsv1.EventType_EVENT_TYPE_VIEW,
		ListingId:  "lst-" + id,
		SessionId:  "s-" + id,
		GaClientId: gaClientID,
		Consent:    &analyticsv1.ConsentState{Analytics: analytics, Version: 1},
	})
	return envelope(t, id, trackingType, principal, te)
}

func produce(t *testing.T, brokers, topic string, recs ...*kgo.Record) {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	require.NoError(t, err)
	defer client.Close()
	for _, r := range recs {
		r.Topic = topic
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, client.ProduceSync(ctx, recs...).FirstErr())
}

func committedOffset(t *testing.T, brokers, group, topic string) int64 {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumerGroup(group), kgo.ConsumeTopics(topic), kgo.DisableAutoCommit())
	require.NoError(t, err)
	defer cl.Close()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		cl.PollFetches(ctx)
		cancel()
		for _, o := range cl.CommittedOffsets()[topic] {
			return o.Offset
		}
	}
	t.Fatalf("no committed offsets for group %s on %s", group, topic)
	return 0
}

func headerValue(r *kgo.Record, key string) (string, bool) {
	for _, h := range r.Headers {
		if h.Key == key {
			return string(h.Value), true
		}
	}
	return "", false
}

func dialErr(addr string) error {
	c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err == nil {
		_ = c.Close()
	}
	return err
}
