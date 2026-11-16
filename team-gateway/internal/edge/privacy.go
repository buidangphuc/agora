package edge

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	identityv1 "github.com/buidangphuc/team-gateway/generated/platform/identity/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/identity/v1/identityv1connect"
)

type PrivacyForwarder struct {
	identityv1connect.UnimplementedPrivacyServiceHandler
	client identityv1.PrivacyServiceClient
	edge   *Edge
}

func NewPrivacyForwarder(client identityv1.PrivacyServiceClient, edge *Edge) *PrivacyForwarder {
	return &PrivacyForwarder{client: client, edge: edge}
}

func requireSignedIn(ctx context.Context) error {
	if p, ok := principalFrom(ctx); !ok || p.ptype == "anonymous" || p.id == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return nil
}

func (f *PrivacyForwarder) RecordConsent(
	ctx context.Context,
	req *connect.Request[identityv1.RecordConsentRequest],
) (*connect.Response[identityv1.RecordConsentResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.RecordConsentResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RecordConsent(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *PrivacyForwarder) ListMyConsents(
	ctx context.Context,
	req *connect.Request[identityv1.ListMyConsentsRequest],
) (*connect.Response[identityv1.ListMyConsentsResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.ListMyConsentsResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListMyConsents(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *PrivacyForwarder) RequestAccountDeletion(
	ctx context.Context,
	req *connect.Request[identityv1.RequestAccountDeletionRequest],
) (*connect.Response[identityv1.RequestAccountDeletionResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.RequestAccountDeletionResponse
	err := f.edge.callWrite(f.edge.outgoingWithSID(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RequestAccountDeletion(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *PrivacyForwarder) CancelAccountDeletion(
	ctx context.Context,
	req *connect.Request[identityv1.CancelAccountDeletionRequest],
) (*connect.Response[identityv1.CancelAccountDeletionResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.CancelAccountDeletionResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CancelAccountDeletion(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *PrivacyForwarder) GetAccountDeletionStatus(
	ctx context.Context,
	req *connect.Request[identityv1.GetAccountDeletionStatusRequest],
) (*connect.Response[identityv1.GetAccountDeletionStatusResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.GetAccountDeletionStatusResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetAccountDeletionStatus(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *PrivacyForwarder) ExportMyData(
	ctx context.Context,
	req *connect.Request[identityv1.ExportMyDataRequest],
) (*connect.Response[identityv1.ExportMyDataResponse], error) {
	if err := requireSignedIn(ctx); err != nil {
		return nil, err
	}
	var out *identityv1.ExportMyDataResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ExportMyData(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	res := connect.NewResponse(out)
	res.Header().Set("Cache-Control", "no-store")
	return res, nil
}
