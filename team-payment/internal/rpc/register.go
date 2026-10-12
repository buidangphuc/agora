package rpc

import (
	"google.golang.org/grpc"

	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
)

// Register attaches the PaymentService handler to srv.
func Register(srv grpc.ServiceRegistrar, h *PaymentHandler) {
	paymentv1.RegisterPaymentServiceServer(srv, h)
}
