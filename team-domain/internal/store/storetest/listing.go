package storetest

import (
	"context"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/pagination"
	"github.com/buidangphuc/team-domain/internal/store"
	"sort"
	"sync"
	"time"
)

type InMemoryListingRepository struct {
	mu           sync.RWMutex
	byID         map[string]store.Listing
	reservations map[string]memReservation
	stockEvent   store.StockEventBuilder
	stockRows    []store.OutboxRow
}
type memReservation struct {
	listingID string
	variantID string
	quantity  int32
	expiresAt time.Time
	status    string
}

func NewInMemoryListingRepository(seed ...store.Listing) *InMemoryListingRepository {
	if len(seed) == 0 {
		seed = []store.Listing{{
			ID:          "seed-1",
			Title:       "Seed listing",
			Description: "In-memory row proving the read path end-to-end.",
			Price:       1_500_000_000,
			Currency:    "VND",
			Status:      "published",
			Stock:       100,
		}}
	}
	byID := make(map[string]store.Listing, len(seed))
	for _, l := range seed {
		byID[l.ID] = l
	}
	return &InMemoryListingRepository{byID: byID, reservations: map[string]memReservation{}}
}
func (r *InMemoryListingRepository) Get(_ context.Context, id string) (store.Listing, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.byID[id]
	if !ok {
		return store.Listing{}, store.ErrNotFound
	}
	return l, nil
}
func (r *InMemoryListingRepository) List(_ context.Context, cursor string, pageSize int32, status string) (store.Page, error) {
	return r.page(cursor, pageSize, func(l store.Listing) bool {
		return status == "" || l.Status == status
	}), nil
}
func (r *InMemoryListingRepository) ListByOwner(_ context.Context, ownerID, cursor string, pageSize int32) (store.Page, error) {
	return r.page(cursor, pageSize, func(l store.Listing) bool {
		return l.SellerID == ownerID
	}), nil
}
func (r *InMemoryListingRepository) page(cursor string, pageSize int32, keep func(store.Listing) bool) store.Page {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.byID))
	var total int64
	for id, l := range r.byID {
		if keep(l) {
			ids = append(ids, id)
			total++
		}
	}
	sort.Strings(ids)

	limit := pagination.Clamp(int(pageSize), store.DefaultPageSize, store.MaxPageSize)
	items := make([]store.Listing, 0, limit)
	var next string
	for _, id := range ids {
		if cursor != "" && id <= cursor {
			continue
		}
		if len(items) == limit {
			next = items[len(items)-1].ID
			break
		}
		items = append(items, r.byID[id])
	}
	return store.Page{Items: items, NextCursor: next, Total: total}
}
func (r *InMemoryListingRepository) Create(_ context.Context, l store.Listing) (store.Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[l.ID] = l
	return l, nil
}
func (r *InMemoryListingRepository) Update(_ context.Context, l store.Listing) (store.Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[l.ID]; !ok {
		return store.Listing{}, store.ErrNotFound
	}
	r.byID[l.ID] = l
	return l, nil
}
func (r *InMemoryListingRepository) Delete(_ context.Context, id string) (store.Listing, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.byID[id]
	if !ok {
		return store.Listing{}, store.ErrNotFound
	}
	delete(r.byID, id)
	return l, nil
}
func (r *InMemoryListingRepository) reserveLocked(listingID, variantID string, quantity int32) error {
	l, ok := r.byID[listingID]
	if !ok {
		return store.ErrNotFound
	}
	if variantID == "" {
		if l.Stock < quantity {
			return store.ErrOutOfStock
		}
		l.Stock -= quantity
		r.byID[listingID] = l
		return nil
	}
	for i := range l.Variants {
		if l.Variants[i].ID == variantID {
			if l.Variants[i].Stock < quantity {
				return store.ErrOutOfStock
			}
			l.Variants[i].Stock -= quantity
			r.byID[listingID] = l
			return nil
		}
	}
	return store.ErrVariantNotFound
}
func (r *InMemoryListingRepository) releaseLocked(listingID, variantID string, quantity int32) error {
	l, ok := r.byID[listingID]
	if !ok {
		return store.ErrNotFound
	}
	if variantID == "" {
		l.Stock += quantity
		r.byID[listingID] = l
		return nil
	}
	for i := range l.Variants {
		if l.Variants[i].ID == variantID {
			l.Variants[i].Stock += quantity
			r.byID[listingID] = l
			return nil
		}
	}
	return store.ErrVariantNotFound
}
func (r *InMemoryListingRepository) ReserveStockIdempotent(ctx context.Context, reservationID, listingID, variantID string, quantity int32, expiresAt time.Time) error {
	if reservationID == "" {
		return store.ErrReservationIDRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if res, ok := r.reservations[reservationID]; ok {
		if res.status == store.ReservationReleased {
			return store.ErrReservationReleased
		}
		if res.listingID != listingID || res.variantID != variantID || res.quantity != quantity {
			return store.ErrReservationMismatch
		}
		return nil
	}
	if l, ok := r.byID[listingID]; ok && l.Status == "held" {
		return store.ErrListingNotReservable
	}
	return r.txLocked(ctx, func(changes stockChanges) error {
		if err := r.reserveLocked(listingID, variantID, quantity); err != nil {
			return err
		}
		r.reservations[reservationID] = memReservation{
			listingID: listingID,
			variantID: variantID,
			quantity:  quantity,
			expiresAt: expiresAt,
			status:    store.ReservationActive,
		}
		changes.add(listingID, variantID)
		return nil
	})
}
func (r *InMemoryListingRepository) SweepExpiredReservations(ctx context.Context, now time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	released := 0
	err := r.txLocked(ctx, func(changes stockChanges) error {
		for id, res := range r.reservations {
			if res.status != store.ReservationActive || res.expiresAt.After(now) {
				continue
			}
			if err := r.releaseLocked(res.listingID, res.variantID, res.quantity); err == nil {
				changes.add(res.listingID, res.variantID)
			}
			res.status = store.ReservationReleased
			r.reservations[id] = res
			released++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return released, nil
}
func (r *InMemoryListingRepository) ReleaseReservation(ctx context.Context, reservationID string) (store.ReleaseOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return store.ReleaseUnknown, nil
	}
	if res.status == store.ReservationReleased {
		return store.ReleaseNoOp, nil
	}
	err := r.txLocked(ctx, func(changes stockChanges) error {
		if err := r.releaseLocked(res.listingID, res.variantID, res.quantity); err == nil {
			changes.add(res.listingID, res.variantID)
		}
		res.status = store.ReservationReleased
		r.reservations[reservationID] = res
		return nil
	})
	if err != nil {
		return store.ReleaseNoOp, err
	}
	return store.ReleaseApplied, nil
}
func (r *InMemoryListingRepository) CommitReservation(_ context.Context, reservationID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	res, ok := r.reservations[reservationID]
	if !ok {
		return store.ErrReservationNotFound
	}
	switch res.status {
	case store.ReservationActive:
		res.status = store.ReservationCommitted
		r.reservations[reservationID] = res
		return nil
	case store.ReservationCommitted:
		return nil
	default:
		return store.ErrReservationReleased
	}
}

var _ store.ListingRepository = (*InMemoryListingRepository)(nil)

func (r *InMemoryListingRepository) WithStockEvents(b store.StockEventBuilder) *InMemoryListingRepository {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stockEvent = b
	return r
}
func (r *InMemoryListingRepository) StockEventRows() []store.OutboxRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]store.OutboxRow(nil), r.stockRows...)
}
func (r *InMemoryListingRepository) txLocked(ctx context.Context, mutate func(changes stockChanges) error) error {
	savedListings := make(map[string]store.Listing, len(r.byID))
	for id, l := range r.byID {
		l.Variants = append([]store.Variant(nil), l.Variants...)
		savedListings[id] = l
	}
	savedRes := make(map[string]memReservation, len(r.reservations))
	for id, v := range r.reservations {
		savedRes[id] = v
	}
	rollback := func() { r.byID, r.reservations = savedListings, savedRes }

	changes := stockChanges{}
	if err := mutate(changes); err != nil {
		rollback()
		return err
	}
	if r.stockEvent == nil {
		return nil
	}
	var rows []store.OutboxRow
	for _, listingID := range changes.listings() {
		l, ok := r.byID[listingID]
		if !ok {
			continue
		}
		snap := store.StockSnapshot{ListingID: listingID, Stock: l.Stock}
		changed := changes[listingID]
		for _, v := range l.Variants {
			if changed[v.ID] {
				snap.Variants = append(snap.Variants, v)
			}
		}
		row, err := r.stockEvent(ctx, snap)
		if err != nil {
			rollback()
			return err
		}
		rows = append(rows, row)
	}
	r.stockRows = append(r.stockRows, rows...)
	return nil
}

type stockChanges map[string]map[string]bool

func (c stockChanges) add(listingID, variantID string) {
	if c[listingID] == nil {
		c[listingID] = map[string]bool{}
	}
	if variantID != "" {
		c[listingID][variantID] = true
	}
}

func (c stockChanges) listings() []string {
	out := make([]string, 0, len(c))
	for id := range c {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
