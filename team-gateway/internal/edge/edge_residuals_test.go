package edge_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	aiv1 "github.com/buidangphuc/team-gateway/generated/platform/ai/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/ai/v1/aiv1connect"
	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	chatv1 "github.com/buidangphuc/team-gateway/generated/platform/chat/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/chat/v1/chatv1connect"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

// ── fakes ──────────────────────────────────────────────────────────────────

type streamUp struct {
	grpc.ClientStream
	sent bool
}

func (s *streamUp) Recv() (*chatv1.StreamChatResponse, error) {
	if s.sent {
		return nil, io.EOF
	}
	s.sent = true
	return &chatv1.StreamChatResponse{}, nil
}

type fakeAIChat struct {
	chatv1.ChatServiceClient
	calls atomic.Int32
}

func (f *fakeAIChat) StreamChat(context.Context, *chatv1.StreamChatRequest, ...grpc.CallOption) (grpc.ServerStreamingClient[chatv1.StreamChatResponse], error) {
	f.calls.Add(1)
	return &streamUp{}, nil
}

type fakeAI struct {
	aiv1.AIServiceClient
	calls atomic.Int32
}

func (f *fakeAI) MagicListing(context.Context, *aiv1.MagicListingRequest, ...grpc.CallOption) (*aiv1.MagicListingResponse, error) {
	f.calls.Add(1)
	return nil, status.Error(codes.Unavailable, "team-ai down")
}

type countPub struct {
	mu     sync.Mutex
	events int
}

func (p *countPub) PublishTrackingEvent(context.Context, *analyticsv1.TrackingEvent, *commonv1.Principal, string) error {
	p.mu.Lock()
	p.events++
	p.mu.Unlock()
	return nil
}
func (p *countPub) Close() {}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type resFixture struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	logs *syncBuf
	chat *fakeAIChat
	ai   *fakeAI
	pub  *countPub
}

func newResFixture(t *testing.T, rps float64, burst int, trackRPS float64, trackBurst int) *resFixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "kid-1",
			"n": b64(key.PublicKey.N.Bytes()),
			"e": b64(big.NewInt(int64(key.PublicKey.E)).Bytes()),
		}}})
	}))
	t.Cleanup(jwks.Close)

	f := &resFixture{key: key, logs: &syncBuf{}, chat: &fakeAIChat{}, ai: &fakeAI{}, pub: &countPub{}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 2, rps, burst).
		WithTrackLimit(trackRPS, trackBurst).
		WithStreamMaxBytes(1024)
	logger := slog.New(slog.NewJSONHandler(f.logs, nil))
	f.srv = httptest.NewServer(edge.NewMux(&upstream.Clients{AIChat: f.chat, AI: f.ai}, e, f.pub, edge.CockpitConfig{}, logger))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *resFixture) streamChat(auth, rid, msg string) (*http.Response, error, connect.Code) {
	c := chatv1connect.NewChatServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&chatv1.StreamChatRequest{SessionId: "s", Message: msg})
	if auth != "" {
		req.Header().Set("Authorization", auth)
	}
	if rid != "" {
		req.Header().Set("X-Request-Id", rid)
	}
	st, err := c.StreamChat(context.Background(), req)
	if err != nil {
		return nil, err, connect.CodeOf(err)
	}
	defer st.Close()
	for st.Receive() {
	}
	return nil, st.Err(), connect.CodeOf(st.Err())
}

// ── streams ────────────────────────────────────────────────────────────────

func TestStreamInvalidTokenIsUnauthenticatedWithNoUpstreamCall(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 100, 100)
	_, err, code := f.streamChat("Bearer not-a-jwt", "", "hi")
	if err == nil || code != connect.CodeUnauthenticated {
		t.Fatalf("got %v (%v), want unauthenticated", err, code)
	}
	if n := f.chat.calls.Load(); n != 0 {
		t.Fatalf("upstream called %d times, want 0", n)
	}
}

func TestStreamEchoesRequestIDAndLogsOnce(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 100, 100)
	tok := mint(t, f.key, "kid-1", time.Now().Add(time.Hour))
	c := chatv1connect.NewChatServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&chatv1.StreamChatRequest{SessionId: "s", Message: "hi"})
	req.Header().Set("Authorization", "Bearer "+tok)
	req.Header().Set("X-Request-Id", "e2e-stream-1")
	st, err := c.StreamChat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for st.Receive() {
	}
	if st.Err() != nil {
		t.Fatal(st.Err())
	}
	if got := st.ResponseHeader().Get("X-Request-Id"); got != "e2e-stream-1" {
		t.Fatalf("X-Request-Id = %q", got)
	}
	_ = st.Close()
	if n := strings.Count(f.logs.String(), `"msg":"edge.request"`); n != 1 {
		t.Fatalf("edge.request lines = %d, want 1:\n%s", n, f.logs.String())
	}
	if !strings.Contains(f.logs.String(), `"method":"/platform.chat.v1.ChatService/StreamChat"`) {
		t.Fatalf("log missing procedure: %s", f.logs.String())
	}
}

