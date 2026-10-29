package rpc

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
	"github.com/buidangphuc/team-analytics/internal/query"
)

const (
	// scopeAdmin gates the platform-wide order RPCs; it spans every seller.
	scopeAdmin = "admin"
	// defaultOrderWindow is the summary window when the caller sends none.
	defaultOrderWindow = 24 * time.Hour
	// defaultRecentOrders / maxRecentOrders bound ListRecentOrders.
	defaultRecentOrders = 10
	maxRecentOrders     = 100
)

// defaultTopSKULimit caps top-SKU rows when a caller does not (the RPC has no
// limit field yet) — a sensible dashboard default.
const defaultTopSKULimit = 10

// farFuture is the upper bound used when a request omits `to` (open-ended
// window); year 9999 comfortably covers any real occurred_at.
var farFuture = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// Handler implements analyticsv1.AnalyticsQueryServiceServer over a Repository.
// It validates input, maps proto and domain, and delegates aggregation to the
// repository. It never writes.
type Handler struct {
	analyticsv1.UnimplementedAnalyticsQueryServiceServer
	repo       query.Repository
	logger     *slog.Logger
	errs       grpcx.ErrorTable
	thresholds TrackingThresholds
	// attributionHours is RECS_ATTRIBUTION_WINDOW_HOURS.
	attributionHours int
	now              func() time.Time

	reconcileOn    bool
	reconcileMax   float64
	reconcileGauge RatioGauge
}

// TrackingThresholds are the limits GetTrackingQualityReport derives its status
// from (TRACKING_* settings).
type TrackingThresholds struct {
	StaleAfter             time.Duration
	LagP95Max              time.Duration
	MissingListingMaxRatio float64
}

// DefaultTrackingThresholds are the documented setting defaults.
var DefaultTrackingThresholds = TrackingThresholds{
	StaleAfter:             900 * time.Second,
	LagP95Max:              300 * time.Second,
	MissingListingMaxRatio: 0.05,
}

// DefaultAttributionWindowHours is the documented RECS_ATTRIBUTION_WINDOW_HOURS default.
const DefaultAttributionWindowHours = 24

// Option customises a Service.
type Option func(*Handler)

// WithTrackingThresholds overrides the tracking quality thresholds.
func WithTrackingThresholds(t TrackingThresholds) Option {
	return func(s *Handler) { s.thresholds = t }
}

// WithAttributionWindowHours sets how long after a click an add-to-cart or
// purchase is still credited to it (RECS_ATTRIBUTION_WINDOW_HOURS).
func WithAttributionWindowHours(h int) Option {
	return func(s *Handler) { s.attributionHours = h }
}

// WithClock overrides the clock the report window is measured from (tests).
func WithClock(now func() time.Time) Option {
	return func(s *Handler) { s.now = now }
}

// NewHandler builds the query handler over repo.
func NewHandler(repo query.Repository, logger *slog.Logger, opts ...Option) (*Handler, error) {
	if repo == nil {
		return nil, errors.New("rpc: repository is required")
	}
	errs, err := newErrorTable()
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{repo: repo, logger: logger, errs: errs, thresholds: DefaultTrackingThresholds, attributionHours: DefaultAttributionWindowHours, now: time.Now}
	for _, o := range opts {
		o(h)
	}
	return h, nil
}

// Register adds the handler to srv.
func Register(srv grpc.ServiceRegistrar, h *Handler) {
	analyticsv1.RegisterAnalyticsQueryServiceServer(srv, h)
}

// requireSellerAccess gates the per-seller RPCs. It runs before any other work so
// an unauthorised caller learns nothing (not even whether the repo is wired).
// Anonymous or absent principal -> Unauthenticated; the admin scope -> allowed;
// a user principal whose id equals sellerID -> allowed; anything else (another
// user, a service principal without admin) -> PermissionDenied.
func requireAdmin(ctx context.Context) error {
	_, err := auth.RequireScopes(ctx, scopeAdmin)
	return err
}

