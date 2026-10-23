package app_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-payment/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/app"
	"github.com/buidangphuc/team-payment/internal/settlement"
)

func waitLiveness(ctx context.Context, r *running) error {
	_, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true))
	return err
}

func processMock(t *testing.T, r *running) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := paymentv1.NewPaymentServiceClient(r.conn).ProcessMockPayment(asUser(ctx, "buyer-1"),
		&paymentv1.ProcessMockPaymentRequest{TransactionId: "non-existent-tx", SimulateSuccess: true})
	return err
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: An unreachable database fails startup
	s := paySettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		if !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("boot error %q does not name postgres as the failed dependency", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving or hanging (Postgres open must be bounded and fail fast)")
	}
	assertPortFree(t, s.Port)
}

func TestRunRefusesUnknownMockAdapter(t *testing.T) {
	// scenario: An unknown adapter refuses startup
	url := migratedURL(t)
	s := paySettings(testkit.FreePort(t), url, deadBroker)
	s.MockData.Adapters = "no-such-adapter"
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil for an unknown MOCK_DATA adapter")
		}
		if !strings.Contains(err.Error(), "no-such-adapter") {
			t.Fatalf("boot error %q does not name the unknown adapter", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not refuse an unknown adapter")
	}
	assertPortFree(t, s.Port)
}

func TestMockPaymentRefusedWithoutMockData(t *testing.T) {
	// scenario: MOCK_PAYMENTS no longer enables anything
	t.Setenv("MOCK_PAYMENTS", "true")
	url := migratedURL(t)
	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	err := processMock(t, r)
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition || st.Message() != "mock payments are disabled" {
		t.Fatalf("ProcessMockPayment with MOCK_PAYMENTS=true and no MOCK_DATA = %v, want FailedPrecondition mock payments are disabled", err)
	}
}

func TestMockPaymentReachesRealPathWithMockData(t *testing.T) {
	// scenario: Active mock data is logged once
	url := migratedURL(t)
	s := paySettings(testkit.FreePort(t), url, deadBroker)
	s.MockData.Adapters = "payment-provider"
	r := start(t, s)
	waitServing(t, r)

	err := processMock(t, r)
	if code := status.Code(err); code != codes.NotFound {
		t.Fatalf("ProcessMockPayment with payment-provider active = %v, want NotFound from the real lookup", err)
	}
}

func TestActiveMockDataIsLoggedOnce(t *testing.T) {
	// scenario: Active mock data is logged once
	bin := buildServer(t)
	url := migratedURL(t)
	out, _ := bootBinary(t, bin, "DATABASE_URL="+url, "MOCK_DATA=payment-provider")
	got := strings.Count(out.String(), "MOCK_DATA active")
	if got != 1 {
		t.Fatalf("boot log has %d \"MOCK_DATA active\" lines, want 1:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "payment-provider") {
		t.Errorf("boot log does not name the active adapter:\n%s", out.String())
	}
}

func TestNoMockDataLogWhenInactive(t *testing.T) {
	// scenario: MOCK_PAYMENTS no longer enables anything
	bin := buildServer(t)
	url := migratedURL(t)
	out, _ := bootBinary(t, bin, "DATABASE_URL="+url, "MOCK_PAYMENTS=true")
	if strings.Contains(out.String(), "MOCK_DATA active") {
		t.Fatalf("boot log announces mock data with MOCK_DATA unset:\n%s", out.String())
	}
}

func TestReadinessFollowsPostgresLivenessDoesNot(t *testing.T) {
	// scenario: A database outage makes the service not ready but alive
	url := migratedURL(t)
	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
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

func TestRecoveryRestoresReadiness(t *testing.T) {
	// scenario: Recovery restores readiness
	url := migratedURL(t)
	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)
	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestReadinessIgnoresKafka(t *testing.T) {
	// scenario: A broker outage does not make the service not ready
	url := migratedURL(t)
	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	time.Sleep(healthWindow + time.Second)
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(r, "kafka"); got == grpc_health_v1.HealthCheckResponse_SERVING || got == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("a kafka health check is registered (status %v); readiness is postgres only", got)
	}
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Shutdown drains health first
	url := migratedURL(t)
	s := paySettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
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
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Errorf("Run = %v, want nil on shutdown", r.err)
	}
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Reflection is off by default
	url := migratedURL(t)
	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := reflectionpb.NewServerReflectionClient(r.conn).ServerReflectionInfo(ctx, grpc.WaitForReady(true))
	if err == nil {
		err = stream.Send(&reflectionpb.ServerReflectionRequest{MessageRequest: &reflectionpb.ServerReflectionRequest_ListServices{ListServices: ""}})
		if err == nil {
			_, err = stream.Recv()
		} else if _, rerr := stream.Recv(); rerr != nil {
			err = rerr
		}
	}
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("reflection err = %v, want Unimplemented", err)
	}
}

