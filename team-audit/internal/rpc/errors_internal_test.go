package rpc

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInternalErrorIsGenericAndNotLeaked(t *testing.T) {
	h, err := NewHandler(nil, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	err = h.mapErr(context.Background(), errors.New("pq: password authentication failed for user audit"))
	if status.Code(err) != codes.Internal {
		t.Fatalf("got %v, want Internal", status.Code(err))
	}
	if msg := status.Convert(err).Message(); msg != "internal error" {
		t.Fatalf("message %q leaks the cause", msg)
	}
}
