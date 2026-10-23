package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	"github.com/buidangphuc/team-engagement/internal/engagement"
	"github.com/buidangphuc/team-engagement/internal/store"
	"github.com/buidangphuc/team-engagement/internal/store/storetest"
	"github.com/buidangphuc/team-engagement/internal/upstream"
)

type errRepo struct {
	*storetest.FakeRepository
	err error
}

func (r *errRepo) AddFavorite(context.Context, string, string) (bool, error) { return false, r.err }

func (r *errRepo) ListFollowedListings(context.Context, string, string, int32) ([]string, string, int64, error) {
	return nil, "", 0, r.err
}

func uniqueViolation() error {
	return &pgconn.PgError{Code: "23505", ConstraintName: "favorites_pkey", TableName: "favorites", Message: "duplicate key value violates unique constraint \"favorites_pkey\""}
}

func addFavoriteWith(t *testing.T, err error) (error, string) {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := newHandler(t, deps{repo: &errRepo{FakeRepository: storetest.NewFakeRepository(), err: err}, logger: logger})
	_, callErr := h.AddFavorite(userCtx("u-1"), &engagementv1.AddFavoriteRequest{ListingId: "l-1"})
	return callErr, logs.String()
}

// Scenario: Engagement SDK: Unique violation is AlreadyExists
func TestErrorTableUniqueViolationAlreadyExists(t *testing.T) {
	err, _ := addFavoriteWith(t, uniqueViolation())
	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists || st.Message() != "already exists" {
		t.Fatalf("got %v %q, want AlreadyExists \"already exists\"", st.Code(), st.Message())
	}
	for _, leak := range []string{"favorites_pkey", "favorites", "duplicate key"} {
		if strings.Contains(st.Message(), leak) {
			t.Fatalf("message leaks %q: %q", leak, st.Message())
		}
	}
}

// Scenario: Engagement SDK: Context errors keep their codes
func TestErrorTableContextErrors(t *testing.T) {
	for name, c := range map[string]struct {
		err  error
		want codes.Code
	}{
		"canceled":          {context.Canceled, codes.Canceled},
		"deadline exceeded": {context.DeadlineExceeded, codes.DeadlineExceeded},
		"wrapped canceled":  {fmt.Errorf("store: %w", context.Canceled), codes.Canceled},
	} {
		t.Run(name, func(t *testing.T) {
			err, _ := addFavoriteWith(t, c.err)
			if got := status.Code(err); got != c.want {
				t.Fatalf("code = %v, want %v (%v)", got, c.want, err)
			}
		})
	}
}

// Scenario: Engagement SDK: Unmapped error is a fixed Internal
func TestErrorTableHidesInternalDetail(t *testing.T) {
	err, logs := addFavoriteWith(t, errors.New("connection refused to 10.0.0.7:5432"))
	st := status.Convert(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q, want Internal \"internal error\"", st.Code(), st.Message())
	}
	if strings.Contains(err.Error(), "10.0.0.7") {
		t.Fatalf("client error leaks detail: %v", err)
	}
	if !strings.Contains(logs, "10.0.0.7") {
		t.Fatalf("server log must keep the real cause, got:\n%s", logs)
	}
}

type tableRow struct {
	name string
	err  error
	code codes.Code
	msg  string
}

