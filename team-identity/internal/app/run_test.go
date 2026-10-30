package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/jwtx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/config"
)

func runWithTimeout(t *testing.T, s config.Settings) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, s) }()
	select {
	case err := <-done:
		return err
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return; it is serving or hanging")
	}
	return nil
}

func TestRunFailsWhenDatabaseUnreachable(t *testing.T) {
	// scenario: An unreachable database fails boot locally
	s := newSettings(t, closedDatabaseURL, deadBroker)
	begin := time.Now()
	err := runWithTimeout(t, s)
	if err == nil {
		t.Fatal("Run returned nil, want a boot error")
	}
	if d := time.Since(begin); d > 10*time.Second {
		t.Errorf("Run took %v to fail, want under 10s", d)
	}
	assertPortFree(t, s.Port)
	assertPortFree(t, s.JWT.JWKSHTTPPort)
}

func TestRunRefusesDevSigningKeyBeforeConnecting(t *testing.T) {
	// scenario: The development signing key is refused by Run in a deployed ENV
	devPEM := committedDevPEM(t)
	var bodyLines []string
	for _, l := range strings.Split(devPEM, "\n") {
		if l != "" && !strings.HasPrefix(l, "-----") {
			bodyLines = append(bodyLines, l)
		}
	}
	cases := []struct {
		name, kid, key string
	}{
		{"dev kid with a fresh key", "dev-2026", testSigningKey(t)},
		{"committed dev key under another kid", "renamed-kid", devPEM},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			trap := newTCPTrap(t)
			s := newSettings(t, trap.url, deadBroker)
			s.Env = "production"
			s.JWT.KID = tc.kid
			s.JWT.PrivateKey = tc.key

			err := runWithTimeout(t, s)
			if err == nil {
				t.Fatal("Run returned nil, want a refusal")
			}
			msg := err.Error()
			if !strings.Contains(msg, "refusing to start") {
				t.Errorf("error %q does not contain %q", msg, "refusing to start")
			}
			if !strings.Contains(msg, "JWT_KID") && !strings.Contains(msg, "JWT_PRIVATE_KEY") {
				t.Errorf("error %q names neither JWT_KID nor JWT_PRIVATE_KEY", msg)
			}
			for _, l := range bodyLines {
				if strings.Contains(msg, l[:min(len(l), 40)]) {
					t.Fatalf("error text carries key material: %q", l[:min(len(l), 40)])
				}
			}
			if n := trap.accepted(); n != 0 {
				t.Errorf("Run connected to the database %d time(s) before refusing the key", n)
			}
			assertPortFree(t, s.Port)
			assertPortFree(t, s.JWT.JWKSHTTPPort)
		})
	}
}

func TestRunFailsOnUnparsableSigningKey(t *testing.T) {
	// scenario: An unusable signing key fails boot in every ENV
	trap := newTCPTrap(t)
	s := newSettings(t, trap.url, deadBroker)
	s.JWT.PrivateKey = "this is not a PEM RSA key"

	if err := runWithTimeout(t, s); err == nil {
		t.Fatal("Run returned nil, want an error for an unusable signing key")
	}
	if n := trap.accepted(); n != 0 {
		t.Errorf("Run connected to the database %d time(s) before failing on the key", n)
	}
	assertPortFree(t, s.Port)
	assertPortFree(t, s.JWT.JWKSHTTPPort)
}

func TestReadinessFollowsDatabaseLivenessDoesNot(t *testing.T) {
	// scenario: Readiness fails while liveness stays serving
	url := migratedURL(t)
	s := newSettings(t, url, deadBroker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b, err := build(ctx, s, testkit.NewRecorder())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if b.pool == nil || b.health == nil || b.relayer == nil || b.purgeTick == nil || b.run == nil || b.closeAll == nil {
		t.Fatalf("build returned an incomplete assembly: %+v", b)
	}
	done := make(chan struct{})
	go func() { _ = b.run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("run did not stop")
		}
	})
	conn := dialApp(t, s.Port)
	waitLive(t, conn, done)
	testkit.Eventually(t, 30*time.Second, func() bool {
		return healthStatus(conn, "") == grpc_health_v1.HealthCheckResponse_SERVING
	})

	b.pool.Close()

	testkit.Eventually(t, healthWindow, func() bool {
		return healthStatus(conn, "") == grpc_health_v1.HealthCheckResponse_NOT_SERVING
	})
	if got := healthStatus(conn, "liveness"); got != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("liveness = %v with the pool closed, want SERVING", got)
	}
}

