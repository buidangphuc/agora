package rpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-referral/internal/referral"
)

func newTestHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandler(nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestMapErrHidesInternalCause(t *testing.T) {
	h := newTestHandler(t)
	err := h.mapErr(context.Background(), errors.New("pq: connection refused 10.0.0.5"))
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

// Each domain error reaches the client with the code and text it had before
// the error table became ordered rules.
func TestMapErrKeepsClientCodesAndMessages(t *testing.T) {
	h := newTestHandler(t)
	cases := []struct {
		err  error
		code codes.Code
		msg  string
	}{
		{referral.ErrEmptyUser, codes.Unauthenticated, "user id is required"},
		{referral.ErrEmptyCode, codes.InvalidArgument, "code is required"},
		{referral.ErrCodeNotFound, codes.NotFound, "referral code not found"},
		{referral.ErrSelfReferral, codes.FailedPrecondition, "cannot redeem your own referral code"},
		{referral.ErrAlreadyRedeemed, codes.FailedPrecondition, "user has already redeemed a referral code"},
	}
	for _, c := range cases {
		st, _ := status.FromError(h.mapErr(context.Background(), c.err))
		if st.Code() != c.code || st.Message() != c.msg {
			t.Errorf("%v: got %v %q, want %v %q", c.err, st.Code(), st.Message(), c.code, c.msg)
		}
	}
	// A wrapped sentinel still sends only the sentinel's own text.
	st, _ := status.FromError(h.mapErr(context.Background(), fmt.Errorf("db 10.0.0.5: %w", referral.ErrCodeNotFound)))
	if st.Code() != codes.NotFound || st.Message() != "referral code not found" {
		t.Errorf("wrapped: got %v %q", st.Code(), st.Message())
	}
}
