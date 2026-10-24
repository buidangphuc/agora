package app_test

import (
	"context"
	"errors"
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
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-domain/generated/platform/identity/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/app"
	"github.com/buidangphuc/team-domain/internal/store"
)

func createDraft(ctx context.Context, r *running, seller string) (*listingv1.Listing, error) {
	resp, err := listingv1.NewListingServiceClient(r.conn).CreateListing(asSeller(ctx, seller),
		&listingv1.CreateListingRequest{Listing: &listingv1.Listing{
			Title: "draft", Price: 1000, Currency: "VND", Status: listingv1.ListingStatus_LISTING_STATUS_DRAFT, Stock: 1,
		}}, grpc.WaitForReady(true))
	return resp.GetListing(), err
}

func TestRunFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: Domain SDK: An unreachable database stops boot
	s := domainSettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "postgres") && !strings.Contains(msg, "database") {
			t.Fatalf("boot error %q does not name the database as the failed dependency", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("Run did not fail within 6 s: the Postgres open must be bounded and fail fast")
	}
	assertPortFree(t, s.Port)
}

func TestRunRelaysOutbox(t *testing.T) {
	// scenario: Relayer runs without any enable variable
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	s := domainSettings(testkit.FreePort(t), url, brokers)
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	created, err := createDraft(ctx, r, "seller-relay")
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}

	recs := testkit.ReadAll(t, brokers, listingTopic, 1, 60*time.Second)
	if string(recs[0].Key) != created.GetId() {
		t.Fatalf("relayed key = %q, want listing id %q", recs[0].Key, created.GetId())
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(recs[0].Value, &env); err != nil {
		t.Fatalf("relayed value is not an EventEnvelope: %v", err)
	}
	pool := openPool(t, url)
	testkit.Eventually(t, 20*time.Second, func() bool {
		st, _ := outboxStatus(t, pool, env.GetEventId())
		return st == "published"
	})
}