func orderPaidWithoutItems(t *testing.T, eventID, orderID string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&orderv1.OrderPaidEvent{OrderId: orderID, BuyerId: "buyer", TotalAmount: 1000, Currency: "VND"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: settlement.OrderPaidEventType, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func dlqText(t *testing.T, brokers, topic string) string {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	var all bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		fetches := cl.PollFetches(ctx)
		fetches.EachRecord(func(r *kgo.Record) {
			all.Write(r.Key)
			all.Write(r.Value)
			for _, h := range r.Headers {
				all.WriteString(h.Key)
				all.Write(h.Value)
			}
		})
	}
	return all.String()
}

func TestNewConsumerGroupStartsAtLatestOffset(t *testing.T) {
	// scenario: A new group starts at the latest offset
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	s := paySettings(testkit.FreePort(t), url, brokers)
	testkit.Produce(t, brokers, s.Events.OrderTopic, "orders", orderPaidWithoutItems(t, "evt-old", "order-before-group"))

	r := start(t, s)
	waitServing(t, r)

	n := 0
	testkit.Eventually(t, 90*time.Second, func() bool {
		n++
		id := fmt.Sprintf("order-after-group-%d", n)
		testkit.Produce(t, brokers, s.Events.OrderTopic, "orders", orderPaidWithoutItems(t, "evt-"+id, id))
		return strings.Contains(dlqText(t, brokers, s.Events.DLQTopic), "order-after-group-")
	})
	if text := dlqText(t, brokers, s.Events.DLQTopic); strings.Contains(text, "order-before-group") {
		t.Fatal("an event produced before the group existed was consumed; a new group must start at the latest offset")
	}
}

func TestPurgeRunsInsideTheService(t *testing.T) {
	// scenario: The purge runs on a schedule inside the service
	url := migratedURL(t)
	conn := dial(t, url)
	ins := `INSERT INTO payment_outbox_events
		(event_id, aggregate_type, aggregate_id, event_type, payload, request_id, status, attempts, available_at, created_at, published_at)
		VALUES ($1, 'Payment', $2, 'platform.payment.v1.PaymentSettled', $3, '', 'published', 1, $4, $4, $4)`
	now := time.Now()
	old, recent := now.Add(-8*24*time.Hour), now.Add(-6*24*time.Hour)
	if _, err := conn.Exec(context.Background(), ins, "evt-old", "o1", []byte("evt-old"), old); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	if _, err := conn.Exec(context.Background(), ins, "evt-recent", "o2", []byte("evt-recent"), recent); err != nil {
		t.Fatalf("seed recent row: %v", err)
	}

	r := start(t, paySettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	count := func(id string) int {
		var n int
		if err := conn.QueryRow(context.Background(), `SELECT count(*) FROM payment_outbox_events WHERE event_id = $1`, id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", id, err)
		}
		return n
	}
	testkit.Eventually(t, 30*time.Second, func() bool { return count("evt-old") == 0 })
	if count("evt-recent") != 1 {
		t.Error("a published row younger than 7 days was purged")
	}
}
