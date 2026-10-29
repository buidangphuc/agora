package rpc

import (
	"log/slog"
	"time"

	"google.golang.org/grpc"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
)

type RatioGauge interface {
	Set(eventType string, ratio float64)
}

type TrackingThresholds struct {
	StaleAfter             time.Duration
	LagP95Max              time.Duration
	MissingListingMaxRatio float64
}

var DefaultTrackingThresholds = TrackingThresholds{
	StaleAfter:             900 * time.Second,
	LagP95Max:              300 * time.Second,
	MissingListingMaxRatio: 0.05,
}

const DefaultAttributionWindowHours = 24

type Option func(*Handler)

func WithTrackingThresholds(TrackingThresholds) Option { return func(*Handler) {} }
func WithAttributionWindowHours(int) Option            { return func(*Handler) {} }
func WithClock(func() time.Time) Option                { return func(*Handler) {} }
func WithGA4Reconcile(float64, RatioGauge) Option      { return func(*Handler) {} }

type Handler struct {
	analyticsv1.UnimplementedAnalyticsQueryServiceServer
}

func NewHandler(query.Repository, *slog.Logger, ...Option) (*Handler, error) {
	return &Handler{}, nil
}

func Register(srv *grpc.Server, h *Handler) {
	analyticsv1.RegisterAnalyticsQueryServiceServer(srv, h)
}
