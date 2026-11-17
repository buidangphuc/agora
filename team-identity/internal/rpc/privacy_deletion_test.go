package rpc_test

// Tasks 1.4 / 1.5 (identity-account-deletion): PrivacyService RequestAccountDeletion,
// CancelAccountDeletion and GetAccountDeletionStatus.
//
// API assumed by these tests:
//   - identity.NewAccountDeletionService(identity.AccountDeletionDeps{Users, Obligations, Window, Now})
//     where Users is identity.DeletionRepository (GetByID, ScheduleDeletion, CancelDeletion and
//     DeletionEraseAfter, which returns the zero time when nothing is pending) and Obligations is
//     identity.ObligationSource (UserObligations, PayoutObligations).
//   - rpc.PrivacyHandler.WithAccountDeletion(svc) enables the three RPCs.
//   - password mismatch -> PERMISSION_DENIED, empty password -> INVALID_ARGUMENT, any obligation
//     call error -> UNAVAILABLE, non-zero counter -> FAILED_PRECONDITION naming the category only,
//     nothing pending on cancel -> FAILED_PRECONDITION.

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
	"github.com/buidangphuc/team-identity/internal/upstream"
)

const (
	delPassword = "correct-horse"
	delWindow   = 48 * time.Hour
)

var delNow = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

type delRepo struct {
	mu        sync.Mutex
	hash      string
	requested map[string]time.Time
	erase     map[string]time.Time
	sessions  map[string]map[string]bool // user -> session -> active
}

func newDelRepo(t *testing.T, users ...string) *delRepo {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(delPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	r := &delRepo{hash: string(h), requested: map[string]time.Time{}, erase: map[string]time.Time{}, sessions: map[string]map[string]bool{}}
	for _, u := range users {
		r.sessions[u] = map[string]bool{"sess-A": true, "sess-B": true, "sess-C": true}
	}
	return r
}

func (r *delRepo) GetByID(_ context.Context, id string) (store.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return store.User{}, store.ErrNotFound
	}
	return store.User{ID: id, Username: "name-" + id, PasswordHash: r.hash, Status: store.StatusActive}, nil
}

func (r *delRepo) ScheduleDeletion(_ context.Context, userID, keep string, now time.Time, window time.Duration) (store.DeletionSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.erase[userID]; ok {
		return store.DeletionSchedule{RequestedAt: r.requested[userID], EraseAfter: e}, nil
	}
	r.requested[userID] = now
	r.erase[userID] = now.Add(window)
	n := 0
	for id, active := range r.sessions[userID] {
		if id != keep && active {
			r.sessions[userID][id] = false
			n++
		}
	}
	return store.DeletionSchedule{RequestedAt: now, EraseAfter: now.Add(window), Created: true, SessionsRevoked: n}, nil
}

func (r *delRepo) CancelDeletion(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.erase[userID]; !ok {
		return store.ErrNoPendingDeletion
	}
	delete(r.erase, userID)
	delete(r.requested, userID)
	return nil
}

func (r *delRepo) DeletionEraseAfter(_ context.Context, userID string) (time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.erase[userID], nil
}

func (r *delRepo) pending(u string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.erase[u]
	return ok
}

func (r *delRepo) active(u, s string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[u][s]
}

type fakeObligations struct {
	mu       sync.Mutex
	user     map[string]upstream.UserObligations
	payout   map[string]upstream.PayoutObligations
	userErr  error
	payErr   error
	userIDs  []string
	payIDs   []string
	payCalls int
}

func (f *fakeObligations) UserObligations(_ context.Context, id string) (upstream.UserObligations, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.userIDs = append(f.userIDs, id)
	if f.userErr != nil {
		return upstream.UserObligations{}, f.userErr
	}
	return f.user[id], nil
}

func (f *fakeObligations) PayoutObligations(_ context.Context, id string) (upstream.PayoutObligations, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payCalls++
	f.payIDs = append(f.payIDs, id)
	if f.payErr != nil {
		return upstream.PayoutObligations{}, f.payErr
	}
	return f.payout[id], nil
}

func (f *fakeObligations) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.userIDs) + f.payCalls
}

func newDeletion(t *testing.T, users ...string) (*rpc.PrivacyHandler, *delRepo, *fakeObligations) {
	t.Helper()
	repo := newDelRepo(t, users...)
	ob := &fakeObligations{user: map[string]upstream.UserObligations{}, payout: map[string]upstream.PayoutObligations{}}
	svc := identity.NewAccountDeletionService(identity.AccountDeletionDeps{
		Users: repo, Obligations: ob, Window: delWindow, Now: func() time.Time { return delNow },
	})
	h, _ := newPrivacy(t, discardLogger())
	return h.WithAccountDeletion(svc), repo, ob
}

