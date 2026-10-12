package edge

import (
	"encoding/json"
	"net/http"
)

// HandleSessionPrincipal answers GET /api/session/principal: the verified `sub`
// of the bearer token (signature, expiry, revocation). Anything unverified is a
// bare 401. The token is never logged.
func HandleSessionPrincipal(e *Edge) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		tok := bearerToken(r.Header.Get("Authorization"))
		if tok == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		claims, err := e.verifyToken(tok)
		if err != nil || claims.Subject == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"principalId": claims.Subject})
	}
}
