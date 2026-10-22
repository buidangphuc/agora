package rpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/team-sharing/internal/sharelink"
)

// Scenario: A non-domain error hides its cause.
func TestMapErrHidesInternalCause(t *testing.T) {
	err := mapErr(context.Background(), errors.New("pq: connection refused 10.0.0.5"))
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

// Scenarios: Unknown short code is NotFound; Empty target or empty code is InvalidArgument.
func TestMapErrDomainTable(t *testing.T) {
	cases := map[error]codes.Code{
		sharelink.ErrShareLinkNotFound: codes.NotFound,
		sharelink.ErrEmptyTargetType:   codes.InvalidArgument,
		sharelink.ErrEmptyTargetID:     codes.InvalidArgument,
		sharelink.ErrEmptyShortCode:    codes.InvalidArgument,
	}
	for in, want := range cases {
		if got := status.Code(mapErr(context.Background(), in)); got != want {
			t.Errorf("%v: got %v want %v", in, got, want)
		}
	}
}