func tableRows() []tableRow {
	text := func(name string, err error, code codes.Code) tableRow { return tableRow{name, err, code, err.Error()} }
	return []tableRow{
		{"ErrOrderNotEligible", engagement.ErrOrderNotEligible, codes.FailedPrecondition, "order is not eligible for review"},
		{"ErrOrderUnavailable", engagement.ErrOrderUnavailable, codes.Unavailable, "order verification unavailable"},
		{"ErrDuplicateReview", store.ErrDuplicateReview, codes.AlreadyExists, "review already exists for this order and listing"},
		{"ErrOpenDisputeExists", store.ErrOpenDisputeExists, codes.AlreadyExists, "an open dispute already exists for this return"},
		{"ErrInvalidCursor", store.ErrInvalidCursor, codes.InvalidArgument, "invalid cursor"},
		{"ErrReviewNotFound", store.ErrReviewNotFound, codes.NotFound, "review not found"},
		{"ErrQuestionNotFound", store.ErrQuestionNotFound, codes.NotFound, "question not found"},
		{"ErrCollectionNotFound", store.ErrCollectionNotFound, codes.NotFound, "collection not found"},
		{"ErrReportNotFound", store.ErrReportNotFound, codes.NotFound, "report not found"},
		{"ErrDisputeNotFound", store.ErrDisputeNotFound, codes.NotFound, "dispute not found"},
		{"upstream.ErrOrderNotFound", upstream.ErrOrderNotFound, codes.NotFound, "order not found"},
		{"upstream.ErrReturnNotFound", upstream.ErrReturnNotFound, codes.NotFound, "return not found"},
		{"ErrDisputeClosed", engagement.ErrDisputeClosed, codes.FailedPrecondition, "dispute is already closed"},
		{"ErrInvalidTransition", engagement.ErrInvalidTransition, codes.FailedPrecondition, "invalid dispute status transition"},
		{"ErrDisputeNotInState", store.ErrDisputeNotInState, codes.FailedPrecondition, "dispute is not in the expected status"},
		{"ErrReportClosed", engagement.ErrReportClosed, codes.FailedPrecondition, "report is already closed"},
		text("ErrOwnListing", engagement.ErrOwnListing, codes.FailedPrecondition),
		{"ErrInvalidDisputeStatus", engagement.ErrInvalidDisputeStatus, codes.InvalidArgument, "invalid dispute status"},
		text("ErrRulingRequired", engagement.ErrRulingRequired, codes.InvalidArgument),
		text("ErrRulingNotAllowed", engagement.ErrRulingNotAllowed, codes.InvalidArgument),
		text("ErrSameClaimantAndDef", engagement.ErrSameClaimantAndDef, codes.InvalidArgument),
		text("ErrInvalidRating", engagement.ErrInvalidRating, codes.InvalidArgument),
		text("ErrEmptyListing", engagement.ErrEmptyListing, codes.InvalidArgument),
		text("ErrEmptyReviewID", engagement.ErrEmptyReviewID, codes.InvalidArgument),
		text("ErrEmptySeller", engagement.ErrEmptySeller, codes.InvalidArgument),
		text("ErrReasonTooLong", engagement.ErrReasonTooLong, codes.InvalidArgument),
		text("ErrEmptyQuestionText", engagement.ErrEmptyQuestionText, codes.InvalidArgument),
		text("ErrEmptyAnswerText", engagement.ErrEmptyAnswerText, codes.InvalidArgument),
		text("ErrEmptyQuestionID", engagement.ErrEmptyQuestionID, codes.InvalidArgument),
		text("ErrEmptyUserID", engagement.ErrEmptyUserID, codes.InvalidArgument),
		text("ErrEmptyDisputeID", engagement.ErrEmptyDisputeID, codes.InvalidArgument),
		text("ErrEmptyOrderID", engagement.ErrEmptyOrderID, codes.InvalidArgument),
		text("ErrEmptyClaimantID", engagement.ErrEmptyClaimantID, codes.InvalidArgument),
		text("ErrEmptyDefendantID", engagement.ErrEmptyDefendantID, codes.InvalidArgument),
		text("ErrEmptyReason", engagement.ErrEmptyReason, codes.InvalidArgument),
		text("ErrEmptyCollectionName", engagement.ErrEmptyCollectionName, codes.InvalidArgument),
		text("ErrEmptyCollectionID", engagement.ErrEmptyCollectionID, codes.InvalidArgument),
		text("ErrInvalidReportTarget", engagement.ErrInvalidReportTarget, codes.InvalidArgument),
		text("ErrInvalidReportReason", engagement.ErrInvalidReportReason, codes.InvalidArgument),
		text("ErrReportNoteTooLong", engagement.ErrReportNoteTooLong, codes.InvalidArgument),
		text("ErrEmptyReportID", engagement.ErrEmptyReportID, codes.InvalidArgument),
		text("ErrInvalidReportStatus", engagement.ErrInvalidReportStatus, codes.InvalidArgument),
		text("ErrResolutionNoteRequired", engagement.ErrResolutionNoteRequired, codes.InvalidArgument),
		text("ErrResolutionNoteTooLong", engagement.ErrResolutionNoteTooLong, codes.InvalidArgument),
		text("ErrReportCapReached", engagement.ErrReportCapReached, codes.ResourceExhausted),
	}
}

