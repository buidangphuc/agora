package handler_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/handler"
	"github.com/buidangphuc/team-chat/internal/interceptor"
	"github.com/buidangphuc/team-chat/internal/repository"
	"github.com/buidangphuc/team-chat/internal/service"
)

type mockChatPublisher struct {
	mu            sync.Mutex
	publishedMsgs []*chatv1.ChatMessage
}

func (m *mockChatPublisher) PublishMessageSent(_ context.Context, msg *chatv1.ChatMessage, _ *commonv1.Principal, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedMsgs = append(m.publishedMsgs, msg)
	return nil
}

func (m *mockChatPublisher) Close() {}

func (m *mockChatPublisher) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.publishedMsgs)
}

func setupTestHandler() (*handler.ChatHandler, *service.ChatService, *mockChatPublisher, repository.ChatRepository) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewInMemoryChatRepository()
	svc := service.NewChatService(repo, logger)
	publisher := &mockChatPublisher{}
	h := handler.NewChatHandler(svc, publisher, logger)
	return h, svc, publisher, repo
}

func contextWithUser(userID string) context.Context {
	principal := &commonv1.Principal{
		Id:     userID,
		Type:   commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
		Scopes: []string{"chat:read", "chat:write"},
	}
	return interceptor.ContextWithPrincipal(context.Background(), principal)
}

