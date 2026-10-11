package edge_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

type queryAudit struct {
	auditv1.AuditServiceClient
	calls int
}

func (q *queryAudit) QueryAuditLog(context.Context, *auditv1.QueryAuditLogRequest, ...grpc.CallOption) (*auditv1.QueryAuditLogResponse, error) {
	q.calls++
	return &auditv1.QueryAuditLogResponse{}, nil
}

// QueryAuditLog is admin-only at the edge: an anonymous caller is refused (401)
// by the interceptor before any upstream call.
func TestQueryAuditLogAnonymousRejectedAtEdge(t *testing.T) {
	up := &queryAudit{}
	e := edge.NewEdge(nil, []string{"listing.read"}, time.Second, 0, 1000, 1000)
	srv := httptest.NewServer(edge.NewMux(&upstream.Clients{Audit: up}, e, nil, edge.CockpitConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)

	res, err := srv.Client().Post(srv.URL+"/platform.audit.v1.AuditService/QueryAuditLog", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("anonymous QueryAuditLog = %d %s, want 401", res.StatusCode, b)
	}
	if up.calls != 0 {
		t.Fatalf("upstream called %d times, want 0", up.calls)
	}
}
