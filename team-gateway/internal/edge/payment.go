package edge

import (
	"context"

	"connectrpc.com/connect"

	paymentv1 "github.com/buidangphuc/team-gateway/generated/platform/payment/v1"
	"github.com/buidangphuc/team-gateway/generated/platform/payment/v1/paymentv1connect"
)

type PaymentForwarder struct {
	paymentv1connect.UnimplementedPaymentServiceHandler
	client paymentv1.PaymentServiceClient
	edge   *Edge
}

func NewPaymentForwarder(client paymentv1.PaymentServiceClient, edge *Edge) *PaymentForwarder {
	return &PaymentForwarder{client: client, edge: edge}
}

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
