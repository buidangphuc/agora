package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/lifecycle"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-order/internal/app"
	"github.com/buidangphuc/team-order/internal/config"
)

func runExpectingError(t *testing.T, s config.Settings) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, s) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil, want a boot error")
		}
		return err
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving or hanging")
	}
	return nil
}

func TestRunFailsWhenPostgresUnreachable(t *testing.T) {
	// scenario: An unreachable Postgres stops the process
	s := orderSettings(testkit.FreePort(t), closedDatabaseURL, deadBroker)
	err := runExpectingError(t, s)
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "postgres") && !strings.Contains(low, "database") {
		t.Fatalf("boot error %q does not name the database", err)
	}
	assertPortFree(t, s.Port)
}

func TestRunRefusesEmptyFliptAddr(t *testing.T) {
	// scenario: An empty Flipt address refuses startup
	url := migratedURL(t)
	s := orderSettings(testkit.FreePort(t), url, deadBroker)
	s.Flags.FliptAddr = ""
	err := runExpectingError(t, s)
	if !strings.Contains(err.Error(), "FLIPT_ADDR") {
		t.Fatalf("boot error %q does not name FLIPT_ADDR", err)
	}
	assertPortFree(t, s.Port)
}

func TestRunDoesNotMigrate(t *testing.T) {
	// scenario: Boot never migrates
	url := freshURL(t)
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	select {
	case <-r.done:
	default:
		_, _ = grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"}, grpc.WaitForReady(true))
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}
	for _, table := range []string{"orders", "order_outbox_events", "processed_events", "order_sagas", "cart_items"} {
		if tableExists(t, url, table) {
			t.Errorf("table %s exists after boot; boot must never migrate", table)
		}
	}
}

func TestReadinessServesWhenChecksPass(t *testing.T) {
	// scenario: A passing Postgres check makes readiness serve
	url := migratedURL(t)
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING", got)
	}
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v, want SERVING", got)
	}
}

func TestLivenessSurvivesDependencyOutage(t *testing.T) {
	// scenario: A failing Postgres check makes readiness fail and keeps liveness serving
	url := migratedURL(t)
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	setDatabaseAvailable(t, url, false)
	t.Cleanup(func() { setDatabaseAvailable(t, url, true) })
	testkit.Eventually(t, healthWindow+10*time.Second, func() bool {
		return healthStatus(r, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v while Postgres is down, want SERVING", got)
	}
}

func TestKafkaOutageDoesNotAffectReadiness(t *testing.T) {
	// scenario: A Kafka outage does not affect readiness
	url := migratedURL(t)
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)

	time.Sleep(healthWindow + time.Second)
	if got := healthStatus(r, ""); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("readiness = %v with Kafka unreachable, want SERVING", got)
	}
	if got := healthStatus(r, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v with Kafka unreachable, want SERVING", got)
	}
}

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Shutdown drains liveness before the server stops
	url := migratedURL(t)
	s := orderSettings(testkit.FreePort(t), url, deadBroker)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	stream, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{Service: "liveness"})
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
		t.Fatalf("watch ended with %v before reporting NOT_SERVING; liveness must drain before gRPC stops", err)
	}
	if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("liveness after cancel = %v, want NOT_SERVING", next.GetStatus())
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
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
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

func TestShutdownStopsServerBeforeDependents(t *testing.T) {
	// scenario: Shutdown stops the server before the dependents
	var mu sync.Mutex
	var exits []string
	app.SetComponentHook(t, func(name string, c lifecycle.Component) lifecycle.Component {
		return lifecycle.Func(func(ctx context.Context) error {
			err := c.Run(ctx)
			mu.Lock()
			exits = append(exits, name)
			mu.Unlock()
			return err
		})
	})

	url := migratedURL(t)
	r := start(t, orderSettings(testkit.FreePort(t), url, deadBroker))
	waitServing(t, r)
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("app did not stop")
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"relayer", "payment-consumer", "dispute-consumer", "reservation-sweep", "lifecycle-sweep", "outbox-purge"}
	if len(exits) != len(want)+1 {
		t.Fatalf("components that returned = %v, want server plus %v (wrap each with the component hook under these names)", exits, want)
	}
	if exits[0] != "server" {
		t.Fatalf("exit order = %v, want the server first", exits)
	}
	seen := map[string]bool{}
	for _, n := range exits[1:] {
		seen[n] = true
	}
	for _, n := range want {
		if !seen[n] {
			t.Errorf("component %q never returned; exits = %v", n, exits)
		}
	}
}
