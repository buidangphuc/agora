package app

import (
	"context"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	"github.com/buidangphuc/team-audit/internal/config"
)

func TestReadinessFollowsDatabaseLivenessDoesNot(t *testing.T) {
	// scenario: Readiness fails while liveness stays serving
	url := testkit.Postgres(t)
	port := testkit.FreePort(t)
	s := config.Settings{
		Runtime:  sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		GRPC:     sdkconfig.GRPC{Host: "127.0.0.1", Port: port, ShutdownGrace: 5 * time.Second},
		Postgres: sdkconfig.Postgres{URL: url},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	b, err := build(ctx, s)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.pool == nil || b.health == nil || b.srv == nil || b.closeAll == nil {
		t.Fatalf("build returned an incomplete assembly: %+v", b)
	}
	t.Cleanup(func() {
		b.srv.Stop()
		_ = b.closeAll(context.WithoutCancel(ctx))
	})
	go func() { _ = b.srv.Serve(listen(t, port)) }()

	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	hc := grpc_health_v1.NewHealthClient(conn)

	status := func(service string) grpc_health_v1.HealthCheckResponse_ServingStatus {
		c, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		resp, err := hc.Check(c, &grpc_health_v1.HealthCheckRequest{Service: service}, grpc.WaitForReady(true))
		if err != nil {
			return grpc_health_v1.HealthCheckResponse_UNKNOWN
		}
		return resp.GetStatus()
	}

	testkit.Eventually(t, healthx.DefaultInterval+10*time.Second, func() bool {
		return status("") == grpc_health_v1.HealthCheckResponse_SERVING
	})

	b.pool.Close()

	testkit.Eventually(t, healthx.DefaultInterval+10*time.Second, func() bool {
		return status("") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := status("liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v, want SERVING while the pool is closed", got)
	}
}