func TestChatHandler_GetOrCreateThread(t *testing.T) {
	h, _, _, _ := setupTestHandler()

	// 1. Unauthenticated
	_, err := h.GetOrCreateThread(context.Background(), &chatv1.GetOrCreateThreadRequest{
		SellerId: "seller-1",
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Empty seller_id
	ctx := contextWithUser("buyer-1")
	_, err = h.GetOrCreateThread(ctx, &chatv1.GetOrCreateThreadRequest{
		SellerId: "",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 3. Self chat
	_, err = h.GetOrCreateThread(ctx, &chatv1.GetOrCreateThreadRequest{
		SellerId: "buyer-1",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for self-chat, got %v", err)
	}

	// 4. Successful creation
	res, err := h.GetOrCreateThread(ctx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.GetThread().GetBuyerId() != "buyer-1" || res.GetThread().GetSellerId() != "seller-1" {
		t.Fatalf("unexpected thread response: %+v", res.GetThread())
	}
}

func TestChatHandler_SendMessageAndEventEmission(t *testing.T) {
	h, _, publisher, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")

	// Create thread
	res, err := h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := res.GetThread().GetId()

	// 1. Unauthenticated
	_, err = h.SendMessage(context.Background(), &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Hello",
	})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Empty thread_id
	_, err = h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: "",
		Content:  "Hello",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 3. Empty content
	_, err = h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}

	// 4. Unauthorized participant
	intruderCtx := contextWithUser("intruder-99")
	_, err = h.SendMessage(intruderCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Spam message",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}

	// 5. Successful message send by buyer
	sendRes, err := h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Chào shop, có giao hoả tốc không?",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
	if sendRes.GetMessage().GetContent() != "Chào shop, có giao hoả tốc không?" {
		t.Fatalf("unexpected message content: %s", sendRes.GetMessage().GetContent())
	}
	if publisher.count() != 1 {
		t.Fatalf("expected 1 event published, got %d", publisher.count())
	}

	// 6. Successful message send by seller
	sellerCtx := contextWithUser("seller-1")
	sendRes2, err := h.SendMessage(sellerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Dạ có giao hoả tốc trong 2h bạn nhé!",
	})
	if err != nil {
		t.Fatalf("SendMessage seller failed: %v", err)
	}
	if sendRes2.GetMessage().GetSenderId() != "seller-1" {
		t.Fatalf("unexpected sender: %s", sendRes2.GetMessage().GetSenderId())
	}
	if publisher.count() != 2 {
		t.Fatalf("expected 2 events published, got %d", publisher.count())
	}
}

func TestChatHandler_ListThreads(t *testing.T) {
	h, _, _, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")

	// Create 2 threads
	_, _ = h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	_, _ = h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-2",
		ListingId: "listing-2",
	})

	// 1. Unauthenticated
	_, err := h.ListThreads(context.Background(), &chatv1.ListThreadsRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. List threads for buyer
	res, err := h.ListThreads(buyerCtx, &chatv1.ListThreadsRequest{})
	if err != nil {
		t.Fatalf("ListThreads failed: %v", err)
	}
	if len(res.GetThreads()) != 2 || res.GetPage().GetTotal() != 2 {
		t.Fatalf("expected 2 threads, got %d (total %d)", len(res.GetThreads()), res.GetPage().GetTotal())
	}
}

func TestChatHandler_SearchMessages(t *testing.T) {
	h, _, _, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")
	sellerCtx := contextWithUser("seller-1")

	// buyer-1 <-> seller-1 thread with messages.
	threadRes, _ := h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	threadID := threadRes.GetThread().GetId()
	_, _ = h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "Giao hàng tận nơi nhé"})
	_, _ = h.SendMessage(sellerCtx, &chatv1.SendMessageRequest{ThreadId: threadID, Content: "Dạ shop giao hàng toàn quốc"})

	// A separate intruder <-> other thread whose message also contains "hàng".
	intruderCtx := contextWithUser("intruder-9")
	otherRes, _ := h.GetOrCreateThread(intruderCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-2",
		ListingId: "listing-2",
	})
	_, _ = h.SendMessage(intruderCtx, &chatv1.SendMessageRequest{ThreadId: otherRes.GetThread().GetId(), Content: "hàng còn không shop"})

	// 1. Unauthenticated.
	_, err := h.SearchMessages(context.Background(), &chatv1.SearchMessagesRequest{Query: "hàng"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Match found for buyer-1 (both messages in their thread).
	res, err := h.SearchMessages(buyerCtx, &chatv1.SearchMessagesRequest{Query: "hàng"})
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(res.GetMessages()) != 2 || res.GetPage().GetTotal() != 2 {
		t.Fatalf("expected 2 matches, got len=%d total=%d", len(res.GetMessages()), res.GetPage().GetTotal())
	}

	// 3. Cross-user isolation: buyer-1 must not see the intruder's thread message.
	for _, m := range res.GetMessages() {
		if m.GetThreadId() != threadID {
			t.Fatalf("cross-user leak: buyer-1 saw message from thread %s", m.GetThreadId())
		}
	}

	// 4. Blank query → empty result, no error.
	blankRes, err := h.SearchMessages(buyerCtx, &chatv1.SearchMessagesRequest{Query: ""})
	if err != nil {
		t.Fatalf("blank query error: %v", err)
	}
	if len(blankRes.GetMessages()) != 0 || blankRes.GetPage().GetTotal() != 0 {
		t.Fatalf("expected empty for blank query, got len=%d total=%d", len(blankRes.GetMessages()), blankRes.GetPage().GetTotal())
	}
}

func TestChatHandler_SendListingCardRoundtrip(t *testing.T) {
	h, _, publisher, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")

	threadRes, err := h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	threadID := threadRes.GetThread().GetId()

	// Send a LISTING_CARD rich message with listing_id + JSON payload.
	payload := `{"title":"Căn hộ 2PN Quận 7","price":3200000000}`
	sendRes, err := h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId:    threadID,
		Content:     "Mời bạn xem tin đăng này",
		MessageType: chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD,
		ListingId:   "listing-42",
		Payload:     payload,
	})
	if err != nil {
		t.Fatalf("send listing card: %v", err)
	}
	got := sendRes.GetMessage()
	if got.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD {
		t.Fatalf("expected LISTING_CARD, got %v", got.GetMessageType())
	}
	if got.GetListingId() != "listing-42" || got.GetPayload() != payload {
		t.Fatalf("rich fields not returned on send: listing_id=%q payload=%q", got.GetListingId(), got.GetPayload())
	}
	if publisher.count() != 1 {
		t.Fatalf("expected 1 event published, got %d", publisher.count())
	}

	// Read it back — the type/listing_id/payload must survive persistence.
	msgsRes, err := h.GetThreadMessages(buyerCtx, &chatv1.GetThreadMessagesRequest{ThreadId: threadID})
	if err != nil {
		t.Fatalf("get thread messages: %v", err)
	}
	if len(msgsRes.GetMessages()) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgsRes.GetMessages()))
	}
	round := msgsRes.GetMessages()[0]
	if round.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_LISTING_CARD ||
		round.GetListingId() != "listing-42" || round.GetPayload() != payload {
		t.Fatalf("rich fields did not roundtrip: %+v", round)
	}
}

func TestChatHandler_SendPlainTextBackCompat(t *testing.T) {
	h, _, _, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")

	threadRes, _ := h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	threadID := threadRes.GetThread().GetId()

	// A legacy-style request that never sets message_type/listing_id/payload.
	sendRes, err := h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Xin chào shop",
	})
	if err != nil {
		t.Fatalf("send plain text: %v", err)
	}
	got := sendRes.GetMessage()
	if got.GetMessageType() != chatv1.MessageType_MESSAGE_TYPE_TEXT {
		t.Fatalf("expected TEXT (zero value), got %v", got.GetMessageType())
	}
	if got.GetListingId() != "" || got.GetPayload() != "" {
		t.Fatalf("expected empty rich fields, got listing_id=%q payload=%q", got.GetListingId(), got.GetPayload())
	}
	if got.GetContent() != "Xin chào shop" {
		t.Fatalf("unexpected content: %q", got.GetContent())
	}
}

