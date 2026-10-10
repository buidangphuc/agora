package handler

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapErrsHideInternalCause(t *testing.T) {
	cause := errors.New("pq: connection refused 10.0.0.5")
	for name, err := range map[string]error{
		"alert":    mapAlertErr(context.Background(), cause),
		"prefs":    mapPrefsErr(context.Background(), cause),
		"internal": internalErr(context.Background(), "op", cause),
	} {
		st, _ := status.FromError(err)
		if st.Code() != codes.Internal || st.Message() != "internal error" {
			t.Errorf("%s: got %v %q", name, st.Code(), st.Message())
		}
	}
}
