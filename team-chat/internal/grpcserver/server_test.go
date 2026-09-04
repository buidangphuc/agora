package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	"github.com/buidangphuc/team-chat/internal/config"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/grpcserver"
	"github.com/buidangphuc/team-chat/internal/handler"
	"github.com/buidangphuc/team-chat/internal/repository"
	"github.com/buidangphuc/team-chat/internal/service"
)

func startServer(t *testing.T) chatv1.ChatServiceClient {
	t.Helper()
	s := &config.Settings{}
	s.Server.Port = 0
	s.Server.ReflectionEnabled = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewInMemoryChatRepository()
	svc := service.NewChatService(repo, logger)
	h := handler.NewChatHandler(svc, events.NoopPublisher{}, logger)
	srv := grpcserver.Build(s, h, nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(); srv.Stop() })
	return chatv1.NewChatServiceClient(conn)
}

func userCtx(t *testing.T, id string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs("x-principal-id", id, "x-principal-type", "user", "x-principal-scopes", "chat:read,chat:write")
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func TestChatThreadAndMessageLifecycle(t *testing.T) {
	client := startServer(t)

	buyerCtx, buyerCancel := userCtx(t, "buyer-123")
	defer buyerCancel()

	// 1. GetOrCreateThread by buyer
	res, err := client.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-456",
		ListingId: "listing-789",
	})
	if err != nil {
		t.Fatalf("GetOrCreateThread: %v", err)
	}
	thread := res.GetThread()
	if thread.GetBuyerId() != "buyer-123" || thread.GetSellerId() != "seller-456" {
		t.Fatalf("unexpected thread parties: %+v", thread)
	}

	// 2. Send message from buyer
	sendRes, err := client.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: thread.GetId(),
		Content:  "Xin chào shop, sản phẩm còn hàng không ạ?",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if sendRes.GetMessage().GetContent() != "Xin chào shop, sản phẩm còn hàng không ạ?" {
		t.Fatalf("unexpected message content: %+v", sendRes.GetMessage())
	}

	// 3. Seller lists threads and sees unread count = 1
	sellerCtx, sellerCancel := userCtx(t, "seller-456")
	defer sellerCancel()

	threadsList, err := client.ListThreads(sellerCtx, &chatv1.ListThreadsRequest{})
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threadsList.GetThreads()) != 1 {
		t.Fatalf("want 1 thread for seller, got %d", len(threadsList.GetThreads()))
	}
	if threadsList.GetThreads()[0].GetUnreadCountSeller() != 1 {
		t.Fatalf("want unread seller count 1, got %d", threadsList.GetThreads()[0].GetUnreadCountSeller())
	}

	// 4. Seller replies to thread
	_, err = client.SendMessage(sellerCtx, &chatv1.SendMessageRequest{
		ThreadId: thread.GetId(),
		Content:  "Chào bạn, sản phẩm vẫn còn sẵn hàng bạn nhé!",
	})
	if err != nil {
		t.Fatalf("Seller SendMessage: %v", err)
	}

	// 5. Get messages for thread
	msgsRes, err := client.GetThreadMessages(buyerCtx, &chatv1.GetThreadMessagesRequest{
		ThreadId: thread.GetId(),
	})
	if err != nil {
		t.Fatalf("GetThreadMessages: %v", err)
	}
	if len(msgsRes.GetMessages()) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgsRes.GetMessages()))
	}

	// 6. Mark read
	_, err = client.MarkThreadRead(buyerCtx, &chatv1.MarkThreadReadRequest{
		ThreadId: thread.GetId(),
	})
	if err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
}
