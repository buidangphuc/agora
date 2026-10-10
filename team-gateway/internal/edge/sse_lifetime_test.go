package edge_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/revocation"
	"github.com/buidangphuc/team-gateway/internal/token"
)

// sse-stream-lifetime: an authenticated SSE connection ends when its token
// expires or its session is revoked, with one final `unauthenticated` event.

type sseLife struct {
	srv      *httptest.Server
	key      *rsa.PrivateKey
	denylist *revocation.Denylist
	broker   *edge.RealtimeBroker
}

func newSSELife(t *testing.T) *sseLife {
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
	f := &sseLife{key: key, denylist: revocation.NewDenylist(), broker: edge.NewRealtimeBroker()}
	e := edge.NewEdge(token.NewVerifier(jwks.URL, time.Minute), []string{"listing.read"}, time.Second, 0, 1000, 1000).
		WithRevocations(f.denylist).
		WithStreamRevocationCheck(20 * time.Millisecond)
	f.srv = httptest.NewServer(edge.NewSSEHandler(e, f.broker))
	t.Cleanup(f.srv.Close)
	return f
}

// connect opens the SSE route and returns the response (handshake not yet read).
func (f *sseLife) connect(t *testing.T, ctx context.Context, room, tok string) *http.Response {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, f.srv.URL+"?room="+room, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	return resp
}

// readAll reads the body until the server closes it, failing after `limit`.
func readAll(t *testing.T, resp *http.Response, limit time.Duration) (string, time.Duration) {
	t.Helper()
	start := time.Now()
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		var sb strings.Builder
		sc := bufio.NewReader(resp.Body)
		for {
			line, err := sc.ReadString('\n')
			sb.WriteString(line)
			if err != nil {
				ch <- res{sb.String(), err}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		if r.err != io.EOF {
			t.Fatalf("body ended with %v, want a clean close", r.err)
		}
		return r.s, time.Since(start)
	case <-time.After(limit):
		t.Fatalf("the connection was still open after %v", limit)
		return "", 0
	}
}

func TestSSEEndsWithUnauthenticatedWhenTokenExpires(t *testing.T) {
	f := newSSELife(t)
	tok := mintSID(t, f.key, "kid-1", time.Now().Add(time.Second), "sid-1")
	resp := f.connect(t, context.Background(), "user:user-1", tok)
	body, took := readAll(t, resp, 4*time.Second)
	if !strings.Contains(body, "event: connected") {
		t.Fatalf("missing handshake: %q", body)
	}
	if !strings.HasSuffix(body, "event: unauthenticated\ndata: {\"code\":\"unauthenticated\",\"reason\":\"token_expired\"}\n\n") {
		t.Fatalf("body must end with the unauthenticated event, got %q", body)
	}
	if took > 3*time.Second {
		t.Fatalf("closed after %v, want near the 1s expiry", took)
	}
}

func TestSSEEndsWithUnauthenticatedWhenSessionRevoked(t *testing.T) {
	f := newSSELife(t)
	exp := time.Now().Add(time.Hour)
	tok := mintSID(t, f.key, "kid-1", exp, "sid-rev")
	resp := f.connect(t, context.Background(), "user:user-1", tok)
	go func() {
		time.Sleep(200 * time.Millisecond)
		f.denylist.Add("sid-rev", exp)
	}()
	body, took := readAll(t, resp, 3*time.Second)
	if !strings.HasSuffix(body, "event: unauthenticated\ndata: {\"code\":\"unauthenticated\",\"reason\":\"session_revoked\"}\n\n") {
		t.Fatalf("body must end with the unauthenticated event, got %q", body)
	}
	if took > 2*time.Second {
		t.Fatalf("closed %v after open, want soon after the revoke", took)
	}
}

func TestSSENothingStreamedAfterTheCut(t *testing.T) {
	f := newSSELife(t)
	exp := time.Now().Add(time.Hour)
	tok := mintSID(t, f.key, "kid-1", exp, "sid-cut")
	resp := f.connect(t, context.Background(), "user:user-1", tok)
	f.denylist.Add("sid-cut", exp)
	body, _ := readAll(t, resp, 3*time.Second)
	// A broadcast after the cut reaches nobody: the subscription was released.
	f.broker.Broadcast("user:user-1", "late", map[string]string{"x": "y"})
	if strings.Contains(body, `"late"`) || !strings.HasSuffix(body, "\n\n") ||
		strings.Count(body, "event: unauthenticated") != 1 {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestSSEPublicRoomIsNotCut(t *testing.T) {
	f := newSSELife(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resp := f.connect(t, ctx, "listing:42", "")
	time.Sleep(400 * time.Millisecond)
	f.broker.Broadcast("listing:42", "stock", 1)
	r := bufio.NewReader(resp.Body)
	var got string
	for !strings.Contains(got, `"stock"`) {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("public connection closed early: %v (%q)", err, got)
		}
		got += line
	}
	if strings.Contains(got, "unauthenticated") {
		t.Fatalf("public room got %q", got)
	}
}

func TestSSEClientCloseReleasesWatcher(t *testing.T) {
	f := newSSELife(t)
	exp := time.Now().Add(time.Hour)
	open := func(sid string) {
		tok := mintSID(t, f.key, "kid-1", exp, sid)
		ctx, cancel := context.WithCancel(context.Background())
		resp := f.connect(t, ctx, "user:user-1", tok)
		if line, _ := bufio.NewReader(resp.Body).ReadString('\n'); !strings.Contains(line, "connected") {
			t.Fatalf("handshake = %q", line)
		}
		cancel()
		_ = resp.Body.Close()
		http.DefaultClient.CloseIdleConnections()
	}
	settle := func(want int) int {
		deadline := time.Now().Add(3 * time.Second)
		n := runtime.NumGoroutine()
		for n > want && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
			n = runtime.NumGoroutine()
		}
		return n
	}
	open("sid-warm") // warm the transport and server pools
	base := settle(0)
	for i := 0; i < 10; i++ {
		open("sid-leak")
	}
	if n := settle(base); n > base {
		t.Fatalf("goroutines = %d, baseline %d: a lifetime watcher leaked", n, base)
	}
}
