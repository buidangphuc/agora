// Package ga4relay relays purchase events to GA4.
package ga4relay

import (
	"context"
	"sync"
	"time"

	"github.com/buidangphuc/team-analytics/internal/warehouse"
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

type memEntry struct {
	clientID string
	expires  time.Time
}

type memoryMappingStore struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[string]memEntry
}

// NewMemoryMappingStore returns an in-memory MappingStore with 30d expiry.
func NewMemoryMappingStore(now func() time.Time) MappingStore {
	return &memoryMappingStore{now: now, m: map[string]memEntry{}}
}

func (s *memoryMappingStore) Get(_ context.Context, buyerID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[buyerID]
	if !ok {
		return "", false, nil
	}
	if !s.now().Before(e.expires) {
		delete(s.m, buyerID)
		return "", false, nil
	}
	return e.clientID, true, nil
}

func (s *memoryMappingStore) Put(_ context.Context, buyerID, clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[buyerID] = memEntry{clientID: clientID, expires: s.now().Add(mappingTTL)}
	return nil
}

func (s *memoryMappingStore) Delete(_ context.Context, buyerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, buyerID)
	return nil
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

// Handle applies one tracking record to the mapping.
func (h *MappingHandler) Handle(ctx context.Context, rec warehouse.TrackingRecord) error {
	if rec.PrincipalID == "" {
		return nil
	}
	if !rec.ConsentAnalytics {
		h.skip("no_consent")
		return h.store.Delete(ctx, rec.PrincipalID)
	}
	if rec.GaClientID == "" {
		return nil
	}
	return h.store.Put(ctx, rec.PrincipalID, rec.GaClientID)
}
