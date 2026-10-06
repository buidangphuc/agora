package edge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/token"
)

func (f *cockpitFixture) tokenFor(t *testing.T, sub, typ string, scopes ...string) string {
	t.Helper()
	claims := &token.Claims{
		Type:   typ,
		Scopes: scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "kid-1"
	s, err := tok.SignedString(f.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// sse opens the stream and returns the status plus body. Allowed streams block
// until the request context ends, so it is cancelled shortly after connecting.
func sse(h http.Handler, room string, mod func(*http.Request)) (int, string, http.Header) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/events/live?room="+room, nil).WithContext(ctx)
	req.Header.Set("Origin", "http://evil.example")
	if mod != nil {
		mod(req)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, w.Body.String(), w.Header()
}

func bearer(tok string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }
}

func TestSSERoomAuthorization(t *testing.T) {
	f := newCockpitFixture(t)
	h := edge.NewSSEHandler(f.edge, edge.NewRealtimeBroker())

	owner := f.tokenFor(t, "u1", "user", "listing.read")
	other := f.tokenFor(t, "u2", "user", "listing.read")
	admin := f.tokenFor(t, "a1", "user", "admin")

	tests := []struct {
		name string
		room string
		mod  func(*http.Request)
		want int
	}{
		{"anonymous global", "global", nil, 200},
		{"anonymous listing", "listing:42", nil, 200},
		{"anonymous user room", "user:u1", nil, 401},
		{"other user denied", "user:u1", bearer(other), 403},
		{"owner allowed", "user:u1", bearer(owner), 200},
		{"owner via session cookie", "user:u1", func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: "session", Value: owner})
		}, 200},
		{"invalid token", "user:u1", bearer("garbage"), 401},
		{"invalid token on public room", "global", bearer("garbage"), 200},
		{"anonymous chat", "chat:t1", nil, 401},
		{"authed chat", "chat:t1", bearer(other), 200},
		{"ops needs admin", "ops:orders", bearer(owner), 403},
		{"ops admin", "ops:orders", bearer(admin), 200},
		{"anonymous ops", "ops:orders", nil, 401},
		{"unknown room", "secret:x", bearer(admin), 403},
		{"empty user id", "user:", bearer(owner), 403},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body, hdr := sse(h, tc.room, tc.mod)
			if code != tc.want {
				t.Fatalf("status = %d, want %d (body %q)", code, tc.want, body)
			}
			if got := hdr.Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("handler must not set CORS headers, got %q", got)
			}
			if tc.want == 200 && !strings.Contains(body, "event: connected") {
				t.Fatalf("missing handshake: %q", body)
			}
		})
	}
}
