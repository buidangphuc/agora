package handler

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/interceptor"
	"github.com/buidangphuc/team-chat/internal/repository"
	"github.com/buidangphuc/team-chat/internal/service"
)

type ChatHandler struct {
	chatv1.UnimplementedChatServiceServer
	svc       *service.ChatService
	publisher events.ChatPublisher
	logger    *slog.Logger
}

func NewChatHandler(svc *service.ChatService, publisher events.ChatPublisher, logger *slog.Logger) *ChatHandler {
	if publisher == nil {
		publisher = events.NoopPublisher{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ChatHandler{
		svc:       svc,
		publisher: publisher,
		logger:    logger,
	}
}

func (h *ChatHandler) GetOrCreateThread(
	ctx context.Context,
	req *chatv1.GetOrCreateThreadRequest,
) (*chatv1.GetOrCreateThreadResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if principal.GetId() == req.GetSellerId() {
		return nil, status.Error(codes.InvalidArgument, "cannot create chat thread with oneself")
	}

	thread, err := h.svc.GetOrCreateThread(
		ctx,
		principal.GetId(),
		req.GetSellerId(),
		req.GetListingId(),
		"Sản phẩm",
		"",
	)
	if err != nil {
		if errors.Is(err, service.ErrSelfChat) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "get or create thread: %v", err)
	}

	return &chatv1.GetOrCreateThreadResponse{
		Thread: toWireThread(thread),
	}, nil
}

func (h *ChatHandler) ListThreads(
	ctx context.Context,
	req *chatv1.ListThreadsRequest,
) (*chatv1.ListThreadsResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
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

	threads, total, err := h.svc.ListThreads(ctx, principal.GetId(), page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list threads: %v", err)
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

func (h *ChatHandler) GetThreadMessages(
	ctx context.Context,
	req *chatv1.GetThreadMessagesRequest,
) (*chatv1.GetThreadMessagesResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
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

	msgs, total, err := h.svc.GetThreadMessages(ctx, req.GetThreadId(), principal.GetId(), page, pageSize)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to view this thread")
		}
		if errors.Is(err, repository.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, status.Errorf(codes.Internal, "get thread messages: %v", err)
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

func (h *ChatHandler) SearchMessages(
	ctx context.Context,
	req *chatv1.SearchMessagesRequest,
) (*chatv1.SearchMessagesResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
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

	msgs, total, err := h.svc.SearchMessages(ctx, principal.GetId(), req.GetQuery(), page, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "search messages: %v", err)
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

func (h *ChatHandler) SendMessage(
	ctx context.Context,
	req *chatv1.SendMessageRequest,
) (*chatv1.SendMessageResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
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
	if len(principal.GetId()) > 6 {
		senderName = "User " + principal.GetId()[:6]
	}

	msg, err := h.svc.SendRichMessage(
		ctx,
		req.GetThreadId(),
		principal.GetId(),
		senderName,
		req.GetContent(),
		int32(req.GetMessageType()),
		req.GetListingId(),
		req.GetPayload(),
	)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to send message to this thread")
		}
		if errors.Is(err, repository.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, status.Errorf(codes.Internal, "send message: %v", err)
	}

	wireMsg := toWireMessage(msg)

	// Emit event to Kafka for real-time SSE push (Edge)
	reqID, _ := interceptor.RequestIDFromContext(ctx)
	if err := h.publisher.PublishMessageSent(ctx, wireMsg, principal, reqID); err != nil {
		h.logger.WarnContext(ctx, "failed to emit chat event", slog.Any("err", err), slog.String("thread_id", req.GetThreadId()))
	}

	return &chatv1.SendMessageResponse{
		Message: wireMsg,
	}, nil
}

func (h *ChatHandler) MarkThreadRead(
	ctx context.Context,
	req *chatv1.MarkThreadReadRequest,
) (*chatv1.MarkThreadReadResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}

	if err := h.svc.MarkThreadRead(ctx, req.GetThreadId(), principal.GetId()); err != nil {
		if errors.Is(err, service.ErrUnauthorizedChat) {
			return nil, status.Error(codes.PermissionDenied, "unauthorized to access this thread")
		}
		if errors.Is(err, repository.ErrThreadNotFound) {
			return nil, status.Error(codes.NotFound, "thread not found")
		}
		return nil, status.Errorf(codes.Internal, "mark thread read: %v", err)
	}

	return &chatv1.MarkThreadReadResponse{}, nil
}

func (h *ChatHandler) ListQuickReplies(
	ctx context.Context,
	req *chatv1.ListQuickRepliesRequest,
) (*chatv1.ListQuickRepliesResponse, error) {
	principal, err := interceptor.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}

	// Buyers fetch a seller's canned replies; a seller viewing their own may
	// omit seller_id, in which case we scope to the caller.
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = principal.GetId()
	}

	replies, err := h.svc.ListQuickReplies(ctx, sellerID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list quick replies: %v", err)
	}

	return &chatv1.ListQuickRepliesResponse{
		QuickReplies: replies,
	}, nil
}

func toWireThread(t repository.ChatThread) *chatv1.ChatThread {
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

func toWireMessage(m repository.ChatMessage) *chatv1.ChatMessage {
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
