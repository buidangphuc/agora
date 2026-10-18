package handler_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
)

// Login enforcement: Refusal does not leak the reason or note
func TestLoginRefusalLeaksNothing(t *testing.T) {
	e := newAccEnv(t)
	banned := e.seller("banned_user")
	susp := e.seller("susp_user")
	const note = "NOTE-LEAK-MARKER"
	until := accNow.Add(90 * time.Minute)
	if _, err := e.svc.SetUserStatus(context.Background(), e.adminID, banned, "banned", "scam", note, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetUserStatus(context.Background(), e.adminID, susp, "suspended", "scam", note, &until); err != nil {
		t.Fatal(err)
	}

	ah := handler.NewAuthHandler(e.svc)
	var msgs []string
	for _, name := range []string{"banned_user", "susp_user"} {
		_, err := ah.Login(context.Background(), &identityv1.LoginRequest{Username: name, Password: "password123"})
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.PermissionDenied {
			t.Fatalf("%s: err = %v, want PermissionDenied", name, err)
		}
		if st.Message() != "tài khoản bị khóa" {
			t.Fatalf("%s: message = %q", name, st.Message())
		}
		if len(st.Details()) != 0 {
			t.Fatalf("%s: details = %v, want none", name, st.Details())
		}
		for _, leak := range []string{"scam", note, "2026", "10:", "suspend", "ban"} {
			if strings.Contains(strings.ToLower(st.Message()), strings.ToLower(leak)) {
				t.Fatalf("%s: message leaks %q", name, leak)
			}
		}
		msgs = append(msgs, st.Message())
	}
	if msgs[0] != msgs[1] {
		t.Fatalf("messages differ: %v", msgs)
	}
}
