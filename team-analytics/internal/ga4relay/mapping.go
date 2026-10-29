// Package ga4relay relays purchase events to GA4.
package ga4relay

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"google.golang.org/protobuf/proto"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	eventsv1 "github.com/buidangphuc/team-analytics/generated/platform/events/v1"
)

// mappingTTL is how long a buyer -> GA client id mapping lives after the last
// consented event.
const mappingTTL = 30 * 24 * time.Hour

// MappingStore holds the buyer -> GA client id mapping.
type MappingStore interface {
	Get(ctx context.Context, buyerID string) (string, bool, error)
	Put(ctx context.Context, buyerID, clientID string) error
	Delete(ctx context.Context, buyerID string) error
}

// MappingHandler maintains the mapping from tracking records.
type MappingHandler struct {
	store   MappingStore
	mu      sync.Mutex
	skipped map[string]int64
}

// NewMappingHandler returns a handler writing to store.
func NewMappingHandler(store MappingStore) *MappingHandler {
	return &MappingHandler{store: store, skipped: map[string]int64{}}
}

func (h *MappingHandler) skip(reason string) {
	h.mu.Lock()
	h.skipped[reason]++
	h.mu.Unlock()
}

// SkippedCount returns the skipped-event count for reason.
func (h *MappingHandler) SkippedCount(reason string) int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.skipped[reason]
}

const trackingEventType = "platform.analytics.v1.TrackingEvent"

// Handle applies one tracking record to the mapping.
func (h *MappingHandler) Handle(ctx context.Context, rec consume.Record) error {
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(rec.Value, &env); err != nil {
		return consume.Permanent(fmt.Errorf("%w: invalid envelope", ErrDeadLetter))
	}
	if env.GetType() != trackingEventType {
		return nil
	}
	var te analyticsv1.TrackingEvent
	if err := proto.Unmarshal(env.GetPayload(), &te); err != nil {
		return consume.Permanent(fmt.Errorf("%w: invalid tracking event", ErrDeadLetter))
	}
	principalID := env.GetPrincipal().GetId()
	if principalID == "" {
		return nil
	}
	if !te.GetConsent().GetAnalytics() {
		h.skip("no_consent")
		return h.store.Delete(ctx, principalID)
	}
	if te.GetGaClientId() == "" {
		return nil
	}
	return h.store.Put(ctx, principalID, te.GetGaClientId())
}
