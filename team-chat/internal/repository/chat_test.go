package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/buidangphuc/team-chat/internal/repository"
)

func TestInMemoryChatRepository_GetOrCreateThread(t *testing.T) {
	repo := repository.NewInMemoryChatRepository()
	ctx := context.Background()

	// 1. Create a new thread
	thread1, err := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Macbook Pro M3", "http://img.png")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if thread1.ID == "" {
		t.Fatal("expected non-empty thread ID")
	}
	if thread1.BuyerID != "buyer-1" || thread1.SellerID != "seller-1" || thread1.ListingID != "listing-1" {
		t.Fatalf("unexpected thread data: %+v", thread1)
	}

	// 2. Fetch existing thread (should return same thread ID)
	thread2, err := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Other Title", "")
	if err != nil {
		t.Fatalf("unexpected error fetching existing thread: %v", err)
	}
	if thread2.ID != thread1.ID {
		t.Fatalf("expected same thread ID %s, got %s", thread1.ID, thread2.ID)
	}

	// 3. Get thread by ID
	fetched, err := repo.GetThreadByID(ctx, thread1.ID)
	if err != nil {
		t.Fatalf("GetThreadByID failed: %v", err)
	}
	if fetched.ID != thread1.ID {
		t.Fatalf("expected ID %s, got %s", thread1.ID, fetched.ID)
	}

	// 4. Not found thread
	_, err = repo.GetThreadByID(ctx, "non-existent")
	if err != repository.ErrThreadNotFound {
		t.Fatalf("expected ErrThreadNotFound, got %v", err)
	}
}

