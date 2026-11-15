package upstream

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdkupstream "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"

	verificationv1 "github.com/buidangphuc/team-payment/generated/platform/verification/v1"
)

const (
	verificationScope   = "verification.read"
	verificationTimeout = 2 * time.Second
)

type PayoutGate struct {
	client verificationv1.VerificationServiceClient
}

func DialVerificationService(addr string) (verificationv1.VerificationServiceClient, *grpc.ClientConn, error) {
	conn, err := sdkupstream.NewClient(addr, sdkupstream.AsService(servicePrincipalID, verificationScope))
	if err != nil {
		return nil, nil, fmt.Errorf("dial verification service %s: %w", addr, err)
	}
	return verificationv1.NewVerificationServiceClient(conn), conn, nil
}

func NewPayoutGate(client verificationv1.VerificationServiceClient) *PayoutGate {
	return &PayoutGate{client: client}
}

func (g *PayoutGate) Require(ctx context.Context, sellerID string) error {
	cctx, cancel := context.WithTimeout(ctx, verificationTimeout)
	defer cancel()
	resp, err := g.client.GetVerificationStatus(cctx, &verificationv1.GetVerificationStatusRequest{UserId: sellerID})
	if err != nil {
		return status.Error(codes.Unavailable, "verification unavailable")
	}
	if resp.GetStatus() != verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED {
		return status.Error(codes.FailedPrecondition, "seller verification required")
	}
	return nil
}
