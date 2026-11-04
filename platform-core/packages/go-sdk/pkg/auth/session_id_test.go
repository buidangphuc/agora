package auth_test

import (
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
)

func TestPrincipalSessionIDReadFromMetadata(t *testing.T) {
	if auth.MDPrincipalSessionID != "x-principal-sid" {
		t.Fatalf("MDPrincipalSessionID = %q, want x-principal-sid", auth.MDPrincipalSessionID)
	}
	t.Run("sid is populated from x-principal-sid", func(t *testing.T) {
		ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "user", auth.MDPrincipalSessionID, "sid-1"), t)
		p, err := auth.RequireUser(ctx)
		if err != nil {
			t.Fatalf("RequireUser: %v", err)
		}
		if p.SessionID != "sid-1" {
			t.Fatalf("SessionID = %q, want sid-1", p.SessionID)
		}
	})
	t.Run("absent header leaves SessionID empty", func(t *testing.T) {
		ctx := run(incoming(auth.MDPrincipalID, "u1", auth.MDPrincipalType, "user"), t)
		p, err := auth.RequireUser(ctx)
		if err != nil {
			t.Fatalf("RequireUser: %v", err)
		}
		if p.SessionID != "" {
			t.Fatalf("SessionID = %q, want empty", p.SessionID)
		}
	})
}
