package query

// RED-FIRST STUB (analytics-experiment-reports T1): the implementer deletes this file,
// moves the types and interfaces into repository.go and implements the methods in reports.go.

import (
	"context"
	"time"
)

type RetentionCohortRow struct {
	CohortWeek   string // ISO date of the UTC Monday
	CohortSize   int64
	D1Active     int64
	D1Eligible   int64
	D7Active     int64
	D7Eligible   int64
	D30Active    int64
	D30Eligible  int64
	WeeklyActive []int64 // active members per fully elapsed week, week 0 first
	// RepeatBuyers / RepeatEligible: second paid order within 30 days of the first.
	RepeatBuyers   int64
	RepeatEligible int64
}

type ExperimentRow struct {
	Variant   string
	Exposed   int64
	Converted int64
}

type AiSurfaceRow struct {
	Surface      string
	ModelVersion string
	Shown        int64
	Accepted     int64
	Edited       int64
	Dismissed    int64
}

type RetentionRepository interface {
	RetentionCohorts(ctx context.Context, from time.Time, weeks int, now time.Time) ([]RetentionCohortRow, error)
}

type ExperimentRepository interface {
	ExperimentReadout(ctx context.Context, flag string, from, to time.Time, windowDays int) ([]ExperimentRow, error)
}

type AiSurfaceRepository interface {
	AiSurfaceReport(ctx context.Context, from, to time.Time) ([]AiSurfaceRow, error)
}

func (r *DuckDBRepository) RetentionCohorts(ctx context.Context, from time.Time, weeks int, now time.Time) ([]RetentionCohortRow, error) {
	return nil, nil
}

func (r *DuckDBRepository) ExperimentReadout(ctx context.Context, flag string, from, to time.Time, windowDays int) ([]ExperimentRow, error) {
	return nil, nil
}

func (r *DuckDBRepository) AiSurfaceReport(ctx context.Context, from, to time.Time) ([]AiSurfaceRow, error) {
	return nil, nil
}
