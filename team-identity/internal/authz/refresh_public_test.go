package authz_test

import (
	"testing"

	"github.com/buidangphuc/team-identity/internal/authz"
)

// Scenario: Refresh is callable without a Principal.
//
// API assumed (implementer adds it to authz/scopes.go):
//
//	func IsPublicRPC(fullMethod string) bool
//
// true for RPCs callable with no Principal; RefreshSession is one, like Login.
func TestRefreshSessionIsPublic(t *testing.T) {
	const prefix = "/platform.identity.v1.AuthService/"
	for _, m := range []string{"RefreshSession", "Login", "Register"} {
		if !authz.IsPublicRPC(prefix + m) {
			t.Errorf("%s must be classified public", m)
		}
	}
	// Control: an authenticated RPC must not be public.
	for _, m := range []string{"ChangePassword"} {
		if authz.IsPublicRPC(prefix + m) {
			t.Errorf("%s must not be public", m)
		}
	}
	if authz.IsPublicRPC("/platform.identity.v1.SessionService/RevokeSession") {
		t.Error("SessionService/RevokeSession must not be public")
	}
}
