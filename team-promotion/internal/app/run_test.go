package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/app"
)

func TestRunFailsOnUnreachableDatabase(t *testing.T) {
	// scenario: Unreachable database fails boot
	s := promoSettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	done := make(chan error, 1)
	go func() { done <- app.Run(t.Context(), s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		if !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("boot error %q does not name postgres as the failed dependency", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Run did not fail within 8s; it is serving or hanging")
	}
	assertPortFree(t, s.Port)
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Boot does not migrate
	url := freshURL(t)
	r := start(t, promoSettings(testkit.FreePort(t), url, deadBroker))
	waitLiveness(t, r)
	for _, table := range []string{"schema_migrations", "vouchers"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; the service must not migrate", table)
		}
	}
}

func TestHealthServingAfterBoot(t *testing.T) {
	// scenario: Ready and live after boot
	r := start(t, promoSettings(testkit.FreePort(t), migratedURL(t), deadBroker))
	waitServing(t, r)
	waitLiveness(t, r)
	testkit.Eventually(t, healthWindow, func() bool {
		return healthStatus(t, r, "") == grpc_health_v1.HealthCheckResponse_SERVING &&
			healthStatus(t, r, "liveness") == grpc_health_v1.HealthCheckResponse_SERVING
	})
}

func TestReadinessFlipsLivenessStays(t *testing.T) {
	// scenario: A failing database check flips readiness only
	url := migratedURL(t)
	r := start(t, promoSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	testkit.Eventually(t, 2*healthWindow+10*time.Second, func() bool {
		return healthStatus(t, r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(t, r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while the database is down, want SERVING", got)
	}
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Shutdown drains health before the stop
	url := migratedURL(t)
	id, listing, price := seedCampaign(t, url, 5)
	s := promoSettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	wctx, wcancel := context30(t)
	defer wcancel()
	hc := grpc_health_v1.NewHealthClient(r.conn)
	streams := map[string]grpc_health_v1.Health_WatchClient{}
	for _, svc := range []string{"", "liveness"} {
		st, err := hc.Watch(wctx, &grpc_health_v1.HealthCheckRequest{Service: svc})
		if err != nil {
			t.Fatal(err)
		}
		first, err := st.Recv()
		if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("first watch %q = %v, %v, want SERVING", svc, first.GetStatus(), err)
		}
		streams[svc] = st
	}

	r.cancel()
	for svc, st := range streams {
		next, err := st.Recv()
		if err != nil {
			t.Fatalf("watch %q ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", svc, err)
		}
		if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Fatalf("watch %q after cancel = %v, want NOT_SERVING", svc, next.GetStatus())
		}
	}
	select {
	case <-r.done:
		t.Fatal("Run returned before the drain delay elapsed")
	default:
	}
	rctx, rcancel := context30(t)
	defer rcancel()
	resp, err := promotionv1.NewFlashSaleServiceClient(r.conn).ReserveFlashSaleStock(asOrderService(rctx), &promotionv1.ReserveFlashSaleStockRequest{
		ReservationId: "drain-1", CampaignId: id, ListingId: listing, Quantity: 1, ExpectedSalePrice: price,
	})
	if err != nil || !resp.GetReserved() {
		t.Fatalf("reserve during drain = %v %+v, want reserved=true", err, resp)
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

func TestKafkaOutageKeepsReadiness(t *testing.T) {
	// scenario: A Kafka outage keeps readiness SERVING
	r := start(t, promoSettings(testkit.FreePort(t), migratedURL(t), deadBroker))
	waitServing(t, r)
	ctx, cancel := context30(t)
	defer cancel()
	if _, err := promotionv1.NewVoucherServiceClient(r.conn).CreateVoucher(asSeller(ctx), shopVoucher("OUTAGE-READY")); err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if got := healthStatus(t, r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(t, r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(t, r, "kafka"); got == grpc_health_v1.HealthCheckResponse_SERVING || got == grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Errorf("a kafka health check is registered (status %v); readiness is postgres only", got)
	}
}

func TestKafkaOutageKeepsWriteAndRelaysLater(t *testing.T) {
	// scenario: A Kafka outage does not fail the write
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	fwd := newForwarder(t, testkit.FreePort(t), brokers)
	s := promoSettings(testkit.FreePort(t), url, fwd.listen)
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context30(t)
	defer cancel()
	resp, err := promotionv1.NewVoucherServiceClient(r.conn).CreateVoucher(asSeller(ctx), shopVoucher("OUTAGE-WRITE"))
	if err != nil {
		t.Fatalf("CreateVoucher during outage: %v", err)
	}
	vid := resp.GetVoucher().GetId()
	conn := dial(t, url)
	var published bool
	err = conn.QueryRow(t.Context(),
		`SELECT status = 'published' FROM promotion_outbox WHERE aggregate_id = $1`, vid).Scan(&published)
	if err != nil {
		t.Fatalf("read outbox row: %v", err)
	}
	if published {
		t.Fatal("row is published although the brokers are unreachable")
	}

	fwd.open(t)
	testkit.Eventually(t, 90*time.Second, func() bool {
		var ok bool
		if err := conn.QueryRow(t.Context(),
			`SELECT status = 'published' FROM promotion_outbox WHERE aggregate_id = $1`, vid).Scan(&ok); err != nil {
			return false
		}
		return ok
	})
	recs := testkit.ReadAll(t, brokers, eventsTopic, 1, 30*time.Second)
	if string(recs[0].Key) != vid {
		t.Errorf("relayed record key = %q, want voucher id %q", recs[0].Key, vid)
	}
}

func TestUnreachableFliptBootsAndFailsOpen(t *testing.T) {
	// scenario: Unreachable Flipt still boots and fails open
	url := migratedURL(t)
	id, listing, price := seedCampaign(t, url, 5)
	s := promoSettings(testkit.FreePort(t), url, deadBroker)
	s.Flags.FliptAddr = deadBroker
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context30(t)
	defer cancel()
	resp, err := promotionv1.NewFlashSaleServiceClient(r.conn).ReserveFlashSaleStock(asOrderService(ctx), &promotionv1.ReserveFlashSaleStockRequest{
		ReservationId: "flipt-1", CampaignId: id, ListingId: listing, Quantity: 1, ExpectedSalePrice: price,
	})
	if err != nil || !resp.GetReserved() {
		t.Fatalf("reserve with Flipt unreachable = %v %+v, want reserved=true", err, resp)
	}
}

func TestOutboxPurgeKeepsRecentAndUnpublished(t *testing.T) {
	// scenario: Old published rows are purged
	url := migratedURL(t)
	conn := dial(t, url)
	const ins = `INSERT INTO promotion_outbox
		(event_id, aggregate_type, aggregate_id, event_type, payload, status, attempts, available_at, created_at, published_at)
		VALUES ($1, 'Voucher', $1, 'platform.promotion.v1.VoucherChanged', $2, $3, 1, now() + interval '1 day', $4, $5)`
	now := time.Now()
	old, recent := now.Add(-8*24*time.Hour), now.Add(-6*24*time.Hour)
	seeds := []struct {
		id, status string
		at, pub    any
	}{
		{"evt-old", "published", old, old},
		{"evt-recent", "published", recent, recent},
		{"evt-pending", "pending", old, nil},
	}
	for _, sd := range seeds {
		if _, err := conn.Exec(t.Context(), ins, sd.id, []byte(sd.id), sd.status, sd.at, sd.pub); err != nil {
			t.Fatalf("seed %s: %v", sd.id, err)
		}
	}

	r := start(t, promoSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	count := func(id string) int {
		var n int
		if err := conn.QueryRow(t.Context(), `SELECT count(*) FROM promotion_outbox WHERE event_id = $1`, id).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", id, err)
		}
		return n
	}
	testkit.Eventually(t, 30*time.Second, func() bool { return count("evt-old") == 0 })
	for _, id := range []string{"evt-recent", "evt-pending"} {
		if count(id) != 1 {
			t.Errorf("row %s was purged", id)
		}
	}
}

func context30(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(t.Context(), 30*time.Second)
}

func shopVoucher(code string) *promotionv1.CreateVoucherRequest {
	return &promotionv1.CreateVoucherRequest{
		Code: code, Scope: promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}
}
