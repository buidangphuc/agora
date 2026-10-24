package sponsored

import (
	"context"
	"errors"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
)

type callLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *callLog) add(c string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *callLog) get() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.calls...)
}

type recordingAuditWriter struct {
	log    *callLog
	err    error
	mu     sync.Mutex
	events []auditx.Event
}

func (w *recordingAuditWriter) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	if w.log != nil {
		w.log.add("audit")
	}
	w.mu.Lock()
	w.events = append(w.events, ev)
	w.mu.Unlock()
	return w.err
}

func (w *recordingAuditWriter) got() []auditx.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]auditx.Event(nil), w.events...)
}

var auditFailures = map[string]error{
	"upstream error": status.Error(codes.Internal, "audit db down"),
	"timeout":        context.DeadlineExceeded,
	"unavailable":    status.Error(codes.Unavailable, "team-audit down"),
	"plain error":    errors.New("dial tcp: refused"),
}

func assertAuditUnavailable(t *testing.T, err error) {
	t.Helper()
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != auditx.Message {
		t.Fatalf("err = %v, want Unavailable %q", err, auditx.Message)
	}
}

type fakeListings struct {
	owners map[string]string
	err    error
	calls  int
}

func (f *fakeListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	seller, ok := f.owners[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}
