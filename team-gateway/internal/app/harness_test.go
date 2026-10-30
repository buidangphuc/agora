package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/testcontainers/testcontainers-go"
	tcredpanda "github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/kmsg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-gateway/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/internal/config"
	"github.com/buidangphuc/team-gateway/internal/token"
)

const (
	redpandaImage     = "docker.redpanda.com/redpandadata/redpanda:v24.3.1"
	sessionRevokedTyp = "platform.identity.v1.SessionRevoked"
	serving           = grpc_health_v1.HealthCheckResponse_SERVING
	notServing        = grpc_health_v1.HealthCheckResponse_NOT_SERVING
)

var shared struct {
	once    sync.Once
	ctr     *tcredpanda.Container
	brokers string
	err     error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if shared.ctr != nil {
		_ = testcontainers.TerminateContainer(shared.ctr)
	}
	os.Exit(code)
}

func startRedpanda(ctx context.Context) (*tcredpanda.Container, string, error) {
	ctr, err := tcredpanda.Run(ctx, redpandaImage, tcredpanda.WithAutoCreateTopics())
	if err != nil {
		return ctr, "", err
	}
	brokers, err := ctr.KafkaSeedBroker(ctx)
	return ctr, brokers, err
}

func waitBroker(t *testing.T, brokers string) {
	t.Helper()
	testkit.Eventually(t, 30*time.Second, func() bool {
		cl, err := kgo.NewClient(kgo.SeedBrokers(brokers))
		if err != nil {
			return false
		}
		defer cl.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return cl.Ping(ctx) == nil
	})
}

func sharedBroker(t *testing.T) string {
	t.Helper()
	shared.once.Do(func() {
		shared.ctr, shared.brokers, shared.err = startRedpanda(context.Background())
	})
	if shared.err != nil {
		t.Fatalf("start redpanda: %v", shared.err)
	}
	waitBroker(t, shared.brokers)
	return shared.brokers
}

func ownBroker(t *testing.T) (*tcredpanda.Container, string) {
	t.Helper()
	ctr, brokers, err := startRedpanda(context.Background())
	if ctr != nil {
		t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	}
	if err != nil {
		t.Fatalf("start redpanda: %v", err)
	}
	waitBroker(t, brokers)
	return ctr, brokers
}

func identityTopic(t *testing.T) string {
	return "identity.events." + strings.ReplaceAll(t.Name(), "/", "-")
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

func revocationEnvelope(t *testing.T, sid string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&identityv1.SessionRevoked{SessionId: sid, UserId: "u1", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e-" + sid, Type: sessionRevokedTyp, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func produceRevocations(t *testing.T, brokers, topic string, n int) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	var recs []*kgo.Record
	for i := range n {
		sid := fmt.Sprintf("sid-%d", i)
		recs = append(recs, &kgo.Record{Topic: topic, Key: []byte(sid), Value: revocationEnvelope(t, sid)})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cl.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		t.Fatal(err)
	}
}

func closedAddr(t *testing.T) string {
	t.Helper()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(testkit.FreePort(t)))
}

type jwksStub struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	down  atomic.Bool
	hits  atomic.Int64
	okHit atomic.Int64
}

func newJWKSStub(t *testing.T) *jwksStub {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	j := &jwksStub{key: key}
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	j.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		j.hits.Add(1)
		if j.down.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		j.okHit.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(j.srv.Close)
	return j
}

func (j *jwksStub) url() string { return j.srv.URL }

