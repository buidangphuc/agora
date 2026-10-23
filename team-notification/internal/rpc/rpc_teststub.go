package rpc

import (
	"log/slog"

	"google.golang.org/grpc"

	notificationv1 "github.com/buidangphuc/team-notification/generated/platform/notification/v1"
	"github.com/buidangphuc/team-notification/internal/alerts"
	"github.com/buidangphuc/team-notification/internal/prefs"
)

type Handler struct {
	notificationv1.UnimplementedNotificationServiceServer
}

func NewHandler(_ any, _ *alerts.Service, _ *prefs.Service, _ *slog.Logger) (*Handler, error) {
	return &Handler{}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	notificationv1.RegisterNotificationServiceServer(srv, h)
}
