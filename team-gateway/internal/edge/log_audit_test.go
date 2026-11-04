package edge_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-gateway/generated/platform/search/v1"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type failingSearch struct {
	searchv1.SearchServiceClient
	err error
}

func (f *failingSearch) SearchListings(context.Context, *searchv1.SearchListingsRequest, ...grpc.CallOption) (*searchv1.SearchListingsResponse, error) {
	return nil, f.err
}

func newFailingFixture(t *testing.T, upErr error) *fpFixture {
	t.Helper()
	return newFPFixtureWith(t, fpSecret, 1000, 1000, &upstream.Clients{Search: &failingSearch{err: upErr}})
}

func upstreamErrorLine(t *testing.T, f *fpFixture) string {
	t.Helper()
	for _, l := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
		if strings.Contains(l, `"msg":"edge.upstream_error"`) {
			return l
		}
	}
	t.Fatalf("no edge.upstream_error line:\n%s", f.logs.String())
	return ""
}

// Scenario: GW-FP-7 no raw id on error paths (UNAVAILABLE with the id in the upstream text)
func TestGWFP7_UnavailableUpstreamErrorLogsNoRawID(t *testing.T) {
	f := newFailingFixture(t, status.Error(codes.Unavailable, "dial tcp: lookup shard for user-1: connection refused"))
	err := f.grpc(t, f.mint(t, "user", "user-1"))
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("code = %v, want unavailable", connect.CodeOf(err))
	}
	line := upstreamErrorLine(t, f)
	if !strings.Contains(line, "connection refused") {
		t.Errorf("upstream_error lost the diagnostic text: %s", line)
	}
	assertKind(t, f.lastEdgeLine(t, searchRoute), "user", wantFP(t, fpSecret, "user-1"))
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-7 no raw id on error paths (non-status error becomes INTERNAL)
func TestGWFP7_InternalUpstreamErrorLogsNoRawID(t *testing.T) {
	f := newFailingFixture(t, errors.New(`pq: duplicate key for owner "user-1"`))
	err := f.grpc(t, f.mint(t, "user", "user-1"))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("code = %v, want internal", connect.CodeOf(err))
	}
	if line := upstreamErrorLine(t, f); !strings.Contains(line, "duplicate key") {
		t.Errorf("upstream_error lost the diagnostic text: %s", line)
	}
	assertNoRaw(t, f, "user-1")
}

// Scenario: GW-FP-7 no raw id on error paths (service principal id in the upstream text)
func TestGWFP7_ServicePrincipalIDScrubbedFromUpstreamError(t *testing.T) {
	f := newFailingFixture(t, status.Error(codes.Unavailable, "svc-order rejected by shard"))
	_ = f.grpc(t, f.mint(t, "service", "svc-order"))
	upstreamErrorLine(t, f)
	assertNoRaw(t, f, "svc-order")
}

// Scenario: GW-FP-7 no raw id on error paths (unrelated upstream text is kept verbatim)
func TestGWFP7_UpstreamErrorWithoutIDIsKeptVerbatim(t *testing.T) {
	f := newFailingFixture(t, status.Error(codes.Unavailable, "dial tcp 10.0.0.7:9000: connection refused"))
	_ = f.grpc(t, f.mint(t, "user", "user-1"))
	if line := upstreamErrorLine(t, f); !strings.Contains(line, "dial tcp 10.0.0.7:9000: connection refused") {
		t.Errorf("upstream text altered: %s", line)
	}
}

// Scenario: GW-FP-11 secret is never logged (success, upstream error, 401 and rate-limit paths)
func TestGWFP11_SecretNeverAppearsInGatewayLogs(t *testing.T) {
	f := newFPFixtureWith(t, fpSecret, 0.0001, 1, &upstream.Clients{Search: &failingSearch{err: status.Error(codes.Unavailable, "down")}})
	tok := f.mint(t, "user", "user-1")
	_ = f.grpc(t, tok)
	_ = f.grpc(t, tok)
	_ = f.grpc(t, "not.a.token")
	f.http(t, tok)
	f.http(t, "")
	if !strings.Contains(f.logs.String(), "edge.request") {
		t.Fatalf("no request lines captured:\n%s", f.logs.String())
	}
	assertNoRaw(t, f, fpSecret)
}
