package rpc_test

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-referral/generated/platform/common/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/rpc"
	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

type faultyRepo struct {
	*fake.InMemoryReferralRepo
	getCodeErr    error
	createCodeErr error
	redeemErr     error
	allErr        error
}

func (r *faultyRepo) GetCodeByUser(ctx context.Context, u string) (string, error) {
	if r.allErr != nil {
		return "", r.allErr
	}
	if r.getCodeErr != nil {
		return "", r.getCodeErr
	}
	return r.InMemoryReferralRepo.GetCodeByUser(ctx, u)
}

func (r *faultyRepo) CreateCode(ctx context.Context, u, c string) (string, error) {
	if r.allErr != nil {
		return "", r.allErr
	}
	if r.createCodeErr != nil {
		return "", r.createCodeErr
	}
	return r.InMemoryReferralRepo.CreateCode(ctx, u, c)
}

func (r *faultyRepo) Redeem(ctx context.Context, in store.RedeemInput) error {
	if r.allErr != nil {
		return r.allErr
	}
	if r.redeemErr != nil {
		return r.redeemErr
	}
	return r.InMemoryReferralRepo.Redeem(ctx, in)
}

func (r *faultyRepo) OwnerOfCode(ctx context.Context, c string) (string, error) {
	if r.allErr != nil {
		return "", r.allErr
	}
	return r.InMemoryReferralRepo.OwnerOfCode(ctx, c)
}

func (r *faultyRepo) CountReferralsByReferrer(ctx context.Context, u string) (int64, error) {
	if r.allErr != nil {
		return 0, r.allErr
	}
	return r.InMemoryReferralRepo.CountReferralsByReferrer(ctx, u)
}

func (r *faultyRepo) RewardTotals(ctx context.Context, u string) (store.RewardTotals, error) {
	if r.allErr != nil {
		return store.RewardTotals{}, r.allErr
	}
	return r.InMemoryReferralRepo.RewardTotals(ctx, u)
}

func (r *faultyRepo) ListRewards(ctx context.Context, u string, l, o int) ([]*referralv1.ReferralReward, error) {
	if r.allErr != nil {
		return nil, r.allErr
	}
	return r.InMemoryReferralRepo.ListRewards(ctx, u, l, o)
}

var t0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

func registerNow(t testing.TB, repo store.ReferralRepository, users ...string) {
	t.Helper()
	now := time.Now()
	for _, u := range users {
		err := repo.ApplyUserRegistered(context.Background(), store.UserRegisteredInput{
			EventID: "ur-" + u, UserID: u, RegisteredAt: now.Add(-time.Hour), At: now, LifetimeCap: 50,
		})
		if err != nil {
			t.Fatalf("register %s: %v", u, err)
		}
	}
}

func seedReward(t testing.TB, repo store.ReferralRepository, _, code, referee, order string, releaseAt time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	err := repo.ApplyUserRegistered(ctx, store.UserRegisteredInput{EventID: "ur-" + referee, UserID: referee, ReferralCode: code, RegisteredAt: now.Add(-time.Hour), At: now, LifetimeCap: 1000})
	if err != nil {
		t.Fatalf("signup %s: %v", referee, err)
	}
	err = repo.ApplyOrderCompleted(ctx, store.OrderCompletedInput{EventID: "oc-" + order, OrderID: order, BuyerID: referee, CompletedAt: now, ReleaseAt: releaseAt, At: now, RewardXU: 10000, LifetimeCap: 1000})
	if err != nil {
		t.Fatalf("order %s: %v", order, err)
	}
}

func newFaulty() *faultyRepo {
	return &faultyRepo{InMemoryReferralRepo: fake.NewInMemoryReferralRepo()}
}

func handlerFor(t testing.TB, repo store.ReferralRepository) *rpc.Handler {
	t.Helper()
	h, err := rpc.NewHandler(referral.NewService(repo), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func newHandler(t testing.TB) *rpc.Handler {
	return handlerFor(t, fake.NewInMemoryReferralRepo())
}

func userCtx(id string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: auth.TypeUser})
}

func dial(t *testing.T, h *rpc.Handler) referralv1.ReferralServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServerInterceptor()))
	rpc.Register(srv, h)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return referralv1.NewReferralServiceClient(conn)
}

