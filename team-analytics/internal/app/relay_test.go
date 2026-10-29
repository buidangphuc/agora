package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	_ "github.com/marcboeker/go-duckdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	orderv1 "github.com/buidangphuc/team-analytics/generated/platform/order/v1"
	"github.com/buidangphuc/team-analytics/internal/app"
	"github.com/buidangphuc/team-analytics/internal/ga4relay"
)

const (
	orderPaidType = "platform.order.v1.OrderPaidEvent"
	relayBuyer    = "buyer-1"
	relayClientID = "123.456"
)

type ga4Stub struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []string
	reject string
}

func newGA4Stub(t *testing.T, reject string) *ga4Stub {
	t.Helper()
	g := &ga4Stub{reject: reject}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.bodies = append(g.bodies, string(b))
		g.mu.Unlock()
		if g.reject != "" && bytes.Contains(b, []byte(g.reject)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *ga4Stub) count(contains string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, b := range g.bodies {
		if contains == "" || strings.Contains(b, contains) {
			n++
		}
	}
	return n
}

func relayEnv(g *ga4Stub) map[string]string {
	return map[string]string{
		"GA4_API_SECRET":     "test-secret",
		"GA4_MEASUREMENT_ID": "G-TEST",
		"GA4_ENDPOINT":       g.srv.URL,
	}
}

func seedMapping(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("duckdb", path)
	require.NoError(t, err)
	store, err := ga4relay.NewSQLMappingStore(context.Background(), db, time.Now)
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), relayBuyer, relayClientID))
	require.NoError(t, db.Close())
}

func orderRecord(t *testing.T, orderID string) *kgo.Record {
	t.Helper()
	ev := &orderv1.OrderPaidEvent{
		OrderId:     orderID,
		BuyerId:     relayBuyer,
		TotalAmount: 125000,
		Currency:    "VND",
		PaidAt:      timestamppb.New(time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)),
		Items:       []*orderv1.OrderLineItemFact{{ListingId: "lst-1", SellerId: "s1", Quantity: 1, UnitPrice: 125000, Currency: "VND"}},
	}
	return &kgo.Record{Key: []byte(orderID), Value: envelope(t, "evt-"+orderID, orderPaidType, relayBuyer, marshal(t, ev))}
}

func TestRelaySecretUnsetIsDisabledAndServing(t *testing.T) {
	var built atomic.Bool
	app.SetHooks(t, app.Hooks{
		PingDuckDB:     failingPing,
		HealthInterval: 200 * time.Millisecond,
		Relay:          func(*ga4relay.Relay) { built.Store(true) },
	})
	g := newGA4Stub(t, "")
	s := baseSettings(t, closedBroker(t), map[string]string{"GA4_ENDPOINT": g.srv.URL})
	p := startProc(t, app.RunGA4Relay, s)
	conn := waitUp(t, p)

	requireStatusHolds(t, conn, "", grpc_health_v1.HealthCheckResponse_SERVING, 1500*time.Millisecond)
	requireStatusHolds(t, conn, "liveness", grpc_health_v1.HealthCheckResponse_SERVING, 100*time.Millisecond)
	_, err := os.Stat(s.Warehouse.DuckDBPath)
	assert.True(t, errors.Is(err, os.ErrNotExist), "disabled relay must not open its DuckDB file")
	assert.False(t, built.Load(), "disabled relay must not build a relay")
	assert.Zero(t, g.count(""))
}