func (j *jwksStub) mint(t *testing.T, sid string) string {
	t.Helper()
	claims := &token.Claims{
		Type:      "user",
		Scopes:    []string{"search:read"},
		SessionID: sid,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(j.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type searchUpstream struct {
	searchv1.UnimplementedSearchServiceServer
	calls   atomic.Int64
	block   chan struct{}
	entered chan struct{}
}

func (s *searchUpstream) SearchListings(ctx context.Context, _ *searchv1.SearchListingsRequest) (*searchv1.SearchListingsResponse, error) {
	s.calls.Add(1)
	if s.block != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
		select {
		case <-s.block:
		case <-ctx.Done():
		}
	}
	return &searchv1.SearchListingsResponse{}, nil
}

func startSearchUpstream(t *testing.T, blocking bool) (string, *searchUpstream) {
	t.Helper()
	up := &searchUpstream{}
	if blocking {
		up.block = make(chan struct{})
		up.entered = make(chan struct{}, 1)
	}
	addr := closedAddr(t)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer()
	searchv1.RegisterSearchServiceServer(srv, up)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		if up.block != nil {
			close(up.block)
		}
		srv.Stop()
	})
	return addr, up
}

type gateway struct {
	port   int
	cancel context.CancelFunc
	done   chan error
}

func startGateway(t *testing.T, env map[string]string) *gateway {
	t.Helper()
	port := testkit.FreePort(t)
	vars := map[string]string{
		"ENV":            "local",
		"GRPC_HOST":      "127.0.0.1",
		"GRPC_PORT":      strconv.Itoa(port),
		"SHUTDOWN_GRACE": "2s",
		"JWKS_URL":       "http://" + closedAddr(t) + "/jwks",
		"KAFKA_BROKERS":  closedAddr(t),
	}
	for k, v := range env {
		vars[k] = v
	}
	for k, v := range vars {
		t.Setenv(k, v)
	}
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	g := &gateway{cancel: cancel, done: make(chan error, 1)}
	g.port, _ = strconv.Atoi(vars["GRPC_PORT"])
	go func() { g.done <- Run(ctx, s) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-g.done:
			g.done <- err
		case <-time.After(20 * time.Second):
			t.Error("Run did not return after the context was cancelled")
		}
	})
	return g
}

func (g *gateway) addr() string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(g.port)) }
func (g *gateway) url() string  { return "http://" + g.addr() }

func (g *gateway) waitReturn(t *testing.T, within time.Duration) error {
	t.Helper()
	select {
	case err := <-g.done:
		g.done <- err
		return err
	case <-time.After(within):
		t.Fatalf("Run did not return within %s", within)
		return nil
	}
}

type healthConn struct {
	conn   *grpc.ClientConn
	client grpc_health_v1.HealthClient
}

func dialHealth(t *testing.T, addr string) *healthConn {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &healthConn{conn: conn, client: grpc_health_v1.NewHealthClient(conn)}
}

func (h *healthConn) check(service string) (grpc_health_v1.HealthCheckResponse_ServingStatus, metadata.MD, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var hdr metadata.MD
	resp, err := h.client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: service}, grpc.Header(&hdr))
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, hdr, err
	}
	return resp.GetStatus(), hdr, nil
}

func checkOnce(t *testing.T, addr, service string) (grpc_health_v1.HealthCheckResponse_ServingStatus, error) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return grpc_health_v1.HealthCheckResponse_UNKNOWN, err
	}
	defer func() { _ = conn.Close() }()
	h := &healthConn{conn: conn, client: grpc_health_v1.NewHealthClient(conn)}
	st, _, err := h.check(service)
	return st, err
}

func statusIs(t *testing.T, g *gateway, service string, want grpc_health_v1.HealthCheckResponse_ServingStatus) func() bool {
	return func() bool {
		st, err := checkOnce(t, g.addr(), service)
		return err == nil && st == want
	}
}

func waitLive(t *testing.T, g *gateway) {
	t.Helper()
	testkit.Eventually(t, 10*time.Second, statusIs(t, g, "liveness", serving))
}

func waitReady(t *testing.T, g *gateway, within time.Duration) {
	t.Helper()
	testkit.Eventually(t, within, statusIs(t, g, "", serving))
}

func healthzStatus(t *testing.T, g *gateway) (int, string) {
	t.Helper()
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get(g.url() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var sb strings.Builder
	buf := make([]byte, 64)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, sb.String()
}

func holdsFor(t *testing.T, d time.Duration, every time.Duration, fn func() error) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(every)
	}
}
