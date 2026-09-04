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
