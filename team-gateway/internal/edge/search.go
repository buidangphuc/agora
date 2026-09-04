package edge

import (
	"context"

	"connectrpc.com/connect"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/search/v1/searchv1connect"
)

// SearchForwarder implements the Connect SearchServiceHandler by forwarding to
// the upstream team-search gRPC service.
type SearchForwarder struct {
	searchv1connect.UnimplementedSearchServiceHandler
	client searchv1.SearchServiceClient
	edge   *Edge
}

// NewSearchForwarder builds the forwarder over an upstream client.
func NewSearchForwarder(client searchv1.SearchServiceClient, edge *Edge) *SearchForwarder {
	return &SearchForwarder{client: client, edge: edge}
}

func (f *SearchForwarder) SearchListings(
	ctx context.Context,
	req *connect.Request[searchv1.SearchListingsRequest],
) (*connect.Response[searchv1.SearchListingsResponse], error) {
	var out *searchv1.SearchListingsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SearchListings(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SearchForwarder) Suggest(
	ctx context.Context,
	req *connect.Request[searchv1.SuggestRequest],
) (*connect.Response[searchv1.SuggestResponse], error) {
	var out *searchv1.SuggestResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.Suggest(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Saved Searches ──

func (f *SearchForwarder) SaveSearch(
	ctx context.Context,
	req *connect.Request[searchv1.SaveSearchRequest],
) (*connect.Response[searchv1.SaveSearchResponse], error) {
	var out *searchv1.SaveSearchResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SaveSearch(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SearchForwarder) ListSavedSearches(
	ctx context.Context,
	req *connect.Request[searchv1.ListSavedSearchesRequest],
) (*connect.Response[searchv1.ListSavedSearchesResponse], error) {
	var out *searchv1.ListSavedSearchesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListSavedSearches(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SearchForwarder) DeleteSavedSearch(
	ctx context.Context,
	req *connect.Request[searchv1.DeleteSavedSearchRequest],
) (*connect.Response[searchv1.DeleteSavedSearchResponse], error) {
	var out *searchv1.DeleteSavedSearchResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.DeleteSavedSearch(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *SearchForwarder) RunSavedSearch(
	ctx context.Context,
	req *connect.Request[searchv1.RunSavedSearchRequest],
) (*connect.Response[searchv1.RunSavedSearchResponse], error) {
	var out *searchv1.RunSavedSearchResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RunSavedSearch(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