func TestInMemoryChatRepository_MessagesAndUnreadCounts(t *testing.T) {
	repo := repository.NewInMemoryChatRepository()
	ctx := context.Background()

	thread, err := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Product", "")
	if err != nil {
		t.Fatalf("failed to create thread: %v", err)
	}

	// 1. Buyer sends message
	msg1, err := repo.SaveMessage(ctx, repository.ChatMessage{
		ThreadID:   thread.ID,
		SenderID:   "buyer-1",
		SenderName: "Buyer User",
		Content:    "Hello seller!",
	})
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	if msg1.ID == "" {
		t.Fatal("expected non-empty msg ID")
	}

	// Check thread updated
	updatedThread, err := repo.GetThreadByID(ctx, thread.ID)
	if err != nil {
		t.Fatalf("GetThreadByID failed: %v", err)
	}
	if updatedThread.LastMessageText != "Hello seller!" {
		t.Fatalf("expected last message 'Hello seller!', got '%s'", updatedThread.LastMessageText)
	}
	if updatedThread.UnreadCountSeller != 1 {
		t.Fatalf("expected unread count seller 1, got %d", updatedThread.UnreadCountSeller)
	}
	if updatedThread.UnreadCountBuyer != 0 {
		t.Fatalf("expected unread count buyer 0, got %d", updatedThread.UnreadCountBuyer)
	}

	// 2. Seller marks read
	err = repo.MarkThreadRead(ctx, thread.ID, "seller-1")
	if err != nil {
		t.Fatalf("MarkThreadRead failed: %v", err)
	}
	updatedThread, _ = repo.GetThreadByID(ctx, thread.ID)
	if updatedThread.UnreadCountSeller != 0 {
		t.Fatalf("expected unread count seller 0 after mark read, got %d", updatedThread.UnreadCountSeller)
	}

	// 3. Seller sends reply
	msg2, err := repo.SaveMessage(ctx, repository.ChatMessage{
		ThreadID:   thread.ID,
		SenderID:   "seller-1",
		SenderName: "Seller Shop",
		Content:    "Hi buyer, how can I help?",
	})
	if err != nil {
		t.Fatalf("SaveMessage 2 failed: %v", err)
	}

	updatedThread, _ = repo.GetThreadByID(ctx, thread.ID)
	if updatedThread.UnreadCountBuyer != 1 {
		t.Fatalf("expected unread count buyer 1, got %d", updatedThread.UnreadCountBuyer)
	}

	// 4. Get thread messages (chronological order)
	msgs, total, err := repo.GetThreadMessages(ctx, thread.ID, 0, 10)
	if err != nil {
		t.Fatalf("GetThreadMessages failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].ID != msg1.ID || msgs[1].ID != msg2.ID {
		t.Fatalf("messages not in chronological order: %+v", msgs)
	}

	// Pagination test
	pagedMsgs, total, err := repo.GetThreadMessages(ctx, thread.ID, 1, 1)
	if err != nil {
		t.Fatalf("GetThreadMessages pagination failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(pagedMsgs) != 1 || pagedMsgs[0].ID != msg2.ID {
		t.Fatalf("unexpected paged message: %+v", pagedMsgs)
	}
}

func TestInMemoryChatRepository_SearchMessages(t *testing.T) {
	repo := repository.NewInMemoryChatRepository()
	ctx := context.Background()

	// buyer-1 <-> seller-1 thread
	tA, _ := repo.GetOrCreateThread(ctx, "buyer-1", "seller-1", "listing-1", "Item", "")
	// buyer-2 <-> seller-2 thread (unrelated to buyer-1)
	tB, _ := repo.GetOrCreateThread(ctx, "buyer-2", "seller-2", "listing-2", "Item", "")

	_, _ = repo.SaveMessage(ctx, repository.ChatMessage{ThreadID: tA.ID, SenderID: "buyer-1", Content: "Còn hàng giao nhanh không shop?"})
	time.Sleep(5 * time.Millisecond)
	_, _ = repo.SaveMessage(ctx, repository.ChatMessage{ThreadID: tA.ID, SenderID: "seller-1", Content: "Dạ còn hàng nhé bạn"})
	// A message in an unrelated thread that also contains the term.
	_, _ = repo.SaveMessage(ctx, repository.ChatMessage{ThreadID: tB.ID, SenderID: "buyer-2", Content: "hàng này bảo hành lâu không?"})

	// 1. Match found, scoped to caller's threads, newest-first (case-insensitive ILIKE).
	msgs, total, err := repo.SearchMessages(ctx, "buyer-1", "HÀNG", 0, 10)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if total != 2 || len(msgs) != 2 {
		t.Fatalf("expected 2 matches for buyer-1, got len=%d total=%d", len(msgs), total)
	}
	if msgs[0].Content != "Dạ còn hàng nhé bạn" {
		t.Fatalf("expected newest-first ordering, got %q first", msgs[0].Content)
	}

	// 2. No-match → empty result, no error.
	none, total, err := repo.SearchMessages(ctx, "buyer-1", "xyzzy-no-match", 0, 10)
	if err != nil {
		t.Fatalf("SearchMessages no-match failed: %v", err)
	}
	if total != 0 || len(none) != 0 {
		t.Fatalf("expected 0 matches, got len=%d total=%d", len(none), total)
	}

	// 3. Cross-user isolation: buyer-1 must never see buyer-2's thread messages.
	for _, m := range msgs {
		if m.ThreadID == tB.ID {
			t.Fatalf("cross-user leak: buyer-1 saw message from thread %s", tB.ID)
		}
	}
	// And seller-2 sees only their own matching message.
	other, total, err := repo.SearchMessages(ctx, "seller-2", "hàng", 0, 10)
	if err != nil {
		t.Fatalf("SearchMessages seller-2 failed: %v", err)
	}
	if total != 1 || len(other) != 1 || other[0].ThreadID != tB.ID {
		t.Fatalf("expected seller-2 to see only their 1 message, got len=%d total=%d", len(other), total)
	}
}

func TestInMemoryChatRepository_ListThreadsForUser(t *testing.T) {
	repo := repository.NewInMemoryChatRepository()
	ctx := context.Background()

	t1, err := repo.GetOrCreateThread(ctx, "user-a", "user-b", "listing-1", "Item 1", "")
	if err != nil {
		t.Fatalf("create t1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	t2, err := repo.GetOrCreateThread(ctx, "user-c", "user-a", "listing-2", "Item 2", "")
	if err != nil {
		t.Fatalf("create t2: %v", err)
	}

	// List threads for user-a (participant as buyer in t1, seller in t2)
	threads, total, err := repo.ListThreadsForUser(ctx, "user-a", 0, 10)
	if err != nil {
		t.Fatalf("ListThreadsForUser failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	// t2 was created later so should be first
	if threads[0].ID != t2.ID || threads[1].ID != t1.ID {
		t.Fatalf("expected [t2, t1], got [%s, %s]", threads[0].ID, threads[1].ID)
	}

	// List threads for unrelated user
	unrelated, totalUnrelated, err := repo.ListThreadsForUser(ctx, "user-nobody", 0, 10)
	if err != nil {
		t.Fatalf("ListThreadsForUser nobody: %v", err)
	}
	if totalUnrelated != 0 || len(unrelated) != 0 {
		t.Fatalf("expected 0 threads for user-nobody, got %d (len %d)", totalUnrelated, len(unrelated))
	}
}
