package edge

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	verificationv1 "github.com/buidangphuc/team-gateway/generated/platform/verification/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/verification/v1/verificationv1connect"
)

// VerificationForwarder forwards KYC submit/status/review calls to team-verification.
type VerificationForwarder struct {
	verificationv1connect.UnimplementedVerificationServiceHandler
	client verificationv1.VerificationServiceClient
	edge   *Edge
}

func NewVerificationForwarder(client verificationv1.VerificationServiceClient, edge *Edge) *VerificationForwarder {
	return &VerificationForwarder{client: client, edge: edge}
}

func (f *VerificationForwarder) SubmitKyc(
	ctx context.Context,
	req *connect.Request[verificationv1.SubmitKycRequest],
) (*connect.Response[verificationv1.SubmitKycResponse], error) {
	var out *verificationv1.SubmitKycResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.SubmitKyc(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *VerificationForwarder) GetVerificationStatus(
	ctx context.Context,
	req *connect.Request[verificationv1.GetVerificationStatusRequest],
) (*connect.Response[verificationv1.GetVerificationStatusResponse], error) {
	var out *verificationv1.GetVerificationStatusResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetVerificationStatus(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

func (f *VerificationForwarder) ReviewKyc(
	ctx context.Context,
	req *connect.Request[verificationv1.ReviewKycRequest],
) (*connect.Response[verificationv1.ReviewKycResponse], error) {
	// Coarse admin gate at the edge (defence in depth, like the cockpit): only an
	// admin may review KYC. team-verification still enforces the full rule
	// (admin scope, not one's own submission, PENDING only).
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	var out *verificationv1.ReviewKycResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ReviewKyc(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// requireAdmin rejects a caller that is anonymous (Unauthenticated) or lacks the
// admin scope (PermissionDenied), using the principal the auth interceptor
// resolved for this request. It is a scope check only, no business rule.
func requireAdmin(ctx context.Context) error {
	p, ok := principalFrom(ctx)
	if !ok || p.ptype == "anonymous" || p.id == "" {
		return connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !hasScope(p.scopes, adminScope) {
		return connect.NewError(connect.CodePermissionDenied, errors.New("insufficient_scope: admin required"))
	}
	return nil
}
