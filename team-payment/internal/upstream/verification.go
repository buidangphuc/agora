package upstream

import (
	"context"
	"errors"

	"google.golang.org/grpc"

	verificationv1 "github.com/buidangphuc/team-payment/generated/platform/verification/v1"
)

type PayoutGate struct{}

func DialVerificationService(addr string) (verificationv1.VerificationServiceClient, *grpc.ClientConn, error) {
	return nil, nil, errors.New("not implemented")
}

func NewPayoutGate(verificationv1.VerificationServiceClient) *PayoutGate { return &PayoutGate{} }

func (*PayoutGate) Require(context.Context, string) error { return errors.New("not implemented") }
