// Package rpc is the gRPC transport adapter: it implements chat.v1.ChatService,
// authorizes each call, maps between wire (protobuf) and domain types, and turns
// errors into gRPC statuses. Business rules live in internal/chat.
package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/store"
)

// ChatHandler implements chat.v1.ChatService: it authorizes each call, calls the chat service and maps errors to gRPC statuses.
type ChatHandler struct {
	chatv1.UnimplementedChatServiceServer
	svc    *chat.Service
	logger *slog.Logger
}

// NewChatHandler builds the gRPC rpc. chat.events are not emitted here: the
// repository writes them to the outbox in the message transaction and the relayer
// publishes them.
func NewChatHandler(svc *chat.Service, logger *slog.Logger) *ChatHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChatHandler{
		svc:    svc,
		logger: logger,
	}
}

// GetOrCreateThread opens (or returns) the thread between the caller and a seller.
func (h *ChatHandler) GetOrCreateThread(
	ctx context.Context,
	req *chatv1.GetOrCreateThreadRequest,
) (*chatv1.GetOrCreateThreadResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if principal.ID == req.GetSellerId() {
		return nil, status.Error(codes.InvalidArgument, "cannot create chat thread with oneself")
	}

	thread, err := h.svc.GetOrCreateThread(
		ctx,
		principal.ID,
		req.GetSellerId(),
		req.GetListingId(),
		"Sản phẩm",
		"",
	)
	if err != nil {
		if errors.Is(err, chat.ErrSelfChat) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, internalErr(ctx, h.logger, "get or create thread", err)
	}

	return &chatv1.GetOrCreateThreadResponse{
		Thread: toWireThread(thread),
	}, nil
}

// ListThreads returns the caller's threads, one page at a time.
func (h *ChatHandler) ListThreads(
	ctx context.Context,
	req *chatv1.ListThreadsRequest,
) (*chatv1.ListThreadsResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := 1, 20
	if p := req.GetPage(); p != nil {
		if p.GetPageSize() > 0 {
			pageSize = int(p.GetPageSize())
		}
		if p.GetCursor() != "" {
			if parsedPage, err := strconv.Atoi(p.GetCursor()); err == nil && parsedPage > 0 {
				page = parsedPage
			}
		}
	}

	threads, total, err := h.svc.ListThreads(ctx, principal.ID, page, pageSize)
	if err != nil {
		return nil, internalErr(ctx, h.logger, "list threads", err)
	}

	wireThreads := make([]*chatv1.ChatThread, 0, len(threads))
	for _, t := range threads {
		wireThreads = append(wireThreads, toWireThread(t))
	}

	var nextCursor string
	if int64(page*pageSize) < total {
		nextCursor = strconv.Itoa(page + 1)
	}

	return &chatv1.ListThreadsResponse{
		Threads: wireThreads,
		Page: &commonv1.PageResponse{
			Total:      total,
			NextCursor: nextCursor,
		},
	}, nil
}

// GetThreadMessages returns one page of a thread's messages to a participant.
func (h *ChatHandler) GetThreadMessages(
	ctx context.Context,
	req *chatv1.GetThreadMessagesRequest,
) (*chatv1.GetThreadMessagesResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}

	page, pageSize := 1, 50
	if p := req.GetPage(); p != nil {
		if p.GetPageSize() > 0 {
			pageSize = int(p.GetPageSize())
		}
		if p.GetCursor() != "" {
			if parsedPage, err := strconv.Atoi(p.GetCursor()); err == nil && parsedPage > 0 {
				page = parsedPage
			}
		}
	}

	msgs, total, err := h.svc.GetThreadMessages(ctx, req.GetThreadId(), principal.ID, page, pageSize)
	if err != nil {
		if errors.Is(err, chat.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to view this thread")
		}
		if errors.Is(err, store.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, internalErr(ctx, h.logger, "get thread messages", err)
	}

	wireMsgs := make([]*chatv1.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		wireMsgs = append(wireMsgs, toWireMessage(m))
	}

	var nextCursor string
	if int64(page*pageSize) < total {
		nextCursor = strconv.Itoa(page + 1)
	}

	return &chatv1.GetThreadMessagesResponse{
		Messages: wireMsgs,
		Page: &commonv1.PageResponse{
			Total:      total,
			NextCursor: nextCursor,
		},
	}, nil
}

