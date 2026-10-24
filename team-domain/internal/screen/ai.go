package screen

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/upstream"
	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
)

const (
	servicePrincipalID = "service-team-domain"
	aiScope            = "ai.classify"
)

type AIClient struct {
	conn *grpc.ClientConn
	svc  aiv1.AIServiceClient
}

// ScreenInput is the listing text sent for screening.
type ScreenInput struct {
	ListingID, Title, Description, CategoryID string
}

func NewAIClient(addr string, callTimeout time.Duration) (*AIClient, error) {
	opts := []upstream.Option{upstream.AsService(servicePrincipalID, aiScope)}
	if callTimeout > 0 {
		opts = append(opts, upstream.WithTimeout(callTimeout))
	}
	conn, err := upstream.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("dial team-ai at %s: %w", addr, err)
	}
	return &AIClient{conn: conn, svc: aiv1.NewAIServiceClient(conn)}, nil
}

func (c *AIClient) Close() error {
	return c.conn.Close()
}

// ScreenListing returns team-ai's verdict; any failure is an error (caller fails safe).
func (c *AIClient) ScreenListing(ctx context.Context, in ScreenInput) (*aiv1.ScreenListingResponse, error) {
	res, err := c.svc.ScreenListing(ctx, &aiv1.ScreenListingRequest{
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
