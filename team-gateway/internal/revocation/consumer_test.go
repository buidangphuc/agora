package revocation

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
)

var _ lifecycle.Component = (*Consumer)(nil)

func newTestConsumer(t *testing.T, brokers, topic string, d *Denylist, logger *slog.Logger) *Consumer {
	t.Helper()
	c, err := NewConsumer(sdkconfig.Kafka{Brokers: brokers}, topic, d, logger)
	if err != nil {
		t.Fatalf("NewConsumer: %v", err)
	}
	return c
}

type running struct {
	cancel context.CancelFunc
	done   chan error
}

func start(t *testing.T, c *Consumer) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{cancel: cancel, done: make(chan error, 1)}
	go func() { r.done <- c.Run(ctx) }()
	t.Cleanup(func() { r.stop(t) })
	return r
}

func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case <-r.done:
		r.done <- nil
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(strings.TrimSpace(s.b.String()), "\n")
}

func createTopic(t *testing.T, brokers, topic string) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	req := kmsg.NewPtrCreateTopicsRequest()
	rt := kmsg.NewCreateTopicsRequestTopic()
	rt.Topic = topic
	rt.NumPartitions = 1
	rt.ReplicationFactor = 1
	req.Topics = append(req.Topics, rt)
	resp, err := req.RequestWith(context.Background(), cl)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range resp.Topics {
		if tr.ErrorCode != 0 {
			t.Fatalf("create topic %s: error code %d", topic, tr.ErrorCode)
		}
	}
}

func meterReader(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	prev := otel.GetMeterProvider()
	r := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(r)))
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
	return r
}

func gauge(t *testing.T, r *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := r.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			g, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || len(g.DataPoints) != 1 {
				t.Fatalf("gauge %s: unexpected data %#v", name, m.Data)
			}
			return g.DataPoints[0].Value
		}
	}
	t.Fatalf("gauge %s not reported", name)
	return 0
}

func TestConsumerFeedsDenylistFromTopic(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	testkit.Produce(t, brokers, topic, "u1", envelope(t, sessionRevokedType, revoked("sid-1", time.Now().Add(time.Hour))))

	d := NewDenylist()
	start(t, newTestConsumer(t, brokers, topic, d, nil))

	testkit.Eventually(t, 10*time.Second, func() bool { return d.Revoked("sid-1") })
	if d.Revoked("sid-2") {
		t.Fatal("sid-2 was never revoked")
	}
}

func TestConsumerReplaysFromEarliestOnRestart(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	testkit.Produce(t, brokers, topic, "u1", envelope(t, sessionRevokedType, revoked("sid-1", time.Now().Add(time.Hour))))

	first := NewDenylist()
	r := start(t, newTestConsumer(t, brokers, topic, first, nil))
	testkit.Eventually(t, 10*time.Second, func() bool { return first.Revoked("sid-1") })
	r.stop(t)

	second := NewDenylist()
	start(t, newTestConsumer(t, brokers, topic, second, nil))
	testkit.Eventually(t, 10*time.Second, func() bool { return second.Revoked("sid-1") })
}

func TestConsumerCreatesNoConsumerGroup(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	testkit.Produce(t, brokers, topic, "u1", envelope(t, sessionRevokedType, revoked("sid-1", time.Now().Add(time.Hour))))

	d := NewDenylist()
	start(t, newTestConsumer(t, brokers, topic, d, nil))
	testkit.Eventually(t, 10*time.Second, func() bool { return d.Revoked("sid-1") })

	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	resp, err := kmsg.NewPtrListGroupsRequest().RequestWith(context.Background(), cl)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Groups) != 0 {
		t.Fatalf("the consumer must not create a consumer group, broker lists %+v", resp.Groups)
	}
}

