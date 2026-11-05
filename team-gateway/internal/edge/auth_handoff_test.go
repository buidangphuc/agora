package edge_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
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

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const handoffCode = "HANDOFFCODE-plaintext-0123456789abcdefXYZ"

type fakeHandoffIdentity struct {
	identityv1.AuthServiceClient
	createCalls atomic.Int32
	peekCalls   atomic.Int32
	redeemCalls atomic.Int32
	redeemErr   error
	mu          sync.Mutex
	peekReq     *identityv1.PeekHandoffCodeRequest
	redeemReq   *identityv1.RedeemHandoffCodeRequest
}

func (f *fakeHandoffIdentity) CreateHandoffCode(context.Context, *identityv1.CreateHandoffCodeRequest, ...grpc.CallOption) (*identityv1.CreateHandoffCodeResponse, error) {
	f.createCalls.Add(1)
	return &identityv1.CreateHandoffCodeResponse{Code: handoffCode}, nil
}

func (f *fakeHandoffIdentity) PeekHandoffCode(_ context.Context, in *identityv1.PeekHandoffCodeRequest, _ ...grpc.CallOption) (*identityv1.PeekHandoffCodeResponse, error) {
	f.peekCalls.Add(1)
	f.mu.Lock()
	f.peekReq = in
	f.mu.Unlock()
	return &identityv1.PeekHandoffCodeResponse{Username: "alice"}, nil
}

func (f *fakeHandoffIdentity) RedeemHandoffCode(_ context.Context, in *identityv1.RedeemHandoffCodeRequest, _ ...grpc.CallOption) (*identityv1.RedeemHandoffCodeResponse, error) {
	f.redeemCalls.Add(1)
	f.mu.Lock()
	f.redeemReq = in
	f.mu.Unlock()
	if f.redeemErr != nil {
		return nil, f.redeemErr
	}
	return &identityv1.RedeemHandoffCodeResponse{Result: &identityv1.AuthResult{}}, nil
}

func newHandoffClient(t *testing.T, id *fakeHandoffIdentity, retryMax int, logs *syncBuf) identityv1connect.AuthServiceClient {
	t.Helper()
	e := edge.NewEdge(token.NewVerifier("http://127.0.0.1:1/jwks", time.Minute), []string{"listing.read"}, time.Second, retryMax, 1000, 1000)
	h := slog.New(slog.DiscardHandler)
	if logs != nil {
		h = slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Identity: id}, e, nil, edge.CockpitConfig{}, h))
	t.Cleanup(srv.Close)
	return identityv1connect.NewAuthServiceClient(srv.Client(), srv.URL)
}

// Scenario: Peek needs no token at the gateway.
func TestAuthForwarderPeekPublicHandoff(t *testing.T) {
	id := &fakeHandoffIdentity{}
	c := newHandoffClient(t, id, 2, nil)
	res, err := c.PeekHandoffCode(context.Background(), connect.NewRequest(&identityv1.PeekHandoffCodeRequest{Code: handoffCode, TargetApp: identityv1.HandoffApp_HANDOFF_APP_SELLER}))
	if err != nil {
		t.Fatalf("anonymous PeekHandoffCode: %v", err)
	}
	if res.Msg.GetUsername() != "alice" {
		t.Fatalf("username = %q, want alice", res.Msg.GetUsername())
	}
	if n := id.peekCalls.Load(); n != 1 {
		t.Fatalf("identity peek calls = %d, want 1", n)
	}
	if id.peekReq.GetCode() != handoffCode || id.peekReq.GetTargetApp() != identityv1.HandoffApp_HANDOFF_APP_SELLER {
		t.Fatalf("peek request not forwarded intact: %v", id.peekReq)
	}
}