func TestUnknownHealthServiceNotFound(t *testing.T) {
	// scenario: An unknown health service name is not found
	r := start(t, newSettings(t, migratedURL(t), deadBroker))
	waitLive(t, r.conn, r.done)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := grpc_health_v1.NewHealthClient(r.conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: "nope"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("Check(nope) = %v, want NotFound", err)
	}
}

func TestReflectionOffByDefault(t *testing.T) {
	// scenario: Reflection is not served by default
	r := start(t, newSettings(t, migratedURL(t), deadBroker))
	waitLive(t, r.conn, r.done)

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

func TestShutdownDrainsHealthFirst(t *testing.T) {
	// scenario: Shutdown drains health first
	url := migratedURL(t)
	audit := &fakeAudit{block: make(chan struct{}), entered: make(chan struct{}, 1)}
	s := newSettings(t, url, deadBroker)
	s.Upstream.AuditAddr = startAudit(t, audit)
	s.DrainDelay = 1500 * time.Millisecond
	r := start(t, s)
	waitServing(t, r)

	rctx, rcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer rcancel()
	reg, err := identityv1.NewAuthServiceClient(r.conn).Register(rctx, &identityv1.RegisterRequest{Username: "target", Password: testPassword, Role: "buyer"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	targetID := reg.GetResult().GetPrincipal().GetId()

	wctx, wcancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer wcancel()
	var streams []grpc_health_v1.Health_WatchClient
	for _, name := range []string{"", "liveness"} {
		st, err := grpc_health_v1.NewHealthClient(r.conn).Watch(wctx, &grpc_health_v1.HealthCheckRequest{Service: name})
		if err != nil {
			t.Fatal(err)
		}
		first, err := st.Recv()
		if err != nil || first.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
			t.Fatalf("first watch of %q = %v, %v, want SERVING", name, first.GetStatus(), err)
		}
		streams = append(streams, st)
	}

	callDone := make(chan error, 1)
	go func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := identityv1.NewAccountAdminServiceClient(r.conn).SetUserStatus(asUser(c, "admin-1", "admin"), &identityv1.SetUserStatusRequest{
			UserId: targetID, Status: identityv1.UserStatus_USER_STATUS_SUSPENDED, ReasonCode: "abuse",
			Until: timestamppb.New(time.Now().Add(time.Hour)),
		})
		callDone <- err
	}()
	select {
	case <-audit.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("the in-flight call never reached the audit writer")
	}

	r.cancel()
	for i, st := range streams {
		next, err := st.Recv()
		if err != nil {
			t.Fatalf("watch %d ended with %v before reporting NOT_SERVING; health must drain before gRPC stops", i, err)
		}
		if next.GetStatus() != grpc_health_v1.HealthCheckResponse_NOT_SERVING {
			t.Fatalf("watch %d after cancel = %v, want NOT_SERVING", i, next.GetStatus())
		}
	}
	select {
	case err := <-callDone:
		t.Fatalf("in-flight call finished (%v) before health drained", err)
	case <-r.done:
		t.Fatal("Run returned while a call was still in flight")
	default:
	}

	close(audit.block)
	if err := <-callDone; err != nil {
		t.Errorf("in-flight call = %v, want it to complete", err)
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

func TestJWKSListenerServesAndDrains(t *testing.T) {
	// scenario: The listener starts with the service and drains on shutdown
	s := newSettings(t, migratedURL(t), deadBroker)
	r := start(t, s)
	waitLive(t, r.conn, r.done)

	url := "http://127.0.0.1:" + strconv.Itoa(s.JWT.JWKSHTTPPort) + "/.well-known/jwks.json"
	client := &http.Client{Timeout: 3 * time.Second}
	var set jwtx.JWKS
	testkit.Eventually(t, 15*time.Second, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		return json.NewDecoder(resp.Body).Decode(&set) == nil
	})
	if len(set.Keys) != 1 || set.Keys[0].Kid != "test-kid" {
		t.Fatalf("jwks = %+v, want one key with kid test-kid", set)
	}

	r.stop(t)
	if r.err != nil {
		t.Errorf("Run = %v, want nil", r.err)
	}
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(s.JWT.JWKSHTTPPort), time.Second); err == nil {
		_ = c.Close()
		t.Error("the JWKS port still accepts connections after Run returned")
	}
}

func TestRunSeedsNoAdminByDefault(t *testing.T) {
	// scenario: Default configuration creates no admin
	url := migratedURL(t)
	r := start(t, newSettings(t, url, deadBroker))
	waitServing(t, r)
	r.stop(t)
	if n := queryInt(t, url, `SELECT count(*) FROM users WHERE 'admin' = ANY(roles)`); n != 0 {
		t.Fatalf("admin users = %d, want 0", n)
	}
}

func TestRunSeedsAdminOnce(t *testing.T) {
	// scenario: Enabled seeding creates the admin once
	url := migratedURL(t)
	const adminPassword = "correct-horse-battery"
	var last *running
	for i := 0; i < 2; i++ {
		s := newSettings(t, url, deadBroker)
		s.SeedAdmin = config.SeedAdmin{Enabled: true, Username: "root-admin", Password: adminPassword}
		last = start(t, s)
		waitServing(t, last)
		if i == 0 {
			last.stop(t)
		}
	}
	if n := queryInt(t, url, `SELECT count(*) FROM users WHERE 'admin' = ANY(roles)`); n != 1 {
		t.Fatalf("admin users = %d, want exactly 1", n)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := identityv1.NewAuthServiceClient(last.conn).Login(ctx, &identityv1.LoginRequest{Username: "root-admin", Password: adminPassword})
	if err != nil {
		t.Fatalf("Login as the seeded admin = %v", err)
	}
	var admin bool
	for _, sc := range res.GetResult().GetPrincipal().GetScopes() {
		admin = admin || sc == "admin"
	}
	if !admin {
		t.Errorf("scopes = %v, want admin", res.GetResult().GetPrincipal().GetScopes())
	}
}

func TestSweeperRunsAtStart(t *testing.T) {
	// scenario: Session retention: the sweeper always runs
	url := migratedURL(t)
	execSQL(t, url, `INSERT INTO users (id, username, password_hash) VALUES ('u-old', 'old', 'h')`)
	execSQL(t, url, `INSERT INTO sessions (id, user_id, revoked, revoked_at, last_seen) VALUES ('s-old', 'u-old', true, now() - interval '90 days', now() - interval '90 days')`)
	s := newSettings(t, url, deadBroker)
	s.Retention.SweepInterval = time.Hour
	r := start(t, s)
	waitLive(t, r.conn, r.done)

	testkit.Eventually(t, 5*time.Second, func() bool {
		return queryInt(t, url, `SELECT count(*) FROM sessions WHERE id = 's-old'`) == 0
	})
}

func TestShutdownStopsSweeperBeforePoolCloses(t *testing.T) {
	// scenario: Session retention: shutdown drains the sweeper
	url := migratedURL(t)
	execSQL(t, url, `INSERT INTO users (id, username, password_hash) VALUES ('u-old', 'old', 'h')`)
	execSQL(t, url, `INSERT INTO sessions (id, user_id, revoked, revoked_at, last_seen) VALUES ('s-old', 'u-old', true, now() - interval '90 days', now() - interval '90 days')`)
	execSQL(t, url, `CREATE FUNCTION slow_session_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(30); RETURN OLD; END $$`)
	execSQL(t, url, `CREATE TRIGGER slow_session_delete BEFORE DELETE ON sessions FOR EACH ROW EXECUTE FUNCTION slow_session_delete()`)

	logs := captureLogs(t)
	s := newSettings(t, url, deadBroker)
	s.LogLevel = "info"
	r := start(t, s)
	waitLive(t, r.conn, r.done)
	testkit.Eventually(t, 15*time.Second, func() bool {
		return queryInt(t, url, `SELECT count(*) FROM pg_stat_activity WHERE state = 'active' AND query ILIKE '%DELETE FROM sessions%' AND query NOT ILIKE '%pg_stat_activity%'`) > 0
	})

	r.stop(t)
	if r.err != nil {
		t.Errorf("Run = %v, want nil", r.err)
	}
	for _, l := range logs.Lines() {
		if lvl, _ := l["level"].(string); strings.EqualFold(lvl, "ERROR") {
			t.Errorf("error logged during shutdown: %v", l)
		}
		if msg, _ := l["msg"].(string); strings.Contains(strings.ToLower(msg), "closed pool") {
			t.Errorf("sweeper ran after the pool closed: %v", l)
		}
	}
}

var _ outbox.Publisher = (*testkit.Recorder)(nil)
