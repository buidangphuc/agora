package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	orderv1 "github.com/buidangphuc/team-engagement/generated/platform/order/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/rpc"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

type deps struct {
	repo        store.Repository
	reviews     *engagement.ReviewService
	qa          *engagement.QAService
	disputes    *engagement.DisputeService
	collections *engagement.CollectionService
	reports     *engagement.ReportService
	orders      rpc.OrderPartiesReader
	returns     rpc.ReturnReader
	logger      *slog.Logger
}

type noReturns struct{}

func (noReturns) GetReturn(context.Context, string) (string, orderv1.ReturnStatus, bool, error) {
	return "", orderv1.ReturnStatus_RETURN_STATUS_UNSPECIFIED, false, upstream.ErrReturnNotFound
}

func permissiveGate() *auditx.Gate { return auditx.NewGate(&auditSpy{}, discardLogger()) }

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newHandler(t testing.TB, d deps) *rpc.Handler {
	t.Helper()
	if d.logger == nil {
		d.logger = discardLogger()
	}
	if d.repo == nil {
		d.repo = storetest.NewFakeRepository()
	}
	if d.reviews == nil {
		d.reviews = engagement.NewReviewService(storetest.NewFakeReviewRepository(), &reviewVerifier{orders: map[string]string{}}, permissiveGate(), d.logger)
	}
	if d.qa == nil {
		d.qa = engagement.NewQAService(storetest.NewFakeQARepository(), d.logger)
	}
	if d.disputes == nil {
		d.disputes = engagement.NewDisputeService(storetest.NewFakeDisputeRepository(), permissiveGate(), d.logger)
	}
	if d.collections == nil {
		d.collections = engagement.NewCollectionService(storetest.NewFakeCollectionRepository(), d.logger)
	}
	if d.reports == nil {
		d.reports = engagement.NewReportService(storetest.NewFakeReportRepository(), permissiveGate(), d.logger)
	}
	if d.orders == nil {
		d.orders = fakeOrders{}
	}
	if d.returns == nil {
		d.returns = noReturns{}
	}
	h, err := rpc.NewHandler(d.repo, d.reviews, d.qa, d.disputes, d.collections, d.reports, d.orders, d.returns, d.logger)
	if err != nil {
		t.Fatalf("rpc.NewHandler: %v", err)
	}
	return h
}

func principalCtx(id string, typ auth.PrincipalType, scopes ...string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: typ, Scopes: scopes})
}
