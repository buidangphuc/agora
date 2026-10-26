package storetest

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/buidangphuc/team-engagement/internal/store"
)

type FakeReportRepository struct {
	mu       sync.Mutex
	reports  map[string]store.Report
	listings map[[2]string]bool
}

func NewFakeReportRepository() *FakeReportRepository {
	return &FakeReportRepository{reports: map[string]store.Report{}, listings: map[[2]string]bool{}}
}

// SeedReport inserts r verbatim (test seam, in-memory only).
func (r *FakeReportRepository) SeedReport(rep store.Report) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rep.TargetType == "" {
		rep.TargetType = "LISTING"
	}
	if rep.ReasonCode == "" {
		rep.ReasonCode = "SPAM"
	}
	if rep.Status == "" {
		rep.Status = store.ReportStatusOpen
	}
	if rep.CreatedAt.IsZero() {
		rep.CreatedAt = time.Now().UTC()
	}
	r.reports[rep.ID] = rep
}

// SeedSellerListing records that sellerID owns listingID (test seam, in-memory only).
func (r *FakeReportRepository) SeedSellerListing(sellerID, listingID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listings[[2]string{sellerID, listingID}] = true
}

func (r *FakeReportRepository) CreateReport(_ context.Context, in store.Report) (store.Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.reports {
		if e.Status == store.ReportStatusOpen && e.ReporterID == in.ReporterID &&
			e.TargetType == in.TargetType && e.TargetID == in.TargetID {
			return e, nil
		}
	}
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = time.Now().UTC()
	}
	in.Status = store.ReportStatusOpen
	in.ResolvedAt = nil
	r.reports[in.ID] = in
	return in, nil
}

func (r *FakeReportRepository) CountOpenByReporter(_ context.Context, reporterID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.reports {
		if e.Status == store.ReportStatusOpen && e.ReporterID == reporterID {
			n++
		}
	}
	return n, nil
}

func (r *FakeReportRepository) GetReport(_ context.Context, id string) (store.Report, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.reports[id]
	if !ok {
		return store.Report{}, store.ErrReportNotFound
	}
	return e, nil
}

func (r *FakeReportRepository) ListReports(_ context.Context, status store.ReportStatus, targetID string, offset, limit int) ([]store.Report, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []store.Report
	for _, e := range r.reports {
		if (status == "" || e.Status == status) && (targetID == "" || e.TargetID == targetID) {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].CreatedAt.Before(all[j].CreatedAt)
		}
		return all[i].ID < all[j].ID
	})
	total := int64(len(all))
	if offset < 0 {
		offset = 0
	}
	if offset > len(all) {
		offset = len(all)
	}
	end := len(all)
	if limit >= 0 && offset+limit < end {
		end = offset + limit
	}
	return append([]store.Report{}, all[offset:end]...), total, nil
}

func (r *FakeReportRepository) CloseReport(_ context.Context, id, adminID string, status store.ReportStatus, note string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.reports[id]
	if !ok {
		return false, store.ErrReportNotFound
	}
	if e.Status != store.ReportStatusOpen {
		return false, nil
	}
	now := time.Now().UTC()
	e.Status, e.ResolvedAt, e.ResolvedBy, e.ResolutionNote = status, &now, adminID, note
	r.reports[id] = e
	return true, nil
}

func (r *FakeReportRepository) SweepClosedBefore(_ context.Context, cutoff time.Time, batch int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due []store.Report
	for _, e := range r.reports {
		if e.Status != store.ReportStatusOpen && e.ResolvedAt != nil && e.ResolvedAt.Before(cutoff) &&
			(e.ReporterID != store.ReportRedacted || e.Note != "") {
			due = append(due, e)
		}
	}
	sort.Slice(due, func(i, j int) bool {
		if !due[i].ResolvedAt.Equal(*due[j].ResolvedAt) {
			return due[i].ResolvedAt.Before(*due[j].ResolvedAt)
		}
		return due[i].ID < due[j].ID
	})
	if batch >= 0 && len(due) > batch {
		due = due[:batch]
	}
	for _, e := range due {
		e.ReporterID, e.Note = store.ReportRedacted, ""
		r.reports[e.ID] = e
	}
	return int64(len(due)), nil
}

func (r *FakeReportRepository) HasSellerListing(_ context.Context, sellerID, listingID string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listings[[2]string{sellerID, listingID}], nil
}

var _ store.ReportRepository = (*FakeReportRepository)(nil)
