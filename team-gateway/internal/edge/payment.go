package edge

import (
	"context"

	"connectrpc.com/connect"

	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/payment/v1/paymentv1connect"
)

// PaymentForwarder serves its Connect handler RPCs by forwarding them to the upstream gRPC client.
type PaymentForwarder struct {
	paymentv1connect.UnimplementedPaymentServiceHandler
	client paymentv1.PaymentServiceClient
	edge   *Edge
}

// NewPaymentForwarder returns the forwarder that serves its RPCs from the given upstream client.
func NewPaymentForwarder(client paymentv1.PaymentServiceClient, edge *Edge) *PaymentForwarder {
	return &PaymentForwarder{client: client, edge: edge}
}

// CreatePayment forwards the CreatePayment RPC to the upstream service.
func (f *PaymentForwarder) CreatePayment(
	ctx context.Context,
	req *connect.Request[paymentv1.CreatePaymentRequest],
) (*connect.Response[paymentv1.CreatePaymentResponse], error) {
	var out *paymentv1.CreatePaymentResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.CreatePayment(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// GetPayment forwards the GetPayment RPC to the upstream service.
func (f *PaymentForwarder) GetPayment(
	ctx context.Context,
	req *connect.Request[paymentv1.GetPaymentRequest],
) (*connect.Response[paymentv1.GetPaymentResponse], error) {
	var out *paymentv1.GetPaymentResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetPayment(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ProcessMockPayment forwards the ProcessMockPayment RPC to the upstream service.
func (f *PaymentForwarder) ProcessMockPayment(
	ctx context.Context,
	req *connect.Request[paymentv1.ProcessMockPaymentRequest],
) (*connect.Response[paymentv1.ProcessMockPaymentResponse], error) {
	var out *paymentv1.ProcessMockPaymentResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ProcessMockPayment(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// RefundPayment forwards the RefundPayment RPC to the upstream service.
func (f *PaymentForwarder) RefundPayment(
	ctx context.Context,
	req *connect.Request[paymentv1.RefundPaymentRequest],
) (*connect.Response[paymentv1.RefundPaymentResponse], error) {
	var out *paymentv1.RefundPaymentResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RefundPayment(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Seller Wallet & Bank Payouts ──

// GetSellerWallet forwards the GetSellerWallet RPC to the upstream service.
func (f *PaymentForwarder) GetSellerWallet(
	ctx context.Context,
	req *connect.Request[paymentv1.GetSellerWalletRequest],
) (*connect.Response[paymentv1.GetSellerWalletResponse], error) {
	var out *paymentv1.GetSellerWalletResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetSellerWallet(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// RequestPayout forwards the RequestPayout RPC to the upstream service.
func (f *PaymentForwarder) RequestPayout(
	ctx context.Context,
	req *connect.Request[paymentv1.RequestPayoutRequest],
) (*connect.Response[paymentv1.RequestPayoutResponse], error) {
	var out *paymentv1.RequestPayoutResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RequestPayout(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListPayoutHistory forwards the ListPayoutHistory RPC to the upstream service.
func (f *PaymentForwarder) ListPayoutHistory(
	ctx context.Context,
	req *connect.Request[paymentv1.ListPayoutHistoryRequest],
) (*connect.Response[paymentv1.ListPayoutHistoryResponse], error) {
	var out *paymentv1.ListPayoutHistoryResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListPayoutHistory(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ── Wallet & Ledger ──

// GetWalletBalance forwards the GetWalletBalance RPC to the upstream service.
func (f *PaymentForwarder) GetWalletBalance(
	ctx context.Context,
	req *connect.Request[paymentv1.GetWalletBalanceRequest],
) (*connect.Response[paymentv1.GetWalletBalanceResponse], error) {
	var out *paymentv1.GetWalletBalanceResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.GetWalletBalance(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// ListLedgerEntries forwards the ListLedgerEntries RPC to the upstream service.
func (f *PaymentForwarder) ListLedgerEntries(
	ctx context.Context,
	req *connect.Request[paymentv1.ListLedgerEntriesRequest],
) (*connect.Response[paymentv1.ListLedgerEntriesResponse], error) {
	var out *paymentv1.ListLedgerEntriesResponse
	err := f.edge.callRead(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.ListLedgerEntries(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}

// RequestWalletPayout forwards the RequestWalletPayout RPC to the upstream service.
func (f *PaymentForwarder) RequestWalletPayout(
	ctx context.Context,
	req *connect.Request[paymentv1.RequestWalletPayoutRequest],
) (*connect.Response[paymentv1.RequestWalletPayoutResponse], error) {
	var out *paymentv1.RequestWalletPayoutResponse
	err := f.edge.callWrite(f.edge.outgoing(ctx, req.Header()), func(c context.Context) error {
		var e error
		out, e = f.client.RequestWalletPayout(c, req.Msg)
		return e
	})
	if err != nil {
		return nil, toConnectErr(err)
	}
	return connect.NewResponse(out), nil
}
