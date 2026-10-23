package rpc_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	auditv1 "github.com/buidangphuc/team-audit/generated/platform/audit/v1"
	"github.com/buidangphuc/team-audit/internal/audit"
	"github.com/buidangphuc/team-audit/internal/rpc"
	"github.com/buidangphuc/team-audit/internal/store/storetest"
)

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func capturedLogger(buf *safeBuf) *slog.Logger {
	return logx.NewTo(buf, config.Runtime{Env: "test", LogLevel: "debug", LogJSON: true}, "team-audit")
}

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

type faultRepo struct {
	audit.Repository
	appendErr  error
	queryErr   error
	queryPanic bool
	queries    int
}

func (r *faultRepo) Append(ctx context.Context, ev audit.Event) (audit.Event, error) {
	if r.appendErr != nil {
		return audit.Event{}, r.appendErr
	}
	return r.Repository.Append(ctx, ev)
}

func (r *faultRepo) Query(ctx context.Context, f audit.Filter) ([]audit.Event, int64, error) {
	r.queries++
	if r.queryPanic {
		r.queryPanic = false
		panic("boom: nil pointer in repo")
	}
	if r.queryErr != nil {
		return nil, 0, r.queryErr
	}
	return r.Repository.Query(ctx, f)
}

func newFault() *faultRepo { return &faultRepo{Repository: storetest.NewMemory()} }

func newHandlerOn(t *testing.T, repo audit.Repository, logger *slog.Logger) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(audit.NewService(repo), logger)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func newHandler(t *testing.T) (*rpc.Handler, *faultRepo) {
	t.Helper()
	repo := newFault()
	return newHandlerOn(t, repo, quiet()), repo
}

func principalCtx(p auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), p)
}

func asUser(id string) context.Context {
	return principalCtx(auth.Principal{ID: id, Type: auth.TypeUser, Scopes: []string{"listing.read"}})
}

func asAdmin() context.Context {
	return principalCtx(auth.Principal{ID: "admin-1", Type: auth.TypeUser, Scopes: []string{"admin"}})
}

func asService(id string) context.Context {
	return principalCtx(auth.Principal{ID: id, Type: auth.TypeService, Scopes: []string{"audit.write"}})
}

func asAnonymous() context.Context {
	return principalCtx(auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous})
}

func code(err error) codes.Code { return status.Code(err) }

func startServer(t *testing.T, h *rpc.Handler, logger *slog.Logger, cfg config.GRPC) *grpc.ClientConn {
	t.Helper()
	health, err := healthx.New(time.Second, logger)
	if err != nil {
		t.Fatalf("healthx.New: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: cfg, Logger: logger, Health: health})
	rpc.Register(srv, h)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func dial(t *testing.T, h *rpc.Handler, logger *slog.Logger) auditv1.AuditServiceClient {
	t.Helper()
	return auditv1.NewAuditServiceClient(startServer(t, h, logger, config.GRPC{}))
}

func wire(kv ...string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs(kv...))
}

func wireAdmin() context.Context {
	return wire("x-principal-id", "admin-1", "x-principal-type", "user", "x-principal-scopes", "admin")
}

func wireUser() context.Context {
	return wire("x-principal-id", "u-1", "x-principal-type", "user", "x-principal-scopes", "listing.read")
}

func hr(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func ts(h int) *timestamppb.Timestamp { return timestamppb.New(hr(h)) }

func seedEvent(t *testing.T, repo audit.Repository, id, actor, action, tType, tID string, at time.Time) {
	t.Helper()
	if _, err := repo.Append(context.Background(), audit.Event{
		ID: id, ActorID: actor, Action: action, TargetType: tType, TargetID: tID, CreatedAt: at,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func seedMany(t *testing.T, repo audit.Repository, n int, target string) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedEvent(t, repo, fmt.Sprintf("p-%03d", i), "adm-1", "a.b", "order", target, hr(0).Add(time.Duration(i)*time.Minute))
	}
}

func respIDs(r *auditv1.QueryAuditLogResponse) []string {
	out := []string{}
	for _, e := range r.GetEvents() {
		out = append(out, e.GetId())
	}
	return out
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func storedActors(t *testing.T, h *rpc.Handler) []string {
	t.Helper()
	res, err := h.QueryAuditLog(asAdmin(), &auditv1.QueryAuditLogRequest{})
	if err != nil {
		t.Fatalf("admin query: %v", err)
	}
	var out []string
	for _, e := range res.GetEvents() {
		out = append(out, e.GetActorId())
	}
	return out
}
