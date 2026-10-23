package rpc

import (
	"log/slog"

	"google.golang.org/grpc"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	"github.com/buidangphuc/team-chat/internal/chat"
)

type Handler struct {
	chatv1.UnimplementedChatServiceServer
}

func NewHandler(*chat.Service, *slog.Logger) (*Handler, error) { return &Handler{}, nil }

func Register(srv *grpc.Server, h *Handler) { chatv1.RegisterChatServiceServer(srv, h) }
