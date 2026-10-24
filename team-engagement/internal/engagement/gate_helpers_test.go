package engagement_test

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
)

func requireAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want Unavailable audit trail unavailable, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unavailable || st.Message() != auditx.Message {
		t.Fatalf("err = %v, want status Unavailable %q", err, auditx.Message)
	}
}
