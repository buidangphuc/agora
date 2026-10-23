package rpc

import (
	"log/slog"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/payment"
)

type Handler struct {
	paymentv1.UnimplementedPaymentServiceServer
}

func NewHandler(_ *payment.Service, _ *auditx.Gate, _ *slog.Logger) (*Handler, error) {
	return &Handler{}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	paymentv1.RegisterPaymentServiceServer(srv, h)
}