// SearchMessages searches the caller's messages.
func (h *ChatHandler) SearchMessages(
	ctx context.Context,
	req *chatv1.SearchMessagesRequest,
) (*chatv1.SearchMessagesResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	page, pageSize := 1, 50
	if p := req.GetPage(); p != nil {
		if p.GetPageSize() > 0 {
			pageSize = int(p.GetPageSize())
		}
		if p.GetCursor() != "" {
			if parsedPage, err := strconv.Atoi(p.GetCursor()); err == nil && parsedPage > 0 {
				page = parsedPage
			}
		}
	}

	msgs, total, err := h.svc.SearchMessages(ctx, principal.ID, req.GetQuery(), page, pageSize)
	if err != nil {
		return nil, internalErr(ctx, h.logger, "search messages", err)
	}

	wireMsgs := make([]*chatv1.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		wireMsgs = append(wireMsgs, toWireMessage(m))
	}

	var nextCursor string
	if int64(page*pageSize) < total {
		nextCursor = strconv.Itoa(page + 1)
	}

	return &chatv1.SearchMessagesResponse{
		Messages: wireMsgs,
		Page: &commonv1.PageResponse{
			Total:      total,
			NextCursor: nextCursor,
		},
	}, nil
}

// SendMessage stores a message; its chat.events outbox row is written in the same transaction.
func (h *ChatHandler) SendMessage(
	ctx context.Context,
	req *chatv1.SendMessageRequest,
) (*chatv1.SendMessageResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}
	if req.GetContent() == "" {
		return nil, status.Error(codes.InvalidArgument, "content is required")
	}

	senderName := "Người dùng"
	if len(principal.ID) > 6 {
		senderName = "User " + principal.ID[:6]
	}

	msg, err := h.svc.SendRichMessage(
		ctx,
		req.GetThreadId(),
		principal.ID,
		senderName,
		req.GetContent(),
		int32(req.GetMessageType()),
		req.GetListingId(),
		req.GetPayload(),
	)
	if err != nil {
		if errors.Is(err, chat.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to send message to this thread")
		}
		if errors.Is(err, store.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, internalErr(ctx, h.logger, "send message", err)
	}

	wireMsg := toWireMessage(msg)

	return &chatv1.SendMessageResponse{
		Message: wireMsg,
	}, nil
}

// MarkThreadRead clears the caller's unread counter on a thread.
func (h *ChatHandler) MarkThreadRead(
	ctx context.Context,
	req *chatv1.MarkThreadReadRequest,
) (*chatv1.MarkThreadReadResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}

	if err := h.svc.MarkThreadRead(ctx, req.GetThreadId(), principal.ID); err != nil {
		if errors.Is(err, chat.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to access this thread")
		}
		if errors.Is(err, store.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, internalErr(ctx, h.logger, "mark thread read", err)
	}

	return &chatv1.MarkThreadReadResponse{}, nil
}

// ListQuickReplies returns a seller's canned replies.
func (h *ChatHandler) ListQuickReplies(
	ctx context.Context,
	req *chatv1.ListQuickRepliesRequest,
) (*chatv1.ListQuickRepliesResponse, error) {
	principal, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	// Buyers fetch a seller's canned replies; a seller viewing their own may
	// omit seller_id, in which case we scope to the caller.
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.ID
	}

	replies, err := h.svc.ListQuickReplies(ctx, sellerID)
	if err != nil {
		return nil, internalErr(ctx, h.logger, "list quick replies", err)
	}

	return &chatv1.ListQuickRepliesResponse{
		QuickReplies: replies,
	}, nil
}

func toWireThread(t store.ChatThread) *chatv1.ChatThread {
	var lastMsgAt *timestamppb.Timestamp
	if t.LastMessageAt != nil {
		lastMsgAt = timestamppb.New(*t.LastMessageAt)
	}
	return &chatv1.ChatThread{
		Id:                t.ID,
		BuyerId:           t.BuyerID,
		SellerId:          t.SellerID,
		ListingId:         t.ListingID,
		ListingTitle:      t.ListingTitle,
		ListingImageUrl:   t.ListingImageURL,
		LastMessageText:   t.LastMessageText,
		LastMessageAt:     lastMsgAt,
		UnreadCountBuyer:  t.UnreadCountBuyer,
		UnreadCountSeller: t.UnreadCountSeller,
		CreatedAt:         timestamppb.New(t.CreatedAt),
		UpdatedAt:         timestamppb.New(t.UpdatedAt),
	}
}

func toWireMessage(m store.ChatMessage) *chatv1.ChatMessage {
	return &chatv1.ChatMessage{
		Id:          m.ID,
		ThreadId:    m.ThreadID,
		SenderId:    m.SenderID,
		SenderName:  m.SenderName,
		Content:     m.Content,
		CreatedAt:   timestamppb.New(m.CreatedAt),
		MessageType: chatv1.MessageType(m.MessageType),
		ListingId:   m.ListingID,
		Payload:     m.Payload,
	}
}
