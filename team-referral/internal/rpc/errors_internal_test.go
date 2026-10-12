package rpc

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapErrHidesInternalCause(t *testing.T) {
	h := NewHandler(nil, slog.New(slog.DiscardHandler))
	err := h.mapErr(context.Background(), errors.New("pq: connection refused 10.0.0.5"))
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}
