package rpc_test

import (
	"context"
	"time"

	"github.com/buidangphuc/team-analytics/internal/query"
)

type spyRepo struct {
	calls      int
	lastSeller string
	funnel     query.Funnel
	err        error
	onCall     func(ctx context.Context) error
}

func (s *spyRepo) enter(ctx context.Context, seller string) error {
	s.calls++
	s.lastSeller = seller
	if s.onCall != nil {
		return s.onCall(ctx)
	}
	return s.err
}

func (s *spyRepo) SellerFunnel(ctx context.Context, seller string, _, _ time.Time) (query.Funnel, error) {
	if err := s.enter(ctx, seller); err != nil {
		return query.Funnel{}, err
	}
	return s.funnel, nil
}

func (s *spyRepo) RevenueBreakdown(ctx context.Context, seller string, _, _ time.Time, _ int) (query.Breakdown, error) {
	return query.Breakdown{}, s.enter(ctx, seller)
}

func (s *spyRepo) DemandForecast(ctx context.Context, seller, _ string, _ int) (query.ForecastResult, error) {
	return query.ForecastResult{}, s.enter(ctx, seller)
}

func (s *spyRepo) PlatformOrderSummary(ctx context.Context, _ time.Time) (query.OrderSummary, error) {
	return query.OrderSummary{}, s.enter(ctx, "")
}

func (s *spyRepo) RecentOrders(ctx context.Context, _ int) ([]query.RecentOrder, error) {
	return nil, s.enter(ctx, "")
}

type capRepo struct {
	spyRepo
	channelErr error
}

func (c *capRepo) TrackingQuality(ctx context.Context, _, _ time.Time) (query.TrackingQualityData, error) {
	return query.TrackingQualityData{}, c.enter(ctx, "")
}

func (c *capRepo) ChannelFunnel(ctx context.Context, _ string, _, _ time.Time) ([]query.ChannelFunnelRow, error) {
	c.calls++
	return nil, c.channelErr
}
