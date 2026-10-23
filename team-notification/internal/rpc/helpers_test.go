package rpc_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/alerts"
	"github.com/buidangphuc/team-notification/internal/inbox"
	"github.com/buidangphuc/team-notification/internal/prefs"
	"github.com/buidangphuc/team-notification/internal/rpc"
)

type fakeInbox struct {
	mu        sync.Mutex
	rows      []*notificationv1.Notification
	err       error
	panicNext bool
	calls     int
	lastLimit int
	lastOff   int
}

func (f *fakeInbox) enter() {
	f.calls++
	if f.panicNext {
		f.panicNext = false
		panic("boom: nil pointer in store")
	}
}

func (f *fakeInbox) CreateNotification(_ context.Context, n *notificationv1.Notification) (*notificationv1.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows = append(f.rows, n)
	return n, nil
}

func (f *fakeInbox) ListNotifications(_ context.Context, userID string, limit, offset int) ([]*notificationv1.Notification, int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enter()
	f.lastLimit, f.lastOff = limit, offset
	if f.err != nil {
		return nil, 0, f.err
	}
	var out []*notificationv1.Notification
	var unread int32
	for i := len(f.rows) - 1; i >= 0; i-- {
		n := f.rows[i]
		if n.UserId != userID {
			continue
		}
		if !n.IsRead {
			unread++
		}
		out = append(out, n)
	}
	if offset > len(out) {
		offset = len(out)
	}
	out = out[offset:]
	if limit < len(out) {
		out = out[:limit]
	}
	return out, unread, nil
}

func (f *fakeInbox) MarkAsRead(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enter()
	if f.err != nil {
		return f.err
	}
	if id == "" {
		for _, n := range f.rows {
			if n.UserId == userID {
				n.IsRead = true
			}
		}
		return nil
	}
	for _, n := range f.rows {
		if n.Id == id && n.UserId == userID {
			n.IsRead = true
			return nil
		}
	}
	return inbox.ErrNotFound
}

func (f *fakeInbox) GetUnreadCount(_ context.Context, userID string) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enter()
	if f.err != nil {
		return 0, f.err
	}
	var c int32
	for _, n := range f.rows {
		if n.UserId == userID && !n.IsRead {
			c++
		}
	}
	return c, nil
}

func (f *fakeInbox) read(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.rows {
		if n.Id == id {
			return n.IsRead
		}
	}
	return false
}

type fakeAlerts struct {
	mu        sync.Mutex
	subs      []*notificationv1.AlertSubscription
	createErr error
	seq       int
}

func (f *fakeAlerts) Create(_ context.Context, sub *notificationv1.AlertSubscription) (*notificationv1.AlertSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	for _, s := range f.subs {
		if s.UserId == sub.UserId && s.ListingId == sub.ListingId && s.Type == sub.Type {
			return s, nil
		}
	}
	f.seq++
	c := &notificationv1.AlertSubscription{Id: fmt.Sprintf("sub-%d", f.seq), UserId: sub.UserId, ListingId: sub.ListingId, Type: sub.Type}
	f.subs = append(f.subs, c)
	return c, nil
}

func (f *fakeAlerts) Delete(_ context.Context, id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.subs {
		if s.Id == id && s.UserId == userID {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeAlerts) ListByUser(_ context.Context, userID string) ([]*notificationv1.AlertSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*notificationv1.AlertSubscription
	for _, s := range f.subs {
		if s.UserId == userID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeAlerts) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subs)
}

type fakePrefs struct {
	mu        sync.Mutex
	byUser    map[string]*notificationv1.NotificationPrefs
	upsertErr error
}

func (f *fakePrefs) Get(_ context.Context, userID string) (*notificationv1.NotificationPrefs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byUser[userID], nil
}

func (f *fakePrefs) Upsert(_ context.Context, userID string, p *notificationv1.NotificationPrefs) (*notificationv1.NotificationPrefs, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	if f.byUser == nil {
		f.byUser = map[string]*notificationv1.NotificationPrefs{}
	}
	f.byUser[userID] = p
	return p, nil
}

func (f *fakePrefs) ListUsersByDigestFreq(_ context.Context, freq notificationv1.DigestFrequency) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for u, p := range f.byUser {
		if p.DigestFreq == freq {
			out = append(out, u)
		}
	}
	sort.Strings(out)
	return out, nil
}

type env struct {
	h      *rpc.Handler
	inbox  *fakeInbox
	alerts *fakeAlerts
	prefs  *fakePrefs
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{inbox: &fakeInbox{}, alerts: &fakeAlerts{}, prefs: &fakePrefs{}}
	h, err := rpc.NewHandler(e.inbox, alerts.NewService(e.alerts), prefs.NewService(e.prefs), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	e.h = h
	return e
}

func principalCtx(p auth.Principal) context.Context {
	return auth.WithPrincipal(context.Background(), p)
}

func asUser(id string) context.Context {
	return principalCtx(auth.Principal{ID: id, Type: auth.TypeUser})
}

func asService(id string) context.Context {
	return principalCtx(auth.Principal{ID: id, Type: auth.TypeService, Scopes: []string{"listing.read", "identity.read"}})
}

func asAnonymous() context.Context {
	return principalCtx(auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous})
}

func code(err error) codes.Code { return status.Code(err) }

func wantCode(t *testing.T, label string, err error, want codes.Code) {
	t.Helper()
	if got := code(err); got != want {
		t.Fatalf("%s: code = %v (%v), want %v", label, got, err, want)
	}
}

func wire(kv ...string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs(kv...))
}

func wireUser(id string) context.Context {
	return wire("x-principal-id", id, "x-principal-type", "user")
}

func dial(t *testing.T, h *rpc.Handler) notificationv1.NotificationServiceClient {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	health, err := healthx.New(time.Second, logger)
	if err != nil {
		t.Fatalf("healthx.New: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: config.GRPC{}, Logger: logger, Health: health})
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
	return notificationv1.NewNotificationServiceClient(conn)
}

func seed(f *fakeInbox, user string, ids ...string) {
	for _, id := range ids {
		_, _ = f.CreateNotification(context.Background(), &notificationv1.Notification{Id: id, UserId: user, Title: "t " + id})
	}
}
