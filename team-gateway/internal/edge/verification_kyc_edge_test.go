package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"

	verificationv1 "github.com/buidangphuc/team-gateway/generated/platform/verification/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type kycUpstream struct {
	verificationv1.VerificationServiceClient
	badges, upload, list, docURL atomic.Int32
}

func (k *kycUpstream) GetPublicBadges(_ context.Context, in *verificationv1.GetPublicBadgesRequest, _ ...grpc.CallOption) (*verificationv1.GetPublicBadgesResponse, error) {
	k.badges.Add(1)
	out := &verificationv1.GetPublicBadgesResponse{}
	for _, id := range in.GetUserIds() {
		out.Badges = append(out.Badges, &verificationv1.PublicBadge{UserId: id, Verified: true})
	}
	return out, nil
}

func (k *kycUpstream) CreateKycUploadUrl(context.Context, *verificationv1.CreateKycUploadUrlRequest, ...grpc.CallOption) (*verificationv1.CreateKycUploadUrlResponse, error) { //nolint:revive // bound to generated interface
	k.upload.Add(1)
	return &verificationv1.CreateKycUploadUrlResponse{UploadUrl: "http://s/put", DocRef: "kyc/u/1"}, nil
}

func (k *kycUpstream) ListKycSubmissions(context.Context, *verificationv1.ListKycSubmissionsRequest, ...grpc.CallOption) (*verificationv1.ListKycSubmissionsResponse, error) {
	k.list.Add(1)
	return &verificationv1.ListKycSubmissionsResponse{}, nil
}

func (k *kycUpstream) GetKycDocumentUrl(context.Context, *verificationv1.GetKycDocumentUrlRequest, ...grpc.CallOption) (*verificationv1.GetKycDocumentUrlResponse, error) { //nolint:revive // bound to generated interface
	k.docURL.Add(1)
	return &verificationv1.GetKycDocumentUrlResponse{Url: "http://s/get"}, nil
}

const verificationSvc = "/platform.verification.v1.VerificationService/"

func callKyc(t *testing.T, up *kycUpstream, rpc, body, auth string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Verification: up}, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+verificationSvc+rpc, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", "Bearer "+f.token(t, strings.Fields(auth)...))
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Scenario "Anonymous public badge call succeeds through the gateway".
func TestScenarioAnonymousPublicBadgeCallSucceedsThroughTheGateway(t *testing.T) {
	up := &kycUpstream{}
	code, body := callKyc(t, up, "GetPublicBadges", `{"user_ids":["u-1"]}`, "")
	if code != http.StatusOK {
		t.Fatalf("anonymous GetPublicBadges = %d %s, want 200", code, body)
	}
	if !strings.Contains(body, "u-1") || up.badges.Load() != 1 {
		t.Fatalf("body %s, upstream calls %d, want one badge entry from one call", body, up.badges.Load())
	}
}

// Scenario "Admin-only RPCs refuse non-admins at the edge".
func TestScenarioAdminOnlyRPCsRefuseNonAdminsAtTheEdge(t *testing.T) {
	for _, rpc := range []string{"ListKycSubmissions", "GetKycDocumentUrl"} {
		for _, c := range []struct {
			name, auth string
			want       int
		}{
			{"seller token", "listing:write", http.StatusForbidden},
			{"anonymous", "", http.StatusUnauthorized},
		} {
			t.Run(rpc+"/"+c.name, func(t *testing.T) {
				up := &kycUpstream{}
				code, body := callKyc(t, up, rpc, `{"submission_id":"kyc-1"}`, c.auth)
				if code != c.want {
					t.Fatalf("%s %s = %d %s, want %d", c.name, rpc, code, body, c.want)
				}
				if n := up.list.Load() + up.docURL.Load(); n != 0 {
					t.Fatalf("team-verification received %d calls, want 0", n)
				}
			})
		}
	}
}

func TestAdminReachesKycAdminRPCsThroughTheGateway(t *testing.T) {
	up := &kycUpstream{}
	if code, body := callKyc(t, up, "ListKycSubmissions", `{}`, "admin"); code != http.StatusOK {
		t.Fatalf("admin ListKycSubmissions = %d %s, want 200", code, body)
	}
	if code, body := callKyc(t, up, "GetKycDocumentUrl", `{"submission_id":"kyc-1"}`, "admin"); code != http.StatusOK {
		t.Fatalf("admin GetKycDocumentUrl = %d %s, want 200", code, body)
	}
	if up.list.Load() != 1 || up.docURL.Load() != 1 {
		t.Fatalf("upstream calls list=%d doc=%d, want 1 each", up.list.Load(), up.docURL.Load())
	}
}

// Scenario "Unauthenticated upload URL request is refused at the edge".
func TestScenarioUnauthenticatedUploadURLRequestIsRefusedAtTheEdge(t *testing.T) {
	up := &kycUpstream{}
	code, body := callKyc(t, up, "CreateKycUploadUrl", `{"content_type":"image/jpeg","size_bytes":100}`, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("anonymous CreateKycUploadUrl = %d %s, want 401", code, body)
	}
	if up.upload.Load() != 0 {
		t.Fatalf("team-verification received %d calls, want 0", up.upload.Load())
	}
}

func TestAuthenticatedUserCreateKycUploadUrlIsForwarded(t *testing.T) {
	up := &kycUpstream{}
	code, body := callKyc(t, up, "CreateKycUploadUrl", `{"content_type":"image/jpeg","size_bytes":100}`, "listing:write")
	if code != http.StatusOK || !strings.Contains(body, "kyc/u/1") || up.upload.Load() != 1 {
		t.Fatalf("user CreateKycUploadUrl = %d %s (calls %d), want 200 forwarded", code, body, up.upload.Load())
	}
}