func TestRunSweepsExpiredReservations(t *testing.T) {
	// scenario: Domain SDK: The sweeper releases expired reservations without any switch
	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-sweep", Title: "t", Currency: "VND", Status: "published", SellerID: "seller-a", Stock: 5})
	repo := store.NewPostgresListingRepository(pool)
	if err := repo.ReserveStockIdempotent(t.Context(), "r-expired", "L-sweep", "", 2, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	stock := func() int32 {
		l, err := repo.Get(t.Context(), "L-sweep")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		return l.Stock
	}
	if got := stock(); got != 3 {
		t.Fatalf("stock after reserve = %d, want 3", got)
	}

	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	testkit.Eventually(t, 5*time.Second, func() bool {
		var st string
		err := pool.QueryRow(t.Context(), `SELECT status FROM reservations WHERE reservation_id = 'r-expired'`).Scan(&st)
		return err == nil && st == "released"
	})
	if got := stock(); got != 5 {
		t.Fatalf("stock after sweep = %d, want 5", got)
	}
}

func TestSweeperStartLogCarriesTTLAndInterval(t *testing.T) {
	// scenario: Domain SDK: The sweeper releases expired reservations without any switch
	bin := buildServer(t)
	url := migratedURL(t)
	out, _ := bootBinary(t, bin, "DATABASE_URL="+url, "RESERVATION_TTL=7m", "RESERVATION_SWEEP_INTERVAL=45s")
	testkit.Eventually(t, 10*time.Second, func() bool {
		return strings.Contains(out.String(), "reservation sweeper started")
	})
	log := out.String()
	for _, want := range []string{"7m0s", "45s"} {
		if !strings.Contains(log, want) {
			t.Errorf("sweeper start log lacks %q:\n%s", want, log)
		}
	}
}

func statusEnvelope(t *testing.T, eventID, user string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&identityv1.UserStatusChanged{
		UserId: user, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED,
		PreviousStatus: identityv1.UserStatus_USER_STATUS_ACTIVE, ChangedAt: timestamppb.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{
		EventId: eventID, Type: "platform.identity.v1.UserStatusChanged", OccurredAt: timestamppb.Now(), Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestRunConsumesIdentityEvents(t *testing.T) {
	// scenario: Seller hold: Consumer is always running
	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-hold", Title: "t", Currency: "VND", Status: "published", SellerID: "seller-susp", Stock: 1})
	brokers := testkit.Redpanda(t)
	s := domainSettings(testkit.FreePort(t), url, brokers)
	r := start(t, s)
	waitServing(t, r)

	testkit.Eventually(t, 60*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "seller-susp", statusEnvelope(t, "evt-suspend-1", "seller-susp"))
		return listingStatus(t, pool, "L-hold") == "held"
	})
}

func TestReadinessFailsLivenessHoldsWhenPostgresDown(t *testing.T) {
	// scenario: Domain SDK: A database outage fails readiness and keeps liveness
	url := migratedURL(t)
	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	setDatabaseAvailable(t, url, false)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(t, r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(t, r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while Postgres is down, want SERVING", got)
	}

	setDatabaseAvailable(t, url, true)
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(t, r, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestReadinessIgnoresKafkaOutage(t *testing.T) {
	// scenario: Domain SDK: A Kafka outage does not fail readiness
	url := migratedURL(t)
	pool := openPool(t, url)
	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	created, err := createDraft(ctx, r, "seller-outage")
	if err != nil {
		t.Fatalf("CreateListing with Kafka down: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got := healthStatus(t, r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
		}
		if got := healthStatus(t, r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("liveness = %v with Kafka unreachable, want SERVING", got)
		}
		time.Sleep(500 * time.Millisecond)
	}
	if got := healthStatus(t, r, "kafka"); got == grpc_health_v1.HealthCheckResponse_SERVING || got == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("a kafka health check is registered (status %v); readiness is postgres only", got)
	}
	var st string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM outbox_events WHERE aggregate_id = $1`, created.GetId()).Scan(&st); err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if st != "pending" {
		t.Fatalf("outbox row status = %q during a Kafka outage, want pending", st)
	}
}

func TestShutdownDrainsHealthBeforeServerStops(t *testing.T) {
	// scenario: Domain SDK: Shutdown drains health before the server stops
	url := migratedURL(t)
	s := domainSettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	watch := func(service string) grpc_health_v1.Health_WatchClient {
		wctx, wcancel := context.WithTimeout(t.Context(), 30*time.Second)
		t.Cleanup(wcancel)
		stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{Service: service})
		if err != nil {
			t.Fatal(err)
		}
		first, err := stream.Recv()
		if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("first watch of %q = %v, %v, want SERVING", service, first.GetStatus(), err)
		}
		return stream
	}
	ready, live := watch(""), watch("liveness")

	r.cancel()
	for name, stream := range map[string]grpc_health_v1.Health_WatchClient{"": ready, "liveness": live} {
		next, err := stream.Recv()
		if err != nil {
			t.Fatalf("watch %q ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", name, err)
		}
		if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Fatalf("watch %q after cancel = %v, want NOT_SERVING", name, next.GetStatus())
		}
	}
	drained := time.Now()
	if !portAccepts(s.Port) {
		t.Fatal("listener stopped accepting before the drain delay elapsed")
	}
	testkit.Eventually(t, 20*time.Second, func() bool { return !portAccepts(s.Port) })
	if gap := time.Since(drained); gap < 400*time.Millisecond {
		t.Fatalf("listener stopped %v after health drained, want at least 400ms", gap)
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

func TestShutdownRelaysInFlightWrite(t *testing.T) {
	// scenario: Domain SDK: An in-flight write is relayed during shutdown
	url := migratedURL(t)
	pool := openPool(t, url)
	brokers := testkit.Redpanda(t)
	s := domainSettings(testkit.FreePort(t), url, brokers)
	s.DrainDelay = time.Second
	s.ShutdownGrace = 10 * time.Second
	s.Outbox.PollInterval = 30 * time.Second
	r := start(t, s)
	waitServing(t, r)

	lock := dial(t, url)
	tx, err := lock.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(t.Context(), `LOCK TABLE listings IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock listings: %v", err)
	}
	released := false
	release := func() {
		if !released {
			released = true
			_ = tx.Rollback(context.WithoutCancel(t.Context()))
		}
	}
	t.Cleanup(release)

	type result struct {
		l   *listingv1.Listing
		err error
	}
	called := make(chan result, 1)
	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		l, err := createDraft(ctx, r, "seller-inflight")
		called <- result{l, err}
	}()
	waitLockWaiter(t, pool)

	wctx, wcancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("first watch: %v", err)
	}
	r.cancel()
	if next, err := stream.Recv(); err != nil || next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("watch after cancel = %v, %v, want NOT_SERVING", next.GetStatus(), err)
	}
	release()

	res := <-called
	if res.err != nil {
		t.Fatalf("in-flight CreateListing = %v, want OK", res.err)
	}
	select {
	case <-r.done:
	case <-time.After(30 * time.Second):
		t.Fatal("app did not stop")
	}
	if r.err != nil {
		t.Fatalf("Run = %v, want nil on shutdown", r.err)
	}
	var st string
	if err := pool.QueryRow(t.Context(),
		`SELECT status FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'platform.listing.v1.ListingChanged'`, res.l.GetId()).Scan(&st); err != nil {
		t.Fatalf("read outbox row of the in-flight write: %v", err)
	}
	if st != "published" {
		t.Fatalf("outbox row of the in-flight write = %q when Run returned, want published", st)
	}
	recs := testkit.ReadAll(t, brokers, listingTopic, 1, 30*time.Second)
	if string(recs[0].Key) != res.l.GetId() {
		t.Fatalf("relayed key = %q, want %q", recs[0].Key, res.l.GetId())
	}
}

