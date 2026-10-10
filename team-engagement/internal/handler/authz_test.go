package handler_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/handler"
	"github.com/buidangphuc/team-engagement/internal/interceptor"
	"github.com/buidangphuc/team-engagement/internal/repository"
	"github.com/buidangphuc/team-engagement/internal/service"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

var userScopes = []string{"listing.read", "engagement:read", "engagement:write"}

func principalCtx(id string, typ commonv1.PrincipalType, scopes ...string) context.Context {
	return interceptor.ContextWithPrincipal(context.Background(), &commonv1.Principal{Id: id, Type: typ, Scopes: scopes})
}

func userCtx(id string) context.Context {
	return principalCtx(id, commonv1.PrincipalType_PRINCIPAL_TYPE_USER, userScopes...)
}

func adminCtx() context.Context {
	return principalCtx("admin-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, append(append([]string{}, userScopes...), "admin")...)
}

func anonCtx() context.Context {
	return principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS, "listing.read")
}

type fixture struct {
	h        *handler.EngagementHandler
	disputes *countingDisputes
	orders   *orderFake
}

// countingDisputes records how many disputes the service tried to store.
type countingDisputes struct {
	*repository.InMemoryDisputeRepository
	creates int
}

func (c *countingDisputes) CreateDispute(ctx context.Context, d repository.Dispute) (repository.Dispute, error) {
	c.creates++
	return c.InMemoryDisputeRepository.CreateDispute(ctx, d)
}

type orderParties struct{ buyer, seller string }

// fakeOrders is a static OrderPartiesReader for tests that do not need call counts.
type fakeOrders map[string]orderParties

func (f fakeOrders) GetOrderParties(_ context.Context, id string) (string, string, error) {
	o, ok := f[id]
	if !ok {
		return "", "", upstream.ErrOrderNotFound
	}
	return o.buyer, o.seller, nil
}

// orderFake is fakeOrders with a forced error.
type orderFake struct {
	orders fakeOrders
	err    error
}

func (f *orderFake) GetOrderParties(ctx context.Context, id string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	return f.orders.GetOrderParties(ctx, id)
}

// newFixture builds a handler over in-memory repos and seeds an OPEN dispute
// (buyer-1 vs seller-1) directly in the repo, so the read/resolve guards are
// exercised independently of CreateDispute's order verification.
func newFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	disputes := &countingDisputes{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository()}
	orders := &orderFake{orders: fakeOrders{"order-1": {buyer: "buyer-1", seller: "seller-1"}}}
	h := handler.NewEngagementHandler(
		repository.NewInMemoryRepository(),
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil),
		service.NewQAService(repository.NewInMemoryQARepository(), nil),
		service.NewDisputeService(disputes, nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil),
		handler.WithOrderParties(orders),
	)
	d, err := disputes.InMemoryDisputeRepository.CreateDispute(context.Background(), repository.Dispute{
		OrderID: "order-1", ClaimantID: "buyer-1", DefendantID: "seller-1", Reason: "r", Status: repository.DisputeStatusOpen,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{h: h, disputes: disputes, orders: orders}, d.ID
}

func createDispute(f *fixture, ctx context.Context, orderID, defendant string) (*engagementv1.CreateDisputeResponse, error) {
	return f.h.CreateDispute(ctx, &engagementv1.CreateDisputeRequest{OrderId: orderID, DefendantId: defendant, Reason: "broken"})
}

func TestCreateDisputeVerifiesTheOrder(t *testing.T) {
	cases := []struct {
		name      string
		ctx       context.Context
		orderID   string
		defendant string
		orderErr  error
		want      codes.Code
	}{
		{"buyer disputes own order against its seller", userCtx("buyer-1"), "order-1", "seller-1", nil, codes.OK},
		{"unknown order", userCtx("buyer-1"), "order-404", "seller-1", nil, codes.NotFound},
		{"someone else's order", userCtx("buyer-2"), "order-1", "seller-1", nil, codes.NotFound},
		{"seller cannot open against own order", userCtx("seller-1"), "order-1", "buyer-1", nil, codes.NotFound},
		{"wrong defendant", userCtx("buyer-1"), "order-1", "seller-2", nil, codes.PermissionDenied},
		{"order service down", userCtx("buyer-1"), "order-1", "seller-1", status.Error(codes.Unavailable, "down"), codes.Unavailable},
		{"order lookup generic error", userCtx("buyer-1"), "order-1", "seller-1", errors.New("boom"), codes.Unavailable},
		{"anonymous", anonCtx(), "order-1", "seller-1", nil, codes.PermissionDenied},
		{"no principal", context.Background(), "order-1", "seller-1", nil, codes.Unauthenticated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, _ := newFixture(t)
			f.orders.err = c.orderErr
			before := f.disputes.creates
			res, err := createDispute(f, c.ctx, c.orderID, c.defendant)
			if got := status.Code(err); got != c.want {
				t.Fatalf("want %v, got %v (%v)", c.want, got, err)
			}
			if c.want == codes.OK {
				if res.GetDispute().GetClaimantId() != "buyer-1" || res.GetDispute().GetStatus() != engagementv1.DisputeStatus_DISPUTE_STATUS_OPEN {
					t.Fatalf("unexpected dispute %v", res.GetDispute())
				}
				return
			}
			if res != nil {
				t.Fatalf("nothing may be returned on rejection, got %v", res)
			}
			if f.disputes.creates != before {
				t.Fatal("rejected CreateDispute stored a dispute")
			}
		})
	}
}

