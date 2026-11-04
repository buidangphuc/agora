package rpc_test

import (
	"context"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

func hsUserCtxWithSID(uid, sid string) context.Context {
	pairs := []string{"x-principal-id", uid, "x-principal-type", "user", "x-principal-scopes", "identity.read,identity.write"}
	if sid != "" {
		pairs = append(pairs, "x-principal-sid", sid)
	}
	return hsCtx(pairs...)
}

func newTwoSessionEnv(t *testing.T) (hsEnv, string) {
	t.Helper()
	repo := &hsRepo{}
	logs := &hsBuf{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	svc := newSvcWith(t, storetest.NewInMemoryUserRepository(), storetest.NewInMemorySessionRepository(), nopAudit{}, logger).WithHandoff(repo)
	reg, err := svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	second, err := svc.Login(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	conn := startWire(t, rpc.Handlers{Auth: rpc.NewAuthHandler(svc).WithLogger(logger)}, discardLogger())
	e := hsEnv{
		client: identityv1.NewAuthServiceClient(conn),
		repo:   repo,
		logs:   logs,
		uid:    reg.UserID,
		sid:    parseSID(t, reg.Token),
		token:  reg.Token,
	}
	return e, parseSID(t, second.Token)
}

func TestHandoffMintBindsToForwardedPrincipalSID(t *testing.T) {
	t.Run("Minted code binds to the calling principal session among two live sessions", func(t *testing.T) {
		e, secondSID := newTwoSessionEnv(t)
		if e.sid == "" || secondSID == "" || e.sid == secondSID {
			t.Fatalf("need two distinct live sessions: %q %q", e.sid, secondSID)
		}
		for _, want := range []string{secondSID, e.sid} {
			before := len(e.repo.snapshot())
			if _, err := e.client.CreateHandoffCode(hsUserCtxWithSID(e.uid, want), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller}); err != nil {
				t.Fatalf("CreateHandoffCode: %v", err)
			}
			rows := e.repo.snapshot()
			if len(rows) != before+1 {
				t.Fatalf("rows = %d, want %d", len(rows), before+1)
			}
			if got := rows[len(rows)-1].SourceSID; got != want {
				t.Fatalf("SourceSID = %q, want the calling principal sid %q", got, want)
			}
		}
	})

	t.Run("Empty principal sid is Unauthenticated and mints nothing", func(t *testing.T) {
		e, _ := newTwoSessionEnv(t)
		_, err := e.client.CreateHandoffCode(hsUserCtxWithSID(e.uid, ""), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		hsWantCode(t, err, codes.Unauthenticated)
		if n := len(e.repo.snapshot()); n != 0 {
			t.Fatalf("rows = %d, want 0", n)
		}
	})

	t.Run("Authorization header alone does not supply the session", func(t *testing.T) {
		e, _ := newTwoSessionEnv(t)
		ctx := hsCtx("authorization", "Bearer "+e.token, "x-principal-id", e.uid, "x-principal-type", "user", "x-principal-scopes", "identity.read,identity.write")
		_, err := e.client.CreateHandoffCode(ctx, &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		hsWantCode(t, err, codes.Unauthenticated)
		if n := len(e.repo.snapshot()); n != 0 {
			t.Fatalf("rows = %d, want 0", n)
		}
	})
}
