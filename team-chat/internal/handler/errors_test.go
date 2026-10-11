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

func TestInternalErrIsGenericAndLogsCause(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	err := internalErr(context.Background(), logger, "send message", errors.New("pq: password authentication failed"))
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
	if strings.Contains(st.Message(), "pq:") {
		t.Fatal("cause leaked to caller")
	}
	if !strings.Contains(buf.String(), "password authentication failed") {
		t.Fatalf("cause not logged: %s", buf.String())
	}
}
