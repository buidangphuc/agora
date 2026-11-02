package identity_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/buidangphuc/team-identity/internal/store"
)

func tokenPayload(t *testing.T, tok string) map[string]any {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return m
}

func TestLoginTokenCarriesRestrictedStatus(t *testing.T) {
	e := newEnfEnv(t)
	r := e.user("restr1", "seller")
	e.user("active1", "seller")
	e.fixture(r, store.StatusRestricted, "counterfeit", time.Time{})
	e.start()

	res, err := e.svc.Login(e.ctx, "restr1", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if got := tokenPayload(t, res.Token)["status"]; got != "restricted" {
		t.Fatalf("restricted login status claim = %v, want restricted", got)
	}

	refreshed, err := e.svc.RefreshSession(e.ctx, res.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if got := tokenPayload(t, refreshed.Token)["status"]; got != "restricted" {
		t.Fatalf("restricted refresh status claim = %v, want restricted", got)
	}

	active, err := e.svc.Login(e.ctx, "active1", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := tokenPayload(t, active.Token)["status"]; ok {
		t.Fatalf("active login has status claim %v", got)
	}
}
