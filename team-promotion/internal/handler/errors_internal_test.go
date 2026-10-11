package handler

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Unexpected errors must reach the client as a generic INTERNAL, never the raw cause.
func TestInternalErrorIsGeneric(t *testing.T) {
	st, _ := status.FromError(internalError(context.Background(), slog.Default(), "op", errors.New("pq: connection refused to 10.0.0.5")))
	if st.Code() != codes.Internal {
		t.Fatalf("code = %v, want Internal", st.Code())
	}
	if st.Message() != "internal error" || strings.Contains(st.Message(), "pq") {
		t.Fatalf("message leaks internal detail: %q", st.Message())
	}
}