// Scenario: Engagement SDK: Domain sentinels keep their codes and messages
func TestErrorTableKeepsDomainCodes(t *testing.T) {
	for _, row := range tableRows() {
		for _, wrapped := range []bool{false, true} {
			name := row.name
			err := row.err
			if wrapped {
				name += " wrapped"
				err = fmt.Errorf("layer-xyz: %w", row.err)
			}
			t.Run(name, func(t *testing.T) {
				got, _ := addFavoriteWith(t, err)
				st := status.Convert(got)
				if st.Code() != row.code || st.Message() != row.msg {
					t.Fatalf("got %v %q, want %v %q", st.Code(), st.Message(), row.code, row.msg)
				}
				if strings.Contains(st.Message(), "layer-xyz") {
					t.Fatalf("wrapping text reached the client: %q", st.Message())
				}
			})
		}
	}
}

// Scenario: Engagement SDK: A status error from the audit gate passes through
func TestAuditGateStatusPassesThroughTable(t *testing.T) {
	f := newReviewFixture(t)
	r := f.seed(t, "L-1", "s-1", 1, true)
	f.audit.err = errors.New("team-audit down")
	_, err := f.hide(adminCtx(), r.ID, "spam")
	st := status.Convert(err)
	if st.Code() != codes.Unavailable || st.Message() != auditx.Message {
		t.Fatalf("got %v %q, want Unavailable %q", st.Code(), st.Message(), auditx.Message)
	}
}

type pgDuplicateReviews struct {
	*storetest.FakeReviewRepository
}

func (pgDuplicateReviews) CreateReview(context.Context, store.Review) (store.Review, error) {
	return store.Review{}, fmt.Errorf("%w: %w", store.ErrDuplicateReview, uniqueViolation())
}

// Scenario: Engagement SDK: Duplicate review keeps its message
func TestCreateReviewDuplicateKeepsMessage(t *testing.T) {
	const want = "review already exists for this order and listing"
	t.Run("second insert through the store", func(t *testing.T) {
		f := newReviewFixture(t)
		f.verifier.orders["buyer-1|order-1|L-1"] = "seller-1"
		if _, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5)); err != nil {
			t.Fatal(err)
		}
		_, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 4))
		st := status.Convert(err)
		if st.Code() != codes.AlreadyExists || st.Message() != want {
			t.Fatalf("got %v %q", st.Code(), st.Message())
		}
	})
	t.Run("store error wraps SQLSTATE 23505", func(t *testing.T) {
		verifier := &reviewVerifier{orders: map[string]string{"buyer-1|order-1|L-1": "seller-1"}}
		svc := engagement.NewReviewService(pgDuplicateReviews{storetest.NewFakeReviewRepository()}, verifier, permissiveGate(), nil)
		h := newHandler(t, deps{reviews: svc})
		_, err := h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5))
		st := status.Convert(err)
		if st.Code() != codes.AlreadyExists || st.Message() != want {
			t.Fatalf("got %v %q, want AlreadyExists %q", st.Code(), st.Message(), want)
		}
	})
}

// Scenario: Engagement SDK: Ineligible order keeps its message
func TestCreateReviewIneligibleKeepsMessage(t *testing.T) {
	f := newReviewFixture(t)
	_, err := f.h.CreateReview(userCtx("buyer-1"), createReviewReq("L-1", "order-1", 5))
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition || st.Message() != "order is not eligible for review" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if f.count("L-1") != 0 {
		t.Fatal("review stored for an ineligible order")
	}
}

type pgOpenDisputes struct {
	*storetest.FakeDisputeRepository
}

func (pgOpenDisputes) CreateDispute(context.Context, store.Dispute) (store.Dispute, error) {
	return store.Dispute{}, fmt.Errorf("%w: %w", store.ErrOpenDisputeExists, uniqueViolation())
}

