package service_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/buidangphuc/team-chat/internal/repository"
	"github.com/buidangphuc/team-chat/internal/service"
)

func setupTestService() (*service.ChatService, repository.ChatRepository) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := repository.NewInMemoryChatRepository()
	svc := service.NewChatService(repo, logger)
	return svc, repo
}

func TestChatService_GetOrCreateThread(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	// 1. Success
	thread, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-100", "Shoes", "http://shoes.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if thread.ID == "" || thread.BuyerID != "buyer-1" || thread.SellerID != "seller-1" {
		t.Fatalf("invalid thread created: %+v", thread)
	}

	// 2. Empty buyer
	_, err = svc.GetOrCreateThread(ctx, "", "seller-1", "listing-100", "Shoes", "")
	if err == nil {
		t.Fatal("expected error for empty buyer_id")
	}

	// 3. Empty seller
	_, err = svc.GetOrCreateThread(ctx, "buyer-1", "", "listing-100", "Shoes", "")
	if err == nil {
		t.Fatal("expected error for empty seller_id")
	}

	// 4. Same buyer and seller (self chat)
	_, err = svc.GetOrCreateThread(ctx, "user-1", "user-1", "listing-100", "Shoes", "")
	if !errors.Is(err, service.ErrSelfChat) {
		t.Fatalf("expected ErrSelfChat, got %v", err)
	}
}

func TestChatService_SearchMessages(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	thread, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Book", "")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := svc.SendMessage(ctx, thread.ID, "buyer-1", "Buyer", "Sách này còn bản mới không?"); err != nil {
		t.Fatalf("send message: %v", err)
	}

	// 1. Match found.
	msgs, total, err := svc.SearchMessages(ctx, "buyer-1", "sách", 1, 20)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if total != 1 || len(msgs) != 1 {
		t.Fatalf("expected 1 match, got len=%d total=%d", len(msgs), total)
	}

	// 2. Blank query → empty result, no error, no repo scan.
	blank, total, err := svc.SearchMessages(ctx, "buyer-1", "   ", 1, 20)
	if err != nil {
		t.Fatalf("blank query returned error: %v", err)
	}
	if total != 0 || len(blank) != 0 {
		t.Fatalf("expected empty result for blank query, got len=%d total=%d", len(blank), total)
	}

	// 3. Empty userID → error.
	if _, _, err := svc.SearchMessages(ctx, "", "sách", 1, 20); !errors.Is(err, service.ErrEmptySender) {
		t.Fatalf("expected ErrEmptySender, got %v", err)
	}
}

func TestChatService_SendMessage(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	thread, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Book", "")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// 1. Valid message from buyer
	msg, err := svc.SendMessage(ctx, thread.ID, "buyer-1", "Buyer", "Is this book in good condition?")
	if err != nil {
		t.Fatalf("unexpected error sending message: %v", err)
	}
	if msg.Content != "Is this book in good condition?" || msg.SenderID != "buyer-1" {
		t.Fatalf("unexpected message: %+v", msg)
	}

	// 2. Empty thread ID
	_, err = svc.SendMessage(ctx, "", "buyer-1", "Buyer", "Hello")
	if !errors.Is(err, service.ErrEmptyThreadID) {
		t.Fatalf("expected ErrEmptyThreadID, got %v", err)
	}

	// 3. Empty sender ID
	_, err = svc.SendMessage(ctx, thread.ID, "", "Buyer", "Hello")
	if !errors.Is(err, service.ErrEmptySender) {
		t.Fatalf("expected ErrEmptySender, got %v", err)
	}

	// 4. Empty content
	_, err = svc.SendMessage(ctx, thread.ID, "buyer-1", "Buyer", "")
	if !errors.Is(err, service.ErrEmptyContent) {
		t.Fatalf("expected ErrEmptyContent, got %v", err)
	}

	// 5. Non-participant sends message
	_, err = svc.SendMessage(ctx, thread.ID, "intruder-99", "Intruder", "Hey!")
	if !errors.Is(err, service.ErrUnauthorizedChat) {
		t.Fatalf("expected ErrUnauthorizedChat, got %v", err)
	}

	// 6. Non-existent thread
	_, err = svc.SendMessage(ctx, "invalid-thread", "buyer-1", "Buyer", "Hey!")
	if !errors.Is(err, repository.ErrThreadNotFound) {
		t.Fatalf("expected ErrThreadNotFound, got %v", err)
	}
}

