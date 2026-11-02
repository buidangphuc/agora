package rpc_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/store/storetest"
)

const (
	hsSeller   = identityv1.HandoffApp_HANDOFF_APP_SELLER
	hsSeedCode = "S3eded-handoff-code-aaaaaaaaaaaaaaaaaaaaaaa"
	hsBadCode  = "B4d-handoff-code-never-minted-zzzzzzzzzzzzz"
)

type hsBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *hsBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *hsBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type hsRepo struct {
	mu   sync.Mutex
	rows []store.HandoffCode
}

func (r *hsRepo) live(userID string, now time.Time) int {
	n := 0
	for _, c := range r.rows {
		if c.UserID == userID && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			n++
		}
	}
	return n
}

func (r *hsRepo) InsertHandoffCode(ctx context.Context, c store.HandoffCode, maxLive int, now time.Time, inTx func(context.Context) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.live(c.UserID, now) >= maxLive {
		return store.ErrHandoffCapReached
	}
	r.rows = append(r.rows, c)
	if inTx != nil {
		if err := inTx(ctx); err != nil {
			r.rows = r.rows[:len(r.rows)-1]
			return err
		}
	}
	return nil
}

func (r *hsRepo) RedeemHandoffCode(ctx context.Context, h []byte, app string, now time.Time, inTx func(context.Context, store.HandoffCode) error) (store.HandoffCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, c := range r.rows {
		if bytes.Equal(c.CodeHash, h) && c.TargetApp == app && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			r.rows[i].RedeemedAt = now
			c.RedeemedAt = now
			if inTx != nil {
				if err := inTx(ctx, c); err != nil {
					r.rows[i].RedeemedAt = time.Time{}
					return store.HandoffCode{}, err
				}
			}
			return c, nil
		}
	}
	return store.HandoffCode{}, store.ErrHandoffInvalid
}

func (r *hsRepo) PeekHandoffCode(_ context.Context, h []byte, app string, now time.Time) (store.HandoffCode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.rows {
		if bytes.Equal(c.CodeHash, h) && c.TargetApp == app && c.RedeemedAt.IsZero() && !c.ExpiresAt.Before(now) {
			return c, nil
		}
	}
	return store.HandoffCode{}, store.ErrHandoffInvalid
}

func (r *hsRepo) snapshot() []store.HandoffCode {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]store.HandoffCode(nil), r.rows...)
}

func (r *hsRepo) seedCode(code, userID, sid string, ttl time.Duration) {
	h := sha256.Sum256([]byte(code))
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, store.HandoffCode{
		CodeHash: h[:], UserID: userID, SourceSID: sid, TargetApp: hsSeller.String(),
		ExpiresAt: time.Now().Add(ttl), CreatedAt: time.Now(),
	})
}

type hsEnv struct {
	client   identityv1.AuthServiceClient
	repo     *hsRepo
	logs     *hsBuf
	uid, sid string
}

func newHsEnv(t *testing.T) hsEnv {
	t.Helper()
	repo := &hsRepo{}
	logs := &hsBuf{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	users := storetest.NewInMemoryUserRepository()
	sess := storetest.NewInMemorySessionRepository()
	svc := newSvcWith(t, users, sess, nopAudit{}, logger).WithHandoff(repo)
	reg, err := svc.Register(context.Background(), "alice", "password123", "buyer")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	conn := startWire(t, rpc.Handlers{Auth: rpc.NewAuthHandler(svc).WithLogger(logger)}, discardLogger())
	return hsEnv{
		client: identityv1.NewAuthServiceClient(conn),
		repo:   repo,
		logs:   logs,
		uid:    reg.UserID,
		sid:    parseSID(t, reg.Token),
	}
}

func hsCtx(pairs ...string) context.Context {
	ctx := context.Background()
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
}

func (e hsEnv) userCtx() context.Context {
	return hsCtx("x-principal-id", e.uid, "x-principal-type", "user", "x-principal-scopes", "identity.read,identity.write")
}

func hsWantCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Fatalf("code = %v (%v), want %v", got, err, want)
	}
}

