package rpc

import (
	"google.golang.org/grpc"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
)

// Register attaches the ChatService handler to srv.
func Register(srv grpc.ServiceRegistrar, h *ChatHandler) {
	chatv1.RegisterChatServiceServer(srv, h)
}
