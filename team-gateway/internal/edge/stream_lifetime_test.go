package edge_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	chatv1 "github.com/buidangphuc/team-gateway/generated/platform/chat/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/chat/v1/chatv1connect"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/revocation"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

// authz-residuals-2: a stream ends when its token expires or its session is revoked.

// blockingUp holds the stream open until its context ends, like a provider that
// has not answered yet.
type blockingUp struct {
	grpc.ClientStream
	ctx context.Context
	up  *atomic.Bool
}

func (s *blockingUp) Recv() (*chatv1.StreamChatResponse, error) {
	<-s.ctx.Done()
	s.up.Store(true)
	return nil, s.ctx.Err()
}

type holdChat struct {
	chatv1.ChatServiceClient
	hold      bool
	upCancel  atomic.Bool
	opened    atomic.Int32
	finishNow bool
}

func (h *holdChat) StreamChat(ctx context.Context, _ *chatv1.StreamChatRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[chatv1.StreamChatResponse], error) {
	h.opened.Add(1)
	if h.hold {
		return &blockingUp{ctx: ctx, up: &h.upCancel}, nil
	}
	return &streamUp{}, nil
}

type lifeFixture struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	denylist *revocation.Denylist
	chat     *holdChat
}

type noPub struct{}

func (noPub) PublishTrackingEvent(context.Context, *analyticsv1.TrackingEvent, *commonv1.Principal, string, string) error {
	return nil
}
func (noPub) Close() {}

func newLifeFixture(t *testing.T, hold bool) *lifeFixture {
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
	f := &lifeFixture{key: key, denylist: revocation.NewDenylist(), chat: &holdChat{hold: hold}}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000).
		WithRevocations(f.denylist).
		WithStreamRevocationCheck(20 * time.Millisecond)
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	f.srv = httptest.NewServer(edge.NewMux(&upstream.Clients{AIChat: f.chat}, e, noPub{}, edge.CockpitConfig{}, logger))
	t.Cleanup(f.srv.Close)
	return f
}

// open starts a StreamChat and reports how it ended and after how long.
func (f *lifeFixture) open(t *testing.T, tok string) (connect.Code, time.Duration) {
	t.Helper()
	c := chatv1connect.NewChatServiceClient(f.srv.Client(), f.srv.URL)
	req := connect.NewRequest(&chatv1.StreamChatRequest{SessionId: "s", Message: "hi"})
	req.Header().Set("Authorization", "Bearer "+tok)
	start := time.Now()
	st, err := c.StreamChat(context.Background(), req)
	if err != nil {
		return connect.CodeOf(err), time.Since(start)
	}
	defer st.Close()
	for st.Receive() {
	}
	if st.Err() == nil {
		return 0, time.Since(start)
	}
	return connect.CodeOf(st.Err()), time.Since(start)
}

func TestStreamEndsUnauthenticatedWhenTokenExpires(t *testing.T) {
	f := newLifeFixture(t, true)
	tok := mintSID(t, f.key, "kid-1", time.Now().Add(2*time.Second), "sid-1")
	code, took := f.open(t, tok)
	if code != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", code)
	}
	if took > 4*time.Second {
		t.Fatalf("stream lasted %v, want it cut near the 2s expiry", took)
	}
	deadline := time.Now().Add(time.Second)
	for !f.chat.upCancel.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !f.chat.upCancel.Load() {
		t.Fatal("upstream call was not cancelled when the stream ended")
	}
}

func TestStreamEndsUnauthenticatedWhenSessionRevoked(t *testing.T) {
	f := newLifeFixture(t, true)
	exp := time.Now().Add(time.Hour)
	tok := mintSID(t, f.key, "kid-1", exp, "sid-rev")
	go func() {
		time.Sleep(300 * time.Millisecond)
		f.denylist.Add("sid-rev", exp)
	}()
	code, took := f.open(t, tok)
	if code != connect.CodeUnauthenticated {
		t.Fatalf("code = %v, want unauthenticated", code)
	}
	if took > 3*time.Second {
		t.Fatalf("stream lasted %v after revocation, want well under the token's hour", took)
	}
}

func TestStreamFinishingBeforeExpiryIsUnaffected(t *testing.T) {
	f := newLifeFixture(t, false)
	tok := mintSID(t, f.key, "kid-1", time.Now().Add(time.Hour), "sid-ok")
	if code, _ := f.open(t, tok); code != 0 {
		t.Fatalf("code = %v, want a clean stream", code)
	}
}

func TestStreamOfAnotherSessionIsNotCutByRevocation(t *testing.T) {
	f := newLifeFixture(t, false)
	exp := time.Now().Add(time.Hour)
	f.denylist.Add("sid-other", exp)
	tok := mintSID(t, f.key, "kid-1", exp, "sid-mine")
	if code, _ := f.open(t, tok); code != 0 {
		t.Fatalf("code = %v, want a clean stream", code)
	}
}