func sessCtx(userID, sessionID string) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: userID, Type: auth.TypeUser, SessionID: sessionID})
}

func wantDelCode(t *testing.T, err error, want codes.Code) *status.Status {
	t.Helper()
	st, _ := status.FromError(err)
	if err == nil || st.Code() != want {
		t.Fatalf("err = %v, want code %v", err, want)
	}
	return st
}

func request(ctx context.Context, h *rpc.PrivacyHandler, pw string) (*identityv1.RequestAccountDeletionResponse, error) {
	return h.RequestAccountDeletion(ctx, &identityv1.RequestAccountDeletionRequest{Password: pw})
}

// Scenario: Buyer with no obligations schedules deletion
func TestRequestAccountDeletionBuyerWithNoObligationsSchedules(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	ctx := sessCtx("u-1", "sess-A")
	resp, err := request(ctx, h, delPassword)
	if err != nil {
		t.Fatalf("RequestAccountDeletion: %v", err)
	}
	want := delNow.Add(delWindow)
	if d := resp.GetEraseAfter().AsTime().Sub(want); d < -5*time.Second || d > 5*time.Second {
		t.Fatalf("erase_after = %v, want %v within 5s", resp.GetEraseAfter().AsTime(), want)
	}
	st, err := h.GetAccountDeletionStatus(ctx, &identityv1.GetAccountDeletionStatusRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !st.GetPending() || !st.GetEraseAfter().AsTime().Equal(want) || !repo.pending("u-1") {
		t.Fatalf("status = %v, want pending with erase_after %v", st, want)
	}
}

// Scenario: Wrong password schedules nothing
func TestRequestAccountDeletionWrongPasswordSchedulesNothing(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	_, err := request(sessCtx("u-1", "sess-A"), h, "nope-nope")
	wantDelCode(t, err, codes.PermissionDenied)
	if repo.pending("u-1") || ob.calls() != 0 {
		t.Fatalf("pending=%v obligation calls=%d, want nothing scheduled and no obligation RPC", repo.pending("u-1"), ob.calls())
	}
}

// Scenario: Empty password is rejected
func TestRequestAccountDeletionEmptyPasswordRejected(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	_, err := request(sessCtx("u-1", "sess-A"), h, "")
	wantDelCode(t, err, codes.InvalidArgument)
	if repo.pending("u-1") {
		t.Fatal("deletion scheduled for empty password")
	}
}

// Scenario: Anonymous caller is unauthenticated
func TestRequestAccountDeletionAnonymousUnauthenticated(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	_, err := request(context.Background(), h, delPassword)
	wantDelCode(t, err, codes.Unauthenticated)
	if repo.pending("u-1") {
		t.Fatal("row changed")
	}
}

// Scenario: Service principal is denied
func TestRequestAccountDeletionServicePrincipalDenied(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "u-1", Type: auth.TypeService})
	_, err := request(ctx, h, delPassword)
	wantDelCode(t, err, codes.PermissionDenied)
	if repo.pending("u-1") {
		t.Fatal("row changed")
	}
}

// Scenario: Repeated request keeps the first deadline
func TestRequestAccountDeletionRepeatedKeepsFirstDeadline(t *testing.T) {
	repo := newDelRepo(t, "u-1")
	ob := &fakeObligations{}
	clock := delNow
	svc := identity.NewAccountDeletionService(identity.AccountDeletionDeps{
		Users: repo, Obligations: ob, Window: delWindow, Now: func() time.Time { return clock },
	})
	h, _ := newPrivacy(t, discardLogger())
	h.WithAccountDeletion(svc)
	ctx := sessCtx("u-1", "sess-A")
	first, err := request(ctx, h, delPassword)
	if err != nil {
		t.Fatal(err)
	}
	clock = delNow.Add(3 * time.Hour)
	second, err := request(ctx, h, delPassword)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if !second.GetEraseAfter().AsTime().Equal(first.GetEraseAfter().AsTime()) {
		t.Fatalf("erase_after moved from %v to %v", first.GetEraseAfter().AsTime(), second.GetEraseAfter().AsTime())
	}
}

// Scenario: Open order blocks deletion
func TestRequestAccountDeletionOpenOrderBlocks(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.user["u-1"] = upstream.UserObligations{OpenBuyerOrders: 1}
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	st := wantDelCode(t, err, codes.FailedPrecondition)
	if !strings.Contains(strings.ToLower(st.Message()), "open orders") || repo.pending("u-1") {
		t.Fatalf("message %q pending=%v, want category open orders and nothing scheduled", st.Message(), repo.pending("u-1"))
	}
}