func TestChatService_GetThreadMessages(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	thread, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Book", "")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	_, _ = svc.SendMessage(ctx, thread.ID, "buyer-1", "Buyer", "Msg 1")
	_, _ = svc.SendMessage(ctx, thread.ID, "seller-1", "Seller", "Msg 2")

	// 1. Success for buyer
	msgs, total, err := svc.GetThreadMessages(ctx, thread.ID, "buyer-1", 1, 10)
	if err != nil {
		t.Fatalf("GetThreadMessages buyer: %v", err)
	}
	if total != 2 || len(msgs) != 2 {
		t.Fatalf("expected 2 messages, total=%d, len=%d", total, len(msgs))
	}

	// 2. Success for seller
	msgs, total, err = svc.GetThreadMessages(ctx, thread.ID, "seller-1", 1, 10)
	if err != nil {
		t.Fatalf("GetThreadMessages seller: %v", err)
	}
	if total != 2 || len(msgs) != 2 {
		t.Fatalf("expected 2 messages for seller, total=%d, len=%d", total, len(msgs))
	}

	// 3. Unauthorized access
	_, _, err = svc.GetThreadMessages(ctx, thread.ID, "intruder-99", 1, 10)
	if !errors.Is(err, service.ErrUnauthorizedChat) {
		t.Fatalf("expected ErrUnauthorizedChat, got %v", err)
	}

	// 4. Empty thread ID
	_, _, err = svc.GetThreadMessages(ctx, "", "buyer-1", 1, 10)
	if !errors.Is(err, service.ErrEmptyThreadID) {
		t.Fatalf("expected ErrEmptyThreadID, got %v", err)
	}
}

func TestChatService_ListThreads(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	_, _ = svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Book 1", "")
	_, _ = svc.GetOrCreateThread(ctx, "buyer-1", "seller-2", "listing-2", "Book 2", "")

	// 1. Success
	threads, total, err := svc.ListThreads(ctx, "buyer-1", 1, 10)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if total != 2 || len(threads) != 2 {
		t.Fatalf("expected 2 threads, got total=%d len=%d", total, len(threads))
	}

	// 2. Empty user ID
	_, _, err = svc.ListThreads(ctx, "", 1, 10)
	if !errors.Is(err, service.ErrEmptySender) {
		t.Fatalf("expected ErrEmptySender, got %v", err)
	}
}

func TestChatService_GetThreadAndMarkThreadRead(t *testing.T) {
	svc, _ := setupTestService()
	ctx := context.Background()

	thread, err := svc.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Book", "")
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	// 1. GetThread success
	fetched, err := svc.GetThread(ctx, thread.ID, "buyer-1")
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if fetched.ID != thread.ID {
		t.Fatalf("expected thread %s, got %s", thread.ID, fetched.ID)
	}

	// 2. GetThread unauthorized
	_, err = svc.GetThread(ctx, thread.ID, "intruder")
	if !errors.Is(err, service.ErrUnauthorizedChat) {
		t.Fatalf("expected ErrUnauthorizedChat, got %v", err)
	}

	// 3. MarkThreadRead success
	_, _ = svc.SendMessage(ctx, thread.ID, "buyer-1", "Buyer", "Hello")
	err = svc.MarkThreadRead(ctx, thread.ID, "seller-1")
	if err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}

	// 4. MarkThreadRead unauthorized
	err = svc.MarkThreadRead(ctx, thread.ID, "intruder")
	if !errors.Is(err, service.ErrUnauthorizedChat) {
		t.Fatalf("expected ErrUnauthorizedChat, got %v", err)
	}

	// 5. MarkThreadRead empty thread
	err = svc.MarkThreadRead(ctx, "", "seller-1")
	if !errors.Is(err, service.ErrEmptyThreadID) {
		t.Fatalf("expected ErrEmptyThreadID, got %v", err)
	}
}
