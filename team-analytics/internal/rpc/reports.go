package rpc

import (
	"context"
	"math"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
)

const (
	defaultRetentionWeeks = 12
	maxRetentionWeeks     = 26
	maxFlagLength         = 64
	maxReportWindow       = 366 * 24 * time.Hour
	defaultReportWindow   = 30 * 24 * time.Hour
	experimentWindowDays  = 7
	wilsonZ               = 1.96
)

func ratio(num, den int64) float64 {
	if den <= 0 {
		return 0
	}
	return float64(num) / float64(den)
}

func utcMonday(t time.Time) time.Time {
	t = t.UTC()
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func (s *Handler) reportWindow(from, to *timestamppb.Timestamp) (time.Time, time.Time, error) {
	hi := s.now().UTC()
	if to != nil {
		hi = to.AsTime().UTC()
	}
	lo := hi.Add(-defaultReportWindow)
	if from != nil {
		lo = from.AsTime().UTC()
	}
	if lo.After(hi) {
		return lo, hi, status.Error(codes.InvalidArgument, "from must not be after to")
	}
	if hi.Sub(lo) > maxReportWindow {
		return lo, hi, status.Error(codes.InvalidArgument, "window must not exceed 366 days")
	}
	return lo, hi, nil
}

func wilson(exposed, converted int64) (rate, low, high float64) {
	if exposed <= 0 {
		return 0, 0, 0
	}
	n := float64(exposed)
	p := float64(converted) / n
	z2 := wilsonZ * wilsonZ
	denom := 1 + z2/n
	center := p + z2/(2*n)
	margin := wilsonZ * math.Sqrt(p*(1-p)/n+z2/(4*n*n))
	low = math.Max(0, (center-margin)/denom)
	high = math.Min(1, (center+margin)/denom)
	if converted == 0 {
		low = 0
	}
	if converted >= exposed {
		high = 1
	}
	return p, low, high
}

// GetRetentionCohorts reports signup-week retention. Admin only.
func (s *Handler) GetRetentionCohorts(ctx context.Context, req *analyticsv1.GetRetentionCohortsRequest) (*analyticsv1.GetRetentionCohortsResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	weeks := int(req.GetWeeks())
	if weeks == 0 {
		weeks = defaultRetentionWeeks
	}
	if weeks > maxRetentionWeeks {
		return nil, status.Errorf(codes.InvalidArgument, "weeks must be between 1 and %d", maxRetentionWeeks)
	}
	rr, ok := s.repo.(query.RetentionRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	now := s.now().UTC()
	var from time.Time
	if req.GetFrom() != nil {
		from = utcMonday(req.GetFrom().AsTime())
	} else {
		from = utcMonday(now).AddDate(0, 0, -weeks*7)
	}
	rows, err := rr.RetentionCohorts(ctx, from, weeks, now)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	resp := &analyticsv1.GetRetentionCohortsResponse{Cohorts: make([]*analyticsv1.RetentionCohort, 0, len(rows))}
	for _, r := range rows {
		c := &analyticsv1.RetentionCohort{
			CohortWeek:         r.CohortWeek,
			CohortSize:         r.CohortSize,
			D1:                 ratio(r.D1Active, r.D1Eligible),
			D7:                 ratio(r.D7Active, r.D7Eligible),
			D30:                ratio(r.D30Active, r.D30Eligible),
			RepeatPurchase_30D: ratio(r.RepeatBuyers, r.RepeatEligible),
			WeeklyActive:       make([]float64, 0, len(r.WeeklyActive)),
		}
		for _, a := range r.WeeklyActive {
			c.WeeklyActive = append(c.WeeklyActive, ratio(a, r.CohortSize))
		}
		resp.Cohorts = append(resp.Cohorts, c)
	}
	return resp, nil
}

// GetExperimentReadout reports per-variant conversion with a Wilson interval. Admin only.
func (s *Handler) GetExperimentReadout(ctx context.Context, req *analyticsv1.GetExperimentReadoutRequest) (*analyticsv1.GetExperimentReadoutResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	flag := strings.TrimSpace(req.GetFlag())
	if flag == "" || len(flag) > maxFlagLength {
		return nil, status.Errorf(codes.InvalidArgument, "flag is required and at most %d characters", maxFlagLength)
	}
	from, to, err := s.reportWindow(req.GetFrom(), req.GetTo())
	if err != nil {
		return nil, err
	}
	er, ok := s.repo.(query.ExperimentRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	rows, err := er.ExperimentReadout(ctx, flag, from, to, experimentWindowDays)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	resp := &analyticsv1.GetExperimentReadoutResponse{
		Variants:   make([]*analyticsv1.ExperimentVariantRow, 0, len(rows)),
		WindowDays: experimentWindowDays,
	}
	for _, r := range rows {
		rate, lo, hi := wilson(r.Exposed, r.Converted)
		resp.Variants = append(resp.Variants, &analyticsv1.ExperimentVariantRow{
			Variant: r.Variant, Exposed: r.Exposed, Converted: r.Converted, Rate: rate, CiLow: lo, CiHigh: hi,
		})
	}
	return resp, nil
}

// GetAiSurfaceReport reports AI suggestion outcomes per surface and model. Admin only.
func (s *Handler) GetAiSurfaceReport(ctx context.Context, req *analyticsv1.GetAiSurfaceReportRequest) (*analyticsv1.GetAiSurfaceReportResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	from, to, err := s.reportWindow(req.GetFrom(), req.GetTo())
	if err != nil {
		return nil, err
	}
	ar, ok := s.repo.(query.AiSurfaceRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	rows, err := ar.AiSurfaceReport(ctx, from, to)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	resp := &analyticsv1.GetAiSurfaceReportResponse{Rows: make([]*analyticsv1.AiSurfaceRow, 0, len(rows))}
	for _, r := range rows {
		resp.Rows = append(resp.Rows, &analyticsv1.AiSurfaceRow{
			Surface: r.Surface, ModelVersion: r.ModelVersion,
			Shown: r.Shown, Accepted: r.Accepted, Edited: r.Edited, Dismissed: r.Dismissed,
		})
	}
	return resp, nil
}
