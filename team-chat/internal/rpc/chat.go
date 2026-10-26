package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/chat"
	"github.com/buidangphuc/team-chat/internal/store"
)

const (
	threadsDefaultPageSize  = 20
	messagesDefaultPageSize = 50
	maxPageSize             = 100
)

type Handler struct {
	chatv1.UnimplementedChatServiceServer
	svc    *chat.Service
	logger *slog.Logger
	errs   grpcx.ErrorTable
}

func NewHandler(svc *chat.Service, logger *slog.Logger) (*Handler, error) {
	if svc == nil {
		return nil, errors.New("rpc: service is required")
	}
	if logger == nil {
		return nil, errors.New("rpc: logger is required")
	}
	errs, err := grpcx.NewErrorTable(
		grpcx.Rule{Target: chat.ErrSelfChat, Code: codes.InvalidArgument},
		grpcx.Rule{Target: chat.ErrEmptySender, Code: codes.InvalidArgument},
		grpcx.Rule{Target: chat.ErrEmptyContent, Code: codes.InvalidArgument},
		grpcx.Rule{Target: chat.ErrEmptyThreadID, Code: codes.InvalidArgument},
		grpcx.Rule{Target: chat.ErrUnauthorizedChat, Code: codes.PermissionDenied, Message: "unauthorized to access this thread"},
		grpcx.Rule{Target: store.ErrThreadNotFound, Code: codes.NotFound, Message: "thread not found"},
	)
	if err != nil {
		return nil, err
	}
	return &Handler{svc: svc, logger: logger, errs: errs}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	chatv1.RegisterChatServiceServer(srv, h)
}

func (h *Handler) fail(ctx context.Context, err error) error {
	return grpcx.Status(ctx, h.logger, err, h.errs)
}

func pageParams(p *commonv1.PageRequest, def int) (page, size int) {
	page, size = 1, def
	if n := int(p.GetPageSize()); n > 0 && n <= maxPageSize {
		size = n
	}
	if n, err := strconv.Atoi(p.GetCursor()); err == nil && n > 0 {
		page = n
	}
	return page, size
}

func pageResponse(page, size int, total int64) *commonv1.PageResponse {
	var next string
	if int64(page*size) < total {
		next = strconv.Itoa(page + 1)
	}
	return &commonv1.PageResponse{Total: total, NextCursor: next}
}

func (h *Handler) GetOrCreateThread(ctx context.Context, req *chatv1.GetOrCreateThreadRequest) (*chatv1.GetOrCreateThreadResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSellerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	if p.ID == req.GetSellerId() {
		return nil, status.Error(codes.InvalidArgument, "cannot create chat thread with oneself")
	}
	thread, err := h.svc.GetOrCreateThread(ctx, p.ID, req.GetSellerId(), req.GetListingId(), "Sản phẩm", "")
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.GetOrCreateThreadResponse{Thread: toWireThread(thread)}, nil
}

func (h *Handler) ListThreads(ctx context.Context, req *chatv1.ListThreadsRequest) (*chatv1.ListThreadsResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageParams(req.GetPage(), threadsDefaultPageSize)
	threads, total, err := h.svc.ListThreads(ctx, p.ID, page, size)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	out := make([]*chatv1.ChatThread, 0, len(threads))
	for _, t := range threads {
		out = append(out, toWireThread(t))
	}
	return &chatv1.ListThreadsResponse{Threads: out, Page: pageResponse(page, size, total)}, nil
}

func (h *Handler) GetThreadMessages(ctx context.Context, req *chatv1.GetThreadMessagesRequest) (*chatv1.GetThreadMessagesResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}
	page, size := pageParams(req.GetPage(), messagesDefaultPageSize)
	msgs, total, err := h.svc.GetThreadMessages(ctx, req.GetThreadId(), p.ID, page, size)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.GetThreadMessagesResponse{Messages: toWireMessages(msgs), Page: pageResponse(page, size, total)}, nil
}

func (h *Handler) SearchMessages(ctx context.Context, req *chatv1.SearchMessagesRequest) (*chatv1.SearchMessagesResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	page, size := pageParams(req.GetPage(), messagesDefaultPageSize)
	msgs, total, err := h.svc.SearchMessages(ctx, p.ID, req.GetQuery(), page, size)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.SearchMessagesResponse{Messages: toWireMessages(msgs), Page: pageResponse(page, size, total)}, nil
}

func (h *Handler) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
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
	if len(p.ID) > 6 {
		senderName = "User " + p.ID[:6]
	}
	msg, err := h.svc.SendRichMessage(ctx, req.GetThreadId(), p.ID, senderName, req.GetContent(), int32(req.GetMessageType()), req.GetListingId(), req.GetPayload())
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.SendMessageResponse{Message: toWireMessage(msg)}, nil
}

func (h *Handler) MarkThreadRead(ctx context.Context, req *chatv1.MarkThreadReadRequest) (*chatv1.MarkThreadReadResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetThreadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "thread_id is required")
	}
	if err := h.svc.MarkThreadRead(ctx, req.GetThreadId(), p.ID); err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.MarkThreadReadResponse{}, nil
}

func (h *Handler) ListQuickReplies(ctx context.Context, req *chatv1.ListQuickRepliesRequest) (*chatv1.ListQuickRepliesResponse, error) {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	sellerID := req.GetSellerId()
	if sellerID == "" {
		sellerID = p.ID
	}
	replies, err := h.svc.ListQuickReplies(ctx, sellerID)
	if err != nil {
		return nil, h.fail(ctx, err)
	}
	return &chatv1.ListQuickRepliesResponse{QuickReplies: replies}, nil
}

func toWireMessages(msgs []store.ChatMessage) []*chatv1.ChatMessage {
	out := make([]*chatv1.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toWireMessage(m))
	}
	return out
}

func toWireThread(t store.ChatThread) *chatv1.ChatThread {
	var last *timestamppb.Timestamp
	if t.LastMessageAt != nil {
		last = timestamppb.New(*t.LastMessageAt)
	}
	return &chatv1.ChatThread{
		Id:                t.ID,
		BuyerId:           t.BuyerID,
		SellerId:          t.SellerID,
		ListingId:         t.ListingID,
		ListingTitle:      t.ListingTitle,
		ListingImageUrl:   t.ListingImageURL,
		LastMessageText:   t.LastMessageText,
		LastMessageAt:     last,
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
