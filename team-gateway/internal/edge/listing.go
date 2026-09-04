package edge

import (
	"context"

	"connectrpc.com/connect"

	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/listing/v1/listingv1connect"
)

// ListingForwarder implements the Connect ListingServiceHandler by forwarding to
// the upstream team-domain gRPC service.
type ListingForwarder struct {
	listingv1connect.UnimplementedListingServiceHandler
	client listingv1.ListingServiceClient
	edge   *Edge
}

// NewListingForwarder builds the forwarder over an upstream client.
func NewListingForwarder(client listingv1.ListingServiceClient, edge *Edge) *ListingForwarder {
	return &ListingForwarder{client: client, edge: edge}
}

func (f *ListingForwarder) GetListing(
	ctx context.Context,
	req *connect.Request[listingv1.GetListingRequest],
) (*connect.Response[listingv1.GetListingResponse], error) {
	var out *listingv1.GetListingResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ListListings(
	ctx context.Context,
	req *connect.Request[listingv1.ListListingsRequest],
) (*connect.Response[listingv1.ListListingsResponse], error) {
	var out *listingv1.ListListingsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListListings(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ListMyListings(
	ctx context.Context,
	req *connect.Request[listingv1.ListMyListingsRequest],
) (*connect.Response[listingv1.ListMyListingsResponse], error) {
	var out *listingv1.ListMyListingsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListMyListings(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) CreateListing(
	ctx context.Context,
	req *connect.Request[listingv1.CreateListingRequest],
) (*connect.Response[listingv1.CreateListingResponse], error) {
	var out *listingv1.CreateListingResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) UpdateListing(
	ctx context.Context,
	req *connect.Request[listingv1.UpdateListingRequest],
) (*connect.Response[listingv1.UpdateListingResponse], error) {
	var out *listingv1.UpdateListingResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpdateListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) DeleteListing(
	ctx context.Context,
	req *connect.Request[listingv1.DeleteListingRequest],
) (*connect.Response[listingv1.DeleteListingResponse], error) {
	var out *listingv1.DeleteListingResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.DeleteListing(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) GetImageUploadUrl(
	ctx context.Context,
	req *connect.Request[listingv1.GetImageUploadUrlRequest],
) (*connect.Response[listingv1.GetImageUploadUrlResponse], error) {
	var out *listingv1.GetImageUploadUrlResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetImageUploadUrl(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ListCategories(
	ctx context.Context,
	req *connect.Request[listingv1.ListCategoriesRequest],
) (*connect.Response[listingv1.ListCategoriesResponse], error) {
	var out *listingv1.ListCategoriesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListCategories(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) GetCategory(
	ctx context.Context,
	req *connect.Request[listingv1.GetCategoryRequest],
) (*connect.Response[listingv1.GetCategoryResponse], error) {
	var out *listingv1.GetCategoryResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetCategory(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ReserveStock(
	ctx context.Context,
	req *connect.Request[listingv1.ReserveStockRequest],
) (*connect.Response[listingv1.ReserveStockResponse], error) {
	var out *listingv1.ReserveStockResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ReserveStock(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ReleaseStock(
	ctx context.Context,
	req *connect.Request[listingv1.ReleaseStockRequest],
) (*connect.Response[listingv1.ReleaseStockResponse], error) {
	var out *listingv1.ReleaseStockResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ReleaseStock(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Storefront ──

func (f *ListingForwarder) UpsertStorefront(
	ctx context.Context,
	req *connect.Request[listingv1.UpsertStorefrontRequest],
) (*connect.Response[listingv1.UpsertStorefrontResponse], error) {
	var out *listingv1.UpsertStorefrontResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.UpsertStorefront(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) GetStorefront(
	ctx context.Context,
	req *connect.Request[listingv1.GetStorefrontRequest],
) (*connect.Response[listingv1.GetStorefrontResponse], error) {
	var out *listingv1.GetStorefrontResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetStorefront(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Bundles ──

func (f *ListingForwarder) CreateBundle(
	ctx context.Context,
	req *connect.Request[listingv1.CreateBundleRequest],
) (*connect.Response[listingv1.CreateBundleResponse], error) {
	var out *listingv1.CreateBundleResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreateBundle(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) GetBundle(
	ctx context.Context,
	req *connect.Request[listingv1.GetBundleRequest],
) (*connect.Response[listingv1.GetBundleResponse], error) {
	var out *listingv1.GetBundleResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetBundle(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *ListingForwarder) ListBundlesBySeller(
	ctx context.Context,
	req *connect.Request[listingv1.ListBundlesBySellerRequest],
) (*connect.Response[listingv1.ListBundlesBySellerResponse], error) {
	var out *listingv1.ListBundlesBySellerResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListBundlesBySeller(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
