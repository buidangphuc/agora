package edge

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"

	verificationv1 "github.com/buidangphuc/team-gateway/generated/platform/verification/v1"
)

type fakeVerificationClient struct {
	verificationv1.VerificationServiceClient
	reviews int
}

func (f *fakeVerificationClient) ReviewKyc(context.Context, *verificationv1.ReviewKycRequest, ...grpc.CallOption) (*verificationv1.ReviewKycResponse, error) {
	f.reviews++
	return &verificationv1.ReviewKycResponse{Status: verificationv1.VerificationStatus_VERIFICATION_STATUS_VERIFIED}, nil
}

// ReviewKyc is admin-only at the edge: anonymous and non-admin callers are
// rejected before any upstream call; an admin is forwarded.
func TestReviewKycRequiresAdminAtTheEdge(t *testing.T) {
	cases := []struct {
		name     string
		p        *resolvedPrincipal
		wantCode connect.Code
		forward  bool
	}{
		{"no principal", nil, connect.CodeUnauthenticated, false},
		{"anonymous", &resolvedPrincipal{id: "anonymous", ptype: "anonymous", scopes: []string{"listing.read"}}, connect.CodeUnauthenticated, false},
		{"buyer", &resolvedPrincipal{id: "u-1", ptype: "user", scopes: []string{"listing.read", "engagement:write"}}, connect.CodePermissionDenied, false},
		{"admin", &resolvedPrincipal{id: "u-2", ptype: "user", scopes: []string{"listing.read", "admin"}}, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakeVerificationClient{}
			f := NewVerificationForwarder(client, NewEdge(nil, []string{"listing.read"}, time.Second, 0, 1000, 1000))
			ctx := context.Background()
			if tc.p != nil {
				ctx = withPrincipal(ctx, *tc.p)
			}
			_, err := f.ReviewKyc(ctx, connect.NewRequest(&verificationv1.ReviewKycRequest{Id: "kyc-1", Decision: "approve"}))
			if tc.forward {
				if err != nil {
					t.Fatalf("admin: unexpected error %v", err)
				}
			} else if connect.CodeOf(err) != tc.wantCode {
				t.Fatalf("want %v, got %v", tc.wantCode, err)
			}
			if got := client.reviews > 0; got != tc.forward {
				t.Fatalf("upstream called = %v, want %v", got, tc.forward)
			}
		})
	}
}
