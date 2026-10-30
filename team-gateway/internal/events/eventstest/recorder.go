package eventstest

import (
	"context"
	"sync"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
)

type Call struct {
	Event     *analyticsv1.TrackingEvent
	Principal *commonv1.Principal
	RequestID string
	EventID   string
}

type Recorder struct {
	mu    sync.Mutex
	calls []Call
	Err   error
}

func (r *Recorder) PublishTrackingEvent(_ context.Context, ev *analyticsv1.TrackingEvent, principal *commonv1.Principal, requestID, eventID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, Call{Event: ev, Principal: principal, RequestID: requestID, EventID: eventID})
	return r.Err
}

func (r *Recorder) Close() {}

func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Call(nil), r.calls...)
}
