package rpc

import (
	"context"
	"time"

	"github.com/buidangphuc/team-analytics/internal/analytics"
)

// RatioGauge receives the per-type divergence ratio.
type RatioGauge interface {
	Set(eventType string, ratio float64)
}

// WithGA4Reconcile enables GA4 reconciliation in the tracking quality report.
func WithGA4Reconcile(maxDiffRatio float64, gauge RatioGauge) Option {
	return func(s *Service) {
		s.reconcileOn, s.reconcileMax, s.reconcileGauge = true, maxDiffRatio, gauge
	}
}

// reconcileReasons returns extra report reasons; read errors add none.
func (s *Service) reconcileReasons(ctx context.Context, now time.Time) []string {
	if !s.reconcileOn {
		return nil
	}
	rr, ok := s.repo.(analytics.GA4ReconcileRepository)
	if !ok {
		return nil
	}
	day := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	rec, err := rr.GA4Reconcile(ctx, day)
	if err != nil {
		return nil
	}
	if !rec.ImportRun {
		return []string{"ga4_missing"}
	}
	diverged := false
	for _, t := range rec.Types {
		var ratio float64
		d := t.GA4 - t.FirstParty
		if d < 0 {
			d = -d
		}
		if t.FirstParty > 0 {
			ratio = float64(d) / float64(t.FirstParty)
		} else {
			ratio = float64(d)
		}
		if s.reconcileGauge != nil {
			s.reconcileGauge.Set(t.EventType, ratio)
		}
		if ratio > s.reconcileMax {
			diverged = true
		}
	}
	if diverged {
		return []string{"ga4_divergence"}
	}
	return nil
}
