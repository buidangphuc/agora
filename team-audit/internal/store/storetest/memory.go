package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/buidangphuc/team-audit/internal/audit"
	"github.com/buidangphuc/team-audit/internal/store"
)

type Memory struct {
	mu     sync.Mutex
	events []audit.Event
}

func NewMemory() *Memory { return &Memory{} }

func (m *Memory) Append(_ context.Context, ev audit.Event) (audit.Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ev.ID == "" {
		ev.ID = store.NewEventID()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now()
	}
	m.events = append(m.events, clone(ev))
	return clone(ev), nil
}

func (m *Memory) Query(_ context.Context, f audit.Filter) ([]audit.Event, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matched []audit.Event
	for _, ev := range m.events {
		if f.ActorID != "" && ev.ActorID != f.ActorID {
			continue
		}
		if f.TargetType != "" && ev.TargetType != f.TargetType {
			continue
		}
		if f.TargetID != "" && ev.TargetID != f.TargetID {
			continue
		}
		if f.Action != "" && ev.Action != f.Action {
			continue
		}
		if !f.CreatedAfter.IsZero() && ev.CreatedAt.Before(f.CreatedAfter) {
			continue
		}
		if !f.CreatedBefore.IsZero() && !ev.CreatedAt.Before(f.CreatedBefore) {
			continue
		}
		matched = append(matched, ev)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if !matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].CreatedAt.After(matched[j].CreatedAt)
		}
		return matched[i].ID > matched[j].ID
	})
	total := int64(len(matched))
	limit := f.Limit
	if limit <= 0 {
		limit = audit.DefaultPageSize
	}
	offset := max(f.Offset, 0)
	page := []audit.Event{}
	if offset < len(matched) {
		end := min(offset+limit, len(matched))
		for _, ev := range matched[offset:end] {
			page = append(page, clone(ev))
		}
	}
	return page, total, nil
}

func clone(ev audit.Event) audit.Event {
	meta := make(map[string]string, len(ev.Metadata))
	for k, v := range ev.Metadata {
		meta[k] = v
	}
	ev.Metadata = meta
	return ev
}