func TestRelayServesOnlyHealth(t *testing.T) {
	g := newGA4Stub(t, "")
	s := baseSettings(t, closedBroker(t), relayEnv(g))
	p := startProc(t, app.RunGA4Relay, s)
	conn := waitUp(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := analyticsv1.NewAnalyticsQueryServiceClient(conn).GetSellerFunnel(ctx, &analyticsv1.GetSellerFunnelRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	st, err := healthOf(conn, "")
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, st)
	assert.Zero(t, g.count(""))
}

func TestRelayDoesNotServeHTTPProbes(t *testing.T) {
	g := newGA4Stub(t, "")
	s := baseSettings(t, closedBroker(t), relayEnv(g))
	p := startProc(t, app.RunGA4Relay, s)
	waitUp(t, p)

	c, err := net.DialTimeout("tcp", s.GRPC.Addr(), 2*time.Second)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	_, err = c.Write([]byte("GET /healthz HTTP/1.1\r\nHost: relay\r\n\r\n"))
	require.NoError(t, err)
	require.NoError(t, c.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 512)
	n, _ := c.Read(buf)
	assert.False(t, bytes.HasPrefix(buf[:n], []byte("HTTP/1.1 200")), "relay answered plain HTTP with 200")
}

func TestRelayReadinessFollowsDuckDBWhenEnabled(t *testing.T) {
	app.SetHooks(t, app.Hooks{PingDuckDB: failingPing, HealthInterval: 200 * time.Millisecond})
	g := newGA4Stub(t, "")
	s := baseSettings(t, closedBroker(t), relayEnv(g))
	p := startProc(t, app.RunGA4Relay, s)
	conn := waitUp(t, p)

	requireStatusWithin(t, conn, "", grpc_health_v1.HealthCheckResponse_NOT_SERVING, healthWindow)
	requireStatusHolds(t, conn, "liveness", grpc_health_v1.HealthCheckResponse_SERVING, 400*time.Millisecond)
}

func TestRelayReadinessIgnoresKafka(t *testing.T) {
	app.SetHooks(t, app.Hooks{HealthInterval: 200 * time.Millisecond})
	g := newGA4Stub(t, "")
	s := baseSettings(t, closedBroker(t), relayEnv(g))
	p := startProc(t, app.RunGA4Relay, s)
	conn := waitUp(t, p)

	requireStatusHolds(t, conn, "", grpc_health_v1.HealthCheckResponse_SERVING, 2*time.Second)
}

func TestRejectedOrderIsDeadLetteredAndNextOrderSent(t *testing.T) {
	brokers := testkit.Redpanda(t)
	g := newGA4Stub(t, "ord-rejected")
	s := baseSettings(t, brokers, relayEnv(g))
	seedMapping(t, s.Warehouse.DuckDBPath)

	produce(t, brokers, s.Kafka.OrderTopic, orderRecord(t, "ord-rejected"), orderRecord(t, "ord-accepted"))
	startProc(t, app.RunGA4Relay, s)

	got := testkit.ReadAll(t, brokers, s.GA4.RelayDLQTopic, 1, dlqWait)
	assert.Equal(t, "ord-rejected", string(got[0].Key))
	_, ok := headerValue(got[0], "dlq-source-topic")
	assert.True(t, ok)
	_, ok = headerValue(got[0], "dlq-error")
	assert.True(t, ok)
	testkit.Eventually(t, dlqWait, func() bool { return g.count("ord-accepted") == 1 })
	assert.Equal(t, 1, g.count("ord-rejected"), "a 4xx is not retried")
}

type flakyLedger struct {
	ga4relay.Ledger
	fail *atomic.Bool
}

func (l flakyLedger) Sent(ctx context.Context, orderID string) (bool, error) {
	if l.fail.Load() {
		return false, errors.New("ledger down")
	}
	return l.Ledger.Sent(ctx, orderID)
}

func TestStoreOutageIsDeadLetteredAfterFiveAttemptsAndReplaySends(t *testing.T) {
	brokers := testkit.Redpanda(t)
	var down atomic.Bool
	down.Store(true)
	var relay atomic.Pointer[ga4relay.Relay]
	app.SetHooks(t, app.Hooks{
		WrapLedger: func(l ga4relay.Ledger) ga4relay.Ledger { return flakyLedger{Ledger: l, fail: &down} },
		Relay:      func(r *ga4relay.Relay) { relay.Store(r) },
	})
	g := newGA4Stub(t, "")
	s := baseSettings(t, brokers, relayEnv(g))
	seedMapping(t, s.Warehouse.DuckDBPath)

	produce(t, brokers, s.Kafka.OrderTopic, orderRecord(t, "ord-o4"))
	startProc(t, app.RunGA4Relay, s)

	got := testkit.ReadAll(t, brokers, s.GA4.RelayDLQTopic, 1, dlqWait)
	assert.Equal(t, "ord-o4", string(got[0].Key))
	_, ok := headerValue(got[0], "dlq-error")
	assert.True(t, ok)
	require.NotNil(t, relay.Load())
	assert.Equal(t, int64(5), relay.Load().FailedCount("store"))
	assert.Zero(t, g.count(""))

	down.Store(false)
	produce(t, brokers, s.Kafka.OrderTopic, &kgo.Record{Key: got[0].Key, Value: got[0].Value})
	testkit.Eventually(t, dlqWait, func() bool { return g.count("ord-o4") >= 1 })
	assert.Equal(t, 1, g.count("ord-o4"))
}