type failingDLQ struct{ err error }

func (f failingDLQ) PublishRecord(context.Context, *kgo.Record) error { return f.err }

func TestConsumerFailureStopsRun(t *testing.T) {
	// scenario: Domain SDK: A failing component stops the process with its error
	dlqErr := errors.New("dlq-unavailable-for-test")
	app.SetDLQWrap(t, func(consume.DLQProducer) consume.DLQProducer { return failingDLQ{err: dlqErr} })
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	s := domainSettings(testkit.FreePort(t), url, brokers)
	r := start(t, s)
	waitServing(t, r)

	testkit.Produce(t, brokers, identityTopic, "poison", []byte{0xff, 0xff, 0xff, 0xff})
	testkit.Eventually(t, 60*time.Second, func() bool {
		select {
		case <-r.done:
			return true
		default:
			return false
		}
	})
	if r.err == nil || !strings.Contains(r.err.Error(), dlqErr.Error()) {
		t.Fatalf("Run = %v, want the consumer's error %q", r.err, dlqErr)
	}
	if portAccepts(s.Port) {
		t.Error("gRPC server still accepts after Run returned")
	}
}

func TestShutdownGraceBoundsHangingCall(t *testing.T) {
	// scenario: Domain SDK: A hanging call cannot outlive the grace period
	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-hang", Title: "t", Currency: "VND", Status: "published", SellerID: "seller-a", Stock: 1})
	s := domainSettings(testkit.FreePort(t), url, deadBroker)
	s.ShutdownGrace = time.Second
	s.DrainDelay = 0
	r := start(t, s)
	waitServing(t, r)

	lock := dial(t, url)
	tx, err := lock.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(t.Context(), `LOCK TABLE listings IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock listings: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.WithoutCancel(t.Context())) })

	go func() {
		ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		defer cancel()
		_, _ = listingv1.NewListingServiceClient(r.conn).GetListing(principal(ctx, "buyer-1", "user", "listing.read"),
			&listingv1.GetListingRequest{Id: "L-hang"})
	}()
	waitLockWaiter(t, pool)

	began := time.Now()
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return: a hanging call outlived the grace period")
	}
	if took := time.Since(began); took > 2*time.Second+s.DrainDelay {
		t.Fatalf("Run took %v after cancel, want within 1.5s plus the drain delay", took)
	}
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Domain SDK: Reflection is off by default
	url := migratedURL(t)
	r := start(t, domainSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
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