// Without an order client CreateDispute fails closed instead of trusting the request.
func TestCreateDisputeFailsClosedWithoutOrderClient(t *testing.T) {
	disputes := &countingDisputes{InMemoryDisputeRepository: repository.NewInMemoryDisputeRepository()}
	h := handler.NewEngagementHandler(repository.NewInMemoryRepository(),
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil),
		service.NewQAService(repository.NewInMemoryQARepository(), nil),
		service.NewDisputeService(disputes, nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil))
	_, err := h.CreateDispute(userCtx("buyer-1"), &engagementv1.CreateDisputeRequest{OrderId: "o", DefendantId: "s", Reason: "r"})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("want UNAVAILABLE, got %v", err)
	}
	if disputes.creates != 0 {
		t.Fatal("a dispute was stored")
	}
}

func TestGetDisputeOnlyForPartiesAndAdmin(t *testing.T) {
	for _, c := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"claimant", userCtx("buyer-1"), codes.OK},
		{"defendant", userCtx("seller-1"), codes.OK},
		{"admin", adminCtx(), codes.OK},
		{"stranger buyer", userCtx("buyer-2"), codes.NotFound},
		{"stranger seller", userCtx("seller-2"), codes.NotFound},
		{"anonymous", anonCtx(), codes.Unauthenticated},
		{"no principal", context.Background(), codes.Unauthenticated},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, id := newFixture(t)
			res, err := f.h.GetDispute(c.ctx, &engagementv1.GetDisputeRequest{DisputeId: id})
			if got := status.Code(err); got != c.want {
				t.Fatalf("want %v, got %v (%v)", c.want, got, err)
			}
			if c.want == codes.OK && res.GetDispute().GetId() != id {
				t.Fatalf("dispute not returned: %v", res)
			}
			if c.want != codes.OK && res != nil {
				t.Fatalf("no dispute data may be returned on rejection, got %v", res)
			}
		})
	}
}

// A stranger cannot tell a real dispute id from a missing one.
func TestGetDisputeStrangerIndistinguishableFromMissing(t *testing.T) {
	f, id := newFixture(t)
	_, errReal := f.h.GetDispute(userCtx("buyer-2"), &engagementv1.GetDisputeRequest{DisputeId: id})
	_, errMissing := f.h.GetDispute(userCtx("buyer-2"), &engagementv1.GetDisputeRequest{DisputeId: "nope"})
	if status.Code(errReal) != codes.NotFound || errReal.Error() != errMissing.Error() {
		t.Fatalf("real=%v missing=%v", errReal, errMissing)
	}
}