func requireSellerAccess(ctx context.Context, sellerID string) error {
	p, err := auth.RequirePrincipal(ctx)
	if err != nil {
		return err
	}
	if p.HasScope(scopeAdmin) {
		return nil
	}
	sellerID = strings.TrimSpace(sellerID)
	if p.Type == auth.TypeUser && sellerID != "" && p.ID == sellerID {
		return nil
	}
	return status.Error(codes.PermissionDenied, "not allowed to read this seller's analytics")
}

// GetSellerFunnel returns impression→view→add→order counts for a seller.
func (s *Handler) GetSellerFunnel(ctx context.Context, req *analyticsv1.GetSellerFunnelRequest) (*analyticsv1.GetSellerFunnelResponse, error) {
	if err := requireSellerAccess(ctx, req.GetSellerId()); err != nil {
		return nil, err
	}
	sellerID := strings.TrimSpace(req.GetSellerId())
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	from, to := window(req.GetFrom(), req.GetTo())

	f, err := s.repo.SellerFunnel(ctx, sellerID, from, to)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return &analyticsv1.GetSellerFunnelResponse{
		Impressions:    f.Impressions,
		Views:          f.Views,
		Adds:           f.Adds,
		Orders:         f.Orders,
		BeginCheckouts: f.BeginCheckouts,
		Purchases:      f.Purchases,
	}, nil
}

// GetRevenueBreakdown returns per-day revenue and the top SKUs for a seller.
func (s *Handler) GetRevenueBreakdown(ctx context.Context, req *analyticsv1.GetRevenueBreakdownRequest) (*analyticsv1.GetRevenueBreakdownResponse, error) {
	if err := requireSellerAccess(ctx, req.GetSellerId()); err != nil {
		return nil, err
	}
	sellerID := strings.TrimSpace(req.GetSellerId())
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	from, to := window(req.GetFrom(), req.GetTo())

	b, err := s.repo.RevenueBreakdown(ctx, sellerID, from, to, defaultTopSKULimit)
	if err != nil {
		return nil, s.fail(ctx, err)
	}

	resp := &analyticsv1.GetRevenueBreakdownResponse{
		Days:    make([]*analyticsv1.DayRevenue, 0, len(b.Days)),
		TopSkus: make([]*analyticsv1.TopSku, 0, len(b.TopSkus)),
	}
	for _, d := range b.Days {
		resp.Days = append(resp.Days, &analyticsv1.DayRevenue{
			Day:        d.Day,
			Revenue:    d.Revenue,
			OrderCount: d.OrderCount,
		})
	}
	for _, sku := range b.TopSkus {
		resp.TopSkus = append(resp.TopSkus, &analyticsv1.TopSku{
			Sku:       sku.SKU,
			ListingId: sku.ListingID,
			Revenue:   sku.Revenue,
			UnitsSold: sku.UnitsSold,
		})
	}
	return resp, nil
}

