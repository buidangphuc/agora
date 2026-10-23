package chat_test

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-chat/internal/store"
)

type outboxBuilder func(context.Context, store.ChatMessage) (outbox.Row, error)

type fakeRepo struct {
	mu          sync.Mutex
	builder     outboxBuilder
	threads     map[string]store.ChatThread
	messages    map[string][]store.ChatMessage
	rows        []outbox.Row
	searchCalls int
}

func newFakeRepo(b outboxBuilder) *fakeRepo {
	return &fakeRepo{
		builder:  b,
		threads:  map[string]store.ChatThread{},
		messages: map[string][]store.ChatMessage{},
	}
}

func (r *fakeRepo) outboxRows() []outbox.Row {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]outbox.Row(nil), r.rows...)
}

func (r *fakeRepo) searches() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.searchCalls
}

func (r *fakeRepo) ListQuickReplies(context.Context, string) ([]string, error) {
	return []string{"ok"}, nil
}

func (r *fakeRepo) GetOrCreateThread(_ context.Context, buyerID, sellerID, listingID, listingTitle, listingImage string) (store.ChatThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.threads {
		if t.BuyerID == buyerID && t.SellerID == sellerID && t.ListingID == listingID {
			return t, nil
		}
	}
	now := time.Now()
	t := store.ChatThread{ID: uuid.NewString(), BuyerID: buyerID, SellerID: sellerID, ListingID: listingID, ListingTitle: listingTitle, ListingImageURL: listingImage, CreatedAt: now, UpdatedAt: now}
	r.threads[t.ID] = t
	return t, nil
}

func (r *fakeRepo) GetThreadByID(_ context.Context, id string) (store.ChatThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.threads[id]
	if !ok {
		return store.ChatThread{}, store.ErrThreadNotFound
	}
	return t, nil
}

func page[T any](items []T, offset, limit int) []T {
	if offset > len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

func (r *fakeRepo) ListThreadsForUser(_ context.Context, userID string, offset, limit int) ([]store.ChatThread, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []store.ChatThread
	for _, t := range r.threads {
		if t.BuyerID == userID || t.SellerID == userID {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return page(out, offset, limit), int64(len(out)), nil
}

func (r *fakeRepo) SaveMessage(ctx context.Context, msg store.ChatMessage) (store.ChatMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.CreatedAt = time.Now()
	if r.builder != nil {
		row, err := r.builder(ctx, msg)
		if err != nil {
			return store.ChatMessage{}, fmt.Errorf("build chat outbox row: %w", err)
		}
		r.rows = append(r.rows, row)
	}
	r.messages[msg.ThreadID] = append(r.messages[msg.ThreadID], msg)
	if t, ok := r.threads[msg.ThreadID]; ok {
		t.LastMessageText = msg.Content
		t.LastMessageAt = &msg.CreatedAt
		t.UpdatedAt = msg.CreatedAt
		if t.BuyerID != msg.SenderID {
			t.UnreadCountBuyer++
		}
		if t.SellerID != msg.SenderID {
			t.UnreadCountSeller++
		}
		r.threads[msg.ThreadID] = t
	}
	return msg, nil
}

func (r *fakeRepo) GetThreadMessages(_ context.Context, threadID string, offset, limit int) ([]store.ChatMessage, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msgs := r.messages[threadID]
	return page(msgs, offset, limit), int64(len(msgs)), nil
}

func (r *fakeRepo) SearchMessages(_ context.Context, userID, query string, offset, limit int) ([]store.ChatMessage, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searchCalls++
	q := strings.ToLower(query)
	var out []store.ChatMessage
	for id, msgs := range r.messages {
		t, ok := r.threads[id]
		if !ok || (t.BuyerID != userID && t.SellerID != userID) {
			continue
		}
		for _, m := range msgs {
			if strings.Contains(strings.ToLower(m.Content), q) {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return page(out, offset, limit), int64(len(out)), nil
}

func (r *fakeRepo) MarkThreadRead(_ context.Context, threadID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.threads[threadID]; ok {
		if t.BuyerID == userID {
			t.UnreadCountBuyer = 0
		}
		if t.SellerID == userID {
			t.UnreadCountSeller = 0
		}
		r.threads[threadID] = t
	}
	return nil
}
