package edge_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	auditv1 "github.com/buidangphuc/team-gateway/generated/platform/audit/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
)

// recordingAuditClient records the request that reaches the upstream.
type recordingAuditClient struct {
	auditv1.AuditServiceClient
	gotReq *auditv1.QueryAuditLogRequest
	calls  int
}

func (f *recordingAuditClient) QueryAuditLog(
	_ context.Context, in *auditv1.QueryAuditLogRequest, _ ...grpc.CallOption,
) (*auditv1.QueryAuditLogResponse, error) {
	f.calls++
	f.gotReq = in
	return &auditv1.QueryAuditLogResponse{}, nil
}

// Scenario: Gateway forwards the new fields unchanged.
func TestQueryAuditLogForwardsFilters(t *testing.T) {
	up := &recordingAuditClient{}
	e := edge.NewEdge(nil, nil, time.Second, 0, 1000, 1000)
	f := edge.NewAuditForwarder(up, e)

	after := timestamppb.New(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))
	before := timestamppb.New(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC))
	req := connect.NewRequest(&auditv1.QueryAuditLogRequest{
		ActorId:       "adm-1",
		TargetType:    "order",
		TargetId:      proto.String("o-1"),
		Action:        proto.String("x' OR '1'='1"),
		CreatedAfter:  after,
		CreatedBefore: before,
	})
	if _, err := f.QueryAuditLog(context.Background(), req); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if up.calls != 1 {
		t.Fatalf("want exactly one upstream call, got %d", up.calls)
	}
	g := up.gotReq
	if g.GetActorId() != "adm-1" || g.GetTargetType() != "order" {
		t.Errorf("existing filters altered: %v", g)
	}
	if g.GetTargetId() != "o-1" {
		t.Errorf("target_id = %q, want o-1", g.GetTargetId())
	}
	if g.GetAction() != "x' OR '1'='1" {
		t.Errorf("action = %q, want unchanged", g.GetAction())
	}
	if !g.GetCreatedAfter().AsTime().Equal(after.AsTime()) {
		t.Errorf("created_after = %v, want %v", g.GetCreatedAfter().AsTime(), after.AsTime())
	}
	if !g.GetCreatedBefore().AsTime().Equal(before.AsTime()) {
		t.Errorf("created_before = %v, want %v", g.GetCreatedBefore().AsTime(), before.AsTime())
	}
}