// Scenario: A call without a principal is rejected.
func TestHandlerRejectsAnonymous(t *testing.T) {
	h := newHandler(t)
	anon := context.Background()

	if _, err := h.CreateReferralCode(anon, &referralv1.CreateReferralCodeRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateReferralCode: expected Unauthenticated, got %v", err)
	}
	if _, err := h.GetMyReferral(anon, &referralv1.GetMyReferralRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetMyReferral: expected Unauthenticated, got %v", err)
	}
	if _, err := h.RedeemReferral(anon, &referralv1.RedeemReferralRequest{Code: "REFx"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("RedeemReferral: expected Unauthenticated, got %v", err)
	}
	if _, err := h.ListReferralRewards(anon, &referralv1.ListReferralRewardsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListReferralRewards: expected Unauthenticated, got %v", err)
	}
}

// Scenario: One user cannot see another user's data.
func TestHandlerScopesToPrincipal(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	registerNow(t, repo, "bob", "carol")

	created, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	code := created.GetCode()

	if _, err := h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: code}); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	mine, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if mine.GetInvitedCount() != 1 || mine.GetRewardsTotal() != 0 || mine.GetPendingXu() != 0 || mine.GetReleasedXu() != 0 {
		t.Fatalf("expected invited=1 and no xu, got %v", mine)
	}

	bob, err := h.GetMyReferral(userCtx("bob"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatalf("bob get: %v", err)
	}
	if bob.GetCode() == code || bob.GetInvitedCount() != 0 || bob.GetRewardsTotal() != 0 {
		t.Fatalf("bob sees alice's data: %+v", bob)
	}
	bobRewards, err := h.ListReferralRewards(userCtx("bob"), &referralv1.ListReferralRewardsRequest{})
	if err != nil {
		t.Fatalf("bob list: %v", err)
	}
	if len(bobRewards.GetRewards()) != 0 {
		t.Fatalf("bob sees rewards: %+v", bobRewards.GetRewards())
	}

	if _, err := h.RedeemReferral(userCtx("carol"), &referralv1.RedeemReferralRequest{Code: "REFmissing"}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound for unknown code, got %v", err)
	}
}

// Scenario: An empty code is rejected.
func TestRedeemEmptyCodeIsInvalidArgument(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	_, err := h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
	if _, err := repo.GetReferral(context.Background(), "bob"); err == nil {
		t.Fatal("a row was written for an empty code")
	}
	if n := len(repo.OutboxRows()); n != 0 {
		t.Fatalf("%d outbox rows written for an empty code", n)
	}
}

// Scenario: An anonymous caller cannot redeem.
func TestAnonymousRedeemWritesNothing(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	created, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.RedeemReferral(context.Background(), &referralv1.RedeemReferralRequest{Code: created.GetCode()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("expected Unauthenticated, got %v", err)
	}
	anon := auth.WithPrincipal(context.Background(), auth.Principal{ID: "anonymous", Type: auth.TypeAnonymous})
	_, err = h.RedeemReferral(anon, &referralv1.RedeemReferralRequest{Code: created.GetCode()})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous principal: expected Unauthenticated, got %v", err)
	}
	mine, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if mine.GetInvitedCount() != 0 || mine.GetRewardsTotal() != 0 {
		t.Fatalf("anonymous redeem wrote rows: %+v", mine)
	}
	if n := len(repo.OutboxRows()); n != 0 {
		t.Fatalf("anonymous redeem wrote %d outbox rows", n)
	}
}

// Scenario: The page total is the page length.
func TestPageTotalIsPageLength(t *testing.T) {
	repo := fake.NewInMemoryReferralRepo()
	h := handlerFor(t, repo)
	code, err := repo.CreateCode(context.Background(), "alice", "REFAAAA1111")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		seedReward(t, repo, "alice", code, fmt.Sprintf("ref%d", i), fmt.Sprintf("o%d", i), time.Now().Add(48*time.Hour))
	}
	resp, err := h.ListReferralRewards(userCtx("alice"), &referralv1.ListReferralRewardsRequest{
		Page: &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetRewards()) != 2 || resp.GetPage().GetTotal() != 2 {
		t.Fatalf("rewards=%d total=%d, want 2 and 2", len(resp.GetRewards()), resp.GetPage().GetTotal())
	}
	if resp.GetPage().GetNextCursor() == "" {
		t.Fatal("expected a next cursor")
	}
}

// Scenario: A storage failure does not leak its cause.
func TestMapErrHidesInternalCause(t *testing.T) {
	repo := newFaulty()
	repo.allErr = errString("pq: secret-host connection refused")
	h := handlerFor(t, repo)
	_, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if strings.Contains(err.Error(), "secret-host") {
		t.Fatalf("cause leaked: %v", err)
	}
}

type errString string

func (e errString) Error() string { return string(e) }

// Scenario: A unique violation is AlreadyExists.
func TestUniqueViolationIsAlreadyExists(t *testing.T) {
	repo := newFaulty()
	repo.getCodeErr = store.ErrCodeNotFound
	repo.createCodeErr = &pgconn.PgError{Code: "23505", ConstraintName: "referral_codes_pkey", Message: "duplicate key"}
	h := handlerFor(t, repo)
	_, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.AlreadyExists || st.Message() != "already exists" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if strings.Contains(err.Error(), "referral_codes_pkey") {
		t.Fatalf("constraint detail leaked: %v", err)
	}
}

// Scenario: A cancelled request is Canceled.
func TestCancelledRequestIsCanceled(t *testing.T) {
	repo := newFaulty()
	repo.allErr = context.Canceled
	h := handlerFor(t, repo)
	_, err := h.GetMyReferral(userCtx("alice"), &referralv1.GetMyReferralRequest{})
	if status.Code(err) != codes.Canceled {
		t.Fatalf("expected Canceled, got %v", err)
	}
}

// Scenario: The redeem race keeps FailedPrecondition.
func TestRedeemRaceIsFailedPrecondition(t *testing.T) {
	repo := newFaulty()
	h := handlerFor(t, repo)
	created, err := h.CreateReferralCode(userCtx("alice"), &referralv1.CreateReferralCodeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	repo.redeemErr = store.ErrAlreadyRedeemed
	_, err = h.RedeemReferral(userCtx("bob"), &referralv1.RedeemReferralRequest{Code: created.GetCode()})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}
