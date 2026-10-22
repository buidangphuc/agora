package rpc

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/referral"
	"github.com/buidangphuc/team-referral/internal/store/fake"
)

type failingRepo struct {
	*fake.InMemoryReferralRepo
	err error
}

func (r failingRepo) GetCodeByUser(context.Context, string) (string, error) { return "", r.err }
func (r failingRepo) CreateCode(context.Context, string, string) (string, error) {
	return "", r.err
}

// Scenario: A storage failure does not leak its cause.
func TestErrorTableMessagesAreStable(t *testing.T) {
	user := auth.WithPrincipal(context.Background(), auth.Principal{ID: "bob", Type: auth.TypeUser})
	repo := fake.NewInMemoryReferralRepo()
	h, err := NewHandler(referral.NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.RedeemReferral(user, &referralv1.RedeemReferralRequest{Code: "REFmissing"})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound || st.Message() != referral.ErrCodeNotFound.Error() {
		t.Fatalf("not found: %v", err)
	}
	_, err = h.RedeemReferral(user, &referralv1.RedeemReferralRequest{})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument || st.Message() != referral.ErrEmptyCode.Error() {
		t.Fatalf("empty code: %v", err)
	}
}

// Scenario: A storage failure does not leak its cause.
func TestInternalErrorHidesCause(t *testing.T) {
	user := auth.WithPrincipal(context.Background(), auth.Principal{ID: "bob", Type: auth.TypeUser})
	repo := failingRepo{InMemoryReferralRepo: fake.NewInMemoryReferralRepo(), err: errors.New("pq: connection refused 10.0.0.5")}
	h, err := NewHandler(referral.NewService(repo), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.GetMyReferral(user, &referralv1.GetMyReferralRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" || strings.Contains(st.Message(), "10.0.0.5") {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}