// Scenario: Non-zero wallet balance blocks deletion
func TestRequestAccountDeletionWalletBalanceBlocks(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.payout["u-1"] = upstream.PayoutObligations{AvailableBalance: 50000}
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	st := wantDelCode(t, err, codes.FailedPrecondition)
	if !strings.Contains(strings.ToLower(st.Message()), "wallet balance") || regexp.MustCompile(`\d`).MatchString(st.Message()) || repo.pending("u-1") {
		t.Fatalf("message %q pending=%v, want category wallet balance without any amount", st.Message(), repo.pending("u-1"))
	}
}

// Scenario: Pending payout blocks deletion
func TestRequestAccountDeletionPendingPayoutBlocks(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.payout["u-1"] = upstream.PayoutObligations{PendingPayouts: 1}
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	st := wantDelCode(t, err, codes.FailedPrecondition)
	if !strings.Contains(strings.ToLower(st.Message()), "pending payouts") || repo.pending("u-1") {
		t.Fatalf("message %q pending=%v, want category pending payouts", st.Message(), repo.pending("u-1"))
	}
}

// Scenario: Open order blocks deletion (every other non-zero counter blocks too)
func TestRequestAccountDeletionEveryNonZeroCounterBlocks(t *testing.T) {
	cases := map[string]struct {
		u upstream.UserObligations
		p upstream.PayoutObligations
	}{
		"open seller orders": {u: upstream.UserObligations{OpenSellerOrders: 2}},
		"open returns":       {u: upstream.UserObligations{OpenReturns: 1}},
		"held balance":       {p: upstream.PayoutObligations{HeldBalance: 1}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			h, repo, ob := newDeletion(t, "u-1")
			ob.user["u-1"], ob.payout["u-1"] = c.u, c.p
			_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
			wantDelCode(t, err, codes.FailedPrecondition)
			if repo.pending("u-1") {
				t.Fatal("scheduled despite blocker")
			}
		})
	}
}

// Scenario: Obligations are checked for the caller only
func TestRequestAccountDeletionObligationsCheckedForCallerOnly(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-A", "u-B")
	ob.user["u-B"] = upstream.UserObligations{OpenBuyerOrders: 1}
	if _, err := request(sessCtx("u-A", "sess-A"), h, delPassword); err != nil {
		t.Fatalf("user A: %v", err)
	}
	for _, id := range append(append([]string{}, ob.userIDs...), ob.payIDs...) {
		if id != "u-A" {
			t.Fatalf("obligation call carried id %q, want only u-A", id)
		}
	}
	if len(ob.userIDs) != 1 || len(ob.payIDs) != 1 || !repo.pending("u-A") || repo.pending("u-B") {
		t.Fatalf("calls=%v/%v pendingA=%v pendingB=%v", ob.userIDs, ob.payIDs, repo.pending("u-A"), repo.pending("u-B"))
	}
}

// Scenario: Zero obligations on every counter allows deletion
func TestRequestAccountDeletionZeroObligationsAllows(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.user["u-1"] = upstream.UserObligations{}
	ob.payout["u-1"] = upstream.PayoutObligations{}
	resp, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	if err != nil || resp.GetEraseAfter() == nil || !repo.pending("u-1") {
		t.Fatalf("resp=%v err=%v pending=%v", resp, err, repo.pending("u-1"))
	}
}

// Scenario: Payment service unreachable fails closed
func TestRequestAccountDeletionPaymentTimeoutFailsClosed(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.payErr = status.Error(codes.DeadlineExceeded, "deadline exceeded")
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	wantDelCode(t, err, codes.Unavailable)
	if repo.pending("u-1") {
		t.Fatal("scheduled while payment was unreachable")
	}
}

// Scenario: Order service unreachable fails closed
func TestRequestAccountDeletionOrderErrorFailsClosed(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.userErr = errors.New("connection refused")
	ob.payout["u-1"] = upstream.PayoutObligations{}
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	wantDelCode(t, err, codes.Unavailable)
	if repo.pending("u-1") {
		t.Fatal("scheduled while order was unreachable")
	}
}

// Scenario: Other sessions are revoked, current stays
func TestRequestAccountDeletionOtherSessionsRevokedCurrentStays(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	if _, err := request(sessCtx("u-1", "sess-A"), h, delPassword); err != nil {
		t.Fatal(err)
	}
	if !repo.active("u-1", "sess-A") || repo.active("u-1", "sess-B") || repo.active("u-1", "sess-C") {
		t.Fatal("want session A active, B and C revoked")
	}
}

