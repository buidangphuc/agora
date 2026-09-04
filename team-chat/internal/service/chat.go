package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/buidangphuc/team-chat/internal/repository"
)

var (
	ErrEmptySender      = errors.New("sender_id is required")
	ErrEmptyContent     = errors.New("message content cannot be empty")
	ErrEmptyThreadID    = errors.New("thread_id is required")
	ErrUnauthorizedChat = errors.New("user is not a participant in this chat thread")
	ErrSelfChat         = errors.New("cannot create chat thread with oneself")
)

type ChatService struct {
	repo   repository.ChatRepository
	logger *slog.Logger
}

func NewChatService(repo repository.ChatRepository, logger *slog.Logger) *ChatService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChatService{repo: repo, logger: logger}
}

func (s *ChatService) GetOrCreateThread(
	ctx context.Context,
	buyerID, sellerID, listingID, listingTitle, listingImage string,
) (repository.ChatThread, error) {
	if buyerID == "" || sellerID == "" {
		return repository.ChatThread{}, errors.New("buyer_id and seller_id are required")
	}
	if buyerID == sellerID {
		return repository.ChatThread{}, ErrSelfChat
	}
	return s.repo.GetOrCreateThread(ctx, buyerID, sellerID, listingID, listingTitle, listingImage)
}

func (s *ChatService) ListThreads(
	ctx context.Context,
	userID string,
	page, pageSize int,
) ([]repository.ChatThread, int64, error) {
	if userID == "" {
		return nil, 0, ErrEmptySender
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize
	return s.repo.ListThreadsForUser(ctx, userID, offset, pageSize)
}

func (s *ChatService) GetThreadMessages(
	ctx context.Context,
	threadID, userID string,
	page, pageSize int,
) ([]repository.ChatMessage, int64, error) {
	if threadID == "" {
		return nil, 0, ErrEmptyThreadID
	}
	thread, err := s.repo.GetThreadByID(ctx, threadID)
	if err != nil {
		return nil, 0, err
	}
	if thread.BuyerID != userID && thread.SellerID != userID {
		return nil, 0, ErrUnauthorizedChat
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize
	return s.repo.GetThreadMessages(ctx, threadID, offset, pageSize)
}

// SearchMessages returns the caller's messages whose content matches query,
// scoped to threads where the caller participates. A blank query returns an
// empty result (no full-table scan).
func (s *ChatService) SearchMessages(
	ctx context.Context,
	userID, query string,
	page, pageSize int,
) ([]repository.ChatMessage, int64, error) {
	if userID == "" {
		return nil, 0, ErrEmptySender
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return []repository.ChatMessage{}, 0, nil
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize
	return s.repo.SearchMessages(ctx, userID, query, offset, pageSize)
}

// SendMessage sends a plain TEXT message. Kept for back-compat; delegates to
// SendRichMessage with MessageType TEXT and no rich payload.
func (s *ChatService) SendMessage(
	ctx context.Context,
	threadID, senderID, senderName, content string,
) (repository.ChatMessage, error) {
	return s.SendRichMessage(ctx, threadID, senderID, senderName, content, 0, "", "")
}

// SendRichMessage persists a message that may carry a rich type (LISTING_CARD /
// QUICK_REPLY) plus an optional listing_id and JSON payload. Auth-scoped: only a
// thread participant may send.
func (s *ChatService) SendRichMessage(
	ctx context.Context,
	threadID, senderID, senderName, content string,
	messageType int32, listingID, payload string,
) (repository.ChatMessage, error) {
	if threadID == "" {
		return repository.ChatMessage{}, ErrEmptyThreadID
	}
	if senderID == "" {
		return repository.ChatMessage{}, ErrEmptySender
	}
	if content == "" {
		return repository.ChatMessage{}, ErrEmptyContent
	}

	thread, err := s.repo.GetThreadByID(ctx, threadID)
	if err != nil {
		return repository.ChatMessage{}, err
	}
	if thread.BuyerID != senderID && thread.SellerID != senderID {
		return repository.ChatMessage{}, ErrUnauthorizedChat
	}

	msg := repository.ChatMessage{
		ThreadID:    threadID,
		SenderID:    senderID,
		SenderName:  senderName,
		Content:     content,
		MessageType: messageType,
		ListingID:   listingID,
		Payload:     payload,
	}

	saved, err := s.repo.SaveMessage(ctx, msg)
	if err != nil {
		return repository.ChatMessage{}, fmt.Errorf("save message: %w", err)
	}
	return saved, nil
}

// ListQuickReplies returns a seller's canned quick-reply templates. Falls back
// to seeded defaults when the seller has configured none.
func (s *ChatService) ListQuickReplies(ctx context.Context, sellerID string) ([]string, error) {
	if sellerID == "" {
		return nil, ErrEmptySender
	}
	return s.repo.ListQuickReplies(ctx, sellerID)
}

func (s *ChatService) GetThread(ctx context.Context, threadID, userID string) (repository.ChatThread, error) {
	if threadID == "" {
		return repository.ChatThread{}, ErrEmptyThreadID
	}
	if userID == "" {
		return repository.ChatThread{}, ErrEmptySender
	}
	thread, err := s.repo.GetThreadByID(ctx, threadID)
	if err != nil {
		return repository.ChatThread{}, err
	}
	if thread.BuyerID != userID && thread.SellerID != userID {
		return repository.ChatThread{}, ErrUnauthorizedChat
	}
	return thread, nil
}

func (s *ChatService) MarkThreadRead(ctx context.Context, threadID, userID string) error {
	if threadID == "" {
		return ErrEmptyThreadID
	}
	if userID == "" {
		return ErrEmptySender
	}
	thread, err := s.repo.GetThreadByID(ctx, threadID)
	if err != nil {
		return err
	}
	if thread.BuyerID != userID && thread.SellerID != userID {
		return ErrUnauthorizedChat
	}
	return s.repo.MarkThreadRead(ctx, threadID, userID)
}
