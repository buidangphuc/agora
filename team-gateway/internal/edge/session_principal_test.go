package edge_test

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-gateway/internal/edge"
)

// add-ga4-destination 1.2b — "Gateway principal endpoint accepts only verified
// tokens". Reuses the bearer_test.go fixture (test JWKS + revocation denylist).
// Assumed interface: edge.HandleSessionPrincipal(e *edge.Edge) http.HandlerFunc.
func TestSessionPrincipal_OnlyVerifiedTokens(t *testing.T) {
	f := newBearerFixture(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	e := edge.NewEdge(f.verifier, []string{"search:read"}, time.Second, 0, 1000, 1000).
		WithRevocations(f.denylist)
	h := edge.HandleSessionPrincipal(e)

	exp := time.Now().Add(time.Hour)
	revokedSID := "sid-revoked-principal"
	f.denylist.Add(revokedSID, exp)

	cases := []struct {
		name string
		auth string
		want int
	}{
		{"no bearer", "", http.StatusUnauthorized},
		{"bad signature", "Bearer " + mint(t, other, "kid-1", exp), http.StatusUnauthorized},
		{"expired", "Bearer " + mint(t, f.key, "kid-1", time.Now().Add(-time.Minute)), http.StatusUnauthorized},
		{"revoked", "Bearer " + mintSID(t, f.key, "kid-1", exp, revokedSID), http.StatusUnauthorized},
		{"garbage", "Bearer garbage", http.StatusUnauthorized},
		{"valid", "Bearer " + mint(t, f.key, "kid-1", exp), http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/session/principal", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			res := rec.Result()
			body, _ := io.ReadAll(res.Body)
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
			if tc.want == http.StatusUnauthorized {
				if len(body) != 0 {
					t.Errorf("401 body must be empty, got %q", body)
				}
				return
			}
			if got := strings.TrimSpace(string(body)); got != `{"principalId":"user-1"}` {
				t.Errorf("body = %q, want principalId=sub", got)
			}
			if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
				t.Errorf("Cache-Control = %q, want no-store", cc)
			}
		})
	}
}
