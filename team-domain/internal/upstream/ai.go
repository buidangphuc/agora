package upstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
)

const aiServicePrincipalScope = "ai.classify"

func aiMetadata() metadata.MD {
	return metadata.Pairs(
		"x-principal-id", servicePrincipalID,
		"x-principal-type", servicePrincipalType,
		"x-principal-scopes", aiServicePrincipalScope,
	)
}

// aiPrincipalInterceptor replaces any outgoing principal with the service one.
func aiPrincipalInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	return invoker(metadata.NewOutgoingContext(ctx, aiMetadata()), method, req, reply, cc, opts...)
}

// AIClient calls team-ai's AIService.ScreenListing (single attempt, bounded).
type AIClient struct {
	conn    *grpc.ClientConn
	svc     aiv1.AIServiceClient
	timeout time.Duration
}

// ScreenInput is the listing text sent for screening.
type ScreenInput struct {
	ListingID, Title, Description, CategoryID string
}

// NewAIClient dials addr (UPSTREAM_AI_ADDR). Empty addr returns (nil, nil).
func NewAIClient(addr string, callTimeout time.Duration) (*AIClient, error) {
	if addr == "" {
		return nil, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(aiPrincipalInterceptor))
	if err != nil {
		return nil, fmt.Errorf("dial team-ai at %s: %w", addr, err)
	}
	return &AIClient{conn: conn, svc: aiv1.NewAIServiceClient(conn), timeout: callTimeout}, nil
}

// Close releases the underlying connection. Nil-safe.
func (c *AIClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// ScreenListing returns team-ai's verdict; any failure is an error (caller fails safe).
func (c *AIClient) ScreenListing(ctx context.Context, in ScreenInput) (*aiv1.ScreenListingResponse, error) {
	if c == nil || c.svc == nil {
		return nil, errors.New("ai client not configured")
	}
	callCtx, cancel := boundedCtx(ctx, c.timeout)
	defer cancel()
	res, err := c.svc.ScreenListing(callCtx, &aiv1.ScreenListingRequest{
		ListingId:   in.ListingID,
		Title:       in.Title,
		Description: in.Description,
		CategoryId:  in.CategoryID,
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
