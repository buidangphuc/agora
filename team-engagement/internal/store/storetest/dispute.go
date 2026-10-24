package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/team-engagement/internal/store"
)

type FakeDisputeRepository struct {
	mu   sync.RWMutex
	data map[string]store.Dispute
}

func NewFakeDisputeRepository() *FakeDisputeRepository {
	return &FakeDisputeRepository{
		data: make(map[string]store.Dispute),
	}
}

func disputeIsOpen(s store.DisputeStatus) bool {
	return s == store.DisputeStatusOpen || s == store.DisputeStatusInvestigating
}

func (r *FakeDisputeRepository) CreateDispute(_ context.Context, d store.Dispute) (store.Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	if d.UpdatedAt.IsZero() {
		d.UpdatedAt = now
	}
	if d.Status == "" {
		d.Status = store.DisputeStatusOpen
	}
	if d.EvidenceURLs == nil {
		d.EvidenceURLs = make([]string, 0)
	}
	if d.ReturnID != "" && disputeIsOpen(d.Status) {
		for _, e := range r.data {
			if e.ReturnID == d.ReturnID && disputeIsOpen(e.Status) {
				return store.Dispute{}, store.ErrOpenDisputeExists
			}
		}
	}

	r.data[d.ID] = d
	return d, nil
}

func (r *FakeDisputeRepository) GetDispute(_ context.Context, id string) (store.Dispute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.data[id]
	if !ok {
		return store.Dispute{}, store.ErrDisputeNotFound
	}
	return d, nil
}

func (r *FakeDisputeRepository) UpdateDispute(_ context.Context, d store.Dispute) (store.Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[d.ID]
	if !ok {
		return store.Dispute{}, store.ErrDisputeNotFound
	}

	existing.Status = d.Status
	existing.Resolution = d.Resolution
	existing.Ruling = d.Ruling
	existing.UpdatedAt = time.Now().UTC()
	r.data[d.ID] = existing
	return existing, nil
}

func (r *FakeDisputeRepository) ResolveWithEvent(_ context.Context, d store.Dispute, prev store.DisputeStatus, _ proto.Message) (store.Dispute, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.data[d.ID]
	if !ok {
		return store.Dispute{}, store.ErrDisputeNotFound
	}
	if existing.Status != prev {
		return store.Dispute{}, store.ErrDisputeNotInState
	}
	existing.Status = d.Status
	existing.Resolution = d.Resolution
	existing.Ruling = d.Ruling
	existing.UpdatedAt = time.Now().UTC()
	r.data[d.ID] = existing
	return existing, nil
}

func (r *FakeDisputeRepository) ListByOrder(_ context.Context, orderID string) ([]store.Dispute, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]store.Dispute, 0)
	for _, d := range r.data {
		if d.OrderID == orderID {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func (r *FakeDisputeRepository) ListByStatus(_ context.Context, status store.DisputeStatus, pageSize int, cursor string) ([]store.Dispute, string, error) {
	size := pagination.Clamp(pageSize, store.DefaultDisputePageSize, store.MaxDisputePageSize)
	var curAt time.Time
	curID := ""
	if cursor != "" {
		at, id, err := store.DecodeDisputeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		curAt, curID = at, id
	}

	r.mu.RLock()
	all := make([]store.Dispute, 0, len(r.data))
	for _, d := range r.data {
		if status == "" || d.Status == status {
			all = append(all, d)
		}
	}
	r.mu.RUnlock()
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})

	out := make([]store.Dispute, 0, size+1)
	for _, d := range all {
		if cursor != "" {
			at := d.CreatedAt.Truncate(time.Microsecond)
			if at.Before(curAt) || (at.Equal(curAt) && d.ID <= curID) {
				continue
			}
		}
		out = append(out, d)
		if len(out) > size {
			break
		}
	}
	next := ""
	if len(out) > size {
		out = out[:size]
		next = store.EncodeDisputeCursor(out[size-1])
	}
	return out, next, nil
}

var _ store.DisputeRepository = (*FakeDisputeRepository)(nil)
