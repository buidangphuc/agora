package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
)

func searchCall(g *gateway, bearer string) error {
	c := searchv1connect.NewSearchServiceClient(http.DefaultClient, g.url())
	req := connect.NewRequest(&searchv1.SearchListingsRequest{})
	if bearer != "" {
		req.Header().Set("Authorization", "Bearer "+bearer)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.SearchListings(ctx, req)
	return err
}

func TestHealthServingWhenDependenciesUp(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	g := startGateway(t, map[string]string{"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url()})

	waitReady(t, g, 10*time.Second)
	if st, err := checkOnce(t, g.addr(), "liveness"); err != nil || st != serving {
		t.Fatalf("liveness = %v, %v; want SERVING", st, err)
	}
}

func TestReadinessWaitsForJWKS(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	jwks.down.Store(true)
	g := startGateway(t, map[string]string{"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url()})

	waitLive(t, g)
	testkit.Eventually(t, 10*time.Second, func() bool { return jwks.hits.Load() >= 1 })
	holdsFor(t, 3*time.Second, 500*time.Millisecond, func() error {
		if st, err := checkOnce(t, g.addr(), ""); err != nil || st != notServing {
			return fmt.Errorf("readiness = %v, %w; want NOT_SERVING while the JWKS has never been fetched", st, err)
		}
		if st, err := checkOnce(t, g.addr(), "liveness"); err != nil || st != serving {
			return fmt.Errorf("liveness = %v, %w; want SERVING", st, err)
		}
		return nil
	})

	jwks.down.Store(false)
	waitReady(t, g, 12*time.Second)
}

func TestReadinessIgnoresKafkaAfterCatchUp(t *testing.T) {
	ctr, brokers := ownBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	g := startGateway(t, map[string]string{"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url()})
	waitReady(t, g, 20*time.Second)

	timeout := 20 * time.Second
	if err := ctr.Stop(context.Background(), &timeout); err != nil {
		t.Fatalf("stop broker: %v", err)
	}

	holdsFor(t, 12*time.Second, 250*time.Millisecond, func() error {
		if st, err := checkOnce(t, g.addr(), ""); err != nil || st != serving {
			return fmt.Errorf("readiness = %v, %w; want SERVING after a broker outage", st, err)
		}
		if st, err := checkOnce(t, g.addr(), "liveness"); err != nil || st != serving {
			return fmt.Errorf("liveness = %v, %w; want SERVING", st, err)
		}
		if code, _ := healthzStatus(t, g); code != http.StatusOK {
			return fmt.Errorf("/healthz = %d, want 200", code)
		}
		return nil
	})
}

func TestStartWithoutBrokerIsNotReadyUntilCaughtUp(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	bootstrap := closedAddr(t)
	g := startGateway(t, map[string]string{"KAFKA_BROKERS": bootstrap, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url()})

	waitLive(t, g)
	testkit.Eventually(t, 10*time.Second, func() bool { return jwks.okHit.Load() >= 1 })
	holdsFor(t, 3*time.Second, 500*time.Millisecond, func() error {
		if st, err := checkOnce(t, g.addr(), ""); err != nil || st != notServing {
			return fmt.Errorf("readiness = %v, %w; want NOT_SERVING without a broker", st, err)
		}
		if st, err := checkOnce(t, g.addr(), "liveness"); err != nil || st != serving {
			return fmt.Errorf("liveness = %v, %w; want SERVING", st, err)
		}
		if code, body := healthzStatus(t, g); code != http.StatusOK || body != "ok" {
			return fmt.Errorf("/healthz = %d %q, want 200 ok", code, body)
		}
		return nil
	})

	startForwarder(t, bootstrap, brokers)
	waitReady(t, g, 15*time.Second)
}

func startForwarder(t *testing.T, listen, target string) {
	t.Helper()
	lis, err := net.Listen("tcp", listen)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })
	go func() {
		for {
			in, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = in.Close() }()
				out, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer func() { _ = out.Close() }()
				go func() { _, _ = io.Copy(out, in) }()
				_, _ = io.Copy(in, out)
			}()
		}
	}()
}

func TestReadyAfterJWKSOutageWithCachedKeys(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	addr, _ := startSearchUpstream(t, false)
	g := startGateway(t, map[string]string{
		"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url(), "UPSTREAM_SEARCH_ADDR": addr,
	})
	waitReady(t, g, 12*time.Second)
	tok := jwks.mint(t, "sid-cached")
	if err := searchCall(g, tok); err != nil {
		t.Fatalf("token before the outage: %v", err)
	}

	jwks.srv.Close()

	holdsFor(t, 12*time.Second, 500*time.Millisecond, func() error {
		if st, err := checkOnce(t, g.addr(), ""); err != nil || st != serving {
			return fmt.Errorf("readiness = %v, %w; want SERVING after a JWKS outage", st, err)
		}
		return nil
	})
	if err := searchCall(g, tok); err != nil {
		t.Fatalf("token signed by a cached key must still verify: %v", err)
	}
}

func TestHealthUnknownServiceIsNotFound(t *testing.T) {
	g := startGateway(t, nil)
	waitLive(t, g)

	_, err := checkOnce(t, g.addr(), "billing")
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Check(billing) = %v, want NotFound", err)
	}
}

func TestHealthCheckOverGRPCProtocol(t *testing.T) {
	g := startGateway(t, nil)

	testkit.Eventually(t, 10*time.Second, statusIs(t, g, "liveness", serving))
}

func TestHealthzIsProcessLiveness(t *testing.T) {
	g := startGateway(t, nil)
	waitLive(t, g)

	if st, err := checkOnce(t, g.addr(), ""); err != nil || st != notServing {
		t.Fatalf("readiness = %v, %v; want NOT_SERVING with broker and JWKS unavailable", st, err)
	}
	code, body := healthzStatus(t, g)
	if code != http.StatusOK || body != "ok" {
		t.Fatalf("/healthz = %d %q, want 200 ok", code, body)
	}
}

func TestHealthDrainsBeforeServerStops(t *testing.T) {
	g := startGateway(t, map[string]string{"SHUTDOWN_DRAIN_DELAY": "1s", "SHUTDOWN_GRACE": "2s"})
	waitLive(t, g)
	hc := dialHealth(t, g.addr())
	if st, _, err := hc.check("liveness"); err != nil || st != serving {
		t.Fatalf("liveness before shutdown = %v, %v", st, err)
	}

	g.cancel()

	for _, svc := range []string{"liveness", ""} {
		st, _, err := hc.check(svc)
		if err != nil || st != notServing {
			t.Fatalf("Check(%q) during the drain delay = %v, %v; want NOT_SERVING", svc, st, err)
		}
	}
	conn, err := net.DialTimeout("tcp", g.addr(), time.Second)
	if err != nil {
		t.Fatalf("the listener must still accept connections during the drain delay: %v", err)
	}
	_ = conn.Close()
	if err := g.waitReturn(t, 4*time.Second); err != nil {
		t.Fatalf("Run after a graceful stop: %v", err)
	}
}

func TestRunShutsDownWithinGrace(t *testing.T) {
	addr, up := startSearchUpstream(t, true)
	g := startGateway(t, map[string]string{
		"SHUTDOWN_GRACE": "1s", "UPSTREAM_SEARCH_ADDR": addr, "CALL_TIMEOUT_SECONDS": "60",
	})
	waitLive(t, g)
	go func() { _ = searchCall(g, "") }()
	select {
	case <-up.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the in-flight request never reached the upstream")
	}

	start := time.Now()
	g.cancel()
	g.waitReturn(t, 3*time.Second)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Run took %s, want within 3s", took)
	}
	if conn, err := net.DialTimeout("tcp", g.addr(), time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("the listener must be closed after Run returned")
	}
}

func TestRunFailsWhenPortIsBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	port := busy.Addr().(*net.TCPAddr).Port

	g := startGateway(t, map[string]string{"GRPC_PORT": fmt.Sprint(port)})

	err = g.waitReturn(t, 5*time.Second)
	if err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Run error = %v, want the bind error (address already in use)", err)
	}
}

