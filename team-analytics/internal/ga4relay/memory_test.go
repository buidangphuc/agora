package ga4relay

import (
	"context"
	"sync"
	"time"
)

type memoryLedger struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[string]time.Time
}

func NewMemoryLedger(now func() time.Time) Ledger {
	return &memoryLedger{now: now, m: map[string]time.Time{}}
}

func (l *memoryLedger) Sent(_ context.Context, orderID string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.m[orderID]
	if !ok {
		return false, nil
	}
	if !l.now().Before(exp) {
		delete(l.m, orderID)
		return false, nil
	}
	return true, nil
}

func (l *memoryLedger) MarkSent(_ context.Context, orderID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.m[orderID] = l.now().Add(ledgerTTL)
	return nil
}

func (l *memoryLedger) Purge(_ context.Context) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var n int64
	now := l.now()
	for k, exp := range l.m {
		if !now.Before(exp) {
			delete(l.m, k)
			n++
		}
	}
	return n, nil
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

func (s *memoryMappingStore) Purge(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	now := s.now()
	for k, e := range s.m {
		if !now.Before(e.expires) {
			delete(s.m, k)
			n++
		}
	}
	return n, nil
}