// Scenario: Engagement SDK: Open dispute for a return keeps its message
func TestCreateDisputeOpenExistsKeepsMessage(t *testing.T) {
	const want = "an open dispute already exists for this return"
	t.Run("open dispute on the return", func(t *testing.T) {
		f := newEscFixture(t)
		f.seed(t, store.Dispute{ReturnID: "r-1", Status: store.DisputeStatusOpen})
		_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-1")
		st := status.Convert(err)
		if st.Code() != codes.AlreadyExists || st.Message() != want {
			t.Fatalf("got %v %q", st.Code(), st.Message())
		}
	})
	t.Run("store error wraps SQLSTATE 23505", func(t *testing.T) {
		repo := pgOpenDisputes{storetest.NewFakeDisputeRepository()}
		h := newHandler(t, deps{
			disputes: engagement.NewDisputeService(repo, permissiveGate(), nil),
			orders:   fakeOrders{"o-1": {buyer: "buyer-1", seller: "seller-1"}},
			returns: &returnFake{returns: map[string]returnInfo{
				"r-1": {orderID: "o-1", status: 0, canEscalate: true},
			}},
		})
		_, err := h.CreateDispute(userCtx("buyer-1"), &engagementv1.CreateDisputeRequest{OrderId: "o-1", DefendantId: "seller-1", Reason: "r", ReturnId: "r-1"})
		st := status.Convert(err)
		if st.Code() != codes.AlreadyExists || st.Message() != want {
			t.Fatalf("got %v %q, want AlreadyExists %q", st.Code(), st.Message(), want)
		}
	})
}

// Scenario: Engagement SDK: Unknown or foreign order is not found
func TestCreateDisputeOrderNotFoundKeepsMessage(t *testing.T) {
	for name, c := range map[string]struct {
		ctx   context.Context
		order string
	}{
		"unknown order":       {userCtx("buyer-1"), "o-missing"},
		"another buyer order": {userCtx("buyer-2"), "o-1"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newEscFixture(t)
			_, err := f.create(c.ctx, c.order, "seller-1", "")
			st := status.Convert(err)
			if st.Code() != codes.NotFound || st.Message() != "order not found" {
				t.Fatalf("got %v %q", st.Code(), st.Message())
			}
			if n := len(f.disputesOfOrder(t, c.order)); n != 0 {
				t.Fatalf("dispute rows = %d, want 0", n)
			}
		})
	}
}

// Scenario: Engagement SDK: Unknown return or return of another order is not found
func TestCreateDisputeReturnNotFoundKeepsMessage(t *testing.T) {
	for _, rid := range []string{"r-missing", "r-other"} {
		t.Run(rid, func(t *testing.T) {
			f := newEscFixture(t)
			_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", rid)
			st := status.Convert(err)
			if st.Code() != codes.NotFound || st.Message() != "return not found" {
				t.Fatalf("got %v %q", st.Code(), st.Message())
			}
			if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
				t.Fatalf("dispute rows = %d, want 0", n)
			}
		})
	}
}

// Scenario: Engagement SDK: A return that cannot be escalated is refused
func TestCreateDisputeReturnCannotEscalateKeepsMessage(t *testing.T) {
	f := newEscFixture(t)
	_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-no")
	st := status.Convert(err)
	if st.Code() != codes.FailedPrecondition || st.Message() != "return cannot be escalated" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
		t.Fatalf("dispute rows = %d, want 0", n)
	}
}

// Scenario: Engagement SDK: Failing order or return lookup is Unavailable
func TestCreateDisputeLookupFailureKeepsMessage(t *testing.T) {
	t.Run("order lookup", func(t *testing.T) {
		f := newEscFixture(t)
		f.orders.err = errors.New("connection refused")
		_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "")
		st := status.Convert(err)
		if st.Code() != codes.Unavailable || st.Message() != "order verification unavailable" {
			t.Fatalf("got %v %q", st.Code(), st.Message())
		}
		if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
			t.Fatalf("dispute rows = %d, want 0", n)
		}
	})
	t.Run("return lookup", func(t *testing.T) {
		f := newEscFixture(t)
		f.returns.err = errors.New("connection refused")
		_, err := f.create(userCtx("buyer-1"), "o-1", "seller-1", "r-1")
		st := status.Convert(err)
		if st.Code() != codes.Unavailable || st.Message() != "return verification unavailable" {
			t.Fatalf("got %v %q", st.Code(), st.Message())
		}
		if n := len(f.disputesOfOrder(t, "o-1")); n != 0 {
			t.Fatalf("dispute rows = %d, want 0", n)
		}
	})
}