// Scenario: Redeem needs no token.
func TestAuthForwarderRedeemPublicHandoff(t *testing.T) {
	id := &fakeHandoffIdentity{}
	c := newHandoffClient(t, id, 2, nil)
	res, err := c.RedeemHandoffCode(context.Background(), connect.NewRequest(&identityv1.RedeemHandoffCodeRequest{Code: handoffCode, TargetApp: identityv1.HandoffApp_HANDOFF_APP_BUYER}))
	if err != nil {
		t.Fatalf("anonymous RedeemHandoffCode: %v", err)
	}
	if res.Msg.GetResult() == nil {
		t.Fatal("identity result not returned")
	}
	if n := id.redeemCalls.Load(); n != 1 {
		t.Fatalf("identity redeem calls = %d, want 1", n)
	}
	if id.redeemReq.GetCode() != handoffCode || id.redeemReq.GetTargetApp() != identityv1.HandoffApp_HANDOFF_APP_BUYER {
		t.Fatalf("redeem request not forwarded intact: %v", id.redeemReq)
	}
}

// Scenario: Mint without a token is refused.
func TestAuthForwarderCreateHandoffRequiresToken(t *testing.T) {
	id := &fakeHandoffIdentity{}
	c := newHandoffClient(t, id, 2, nil)
	_, err := c.CreateHandoffCode(context.Background(), connect.NewRequest(&identityv1.CreateHandoffCodeRequest{TargetApp: identityv1.HandoffApp_HANDOFF_APP_SELLER}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("got %v, want unauthenticated", err)
	}
	if n := id.createCalls.Load(); n != 0 {
		t.Fatalf("identity create calls = %d, want 0", n)
	}
}

// Scenario: Redeem is never retried.
func TestAuthForwarderRedeemNotRetriedHandoff(t *testing.T) {
	id := &fakeHandoffIdentity{redeemErr: status.Error(codes.Unavailable, "identity down")}
	c := newHandoffClient(t, id, 3, nil)
	_, err := c.RedeemHandoffCode(context.Background(), connect.NewRequest(&identityv1.RedeemHandoffCodeRequest{Code: handoffCode, TargetApp: identityv1.HandoffApp_HANDOFF_APP_SELLER}))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("got %v, want unavailable", err)
	}
	if n := id.redeemCalls.Load(); n != 1 {
		t.Fatalf("identity redeem calls = %d, want exactly 1", n)
	}
}

// Scenario: Code absent from logs even on failure.
func TestGatewayHandoffCodeNotLogged(t *testing.T) {
	logs := &syncBuf{}
	id := &fakeHandoffIdentity{redeemErr: status.Error(codes.Unavailable, "identity down")}
	c := newHandoffClient(t, id, 3, logs)
	ctx := context.Background()
	if _, err := c.PeekHandoffCode(ctx, connect.NewRequest(&identityv1.PeekHandoffCodeRequest{Code: handoffCode, TargetApp: identityv1.HandoffApp_HANDOFF_APP_SELLER})); err != nil {
		t.Fatal(err)
	}
	_, err := c.RedeemHandoffCode(ctx, connect.NewRequest(&identityv1.RedeemHandoffCodeRequest{Code: handoffCode, TargetApp: identityv1.HandoffApp_HANDOFF_APP_SELLER}))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("got %v, want unavailable", err)
	}
	if id.redeemCalls.Load() != 1 || id.peekCalls.Load() != 1 {
		t.Fatalf("handoff RPCs were not forwarded (peek=%d redeem=%d), log capture is vacuous", id.peekCalls.Load(), id.redeemCalls.Load())
	}
	sum := sha256.Sum256([]byte(handoffCode))
	for _, frag := range []string{
		handoffCode,
		"plaintext-0123456789",
		hex.EncodeToString(sum[:]),
		base64.StdEncoding.EncodeToString(sum[:]),
		base64.RawURLEncoding.EncodeToString(sum[:]),
	} {
		if strings.Contains(logs.String(), frag) {
			t.Fatalf("log leaks %q:\n%s", frag, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "edge.request") {
		t.Fatalf("no edge.request line captured:\n%s", logs.String())
	}
}
