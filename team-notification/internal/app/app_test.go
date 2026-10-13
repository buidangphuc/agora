package app_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/testkit"

	chatv1 "github.com/buidangphuc/team-notification/generated/platform/chat/v1"
	eventsv1 "github.com/buidangphuc/team-notification/generated/platform/events/v1"
	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/app"
	"github.com/buidangphuc/team-notification/internal/config"
)

// closedDatabaseURL points at a port nothing listens on.
const closedDatabaseURL = "postgres://u:p@127.0.0.1:1/db?sslmode=disable"

func load(t *testing.T, env string, port int, databaseURL string) config.Settings {
	t.Helper()
	t.Setenv("ENV", env)
	t.Setenv("GRPC_HOST", "127.0.0.1")
	t.Setenv("GRPC_PORT", strconv.Itoa(port))
	t.Setenv("DATABASE_URL", databaseURL)
	t.Setenv("LOG_LEVEL", "error")
	s, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return s
}

func TestBootFailsWhenDatabaseUnreachableDeployed(t *testing.T) {
	port := testkit.FreePort(t)
	t.Setenv("KAFKA_BROKERS", "127.0.0.1:19092") // required when ENV is staging
	s := load(t, "staging", port, closedDatabaseURL)

	done := make(chan error, 1)
	go func() { done <- app.Run(context.Background(), s) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Run returned nil with an unreachable database in ENV=staging")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not fail; it is serving without its database")
	}

	// No listener was opened: the port is still free.
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d was bound by the failed boot: %v", port, err)
	}
	_ = l.Close()
}

type running struct {
	conn *grpc.ClientConn
	done chan struct{} // closed when Run returned
	err  error
}

func start(t *testing.T, s config.Settings) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{done: make(chan struct{})}
	go func() { r.err = app.Run(ctx, s); close(r.done) }()

	conn, err := grpc.NewClient("127.0.0.1:"+strconv.Itoa(s.Port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	r.conn = conn
	t.Cleanup(func() {
		_ = conn.Close()
		cancel()
		select {
		case <-r.done:
		case <-time.After(30 * time.Second):
			t.Error("app did not stop")
		}
	})
	return r
}

func asUser(id string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "x-principal-id", id, "x-principal-type", "user")
}

// Locally an unreachable database does not stop the service from serving: calls
// that need it fail with Internal until it is up.
func TestLocalStartsWithUnreachableDatabase(t *testing.T) {
	s := load(t, "local", testkit.FreePort(t), closedDatabaseURL)
	r := start(t, s)

	ctx, cancel := context.WithTimeout(asUser("user-1"), 30*time.Second)
	defer cancel()
	_, err := notificationv1.NewNotificationServiceClient(r.conn).GetUnreadCount(ctx, &notificationv1.GetUnreadCountRequest{}, grpc.WaitForReady(true))
	if status.Code(err) != codes.Internal {
		t.Fatalf("GetUnreadCount = %v, want Internal from the unreachable database", err)
	}
}

// A chat event on Kafka becomes an inbox row the recipient can list: the
// consumer, the dedupe ledger and the server run together under Run.
func TestConsumesChatEventIntoInbox(t *testing.T) {
	dbURL := testkit.Postgres(t)
	brokers := testkit.Redpanda(t)

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	files, _ := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	sort.Strings(files)
	for _, f := range files {
		ddl, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(ddl)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}

	t.Setenv("KAFKA_BROKERS", brokers)
	s := load(t, "local", testkit.FreePort(t), dbURL)
	r := start(t, s)

	payload, _ := proto.Marshal(&chatv1.ChatMessage{ThreadId: "thread-1", SenderId: "seller-1", RecipientId: "buyer-1", Content: "xin chào"})
	raw, _ := proto.Marshal(&eventsv1.EventEnvelope{EventId: "evt-1", Type: "platform.chat.v1.ChatMessage", Payload: payload})
	testkit.Produce(t, brokers, "chat.events", "thread-1", raw)

	client := notificationv1.NewNotificationServiceClient(r.conn)
	var last *notificationv1.ListNotificationsResponse
	var lastErr error
	testkit.Eventually(t, 60*time.Second, func() bool {
		ctx, cancel := context.WithTimeout(asUser("buyer-1"), 5*time.Second)
		defer cancel()
		last, lastErr = client.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{}, grpc.WaitForReady(true))
		return lastErr == nil && len(last.GetNotifications()) == 1
	})
	n := last.GetNotifications()[0]
	if !strings.HasPrefix(n.GetTitle(), "Tin nhắn mới từ") || n.GetLinkUrl() != "/chat/thread-1" {
		t.Fatalf("notification = %v (%v)", n, lastErr)
	}
}