// Scenario: Engagement SDK: Order listing with a failing lookup is Unavailable
func TestListOrderDisputesLookupFailureKeepsMessage(t *testing.T) {
	f := newEscFixture(t)
	f.seed(t, store.Dispute{})
	f.orders.err = errors.New("connection refused")
	_, err := f.h.ListOrderDisputes(userCtx("buyer-1"), &engagementv1.ListOrderDisputesRequest{OrderId: "o-1"})
	st := status.Convert(err)
	if st.Code() != codes.Unavailable || st.Message() != "order verification unavailable" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

// Scenario: Engagement SDK: Unknown question is not found
func TestAnswerQuestionNotFoundKeepsMessage(t *testing.T) {
	h, _ := shopReplyFixture(t)
	ctx := principalCtx("seller-1", "user", "engagement:write", "listing.write")
	for _, shop := range []bool{false, true} {
		_, err := h.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{QuestionId: "nope", AnswerText: "a", IsShopReply: shop})
		st := status.Convert(err)
		if st.Code() != codes.NotFound || st.Message() != "question not found" {
			t.Fatalf("is_shop_reply=%v: got %v %q", shop, st.Code(), st.Message())
		}
	}
}

// Scenario: Engagement SDK: A stranger cannot open or read a dispute
func TestDisputePartiesOnly(t *testing.T) {
	f, id := newFixture(t)
	stranger := userCtx("stranger-1")

	_, err := createDispute(f, stranger, "order-1", "seller-1")
	if st := status.Convert(err); st.Code() != codes.NotFound || st.Message() != "order not found" {
		t.Fatalf("CreateDispute: got %v %q", st.Code(), st.Message())
	}
	if f.disputes.creates != 0 {
		t.Fatalf("a dispute row was stored (%d)", f.disputes.creates)
	}

	_, errReal := f.h.GetDispute(stranger, &engagementv1.GetDisputeRequest{DisputeId: id})
	_, errMissing := f.h.GetDispute(stranger, &engagementv1.GetDisputeRequest{DisputeId: "no-such-id"})
	if st := status.Convert(errReal); st.Code() != codes.NotFound || st.Message() != "dispute not found" {
		t.Fatalf("GetDispute: got %v %q", st.Code(), st.Message())
	}
	if errReal.Error() != errMissing.Error() {
		t.Fatalf("real %q differs from missing %q", errReal, errMissing)
	}

	_, err = f.h.ListOrderDisputes(stranger, &engagementv1.ListOrderDisputesRequest{OrderId: "order-1"})
	if st := status.Convert(err); st.Code() != codes.PermissionDenied || st.Message() != "not a party to this order" {
		t.Fatalf("ListOrderDisputes: got %v %q", st.Code(), st.Message())
	}
}

// Scenario: Engagement SDK: A non-owner's shop reply is downgraded to an ordinary answer
func TestShopReplyOwnerOnly(t *testing.T) {
	h, qid := shopReplyFixture(t)
	ctx := principalCtx("seller-2", "user", "engagement:write", "listing.write")
	res, err := h.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{QuestionId: qid, AnswerText: "a", IsShopReply: true})
	if err != nil {
		t.Fatalf("non-owner shop reply must be OK, got %v", err)
	}
	if res.GetAnswer().GetIsShopReply() {
		t.Fatal("returned answer is marked as a shop reply")
	}
	list, err := h.ListQuestionsByListing(context.Background(), &engagementv1.ListQuestionsByListingRequest{ListingId: "listing-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetQuestions()) != 1 || len(list.GetQuestions()[0].GetAnswers()) != 1 {
		t.Fatalf("want 1 question with 1 answer, got %v", list.GetQuestions())
	}
	if list.GetQuestions()[0].GetAnswers()[0].GetIsShopReply() {
		t.Fatal("stored answer is marked as a shop reply")
	}
	_, err = h.AnswerQuestion(ctx, &engagementv1.AnswerQuestionRequest{QuestionId: "nope", AnswerText: "a", IsShopReply: true})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("unknown question: want NotFound, got %v", err)
	}
}