// An admin token that lacks the `admin` scope is not an admin.
func TestIsAdminRequiresScopeAndAuthentication(t *testing.T) {
	if !interceptor.IsAdmin(adminCtx()) {
		t.Fatal("admin scope must count")
	}
	for name, ctx := range map[string]context.Context{
		"user":      userCtx("u"),
		"anonymous": principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS, "admin"),
		"none":      context.Background(),
	} {
		if interceptor.IsAdmin(ctx) {
			t.Fatalf("%s must not be admin", name)
		}
	}
}

// shopReplyFixture: question on listing-1, owned by seller-1 in the seller_listings projection.
func shopReplyFixture(t *testing.T) (*handler.EngagementHandler, string) {
	t.Helper()
	repo := repository.NewInMemoryRepository()
	qaSvc := service.NewQAService(repository.NewInMemoryQARepository(), nil)
	h := handler.NewEngagementHandler(repo,
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil), qaSvc,
		service.NewDisputeService(repository.NewInMemoryDisputeRepository(), nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil))
	if err := repo.IndexSellerListing(context.Background(), "seller-1", "listing-1"); err != nil {
		t.Fatal(err)
	}
	q, err := qaSvc.AskQuestion(context.Background(), "listing-1", "buyer-1", "ok?")
	if err != nil {
		t.Fatal(err)
	}
	return h, q.ID
}

func TestIsShopReplyDerivedFromListingOwnership(t *testing.T) {
	sellerScopes := append(append([]string{}, userScopes...), "listing.write")
	seller := func(id string) context.Context {
		return principalCtx(id, commonv1.PrincipalType_PRINCIPAL_TYPE_USER, sellerScopes...)
	}
	for _, c := range []struct {
		name      string
		ctx       context.Context
		requested bool
		want      bool
	}{
		{"owner asks for shop reply", seller("seller-1"), true, true},
		{"owner without flag", seller("seller-1"), false, false},
		{"other seller with listing.write cannot claim it", seller("seller-2"), true, false},
		{"buyer cannot claim it", userCtx("buyer-1"), true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h, qid := shopReplyFixture(t)
			res, err := h.AnswerQuestion(c.ctx, &engagementv1.AnswerQuestionRequest{QuestionId: qid, AnswerText: "a", IsShopReply: c.requested})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.GetAnswer().GetIsShopReply(); got != c.want {
				t.Fatalf("is_shop_reply = %v, want %v", got, c.want)
			}
		})
	}
}

// A listing the seller_listings projection does not know has no provable owner,
// so nobody (even a listing.write holder) gets the shop badge.
func TestIsShopReplyFailsClosedForUnknownListing(t *testing.T) {
	h, _ := shopReplyFixture(t)
	// Question on a listing absent from the projection.
	repo := repository.NewInMemoryRepository()
	qa := service.NewQAService(repository.NewInMemoryQARepository(), nil)
	h2 := handler.NewEngagementHandler(repo,
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil), qa,
		service.NewDisputeService(repository.NewInMemoryDisputeRepository(), nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil))
	q, err := qa.AskQuestion(context.Background(), "unindexed-listing", "buyer-1", "ok?")
	if err != nil {
		t.Fatal(err)
	}
	ctx := principalCtx("seller-1", commonv1.PrincipalType_PRINCIPAL_TYPE_USER, "engagement:write", "listing.write")
	res, err := h2.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{QuestionId: q.ID, AnswerText: "a", IsShopReply: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetAnswer().GetIsShopReply() {
		t.Fatal("unknown listing owner must not yield a shop reply")
	}
	// Unknown question with the flag set is NOT_FOUND, not a leaked internal error.
	_, err = h.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{QuestionId: "nope", AnswerText: "a", IsShopReply: true})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NOT_FOUND, got %v", err)
	}
}

func resolve(f *fixture, ctx context.Context, id string) error {
	_, err := f.h.ResolveDispute(ctx, &engagementv1.ResolveDisputeRequest{
		DisputeId: id, Status: engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED, Resolution: "refund",
	})
	return err
}