// GetDemandForecast serves probabilistic daily demand forecasts and restock points.
func (s *Handler) GetDemandForecast(ctx context.Context, req *analyticsv1.GetDemandForecastRequest) (*analyticsv1.GetDemandForecastResponse, error) {
	if err := requireSellerAccess(ctx, req.GetSellerId()); err != nil {
		return nil, err
	}
	sellerID := strings.TrimSpace(req.GetSellerId())
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	listingID := strings.TrimSpace(req.GetListingId())
	if listingID == "" {
		return nil, status.Error(codes.InvalidArgument, "listing_id is required")
	}
	horizon := int(req.GetHorizonDays())
	if horizon <= 0 {
		horizon = 28
	}

	res, err := s.repo.DemandForecast(ctx, sellerID, listingID, horizon)
	if err != nil {
		return nil, s.fail(ctx, err)
	}

	leadTime := int(req.GetLeadTimeDays())
	if leadTime <= 0 {
		leadTime = 3
	}
	serviceLevel := req.GetServiceLevel()
	if serviceLevel <= 0.0 {
		serviceLevel = 0.95
	}
	z := 1.65
	if serviceLevel >= 0.99 {
		z = 2.33
	} else if serviceLevel >= 0.90 && serviceLevel < 0.95 {
		z = 1.28
	}

	var leadDemandP50 float64
	var sumSpread float64
	nDays := len(res.DailyForecast)
	for i := 0; i < leadTime && i < nDays; i++ {
		leadDemandP50 += res.DailyForecast[i].P50
		sumSpread += (res.DailyForecast[i].P90 - res.DailyForecast[i].P50)
	}
	avgSpread := 1.0
	if leadTime > 0 && nDays > 0 {
		effectiveCount := float64(leadTime)
		if float64(nDays) < effectiveCount {
			effectiveCount = float64(nDays)
		}
		avgSpread = sumSpread / effectiveCount
	}
	safetyStock := z * avgSpread
	reorderPoint := leadDemandP50 + safetyStock

	resp := &analyticsv1.GetDemandForecastResponse{
		SellerId:              sellerID,
		ListingId:             listingID,
		ModelVersion:          res.ModelVersion,
		IsColdStart:           res.IsColdStart,
		SuggestedReorderPoint: reorderPoint,
		SafetyStock:           safetyStock,
		DailyForecasts:        make([]*analyticsv1.DailyForecast, 0, len(res.DailyForecast)),
	}

	for _, pt := range res.DailyForecast {
		resp.DailyForecasts = append(resp.DailyForecasts, &analyticsv1.DailyForecast{
			Date: pt.Date,
			P10:  pt.P10,
			P50:  pt.P50,
			P90:  pt.P90,
		})
	}

	return resp, nil
}

// GetPlatformOrderSummary returns the platform-wide distinct paid-order count and
// GMV over the trailing window (default 24h). Admin only.
func (s *Handler) GetPlatformOrderSummary(ctx context.Context, req *analyticsv1.GetPlatformOrderSummaryRequest) (*analyticsv1.GetPlatformOrderSummaryResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	win := defaultOrderWindow
	if d := req.GetWindow(); d != nil && d.AsDuration() > 0 {
		win = d.AsDuration()
	}
	sum, err := s.repo.PlatformOrderSummary(ctx, time.Now().UTC().Add(-win))
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	return &analyticsv1.GetPlatformOrderSummaryResponse{OrderCount: sum.OrderCount, Gmv: sum.GMV}, nil
}

// ListRecentOrders returns the latest paid orders, newest first. Admin only; no
// buyer PII is read or returned.
func (s *Handler) ListRecentOrders(ctx context.Context, req *analyticsv1.ListRecentOrdersRequest) (*analyticsv1.ListRecentOrdersResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	limit := int(req.GetLimit())
	if limit <= 0 || limit > maxRecentOrders {
		limit = defaultRecentOrders
	}
	orders, err := s.repo.RecentOrders(ctx, limit)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	resp := &analyticsv1.ListRecentOrdersResponse{Orders: make([]*analyticsv1.RecentOrder, 0, len(orders))}
	for _, o := range orders {
		resp.Orders = append(resp.Orders, &analyticsv1.RecentOrder{
			OrderId:  o.OrderID,
			SellerId: o.SellerID,
			Total:    o.Total,
			PaidAt:   timestamppb.New(o.PaidAt),
		})
	}
	return resp, nil
}

const (
	defaultQualityWindowHours = 24
	maxQualityWindowHours     = 168

	defaultPerformanceWindowHours = 168

	statusOK       = "OK"
	statusDegraded = "DEGRADED"
)