func TestRunStartsWhenJWKSUnreachable(t *testing.T) {
	addr, _ := startSearchUpstream(t, false)
	g := startGateway(t, map[string]string{"UPSTREAM_SEARCH_ADDR": addr})

	testkit.Eventually(t, 5*time.Second, func() bool {
		conn, err := net.DialTimeout("tcp", g.addr(), 500*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	})
	if err := searchCall(g, ""); err != nil {
		t.Fatalf("anonymous public read with identity down: %v", err)
	}
}

func TestJWKSPrimeRetriesInBackgroundWithoutTraffic(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	createTopic(t, brokers, topic)
	jwks := newJWKSStub(t)
	jwks.down.Store(true)
	g := startGateway(t, map[string]string{"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url()})
	waitLive(t, g)
	testkit.Eventually(t, 10*time.Second, func() bool { return jwks.hits.Load() >= 1 })

	jwks.down.Store(false)

	waitReady(t, g, 12*time.Second)
	fetched := jwks.okHit.Load()
	if fetched < 1 {
		t.Fatal("readiness is SERVING but the JWKS was never fetched successfully")
	}
	time.Sleep(7 * time.Second)
	if got := jwks.okHit.Load(); got != fetched {
		t.Fatalf("the background component fetched again after the first success: %d -> %d", fetched, got)
	}
}

func TestRevocationCheckWaitsForCatchUp(t *testing.T) {
	brokers := sharedBroker(t)
	topic := identityTopic(t)
	produceRevocations(t, brokers, topic, 50)
	jwks := newJWKSStub(t)
	addr, up := startSearchUpstream(t, false)
	g := startGateway(t, map[string]string{
		"KAFKA_BROKERS": brokers, "IDENTITY_EVENTS_TOPIC": topic, "JWKS_URL": jwks.url(), "UPSTREAM_SEARCH_ADDR": addr,
	})
	waitLive(t, g)

	waitReady(t, g, 12*time.Second)

	for _, sid := range []string{"sid-0", "sid-25", "sid-49"} {
		err := searchCall(g, jwks.mint(t, sid))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Fatalf("token of revoked %s right after readiness = %v, want unauthenticated", sid, err)
		}
	}
	if n := up.calls.Load(); n != 0 {
		t.Fatalf("a revoked token reached the upstream %d times", n)
	}
	if err := searchCall(g, jwks.mint(t, "sid-live")); err != nil {
		t.Fatalf("a session that was not revoked must pass: %v", err)
	}
}

func TestHealthCheckBypassesAuthAndRateLimit(t *testing.T) {
	addr, up := startSearchUpstream(t, false)
	g := startGateway(t, map[string]string{
		"RATE_LIMIT_RPS": "1", "RATE_LIMIT_BURST": "1", "UPSTREAM_SEARCH_ADDR": addr,
	})
	waitLive(t, g)
	hc := dialHealth(t, g.addr())

	for i := range 200 {
		st, hdr, err := hc.check("")
		if err != nil {
			t.Fatalf("call %d: %v; want a status, never unauthenticated, permission_denied or resource_exhausted", i, err)
		}
		if st != serving && st != notServing {
			t.Fatalf("call %d: status %v", i, st)
		}
		if v := hdr.Get("x-request-id"); len(v) != 0 {
			t.Fatalf("call %d: health response carries X-Request-Id %v", i, v)
		}
	}

	limited := 0
	for range 5 {
		if connect.CodeOf(searchCall(g, "")) == connect.CodeResourceExhausted {
			limited++
		}
	}
	if limited == 0 {
		t.Fatalf("the edge rate limit must still apply to routed calls (upstream saw %d calls)", up.calls.Load())
	}
}

func TestProducerIsBuiltWithoutEnableFlag(t *testing.T) {
	g := startGateway(t, map[string]string{"KAFKA_ENABLED": "false"})

	waitLive(t, g)
	holdsFor(t, 3*time.Second, 500*time.Millisecond, func() error {
		if st, err := checkOnce(t, g.addr(), ""); err != nil || st != notServing {
			return fmt.Errorf("readiness = %v, %w; want NOT_SERVING: KAFKA_ENABLED must not turn the broker off", st, err)
		}
		return nil
	})
	select {
	case err := <-g.done:
		g.done <- err
		t.Fatalf("Run returned: %v", err)
	default:
	}
}

func TestUnknownMockAdapterFailsBoot(t *testing.T) {
	g := startGateway(t, map[string]string{"MOCK_DATA": "payments"})

	err := g.waitReturn(t, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "payments") {
		t.Fatalf("Run error = %v, want one naming payments", err)
	}
	if conn, derr := net.DialTimeout("tcp", g.addr(), 500*time.Millisecond); derr == nil {
		_ = conn.Close()
		t.Fatal("the listen port must accept no connection")
	}
}

func TestOTelDisabledByDefault(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lis.Close() }()
	var accepted atomic.Int64
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()

	g := startGateway(t, map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "http://" + lis.Addr().String()})
	waitLive(t, g)
	g.cancel()
	if err := g.waitReturn(t, 10*time.Second); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n := accepted.Load(); n != 0 {
		t.Fatalf("%d OTLP connection(s) attempted with OTEL_ENABLED unset", n)
	}
}

func TestRuntimeDoesNotImportFakes(t *testing.T) {
	cmd := exec.Command("go", "list", "-e", "-deps", "-f", "{{.ImportPath}}", "./cmd/gateway")
	cmd.Dir = serviceRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list ./cmd/gateway: %v", err)
	}
	const own = "github.com/buidangphuc/"
	seenApp := false
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "github.com/buidangphuc/team-gateway/internal/app" {
			seenApp = true
		}
		if !strings.HasPrefix(pkg, own) {
			continue
		}
		base := path.Base(pkg)
		for _, bad := range []string{"test", "fake", "noop"} {
			if strings.HasSuffix(base, bad) {
				t.Errorf("the runtime binary imports %s", pkg)
			}
		}
	}
	if !seenApp {
		t.Error("cmd/gateway must import internal/app")
	}
}
