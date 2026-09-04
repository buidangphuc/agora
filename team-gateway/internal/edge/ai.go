package edge

import (
	"context"

	"connectrpc.com/connect"

	aiv1 "github.com/buidangphuc/team-gateway/generated/platform/ai/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/ai/v1/aiv1connect"
)

// AIForwarder implements the Connect AIServiceHandler by forwarding to the
// upstream team-ai gRPC service. All three RPCs are read-shaped (no state
// change, no Kafka emit), so they go through callRead.
type AIForwarder struct {
	aiv1connect.UnimplementedAIServiceHandler
	client aiv1.AIServiceClient
	edge   *Edge
}

// NewAIForwarder builds the forwarder over an upstream client.
func NewAIForwarder(client aiv1.AIServiceClient, edge *Edge) *AIForwarder {
	return &AIForwarder{client: client, edge: edge}
}

func (f *AIForwarder) ShoppingAssistant(
	ctx context.Context,
	req *connect.Request[aiv1.ShoppingAssistantRequest],
) (*connect.Response[aiv1.ShoppingAssistantResponse], error) {
	var out *aiv1.ShoppingAssistantResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ShoppingAssistant(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AIForwarder) MagicListing(
	ctx context.Context,
	req *connect.Request[aiv1.MagicListingRequest],
) (*connect.Response[aiv1.MagicListingResponse], error) {
	var out *aiv1.MagicListingResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.MagicListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *AIForwarder) ChatCopilot(
	ctx context.Context,
	req *connect.Request[aiv1.ChatCopilotRequest],
) (*connect.Response[aiv1.ChatCopilotResponse], error) {
	var out *aiv1.ChatCopilotResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ChatCopilot(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// SummarizeReviews synthesizes a review summary — a stateless generation
// (no state change, no Kafka emit), so it goes through callRead.
func (f *AIForwarder) SummarizeReviews(
	ctx context.Context,
	req *connect.Request[aiv1.SummarizeReviewsRequest],
) (*connect.Response[aiv1.SummarizeReviewsResponse], error) {
	var out *aiv1.SummarizeReviewsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SummarizeReviews(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