func TestHandoffRPCAuthz(t *testing.T) {
	t.Run("Anonymous caller cannot mint", func(t *testing.T) {
		e := newHsEnv(t)
		_, err := e.client.CreateHandoffCode(hsCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		hsWantCode(t, err, codes.Unauthenticated)
		if n := len(e.repo.snapshot()); n != 0 {
			t.Fatalf("rows = %d, want 0", n)
		}
	})

	t.Run("Service principal cannot mint", func(t *testing.T) {
		e := newHsEnv(t)
		ctx := hsCtx("x-principal-id", "svc-orders", "x-principal-type", "service", "x-principal-scopes", "identity.read")
		_, err := e.client.CreateHandoffCode(ctx, &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		hsWantCode(t, err, codes.PermissionDenied)
		if n := len(e.repo.snapshot()); n != 0 {
			t.Fatalf("rows = %d, want 0", n)
		}
	})

	t.Run("Authenticated user mints a code for the seller app", func(t *testing.T) {
		e := newHsEnv(t)
		res, err := e.client.CreateHandoffCode(e.userCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		if err != nil {
			t.Fatalf("CreateHandoffCode: %v", err)
		}
		if len(res.GetCode()) != 43 {
			t.Fatalf("code length = %d, want 43", len(res.GetCode()))
		}
		if res.GetExpiresAt() == nil {
			t.Fatal("expires_at missing")
		}
		rows := e.repo.snapshot()
		if len(rows) != 1 || rows[0].UserID != e.uid || rows[0].TargetApp != hsSeller.String() {
			t.Fatalf("rows = %+v", rows)
		}
	})

	t.Run("Eleventh live code is refused", func(t *testing.T) {
		e := newHsEnv(t)
		for i := 0; i < 10; i++ {
			if _, err := e.client.CreateHandoffCode(e.userCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller}); err != nil {
				t.Fatalf("mint %d: %v", i, err)
			}
		}
		_, err := e.client.CreateHandoffCode(e.userCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
		hsWantCode(t, err, codes.ResourceExhausted)
		if n := len(e.repo.snapshot()); n != 10 {
			t.Fatalf("rows = %d, want 10", n)
		}
	})
}

func TestHandoffPeekAnonymousAllowed(t *testing.T) {
	e := newHsEnv(t)
	e.repo.seedCode(hsSeedCode, e.uid, e.sid, time.Minute)

	for i := 0; i < 3; i++ {
		res, err := e.client.PeekHandoffCode(hsCtx(), &identityv1.PeekHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller})
		if err != nil {
			t.Fatalf("anonymous peek %d: %v", i, err)
		}
		if res.GetUsername() != "alice" || res.GetExpiresAt() == nil {
			t.Fatalf("peek = %+v", res)
		}
	}
	if !e.repo.snapshot()[0].RedeemedAt.IsZero() {
		t.Fatal("peek consumed the code")
	}
}

func TestHandoffRedeemAnonymousAllowed(t *testing.T) {
	e := newHsEnv(t)
	e.repo.seedCode(hsSeedCode, e.uid, e.sid, time.Minute)

	res, err := e.client.RedeemHandoffCode(hsCtx(), &identityv1.RedeemHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller})
	if err != nil {
		t.Fatalf("anonymous redeem: %v", err)
	}
	if res.GetResult().GetToken() == "" || res.GetResult().GetRefreshToken() == "" {
		t.Fatalf("redeem result lacks tokens: %+v", res)
	}
	if parseSID(t, res.GetResult().GetToken()) == e.sid {
		t.Fatal("redeem reused the source sid")
	}

	_, err = e.client.RedeemHandoffCode(hsCtx(), &identityv1.RedeemHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller})
	hsWantCode(t, err, codes.Unauthenticated)
}

func TestHandoffCodeNeverLogged(t *testing.T) {
	e := newHsEnv(t)
	e.repo.seedCode(hsSeedCode, e.uid, e.sid, time.Minute)

	minted, err := e.client.CreateHandoffCode(e.userCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	_, _ = e.client.CreateHandoffCode(hsCtx(), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
	_, _ = e.client.CreateHandoffCode(hsCtx("x-principal-id", "svc-x", "x-principal-type", "service"), &identityv1.CreateHandoffCodeRequest{TargetApp: hsSeller})
	if _, err := e.client.PeekHandoffCode(hsCtx(), &identityv1.PeekHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller}); err != nil {
		t.Fatalf("peek: %v", err)
	}
	if _, err := e.client.RedeemHandoffCode(hsCtx(), &identityv1.RedeemHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller}); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	_, err = e.client.RedeemHandoffCode(hsCtx(), &identityv1.RedeemHandoffCodeRequest{Code: hsSeedCode, TargetApp: hsSeller})
	hsWantCode(t, err, codes.Unauthenticated)
	_, err = e.client.RedeemHandoffCode(hsCtx(), &identityv1.RedeemHandoffCodeRequest{Code: hsBadCode, TargetApp: hsSeller})
	hsWantCode(t, err, codes.Unauthenticated)
	_, err = e.client.PeekHandoffCode(hsCtx(), &identityv1.PeekHandoffCodeRequest{Code: hsBadCode, TargetApp: hsSeller})
	hsWantCode(t, err, codes.Unauthenticated)
	_, err = e.client.PeekHandoffCode(hsCtx(), &identityv1.PeekHandoffCodeRequest{Code: "short", TargetApp: hsSeller})
	hsWantCode(t, err, codes.Unauthenticated)

	out := e.logs.String()
	secrets := map[string]string{"user id": e.uid, "source sid": e.sid}
	for name, code := range map[string]string{"minted code": minted.GetCode(), "seeded code": hsSeedCode, "bad code": hsBadCode} {
		sum := sha256.Sum256([]byte(code))
		secrets[name] = code
		secrets[name+" sha256 hex"] = hex.EncodeToString(sum[:])
		secrets[name+" sha256 base64"] = base64.StdEncoding.EncodeToString(sum[:])
		secrets[name+" sha256 base64url"] = base64.RawURLEncoding.EncodeToString(sum[:])
	}
	for name, v := range secrets {
		if v != "" && strings.Contains(out, v) {
			t.Errorf("log output contains the %s:\n%s", name, out)
		}
	}
}