func TestUndecodableRecordIsSkippedWithoutKey(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	testkit.Produce(t, brokers, topic, "user-42", []byte{0xff, 0xff, 0xff})
	testkit.Produce(t, brokers, topic, "u1", envelope(t, sessionRevokedType, revoked("sid-after", time.Now().Add(time.Hour))))

	buf := &syncBuf{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d := NewDenylist()
	start(t, newTestConsumer(t, brokers, topic, d, logger))

	testkit.Eventually(t, 10*time.Second, func() bool { return d.Revoked("sid-after") })

	var warns []string
	for _, l := range buf.lines() {
		if strings.Contains(l, "user-42") {
			t.Fatalf("a log line contains the record key: %s", l)
		}
		if strings.Contains(l, "level=WARN") {
			warns = append(warns, l)
		}
	}
	if len(warns) != 1 {
		t.Fatalf("want exactly one warn line, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0], "partition=0") || !strings.Contains(warns[0], "offset=0") {
		t.Fatalf("warn line must carry partition and offset: %s", warns[0])
	}
}

func TestEmptyTopicIsCaughtUpImmediately(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	createTopic(t, brokers, topic)

	c := newTestConsumer(t, brokers, topic, NewDenylist(), nil)
	if c.CaughtUp() {
		t.Fatal("CaughtUp must be false before Run captured the end offsets")
	}
	start(t, c)
	testkit.Eventually(t, 10*time.Second, c.CaughtUp)
}

func TestCaughtUpWaitsForExistingRecords(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	for _, sid := range []string{"a", "b", "c"} {
		testkit.Produce(t, brokers, topic, sid, envelope(t, sessionRevokedType, revoked(sid, time.Now().Add(time.Hour))))
	}

	d := NewDenylist()
	c := newTestConsumer(t, brokers, topic, d, nil)
	start(t, c)
	testkit.Eventually(t, 10*time.Second, c.CaughtUp)
	if d.Len() != 3 {
		t.Fatalf("caught up with %d of 3 revocations applied", d.Len())
	}
}

func TestCaughtUpStaysAfterBrokerOutage(t *testing.T) {
	ctx := context.Background()
	ctr, err := tcredpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v24.3.1", tcredpanda.WithAutoCreateTopics())
	if ctr != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	}
	if err != nil {
		t.Fatalf("start redpanda: %v", err)
	}
	brokers, err := ctr.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const topic = "identity.events"
	testkit.Produce(t, brokers, topic, "u1", envelope(t, sessionRevokedType, revoked("sid-1", time.Now().Add(time.Hour))))

	reader := meterReader(t)
	d := NewDenylist()
	c := newTestConsumer(t, brokers, topic, d, nil)
	if err := RegisterMetrics(c); err != nil {
		t.Fatal(err)
	}
	start(t, c)
	testkit.Eventually(t, 10*time.Second, c.CaughtUp)
	testkit.Eventually(t, 10*time.Second, func() bool { return gauge(t, reader, "gateway_revocation_consumer_up") == 1 })

	timeout := 20 * time.Second
	if err := ctr.Stop(ctx, &timeout); err != nil {
		t.Fatalf("stop broker: %v", err)
	}

	testkit.Eventually(t, 15*time.Second, func() bool { return gauge(t, reader, "gateway_revocation_consumer_up") == 0 })
	if !c.CaughtUp() {
		t.Fatal("CaughtUp is a latch and must stay true after a broker outage")
	}
	if !d.Revoked("sid-1") {
		t.Fatal("the denylist must keep its entries during an outage")
	}
}

func TestRevocationGaugesReportConsumerState(t *testing.T) {
	brokers := testkit.Redpanda(t)
	const topic = "identity.events"
	reader := meterReader(t)

	d := NewDenylist()
	c := newTestConsumer(t, brokers, topic, d, nil)
	if err := RegisterMetrics(c); err != nil {
		t.Fatal(err)
	}
	if up, size := gauge(t, reader, "gateway_revocation_consumer_up"), gauge(t, reader, "gateway_revocation_denylist_size"); up != 0 || size != 0 {
		t.Fatalf("before Run want up 0 size 0, got up %d size %d", up, size)
	}

	for _, sid := range []string{"s1", "s2"} {
		testkit.Produce(t, brokers, topic, sid, envelope(t, sessionRevokedType, revoked(sid, time.Now().Add(time.Hour))))
	}
	start(t, c)
	testkit.Eventually(t, 10*time.Second, func() bool {
		return gauge(t, reader, "gateway_revocation_consumer_up") == 1 && gauge(t, reader, "gateway_revocation_denylist_size") == 2
	})
}
