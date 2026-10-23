package rpc_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"

	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/store"
)

type pageCall struct {
	offset int
	limit  int
}

type fakeRepo struct {
	mu          sync.Mutex
	threads     []store.ChatThread
	messages    map[string][]store.ChatMessage
	rows        []outbox.Row
	quick       map[string][]string
	quickAsked  []string
	listCalls   []pageCall
	msgCalls    []pageCall
	searchCalls []pageCall
	failList    error
	failSave    error
	failThread  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{messages: map[string][]store.ChatMessage{}, quick: map[string][]string{}}
}

func (r *fakeRepo) setQuickReplies(sellerID string, replies []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quick[sellerID] = replies
}

func (r *fakeRepo) seedThread(id, buyerID, sellerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.threads = append(r.threads, store.ChatThread{ID: id, BuyerID: buyerID, SellerID: sellerID, ListingID: id, CreatedAt: now, UpdatedAt: now})
}

func (r *fakeRepo) seedMessages(threadID, senderID, prefix string, n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < n; i++ {
		r.messages[threadID] = append(r.messages[threadID], store.ChatMessage{
			ID:       fmt.Sprintf("%s-%s-%03d", threadID, prefix, i),
			ThreadID: threadID,
			SenderID: senderID,
			Content:  fmt.Sprintf("%s-%03d", prefix, i),
		})
	}
}

func (r *fakeRepo) outboxRows() []outbox.Row {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]outbox.Row(nil), r.rows...)
}

func (r *fakeRepo) storedMessages(threadID string) []store.ChatMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]store.ChatMessage(nil), r.messages[threadID]...)
}

func (r *fakeRepo) thread(id string) store.ChatThread {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.threads {
		if t.ID == id {
			return t
		}
	}
	return store.ChatThread{}
}

func (r *fakeRepo) lastCall(calls *[]pageCall) pageCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(*calls) == 0 {
		return pageCall{offset: -1, limit: -1}
	}
	return (*calls)[len(*calls)-1]
}

func (r *fakeRepo) lastListCall() pageCall   { return r.lastCall(&r.listCalls) }
func (r *fakeRepo) lastMsgCall() pageCall    { return r.lastCall(&r.msgCalls) }
func (r *fakeRepo) lastSearchCall() pageCall { return r.lastCall(&r.searchCalls) }

func (r *fakeRepo) ListQuickReplies(_ context.Context, sellerID string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quickAsked = append(r.quickAsked, sellerID)
	if q, ok := r.quick[sellerID]; ok {
		return q, nil
	}
	return []string{"default-1", "default-2"}, nil
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
	r.threads = append(r.threads, t)
	return t, nil
}

func (r *fakeRepo) GetThreadByID(_ context.Context, id string) (store.ChatThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failThread != nil {
		return store.ChatThread{}, r.failThread
	}
	for _, t := range r.threads {
		if t.ID == id {
			return t, nil
		}
	}
	return store.ChatThread{}, store.ErrThreadNotFound
}

func window[T any](items []T, offset, limit int) []T {
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
	r.listCalls = append(r.listCalls, pageCall{offset, limit})
	if r.failList != nil {
		return nil, 0, r.failList
	}
	var out []store.ChatThread
	for _, t := range r.threads {
		if t.BuyerID == userID || t.SellerID == userID {
			out = append(out, t)
		}
	}
	return window(out, offset, limit), int64(len(out)), nil
}

func (r *fakeRepo) SaveMessage(ctx context.Context, msg store.ChatMessage) (store.ChatMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSave != nil {
		return store.ChatMessage{}, r.failSave
	}
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.CreatedAt = time.Now()
	row, err := events.BuildMessageOutboxRow(ctx, msg)
	if err != nil {
		return store.ChatMessage{}, err
	}
	r.rows = append(r.rows, row)
	r.messages[msg.ThreadID] = append(r.messages[msg.ThreadID], msg)
	for i, t := range r.threads {
		if t.ID != msg.ThreadID {
			continue
		}
		t.LastMessageText = msg.Content
		at := msg.CreatedAt
		t.LastMessageAt = &at
		t.UpdatedAt = at
		if t.BuyerID != msg.SenderID {
			t.UnreadCountBuyer++
		}
		if t.SellerID != msg.SenderID {
			t.UnreadCountSeller++
		}
		r.threads[i] = t
	}
	return msg, nil
}

func (r *fakeRepo) GetThreadMessages(_ context.Context, threadID string, offset, limit int) ([]store.ChatMessage, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgCalls = append(r.msgCalls, pageCall{offset, limit})
	msgs := r.messages[threadID]
	return window(msgs, offset, limit), int64(len(msgs)), nil
}

func (r *fakeRepo) SearchMessages(_ context.Context, userID, query string, offset, limit int) ([]store.ChatMessage, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.searchCalls = append(r.searchCalls, pageCall{offset, limit})
	q := strings.ToLower(query)
	var out []store.ChatMessage
	for _, t := range r.threads {
		if t.BuyerID != userID && t.SellerID != userID {
			continue
		}
		for _, m := range r.messages[t.ID] {
			if strings.Contains(strings.ToLower(m.Content), q) {
				out = append(out, m)
			}
		}
	}
	return window(out, offset, limit), int64(len(out)), nil
}

func (r *fakeRepo) MarkThreadRead(_ context.Context, threadID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, t := range r.threads {
		if t.ID != threadID {
			continue
		}
		if t.BuyerID == userID {
			t.UnreadCountBuyer = 0
		}
		if t.SellerID == userID {
			t.UnreadCountSeller = 0
		}
		r.threads[i] = t
	}
	return nil
}
