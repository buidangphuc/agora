package query

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	analyticsv1 "github.com/buidangphuc/team-analytics/generated/platform/analytics/v1"
)

// defaultTopSKULimit caps top-SKU rows when a caller does not (the RPC has no
// limit field yet) — a sensible dashboard default.
const defaultTopSKULimit = 10

// farFuture is the upper bound used when a request omits `to` (open-ended
// window); year 9999 comfortably covers any real occurred_at.
var farFuture = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// Service implements analyticsv1.AnalyticsQueryServiceServer over a Repository.
// It is the handler layer: it validates input, maps proto⇄domain, and delegates
// aggregation to the repository. It never writes.
type Service struct {
	analyticsv1.UnimplementedAnalyticsQueryServiceServer
	repo Repository
}

// NewService builds the query servicer over repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetSellerFunnel returns impression→view→add→order counts for a seller.
func (s *Service) GetSellerFunnel(ctx context.Context, req *analyticsv1.GetSellerFunnelRequest) (*analyticsv1.GetSellerFunnelResponse, error) {
	if s.repo == nil {
		return nil, status.Error(codes.Unavailable, "analytics query repository not configured")
	}
	sellerID := strings.TrimSpace(req.GetSellerId())
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	from, to := window(req.GetFrom(), req.GetTo())

	f, err := s.repo.SellerFunnel(ctx, sellerID, from, to)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "seller funnel: %v", err)
	}
	return &analyticsv1.GetSellerFunnelResponse{
		Impressions: f.Impressions,
		Views:       f.Views,
		Adds:        f.Adds,
		Orders:      f.Orders,
	}, nil
}

// GetRevenueBreakdown returns per-day revenue and the top SKUs for a seller.
func (s *Service) GetRevenueBreakdown(ctx context.Context, req *analyticsv1.GetRevenueBreakdownRequest) (*analyticsv1.GetRevenueBreakdownResponse, error) {
	if s.repo == nil {
		return nil, status.Error(codes.Unavailable, "analytics query repository not configured")
	}
	sellerID := strings.TrimSpace(req.GetSellerId())
	if sellerID == "" {
		return nil, status.Error(codes.InvalidArgument, "seller_id is required")
	}
	from, to := window(req.GetFrom(), req.GetTo())

	b, err := s.repo.RevenueBreakdown(ctx, sellerID, from, to, defaultTopSKULimit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "revenue breakdown: %v", err)
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
func (s *Service) GetDemandForecast(ctx context.Context, req *analyticsv1.GetDemandForecastRequest) (*analyticsv1.GetDemandForecastResponse, error) {
	if s.repo == nil {
		return nil, status.Error(codes.Unavailable, "analytics query repository not configured")
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
		return nil, status.Errorf(codes.Internal, "demand forecast: %v", err)
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
var _ analyticsv1.AnalyticsQueryServiceServer = (*Service)(nil)