// Scenario: Failed scheduling revokes nothing
func TestRequestAccountDeletionFailedSchedulingRevokesNothing(t *testing.T) {
	h, repo, ob := newDeletion(t, "u-1")
	ob.user["u-1"] = upstream.UserObligations{OpenBuyerOrders: 1}
	_, err := request(sessCtx("u-1", "sess-A"), h, delPassword)
	wantDelCode(t, err, codes.FailedPrecondition)
	for _, s := range []string{"sess-A", "sess-B", "sess-C"} {
		if !repo.active("u-1", s) {
			t.Fatalf("session %s revoked by a failed request", s)
		}
	}
}

// Scenario: Cancel within the window
func TestCancelAccountDeletionWithinWindow(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-1")
	ctx := sessCtx("u-1", "sess-A")
	if _, err := request(ctx, h, delPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := h.CancelAccountDeletion(ctx, &identityv1.CancelAccountDeletionRequest{}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	st, err := h.GetAccountDeletionStatus(ctx, &identityv1.GetAccountDeletionStatusRequest{})
	if err != nil || st.GetPending() || repo.pending("u-1") {
		t.Fatalf("status=%v err=%v, want not pending", st, err)
	}
}

// Scenario: Cancel with nothing pending is rejected
func TestCancelAccountDeletionNothingPendingRejected(t *testing.T) {
	h, _, _ := newDeletion(t, "u-1")
	_, err := h.CancelAccountDeletion(sessCtx("u-1", "sess-A"), &identityv1.CancelAccountDeletionRequest{})
	wantDelCode(t, err, codes.FailedPrecondition)
}

// Scenario: Anonymous cancel is unauthenticated
func TestCancelAccountDeletionAnonymousUnauthenticated(t *testing.T) {
	h, _, _ := newDeletion(t, "u-1")
	_, err := h.CancelAccountDeletion(context.Background(), &identityv1.CancelAccountDeletionRequest{})
	wantDelCode(t, err, codes.Unauthenticated)
}

// Scenario: A user cannot cancel another user's deletion
func TestCancelAccountDeletionCannotCancelAnotherUsers(t *testing.T) {
	h, repo, _ := newDeletion(t, "u-A", "u-B")
	if _, err := request(sessCtx("u-A", "sess-A"), h, delPassword); err != nil {
		t.Fatal(err)
	}
	_, err := h.CancelAccountDeletion(sessCtx("u-B", "sess-A"), &identityv1.CancelAccountDeletionRequest{})
	wantDelCode(t, err, codes.FailedPrecondition)
	got, _ := repo.DeletionEraseAfter(context.Background(), "u-A")
	if !got.Equal(delNow.Add(delWindow)) {
		t.Fatalf("user A erase_after = %v, want unchanged", got)
	}
}

// Scenario: Status with nothing pending
func TestGetAccountDeletionStatusNothingPending(t *testing.T) {
	h, _, _ := newDeletion(t, "u-1")
	st, err := h.GetAccountDeletionStatus(sessCtx("u-1", "sess-A"), &identityv1.GetAccountDeletionStatusRequest{})
	if err != nil || st.GetPending() || st.GetEraseAfter() != nil {
		t.Fatalf("status=%v err=%v, want pending=false and no erase_after", st, err)
	}
}

// Scenario: Status with pending deletion
func TestGetAccountDeletionStatusPending(t *testing.T) {
	h, _, _ := newDeletion(t, "u-1")
	ctx := sessCtx("u-1", "sess-A")
	if _, err := request(ctx, h, delPassword); err != nil {
		t.Fatal(err)
	}
	st, err := h.GetAccountDeletionStatus(ctx, &identityv1.GetAccountDeletionStatusRequest{})
	if err != nil || !st.GetPending() || !st.GetEraseAfter().AsTime().Equal(delNow.Add(delWindow)) {
		t.Fatalf("status=%v err=%v", st, err)
	}
}

// Scenario: Status reflects the caller's own state (another user's pending deletion is invisible)
func TestGetAccountDeletionStatusIsCallerOnly(t *testing.T) {
	h, _, _ := newDeletion(t, "u-A", "u-B")
	if _, err := request(sessCtx("u-A", "sess-A"), h, delPassword); err != nil {
		t.Fatal(err)
	}
	st, err := h.GetAccountDeletionStatus(sessCtx("u-B", "sess-A"), &identityv1.GetAccountDeletionStatusRequest{})
	if err != nil || st.GetPending() {
		t.Fatalf("status=%v err=%v, want user B not pending", st, err)
	}
}

// Scenario: Anonymous status is unauthenticated
func TestGetAccountDeletionStatusAnonymousUnauthenticated(t *testing.T) {
	h, _, _ := newDeletion(t, "u-1")
	_, err := h.GetAccountDeletionStatus(context.Background(), &identityv1.GetAccountDeletionStatusRequest{})
	wantDelCode(t, err, codes.Unauthenticated)
}