// GetTrackingQualityReport measures the tracking stream over a trailing window
// (1 to 168 hours, default 24) and derives OK or DEGRADED with sorted reasons
// stale, lagging and incomplete. Admin only.
func (s *Handler) GetTrackingQualityReport(ctx context.Context, req *analyticsv1.GetTrackingQualityReportRequest) (*analyticsv1.GetTrackingQualityReportResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	hours := req.GetWindowHours()
	if hours == 0 {
		hours = defaultQualityWindowHours
	}
	if hours > maxQualityWindowHours {
		return nil, status.Errorf(codes.InvalidArgument, "window_hours must be between 1 and %d", maxQualityWindowHours)
	}
	qr, ok := s.repo.(query.QualityRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	now := s.now().UTC()
	data, err := qr.TrackingQuality(ctx, now.Add(-time.Duration(hours)*time.Hour), now)
	if err != nil {
		return nil, s.fail(ctx, err)
	}

	resp := &analyticsv1.GetTrackingQualityReportResponse{
		Types:             make([]*analyticsv1.TrackingTypeQuality, 0, len(data.Types)),
		LagP50Seconds:     data.LagP50Seconds,
		LagP95Seconds:     data.LagP95Seconds,
		DecodeFailures:    data.DecodeFailures,
		DuplicatesSkipped: data.Duplicates,
		WindowHours:       hours,
	}
	if !data.LastIngestedAt.IsZero() {
		resp.LastIngestedAt = timestamppb.New(data.LastIngestedAt)
	}
	var total int64
	incomplete := false
	for _, t := range data.Types {
		total += t.Events
		if t.ListingScoped && t.MissingListingRatio > s.thresholds.MissingListingMaxRatio {
			incomplete = true
		}
		resp.Types = append(resp.Types, &analyticsv1.TrackingTypeQuality{
			EventType:           t.EventType,
			Events:              t.Events,
			Visitors:            t.Visitors,
			MissingListingRatio: t.MissingListingRatio,
			ListingScoped:       t.ListingScoped,
		})
	}

	var reasons []string
	if total == 0 || data.LastIngestedAt.IsZero() || now.Sub(data.LastIngestedAt) > s.thresholds.StaleAfter {
		reasons = append(reasons, "stale")
	}
	if data.HasLag && data.LagP95Seconds > s.thresholds.LagP95Max.Seconds() {
		reasons = append(reasons, "lagging")
	}
	if incomplete {
		reasons = append(reasons, "incomplete")
	}
	reasons = append(reasons, s.reconcileReasons(ctx, now)...)
	sort.Strings(reasons)
	resp.Reasons = reasons
	resp.Status = statusOK
	for _, r := range reasons {
		if r != "ga4_missing" {
			resp.Status = statusDegraded
		}
	}
	return resp, nil
}

// window converts the optional request timestamps into a concrete [from, to]
// range. A missing `from` opens the lower bound (zero time); a missing `to`
// opens the upper bound (farFuture). A from > to window yields no rows, which
// each repository handles as empty/zero results.
func window(from, to *timestamppb.Timestamp) (time.Time, time.Time) {
	lo := time.Time{}
	if from != nil {
		lo = from.AsTime().UTC()
	}
	hi := farFuture
	if to != nil {
		hi = to.AsTime().UTC()
	}
	return lo, hi
}

// compile-time assertion that the servicer satisfies the generated interface.
var _ analyticsv1.AnalyticsQueryServiceServer = (*Handler)(nil)

// GetRecommendationPerformance reports clicks and attributed conversions per
// (placement, model_version) over a trailing window (1 to 168 hours, default
// 24), plus each placement's share of impressions served by the fallback.
// Admin only (recsys-online-evaluation).
func (s *Handler) GetRecommendationPerformance(ctx context.Context, req *analyticsv1.GetRecommendationPerformanceRequest) (*analyticsv1.GetRecommendationPerformanceResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	hours := req.GetWindowHours()
	if hours == 0 {
		// Seven days, not the quality report's 24 h: conversion_rate counts only clicks whose
		// attribution window has closed, so a default no longer than that window reads 0.
		hours = defaultPerformanceWindowHours
	}
	if hours > maxQualityWindowHours {
		return nil, status.Errorf(codes.InvalidArgument, "window_hours must be between 1 and %d", maxQualityWindowHours)
	}
	pr, ok := s.repo.(query.PerformanceRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	now := s.now().UTC()
	data, err := pr.RecommendationPerformance(ctx, now.Add(-time.Duration(hours)*time.Hour), now, s.attributionHours)
	if err != nil {
		return nil, s.fail(ctx, err)
	}

	resp := &analyticsv1.GetRecommendationPerformanceResponse{
		Rows:                   make([]*analyticsv1.RecommendationPerformanceRow, 0, len(data)),
		Fallback:               []*analyticsv1.PlacementFallbackShare{},
		WindowHours:            hours,
		AttributionWindowHours: uint32(s.attributionHours),
	}
	var order []string
	total, fallback := map[string]int64{}, map[string]int64{}
	for _, d := range data {
		row := &analyticsv1.RecommendationPerformanceRow{
			PlacementId:     d.PlacementID,
			ModelVersion:    d.ModelVersion,
			Impressions:     d.Impressions,
			ItemImpressions: d.ItemImpressions,
			Clicks:          d.Clicks,
			AddToCarts:      d.AddToCarts,
			Purchases:       d.Purchases,
		}
		if d.ItemImpressions > 0 {
			row.Ctr = float64(d.Clicks) / float64(d.ItemImpressions)
		}
		// Only clicks whose attribution window had closed can have converted fully
		// (recs-attribution-hardening D3); open ones would bias the rate down.
		if d.MatureClicks > 0 {
			row.ConversionRate = float64(d.MaturePurchases) / float64(d.MatureClicks)
		}
		resp.Rows = append(resp.Rows, row)
		if _, seen := total[d.PlacementID]; !seen {
			order = append(order, d.PlacementID)
		}
		total[d.PlacementID] += d.Impressions
		if d.ModelVersion == query.FallbackModelVersion {
			fallback[d.PlacementID] += d.Impressions
		}
	}
	sort.Strings(order)
	for _, p := range order {
		share := 0.0
		if total[p] > 0 {
			share = float64(fallback[p]) / float64(total[p])
		}
		resp.Fallback = append(resp.Fallback, &analyticsv1.PlacementFallbackShare{PlacementId: p, FallbackShare: share})
	}
	return resp, nil
}

// GetChannelFunnel splits the funnel by acquisition channel (source, medium,
// campaign) over [from, to]. Admin only; seller_id optionally narrows it.
func (s *Handler) GetChannelFunnel(ctx context.Context, req *analyticsv1.GetChannelFunnelRequest) (*analyticsv1.GetChannelFunnelResponse, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if req.GetFrom() != nil && req.GetTo() != nil && req.GetFrom().AsTime().After(req.GetTo().AsTime()) {
		return nil, status.Error(codes.InvalidArgument, "from must not be after to")
	}
	cr, ok := s.repo.(query.ChannelFunnelRepository)
	if !ok {
		return nil, s.fail(ctx, query.ErrUnsupported)
	}
	from, to := window(req.GetFrom(), req.GetTo())
	data, err := cr.ChannelFunnel(ctx, strings.TrimSpace(req.GetSellerId()), from, to)
	if err != nil {
		return nil, s.fail(ctx, err)
	}
	resp := &analyticsv1.GetChannelFunnelResponse{Rows: make([]*analyticsv1.ChannelFunnelRow, 0, len(data))}
	for _, d := range data {
		resp.Rows = append(resp.Rows, &analyticsv1.ChannelFunnelRow{
			Source: d.Source, Medium: d.Medium, Campaign: d.Campaign,
			Impressions: d.Impressions, Views: d.Views, Adds: d.Adds,
			BeginCheckouts: d.BeginCheckouts, Purchases: d.Purchases,
		})
	}
	return resp, nil
}