func TestStreamPastBurstIsResourceExhausted(t *testing.T) {
	f := newResFixture(t, 0.001, 3, 100, 100)
	tok := "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(time.Hour))
	var exhausted int
	for i := 0; i < 6; i++ {
		if _, _, code := f.streamChat(tok, "", "hi"); code == connect.CodeResourceExhausted {
			exhausted++
		}
	}
	if exhausted == 0 {
		t.Fatal("no stream was rate limited past the burst")
	}
	if n := f.chat.calls.Load(); n > 3 {
		t.Fatalf("upstream called %d times, want <= burst 3", n)
	}
}

func TestStreamOversizedRequestIsResourceExhausted(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 100, 100)
	_, err, code := f.streamChat("", "", strings.Repeat("a", 2000))
	if err == nil || code != connect.CodeResourceExhausted {
		t.Fatalf("got %v (%v), want resource_exhausted", err, code)
	}
	if n := f.chat.calls.Load(); n != 0 {
		t.Fatalf("upstream called %d times, want 0", n)
	}
}

// ── plain HTTP routes ──────────────────────────────────────────────────────

func (f *resFixture) track(auth, rid, marker string) *http.Response {
	body := `{"type":"view","listingId":"` + marker + `"}`
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/track", strings.NewReader(body))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if rid != "" {
		req.Header.Set("X-Request-Id", rid)
	}
	res, err := f.srv.Client().Do(req)
	if err != nil {
		panic(err)
	}
	_ = res.Body.Close()
	return res
}

func TestTrackFloodGets429AndProducesNothingForThem(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 0.001, 5)
	var ok, limited int
	for i := 0; i < 20; i++ {
		switch f.track("", "", "m").StatusCode {
		case http.StatusNoContent:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	if limited == 0 || ok != 5 {
		t.Fatalf("ok=%d limited=%d, want 5 accepted and the rest 429", ok, limited)
	}
	if f.pub.events != ok {
		t.Fatalf("produced %d events for %d accepted requests: a 429 must produce nothing", f.pub.events, ok)
	}
}

func TestTrackKeysLoggedInVisitorByUser(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 0.001, 3)
	// Anonymous bucket (by IP) is exhausted ...
	for i := 0; i < 5; i++ {
		f.track("", "", "m")
	}
	if got := f.track("", "", "m").StatusCode; got != http.StatusTooManyRequests {
		t.Fatalf("anonymous visitor not throttled: %d", got)
	}
	// ... but the same IP logged in has its own per-user bucket.
	tok := "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(time.Hour))
	if got := f.track(tok, "", "m").StatusCode; got != http.StatusNoContent {
		t.Fatalf("logged-in visitor shares the IP bucket: %d", got)
	}
	if !strings.Contains(f.logs.String(), `"principal":"user-1"`) {
		t.Fatalf("log line not attributed to the user: %s", f.logs.String())
	}
}

func TestTrackEchoesRequestID(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 100, 100)
	res := f.track("", "e2e-track-1", "m")
	if res.StatusCode != http.StatusNoContent || res.Header.Get("X-Request-Id") != "e2e-track-1" {
		t.Fatalf("status %d rid %q", res.StatusCode, res.Header.Get("X-Request-Id"))
	}
	if !strings.Contains(f.logs.String(), `"request_id":"e2e-track-1"`) {
		t.Fatalf("no edge.request line for the beacon: %s", f.logs.String())
	}
}

func TestAdminMetricsIsRateLimitedOnSharedBucket(t *testing.T) {
	f := newResFixture(t, 0.001, 2, 100, 100)
	var got []int
	for i := 0; i < 4; i++ {
		res, err := f.srv.Client().Get(f.srv.URL + "/api/admin/metrics")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		got = append(got, res.StatusCode)
	}
	if got[0] != http.StatusUnauthorized || got[3] != http.StatusTooManyRequests {
		t.Fatalf("statuses = %v, want 401 first (anonymous) then 429", got)
	}
}

// ── AI calls ───────────────────────────────────────────────────────────────

func TestMagicListingIsSentOnceAndLogsAttempts(t *testing.T) {
	f := newResFixture(t, 1000, 1000, 100, 100)
	c := aiv1connect.NewAIServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&aiv1.MagicListingRequest{})
	req.Header().Set("X-Request-Id", "e2e-ai-1")
	_, err := c.MagicListing(context.Background(), req)
	var ce *connect.Error
	if !errors.As(err, &ce) || ce.Code() != connect.CodeUnavailable {
		t.Fatalf("got %v, want unavailable", err)
	}
	if n := f.ai.calls.Load(); n != 1 {
		t.Fatalf("upstream called %d times, want exactly 1 (no retry)", n)
	}
	var line map[string]any
	for _, l := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["msg"] == "edge.request" && m["request_id"] == "e2e-ai-1" {
			line = m
		}
	}
	if line == nil || line["attempts"] != float64(1) {
		t.Fatalf("edge.request line = %v, want attempts=1", line)
	}
}