func TestChatHandler_ListQuickReplies(t *testing.T) {
	h, _, _, repo := setupTestHandler()
	callerCtx := contextWithUser("buyer-1")

	// 1. Unauthenticated.
	if _, err := h.ListQuickReplies(context.Background(), &chatv1.ListQuickRepliesRequest{SellerId: "seller-1"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}

	// 2. Seller with no configured replies → seeded defaults (non-empty).
	res, err := h.ListQuickReplies(callerCtx, &chatv1.ListQuickRepliesRequest{SellerId: "seller-1"})
	if err != nil {
		t.Fatalf("list quick replies: %v", err)
	}
	if len(res.GetQuickReplies()) == 0 {
		t.Fatal("expected seeded default quick replies, got none")
	}

	// 3. Seller-configured replies are returned verbatim.
	inMem, ok := repo.(*repository.InMemoryChatRepository)
	if !ok {
		t.Fatalf("expected InMemoryChatRepository, got %T", repo)
	}
	custom := []string{"Còn hàng bạn nhé", "Giá có thương lượng"}
	inMem.SetQuickReplies("seller-1", custom)

	res, err = h.ListQuickReplies(callerCtx, &chatv1.ListQuickRepliesRequest{SellerId: "seller-1"})
	if err != nil {
		t.Fatalf("list configured quick replies: %v", err)
	}
	if len(res.GetQuickReplies()) != 2 || res.GetQuickReplies()[0] != "Còn hàng bạn nhé" {
		t.Fatalf("expected configured replies, got %v", res.GetQuickReplies())
	}

	// 4. Empty seller_id falls back to the caller's own id (seller viewing own).
	sellerCtx := contextWithUser("seller-1")
	res, err = h.ListQuickReplies(sellerCtx, &chatv1.ListQuickRepliesRequest{})
	if err != nil {
		t.Fatalf("list own quick replies: %v", err)
	}
	if len(res.GetQuickReplies()) != 2 {
		t.Fatalf("expected caller-scoped replies, got %v", res.GetQuickReplies())
	}
}

func TestChatHandler_GetThreadMessagesAndMarkRead(t *testing.T) {
	h, _, _, _ := setupTestHandler()
	buyerCtx := contextWithUser("buyer-1")
	sellerCtx := contextWithUser("seller-1")

	// Create thread & send messages
	threadRes, _ := h.GetOrCreateThread(buyerCtx, &chatv1.GetOrCreateThreadRequest{
		SellerId:  "seller-1",
		ListingId: "listing-1",
	})
	threadID := threadRes.GetThread().GetId()

	_, _ = h.SendMessage(buyerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Tin nhắn 1",
	})
	_, _ = h.SendMessage(sellerCtx, &chatv1.SendMessageRequest{
		ThreadId: threadID,
		Content:  "Tin nhắn 2",
	})

	// 1. GetThreadMessages
	msgsRes, err := h.GetThreadMessages(buyerCtx, &chatv1.GetThreadMessagesRequest{
		ThreadId: threadID,
	})
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}
	if len(msgsRes.GetMessages()) != 2 || msgsRes.GetPage().GetTotal() != 2 {
		t.Fatalf("expected 2 messages, got len=%d total=%d", len(msgsRes.GetMessages()), msgsRes.GetPage().GetTotal())
	}
	if msgsRes.GetMessages()[0].GetContent() != "Tin nhắn 1" || msgsRes.GetMessages()[1].GetContent() != "Tin nhắn 2" {
		t.Fatalf("messages not ordered chronologically: %+v", msgsRes.GetMessages())
	}

	// 2. GetThreadMessages unauthorized
	intruderCtx := contextWithUser("intruder")
	_, err = h.GetThreadMessages(intruderCtx, &chatv1.GetThreadMessagesRequest{
		ThreadId: threadID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}

	// 3. MarkThreadRead
	_, err = h.MarkThreadRead(buyerCtx, &chatv1.MarkThreadReadRequest{
		ThreadId: threadID,
	})
	if err != nil {
		t.Fatalf("MarkThreadRead failed: %v", err)
	}

	// 4. MarkThreadRead unauthorized
	_, err = h.MarkThreadRead(intruderCtx, &chatv1.MarkThreadReadRequest{
		ThreadId: threadID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected PermissionDenied, got %v", err)
	}
}