func TestResolveDisputeIsAdminOnly(t *testing.T) {
	for _, c := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"claimant buyer", userCtx("buyer-1"), codes.PermissionDenied},
		{"defendant seller", userCtx("seller-1"), codes.PermissionDenied},
		{"stranger", userCtx("stranger"), codes.PermissionDenied},
		{"anonymous", anonCtx(), codes.Unauthenticated},
		{"no principal", context.Background(), codes.Unauthenticated},
		{"anonymous holding admin scope", principalCtx("anonymous", commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS, "admin"), codes.Unauthenticated},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, id := newFixture(t)
			if got := status.Code(resolve(f, c.ctx, id)); got != c.want {
				t.Fatalf("want %v, got %v", c.want, got)
			}
			d, err := f.disputes.GetDispute(context.Background(), id)
			if err != nil || d.Status != repository.DisputeStatusOpen {
				t.Fatalf("dispute must stay OPEN: %v %v", d.Status, err)
			}
		})
	}
}

func TestAdminResolvesOnceThenFailedPrecondition(t *testing.T) {
	f, id := newFixture(t)
	if err := resolve(f, adminCtx(), id); err != nil {
		t.Fatalf("admin resolve: %v", err)
	}
	if got := status.Code(resolve(f, adminCtx(), id)); got != codes.FailedPrecondition {
		t.Fatalf("second resolve: want FAILED_PRECONDITION, got %v", got)
	}
}

// brokenDisputes fails every call with an error carrying storage details.
type brokenDisputes struct {
	*repository.InMemoryDisputeRepository
}

var errLeaky = errors.New(`pq: password authentication failed for user "engagement_svc" at db.internal:5432`)

func (brokenDisputes) CreateDispute(context.Context, repository.Dispute) (repository.Dispute, error) {
	return repository.Dispute{}, errLeaky
}
func (brokenDisputes) GetDispute(context.Context, string) (repository.Dispute, error) {
	return repository.Dispute{}, errLeaky
}

type brokenQA struct {
	*repository.InMemoryQARepository
}

func (brokenQA) CreateAnswer(context.Context, repository.ProductAnswer) (repository.ProductAnswer, error) {
	return repository.ProductAnswer{}, errLeaky
}

func TestInternalErrorsAreGeneric(t *testing.T) {
	disputes := brokenDisputes{repository.NewInMemoryDisputeRepository()}
	qa := brokenQA{repository.NewInMemoryQARepository()}
	h := handler.NewEngagementHandler(repository.NewInMemoryRepository(),
		service.NewReviewService(repository.NewInMemoryReviewRepository(), nil, nil),
		service.NewQAService(qa, nil), service.NewDisputeService(disputes, nil),
		service.NewCollectionService(repository.NewInMemoryCollectionRepository(), nil),
		handler.WithOrderParties(fakeOrders{"order-1": {buyer: "buyer-1", seller: "seller-1"}}))
	q, err := qa.CreateQuestion(context.Background(), repository.ProductQuestion{ListingID: "l", UserID: "buyer-1", QuestionText: "q"})
	if err != nil {
		t.Fatal(err)
	}
	calls := map[string]func() error{
		"AnswerQuestion": func() error {
			_, err := h.AnswerQuestion(userCtx("buyer-1"), &engagementv1.AnswerQuestionRequest{QuestionId: q.ID, AnswerText: "a"})
			return err
		},
		"CreateDispute": func() error {
			_, err := createDispute(&fixture{h: h}, userCtx("buyer-1"), "order-1", "seller-1")
			return err
		},
		"GetDispute": func() error {
			_, err := h.GetDispute(userCtx("buyer-1"), &engagementv1.GetDisputeRequest{DisputeId: "d"})
			return err
		},
		"ResolveDispute": func() error {
			_, err := h.ResolveDispute(adminCtx(), &engagementv1.ResolveDisputeRequest{
				DisputeId: "d", Status: engagementv1.DisputeStatus_DISPUTE_STATUS_RESOLVED})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if status.Code(err) != codes.Internal {
				t.Fatalf("want INTERNAL, got %v", err)
			}
			if status.Convert(err).Message() != "internal error" {
				t.Fatalf("message leaks detail: %q", status.Convert(err).Message())
			}
		})
	}
}
