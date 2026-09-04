package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrThreadNotFound = errors.New("chat thread not found")

type ChatThread struct {
	ID                string
	BuyerID           string
	SellerID          string
	ListingID         string
	ListingTitle      string
	ListingImageURL   string
	LastMessageText   string
	LastMessageAt     *time.Time
	UnreadCountBuyer  int32
	UnreadCountSeller int32
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ChatMessage struct {
	ID          string
	ThreadID    string
	SenderID    string
	SenderName  string
	Content     string
	CreatedAt   time.Time
	MessageType int32  // 0=TEXT, 1=LISTING_CARD, 2=QUICK_REPLY (mirrors chatv1.MessageType)
	ListingID   string // set for LISTING_CARD messages
	Payload     string // JSON blob for rich payloads (card/quick-reply data)
}

type ChatRepository interface {
	GetOrCreateThread(ctx context.Context, buyerID, sellerID, listingID, listingTitle, listingImage string) (ChatThread, error)
	GetThreadByID(ctx context.Context, threadID string) (ChatThread, error)
	ListThreadsForUser(ctx context.Context, userID string, offset, limit int) ([]ChatThread, int64, error)
	SaveMessage(ctx context.Context, msg ChatMessage) (ChatMessage, error)
	GetThreadMessages(ctx context.Context, threadID string, offset, limit int) ([]ChatMessage, int64, error)
	SearchMessages(ctx context.Context, userID, query string, offset, limit int) ([]ChatMessage, int64, error)
	MarkThreadRead(ctx context.Context, threadID, userID string) error
	ListQuickReplies(ctx context.Context, sellerID string) ([]string, error)
}

// defaultQuickReplies are the Vietnamese canned replies returned when a seller
// has not configured any of their own — keeps the feature useful out of the box.
func defaultQuickReplies() []string {
	return []string{
		"Chào bạn, sản phẩm vẫn còn hàng nhé!",
		"Bạn muốn xem thêm hình ảnh không ạ?",
		"Shop hỗ trợ xem nhà/sản phẩm trực tiếp.",
		"Bạn để lại số điện thoại, shop sẽ liên hệ ngay.",
	}
}

// ── Postgres Implementation ──────────────────────────────────────────

type PostgresChatRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresChatRepository(pool *pgxpool.Pool) *PostgresChatRepository {
	return &PostgresChatRepository{pool: pool}
}

const threadColumns = `id, buyer_id, seller_id, listing_id, listing_title, listing_image_url, last_message_text, last_message_at, unread_count_buyer, unread_count_seller, created_at, updated_at`

func scanThread(row pgx.Row) (ChatThread, error) {
	var t ChatThread
	err := row.Scan(
		&t.ID,
		&t.BuyerID,
		&t.SellerID,
		&t.ListingID,
		&t.ListingTitle,
		&t.ListingImageURL,
		&t.LastMessageText,
		&t.LastMessageAt,
		&t.UnreadCountBuyer,
		&t.UnreadCountSeller,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	return t, err
}

func (r *PostgresChatRepository) GetOrCreateThread(
	ctx context.Context,
	buyerID, sellerID, listingID, listingTitle, listingImage string,
) (ChatThread, error) {
	const selectQ = `SELECT ` + threadColumns + ` FROM chat_threads WHERE buyer_id = $1 AND seller_id = $2 AND listing_id = $3`
	row := r.pool.QueryRow(ctx, selectQ, buyerID, sellerID, listingID)
	t, err := scanThread(row)
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ChatThread{}, fmt.Errorf("query existing thread: %w", err)
	}

	// Insert new thread
	now := time.Now()
	newThread := ChatThread{
		ID:              uuid.NewString(),
		BuyerID:         buyerID,
		SellerID:        sellerID,
		ListingID:       listingID,
		ListingTitle:    listingTitle,
		ListingImageURL: listingImage,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	const insertQ = `INSERT INTO chat_threads (id, buyer_id, seller_id, listing_id, listing_title, listing_image_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (buyer_id, seller_id, listing_id) DO NOTHING`
	_, err = r.pool.Exec(ctx, insertQ, newThread.ID, newThread.BuyerID, newThread.SellerID, newThread.ListingID, newThread.ListingTitle, newThread.ListingImageURL, newThread.CreatedAt, newThread.UpdatedAt)
	if err != nil {
		return ChatThread{}, fmt.Errorf("insert thread: %w", err)
	}

	// Re-fetch in case of conflict race
	row = r.pool.QueryRow(ctx, selectQ, buyerID, sellerID, listingID)
	return scanThread(row)
}

func (r *PostgresChatRepository) GetThreadByID(ctx context.Context, threadID string) (ChatThread, error) {
	const q = `SELECT ` + threadColumns + ` FROM chat_threads WHERE id = $1`
	t, err := scanThread(r.pool.QueryRow(ctx, q, threadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ChatThread{}, ErrThreadNotFound
	}
	return t, err
}

func (r *PostgresChatRepository) ListThreadsForUser(ctx context.Context, userID string, offset, limit int) ([]ChatThread, int64, error) {
	const countQ = `SELECT COUNT(*) FROM chat_threads WHERE buyer_id = $1 OR seller_id = $1`
	var total int64
	_ = r.pool.QueryRow(ctx, countQ, userID).Scan(&total)

	const listQ = `SELECT ` + threadColumns + ` FROM chat_threads WHERE buyer_id = $1 OR seller_id = $1 ORDER BY updated_at DESC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, listQ, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list threads: %w", err)
	}
	defer rows.Close()

	var threads []ChatThread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, 0, err
		}
		threads = append(threads, t)
	}
	return threads, total, nil
}

func (r *PostgresChatRepository) SaveMessage(ctx context.Context, msg ChatMessage) (ChatMessage, error) {
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.CreatedAt = time.Now()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ChatMessage{}, err
	}
	defer tx.Rollback(ctx) // nolint:errcheck

	const insertMsg = `INSERT INTO chat_messages (id, thread_id, sender_id, sender_name, content, created_at, message_type, listing_id, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	if _, err := tx.Exec(ctx, insertMsg, msg.ID, msg.ThreadID, msg.SenderID, msg.SenderName, msg.Content, msg.CreatedAt, msg.MessageType, msg.ListingID, msg.Payload); err != nil {
		return ChatMessage{}, fmt.Errorf("insert message: %w", err)
	}

	// Update thread last_message and increment other party's unread_count
	const updateThread = `UPDATE chat_threads SET
		last_message_text = $1,
		last_message_at = $2,
		updated_at = $2,
		unread_count_buyer = CASE WHEN buyer_id != $3 THEN unread_count_buyer + 1 ELSE unread_count_buyer END,
		unread_count_seller = CASE WHEN seller_id != $3 THEN unread_count_seller + 1 ELSE unread_count_seller END
		WHERE id = $4`
	if _, err := tx.Exec(ctx, updateThread, msg.Content, msg.CreatedAt, msg.SenderID, msg.ThreadID); err != nil {
		return ChatMessage{}, fmt.Errorf("update thread on message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return ChatMessage{}, err
	}
	return msg, nil
}

func (r *PostgresChatRepository) GetThreadMessages(ctx context.Context, threadID string, offset, limit int) ([]ChatMessage, int64, error) {
	const countQ = `SELECT COUNT(*) FROM chat_messages WHERE thread_id = $1`
	var total int64
	_ = r.pool.QueryRow(ctx, countQ, threadID).Scan(&total)

	const listQ = `SELECT id, thread_id, sender_id, sender_name, content, created_at, message_type, listing_id, payload
		FROM chat_messages WHERE thread_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, listQ, threadID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.SenderName, &m.Content, &m.CreatedAt, &m.MessageType, &m.ListingID, &m.Payload); err != nil {
			return nil, 0, err
		}
		messages = append(messages, m)
	}
	return messages, total, nil
}

// SearchMessages returns the caller's messages whose content matches query (ILIKE),
// scoped to threads where the caller is buyer or seller, newest-first, paginated.
func (r *PostgresChatRepository) SearchMessages(ctx context.Context, userID, query string, offset, limit int) ([]ChatMessage, int64, error) {
	const countQ = `SELECT COUNT(*) FROM chat_messages m JOIN chat_threads t ON m.thread_id = t.id
		WHERE (t.buyer_id = $1 OR t.seller_id = $1) AND m.content ILIKE '%' || $2 || '%'`
	var total int64
	_ = r.pool.QueryRow(ctx, countQ, userID, query).Scan(&total)

	const listQ = `SELECT m.id, m.thread_id, m.sender_id, m.sender_name, m.content, m.created_at, m.message_type, m.listing_id, m.payload
		FROM chat_messages m JOIN chat_threads t ON m.thread_id = t.id
		WHERE (t.buyer_id = $1 OR t.seller_id = $1) AND m.content ILIKE '%' || $2 || '%'
		ORDER BY m.created_at DESC LIMIT $3 OFFSET $4`
	rows, err := r.pool.Query(ctx, listQ, userID, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.SenderID, &m.SenderName, &m.Content, &m.CreatedAt, &m.MessageType, &m.ListingID, &m.Payload); err != nil {
			return nil, 0, err
		}
		messages = append(messages, m)
	}
	return messages, total, nil
}

// ListQuickReplies returns a seller's configured canned replies ordered by
// sort_order. If the seller has configured none, seeded defaults are returned.
func (r *PostgresChatRepository) ListQuickReplies(ctx context.Context, sellerID string) ([]string, error) {
	const q = `SELECT body FROM quick_replies WHERE seller_id = $1 ORDER BY sort_order ASC, created_at ASC`
	rows, err := r.pool.Query(ctx, q, sellerID)
	if err != nil {
		return nil, fmt.Errorf("query quick replies: %w", err)
	}
	defer rows.Close()

	var replies []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		replies = append(replies, body)
	}
	if len(replies) == 0 {
		return defaultQuickReplies(), nil
	}
	return replies, nil
}

func (r *PostgresChatRepository) MarkThreadRead(ctx context.Context, threadID, userID string) error {
	const q = `UPDATE chat_threads SET
		unread_count_buyer = CASE WHEN buyer_id = $1 THEN 0 ELSE unread_count_buyer END,
		unread_count_seller = CASE WHEN seller_id = $1 THEN 0 ELSE unread_count_seller END
		WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, userID, threadID)
	return err
}

// ── InMemory Implementation ───────────────────────────────────────────

type InMemoryChatRepository struct {
	mu           sync.RWMutex
	threads      map[string]ChatThread
	messages     map[string][]ChatMessage
	quickReplies map[string][]string // sellerID -> canned replies
}

func NewInMemoryChatRepository() *InMemoryChatRepository {
	return &InMemoryChatRepository{
		threads:      make(map[string]ChatThread),
		messages:     make(map[string][]ChatMessage),
		quickReplies: make(map[string][]string),
	}
}

// SetQuickReplies seeds a seller's canned replies (test/helper convenience).
func (r *InMemoryChatRepository) SetQuickReplies(sellerID string, replies []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quickReplies[sellerID] = replies
}

func (r *InMemoryChatRepository) ListQuickReplies(_ context.Context, sellerID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if replies, ok := r.quickReplies[sellerID]; ok && len(replies) > 0 {
		return replies, nil
	}
	return defaultQuickReplies(), nil
}

func (r *InMemoryChatRepository) GetOrCreateThread(
	_ context.Context,
	buyerID, sellerID, listingID, listingTitle, listingImage string,
) (ChatThread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, t := range r.threads {
		if t.BuyerID == buyerID && t.SellerID == sellerID && t.ListingID == listingID {
			return t, nil
		}
	}

	now := time.Now()
	t := ChatThread{
		ID:              uuid.NewString(),
		BuyerID:         buyerID,
		SellerID:        sellerID,
		ListingID:       listingID,
		ListingTitle:    listingTitle,
		ListingImageURL: listingImage,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	r.threads[t.ID] = t
	return t, nil
}

func (r *InMemoryChatRepository) GetThreadByID(_ context.Context, threadID string) (ChatThread, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.threads[threadID]
	if !ok {
		return ChatThread{}, ErrThreadNotFound
	}
	return t, nil
}

func (r *InMemoryChatRepository) ListThreadsForUser(_ context.Context, userID string, offset, limit int) ([]ChatThread, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var userThreads []ChatThread
	for _, t := range r.threads {
		if t.BuyerID == userID || t.SellerID == userID {
			userThreads = append(userThreads, t)
		}
	}

	sort.Slice(userThreads, func(i, j int) bool {
		return userThreads[i].UpdatedAt.After(userThreads[j].UpdatedAt)
	})

	total := int64(len(userThreads))
	if offset > len(userThreads) {
		return []ChatThread{}, total, nil
	}
	end := offset + limit
	if end > len(userThreads) {
		end = len(userThreads)
	}
	return userThreads[offset:end], total, nil
}

func (r *InMemoryChatRepository) SaveMessage(_ context.Context, msg ChatMessage) (ChatMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.CreatedAt = time.Now()
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

func (r *InMemoryChatRepository) GetThreadMessages(_ context.Context, threadID string, offset, limit int) ([]ChatMessage, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msgs := r.messages[threadID]
	total := int64(len(msgs))
	if offset > len(msgs) {
		return []ChatMessage{}, total, nil
	}
	end := offset + limit
	if end > len(msgs) {
		end = len(msgs)
	}
	return msgs[offset:end], total, nil
}

func (r *InMemoryChatRepository) SearchMessages(_ context.Context, userID, query string, offset, limit int) ([]ChatMessage, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	q := strings.ToLower(query)
	var matches []ChatMessage
	for threadID, msgs := range r.messages {
		t, ok := r.threads[threadID]
		if !ok || (t.BuyerID != userID && t.SellerID != userID) {
			continue // skip threads the caller does not participate in
		}
		for _, m := range msgs {
			if strings.Contains(strings.ToLower(m.Content), q) {
				matches = append(matches, m)
			}
		}
	}

	// Newest-first, mirroring the Postgres ORDER BY created_at DESC.
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].CreatedAt.After(matches[j].CreatedAt)
	})

	total := int64(len(matches))
	if offset > len(matches) {
		return []ChatMessage{}, total, nil
	}
	end := offset + limit
	if end > len(matches) {
		end = len(matches)
	}
	return matches[offset:end], total, nil
}

func (r *InMemoryChatRepository) MarkThreadRead(_ context.Context, threadID, userID string) error {
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
