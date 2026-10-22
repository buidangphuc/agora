package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	sharingv1 "github.com/buidangphuc/team-sharing/generated/platform/sharing/v1"
	"github.com/buidangphuc/team-sharing/internal/rpc"
	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

type fakeRepo struct {
	mu    sync.Mutex
	links map[string]*sharelink.ShareLink
}

func newFakeRepo() *fakeRepo { return &fakeRepo{links: map[string]*sharelink.ShareLink{}} }

func (r *fakeRepo) Create(_ context.Context, l *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[l.ShortCode]; ok {
		return nil, sharelink.ErrShortCodeExists
	}
	c := *l
	r.links[l.ShortCode] = &c
	out := c
	return &out, nil
}

func (r *fakeRepo) Resolve(_ context.Context, code string) (*sharelink.ShareLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[code]
	if !ok {
		return nil, sharelink.ErrShareLinkNotFound
	}
	l.ClickCount++
	c := *l
	return &c, nil
}

func (r *fakeRepo) creator(t *testing.T, code string) string {
	t.Helper()
	l, err := r.Resolve(context.Background(), code)
	if err != nil {
		t.Fatalf("resolve %q: %v", code, err)
	}
	return l.CreatedBy
}

func newHandler(t *testing.T, repo sharelink.Repository) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(sharelink.NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func dial(t *testing.T, h *rpc.Handler) sharingv1.SharingServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor()))
	rpc.Register(srv, h)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return sharingv1.NewSharingServiceClient(conn)
}

// Scenario: Handler round trip.
func TestHandlerCreateThenResolve_HandlerRoundTrip(t *testing.T) {
	h := newHandler(t, newFakeRepo())
	ctx := context.Background()

	created, err := h.CreateShareLink(ctx, &sharingv1.CreateShareLinkRequest{
		TargetType: "listing",
		TargetId:   "listing_42",
		Utm:        map[string]string{"utm_campaign": "tet"},
	})
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	if created.GetShortCode() == "" {
		t.Fatal("expected non-empty short code")
	}
	resolved, err := h.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{ShortCode: created.GetShortCode()})
	if err != nil {
		t.Fatalf("ResolveShareLink: %v", err)
	}
	if resolved.GetTargetType() != "listing" || resolved.GetTargetId() != "listing_42" {
		t.Fatalf("target mismatch: %+v", resolved)
	}
	if resolved.GetUtm()["utm_campaign"] != "tet" {
		t.Fatalf("utm mismatch: %+v", resolved.GetUtm())
	}
	if resolved.GetOgMeta().GetTitle() == "" {
		t.Fatal("expected OG meta title")
	}
}

// Scenario: Handler round trip, over a real gRPC server with the SDK auth interceptor.
func TestHandlerCreateThenResolve_OverGRPC(t *testing.T) {
	client := dial(t, newHandler(t, newFakeRepo()))
	ctx := context.Background()

	created, err := client.CreateShareLink(ctx, &sharingv1.CreateShareLinkRequest{TargetType: "listing", TargetId: "l9"})
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := client.ResolveShareLink(ctx, &sharingv1.ResolveShareLinkRequest{ShortCode: created.GetShortCode()})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.GetTargetId() != "l9" {
		t.Fatalf("got %+v", resolved)
	}
}

// Scenario: Unknown short code is NotFound.
func TestHandlerResolveUnknownIsNotFound(t *testing.T) {
	h := newHandler(t, newFakeRepo())
	_, err := h.ResolveShareLink(context.Background(), &sharingv1.ResolveShareLinkRequest{ShortCode: "nope"})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("expected NotFound, got %v (err=%v)", got, err)
	}
}

// Scenario: Empty target or empty code is InvalidArgument.
func TestHandlerCreateInvalidArgument(t *testing.T) {
	h := newHandler(t, newFakeRepo())
	cases := []*sharingv1.CreateShareLinkRequest{
		{TargetType: "", TargetId: "x"},
		{TargetType: "listing", TargetId: ""},
		{},
	}
	for _, req := range cases {
		_, err := h.CreateShareLink(context.Background(), req)
		if got := status.Code(err); got != codes.InvalidArgument {
			t.Errorf("%+v: expected InvalidArgument, got %v (err=%v)", req, got, err)
		}
	}
}

// Scenario: Empty target or empty code is InvalidArgument (resolve side).
func TestHandlerResolveEmptyCodeInvalidArgument(t *testing.T) {
	h := newHandler(t, newFakeRepo())
	_, err := h.ResolveShareLink(context.Background(), &sharingv1.ResolveShareLinkRequest{ShortCode: ""})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v (err=%v)", got, err)
	}
}

// Scenario: A non-domain error hides its cause (through the handler).
func TestHandlerRepoFailureHidesCause(t *testing.T) {
	h := newHandler(t, failingRepo{})
	_, err := h.ResolveShareLink(context.Background(), &sharingv1.ResolveShareLinkRequest{ShortCode: "abc"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

type failingRepo struct{}

func (failingRepo) Create(context.Context, *sharelink.ShareLink) (*sharelink.ShareLink, error) {
	return nil, io.ErrClosedPipe
}
func (failingRepo) Resolve(context.Context, string) (*sharelink.ShareLink, error) {
	return nil, io.ErrClosedPipe
}
