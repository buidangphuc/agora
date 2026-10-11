package handler

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInternalErrHidesUnderlyingError(t *testing.T) {
	st, _ := status.FromError(internalErr("reserve stock", errors.New("pq: password authentication failed")))
	if st.Code() != codes.Internal || strings.Contains(st.Message(), "pq:") {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}
