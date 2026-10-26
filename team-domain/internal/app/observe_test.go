package app_test

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	aiv1 "github.com/buidangphuc/team-domain/generated/platform/ai/v1"
	auditv1 "github.com/buidangphuc/team-domain/generated/platform/audit/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/store"
)

func serveFake(t *testing.T, register func(*grpc.Server)) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

type fakeCollector struct {
	coltracepb.UnimplementedTraceServiceServer
	mu       sync.Mutex
	services []string
}

func (c *fakeCollector) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rs := range req.GetResourceSpans() {
		for _, kv := range rs.GetResource().GetAttributes() {
			if kv.GetKey() == "service.name" {
				c.services = append(c.services, kv.GetValue().GetStringValue())
			}
		}
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (c *fakeCollector) seen() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.services...)
}

func startCollector(t *testing.T) (*fakeCollector, string) {
	t.Helper()
	c := &fakeCollector{}
	addr := serveFake(t, func(s *grpc.Server) { coltracepb.RegisterTraceServiceServer(s, c) })
	return c, "http://" + addr
}

func TestOtelServiceNameDefaultsToTeamDomain(t *testing.T) {
	// scenario: Domain SDK: The telemetry service name is team-domain
	for _, tc := range []struct{ name, configured, want string }{
		{"unset", "", "team-domain"},
		{"custom", "custom", "custom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector, endpoint := startCollector(t)
			url := migratedURL(t)
			s := domainSettings(testkit.FreePort(t), url, deadBroker)
			s.OTel = sdkconfig.OTel{Enabled: true, Endpoint: endpoint, ServiceName: tc.configured}
			r := start(t, s)
			waitServing(t, r)
			r.cancel()
			select {
			case <-r.done:
			case <-time.After(30 * time.Second):
				t.Fatal("app did not stop")
			}
			if r.err != nil {
				t.Fatalf("Run = %v, want nil on shutdown", r.err)
			}
			got := collector.seen()
			if len(got) == 0 {
				t.Fatal("no spans were exported to the collector on shutdown")
			}
			for _, name := range got {
				if name != tc.want {
					t.Fatalf("exported service.name = %q, want %q (all: %v)", name, tc.want, got)
				}
			}
		})
	}
}

type fakeAudit struct {
	auditv1.UnimplementedAuditServiceServer
	mu   sync.Mutex
	mds  []metadata.MD
	reqs []*auditv1.WriteAuditEventRequest
}

func (f *fakeAudit) WriteAuditEvent(ctx context.Context, req *auditv1.WriteAuditEventRequest) (*auditv1.WriteAuditEventResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mds = append(f.mds, md)
	f.reqs = append(f.reqs, req)
	return &auditv1.WriteAuditEventResponse{}, nil
}

func (f *fakeAudit) calls() []metadata.MD {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]metadata.MD(nil), f.mds...)
}

type fakeAI struct {
	aiv1.UnimplementedAIServiceServer
}

func (fakeAI) ScreenListing(context.Context, *aiv1.ScreenListingRequest) (*aiv1.ScreenListingResponse, error) {
	return &aiv1.ScreenListingResponse{Verdict: aiv1.ScreenVerdict_SCREEN_VERDICT_ALLOW, Backend: "fake"}, nil
}

func traceID(t *testing.T, traceparent string) string {
	t.Helper()
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 || len(parts[1]) != 32 {
		t.Fatalf("malformed traceparent %q", traceparent)
	}
	return parts[1]
}

func headerValue(h []kgo.RecordHeader, key string) string {
	for _, x := range h {
		if x.Key == key {
			return string(x.Value)
		}
	}
	return ""
}

func TestRequestIDAndTraceparentContinuity(t *testing.T) {
	// scenario: Listing audit: A request id reaches the audit call and the produced record
	const inboundTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	const inboundParent = "00-" + inboundTrace + "-00f067aa0ba902b7-01"

	audit := &fakeAudit{}
	auditAddr := serveFake(t, func(s *grpc.Server) { auditv1.RegisterAuditServiceServer(s, audit) })
	aiAddr := serveFake(t, func(s *grpc.Server) { aiv1.RegisterAIServiceServer(s, fakeAI{}) })
	_, collectorURL := startCollector(t)

	url := migratedURL(t)
	pool := openPool(t, url)
	seedListing(t, pool, store.Listing{ID: "L-trace", Title: "orig", Currency: "VND", Status: "published", SellerID: "seller-a", Stock: 1})
	brokers := testkit.Redpanda(t)
	s := domainSettings(testkit.FreePort(t), url, brokers)
	s.Upstream.AuditAddr, s.Upstream.AIAddr = auditAddr, aiAddr
	s.OTel = sdkconfig.OTel{Enabled: true, Endpoint: collectorURL}
	r := start(t, s)
	waitServing(t, r)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ctx = principal(ctx, "admin-1", "user", "listing.write", "admin")
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "req-42", "traceparent", inboundParent)
	_, err := listingv1.NewListingServiceClient(r.conn).UpdateListing(ctx, &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-trace", Title: "new", Currency: "VND", Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, Stock: 1,
	}}, grpc.WaitForReady(true))
	if err != nil {
		t.Fatalf("admin UpdateListing: %v", err)
	}

	calls := audit.calls()
	if len(calls) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(calls))
	}
	if got := calls[0].Get("x-request-id"); len(got) != 1 || got[0] != "req-42" {
		t.Errorf("audit call x-request-id = %v, want [req-42]", got)
	}
	if got := calls[0].Get("traceparent"); len(got) != 1 || traceID(t, got[0]) != inboundTrace {
		t.Errorf("audit call traceparent = %v, want trace id %s", got, inboundTrace)
	}

	recs := testkit.ReadAll(t, brokers, listingTopic, 1, 60*time.Second)
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(recs[0].Value, &env); err != nil || env.GetType() != "platform.listing.v1.ListingChanged" {
		t.Fatalf("relayed record = %v (type %q), want ListingChanged", err, env.GetType())
	}
	hs := recs[0].Headers
	if got := headerValue(hs, "x-request-id"); got != "req-42" {
		t.Errorf("relayed record x-request-id = %q, want req-42", got)
	}
	if got := headerValue(hs, "traceparent"); got == "" || traceID(t, got) != inboundTrace {
		t.Errorf("relayed record traceparent = %q, want trace id %s", got, inboundTrace)
	}
}
