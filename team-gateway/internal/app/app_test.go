package app_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	sdkconfig "github.com/buidangphuc/platform-core/packages/go-sdk/pkg/config"

	"github.com/buidangphuc/team-gateway/internal/app"
	"github.com/buidangphuc/team-gateway/internal/config"
)

// freePort asks the kernel for an unused port and releases it for the gateway.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// testSettings builds Settings by hand: every upstream, JWKS and Kafka address
// points at a closed local port, so nothing outside the process is needed.
func testSettings(port int) config.Settings {
	dead := "127.0.0.1:1"
	return config.Settings{
		Runtime: sdkconfig.Runtime{Env: "local", LogLevel: "error", LogJSON: true},
		Kafka:   sdkconfig.Kafka{Brokers: dead},
		Server:  config.Server{Host: "127.0.0.1", Port: port, ShutdownGrace: 10},
		Upstream: config.Upstream{
			SearchAddr: dead, ListingAddr: dead, IdentityAddr: dead, EngagementAddr: dead,
			OrderAddr: dead, PaymentAddr: dead, ChatAddr: dead, AIAddr: dead, RecAddr: dead,
			PromotionAddr: dead, NotificationAddr: dead, AnalyticsAddr: dead,
			ReferralAddr: dead, VerificationAddr: dead, SharingAddr: dead, AuditAddr: dead,
			DialTimeout: 1,
		},
		Auth:     config.Auth{JWKSURL: "http://" + dead + "/jwks", JWKSCacheTTLSeconds: 300, PublicScopes: "listing.read"},
		Edge:     config.Edge{RateLimitRPS: 1000, RateLimitBurst: 2000, CallTimeoutSecs: 1, TrackRateLimitRPS: 1000, TrackRateLimitBurst: 2000, CORSOrigins: "http://localhost:3000"},
		Events:   config.Events{AnalyticsTopic: "analytics.events"},
		Identity: config.IdentityEvents{Topic: "identity.events"},
	}
}

func waitHealthy(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("gateway did not become healthy")
}

// TestShutdownDrainsInFlightRequest: cancelling the gateway context stops the
// listener at once but lets a request that is already running finish.
func TestShutdownDrainsInFlightRequest(t *testing.T) {
	port := freePort(t)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	base := "http://" + addr

	started := make(chan struct{})
	release := make(chan struct{})
	slow := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/slow" {
				next.ServeHTTP(w, r)
				return
			}
			close(started)
			<-release
			_, _ = w.Write([]byte("finished"))
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() {
		runDone <- app.RunWith(ctx, testSettings(port), app.Options{Wrap: slow, LogOut: io.Discard})
	}()
	waitHealthy(t, base)

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		got <- result{body: string(b), err: err}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("slow request never reached the handler")
	}

	cancel()

	// New connections are refused once shutdown begins ...
	refused := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			refused = true
			break
		}
		_ = c.Close()
		time.Sleep(20 * time.Millisecond)
	}
	if !refused {
		t.Error("the listener still accepts connections after shutdown began")
	}
	// ... while the request in flight is neither finished nor cut off.
	select {
	case r := <-got:
		t.Fatalf("in-flight request ended before release: %+v", r)
	case err := <-runDone:
		t.Fatalf("Run returned with a request still in flight: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	r := <-got
	if r.err != nil || r.body != "finished" {
		t.Fatalf("in-flight request = %+v, want body %q and no error", r, "finished")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run = %v, want nil after a clean drain", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the drain")
	}
}
