package edge

import (
	"context"

	"connectrpc.com/connect"

	recommendationv1 "github.com/buidangphuc/team-gateway/generated/platform/recommendation/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/recommendation/v1/recommendationv1connect"
)

// RecommendationForwarder implements the Connect RecommendationServiceHandler by
// forwarding to the upstream team-ai gRPC service. Recommend is read-shaped (no
// state change, no Kafka emit), so it goes through callRead. The gateway holds
// no retrieval/ranking/filtering logic — it verifies auth once and forwards the
// x-principal-* metadata downstream (Rules 1–2).
type RecommendationForwarder struct {
	recommendationv1connect.UnimplementedRecommendationServiceHandler
	client recommendationv1.RecommendationServiceClient
	edge   *Edge
}

// NewRecommendationForwarder builds the forwarder over an upstream client.
func NewRecommendationForwarder(client recommendationv1.RecommendationServiceClient, edge *Edge) *RecommendationForwarder {
	return &RecommendationForwarder{client: client, edge: edge}
}

func (f *RecommendationForwarder) Recommend(
	ctx context.Context,
	req *connect.Request[recommendationv1.RecommendRequest],
) (*connect.Response[recommendationv1.RecommendResponse], error) {
	var out *recommendationv1.RecommendResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Recommend(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
