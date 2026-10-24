package rpc_test

import (
	"context"
	"io/fs"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	listingv1 "github.com/buidangphuc/team-promotion/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/flashsale"
	"github.com/buidangphuc/team-promotion/internal/listing"
	"github.com/buidangphuc/team-promotion/internal/rpc"
	"github.com/buidangphuc/team-promotion/internal/sponsored"
	"github.com/buidangphuc/team-promotion/internal/store"
	"github.com/buidangphuc/team-promotion/internal/store/storefake"
	"github.com/buidangphuc/team-promotion/internal/subscription"
	"github.com/buidangphuc/team-promotion/internal/voucher"
)

const outboxTable = "promotion_outbox"

type recordingAudit struct {
	mu     sync.Mutex
	events []auditx.Event
	err    error
}

func (r *recordingAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return r.err
}

func (r *recordingAudit) got() []auditx.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]auditx.Event(nil), r.events...)
}

func (r *recordingAudit) requireNone(t *testing.T, what string) {
	t.Helper()
	if evs := r.got(); len(evs) != 0 {
		t.Fatalf("%s: wrote %d audit events, want 0: %+v", what, len(evs), evs)
	}
}

type ownedListings map[string]string

func (o ownedListings) GetListing(_ context.Context, req *listingv1.GetListingRequest, _ ...grpc.CallOption) (*listingv1.GetListingResponse, error) {
	seller, ok := o[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &listingv1.GetListingResponse{Listing: &listingv1.Listing{Id: req.GetId(), SellerId: seller}}, nil
}

func owned() ownedListings { return ownedListings{"l1": "seller-1", "l2": "seller-2"} }

func noVoucherEvent(context.Context, store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil }

func noCampaignEvent(context.Context, store.FlashSaleCampaign) (outbox.Row, error) {
	return outbox.Row{}, nil
}

func campaignRow(store.FlashSaleCampaign) (outbox.Row, error) { return outbox.Row{}, nil }

func voucherRow(store.Voucher) (outbox.Row, error) { return outbox.Row{}, nil }

type rigOpts struct {
	listings     listing.Getter
	audit        auditx.Writer
	vouchers     store.VoucherRepository
	reservations store.ReservationRepository
	holds        func(*storefake.InMemoryFlashHoldRepository) store.FlashHoldRepository
	voucherEvent func(context.Context, store.Voucher) (outbox.Row, error)
	logger       *slog.Logger
}

type rig struct {
	vc           promotionv1.VoucherServiceClient
	fc           promotionv1.FlashSaleServiceClient
	sub          promotionv1.SubscriptionServiceClient
	ads          promotionv1.SponsoredServiceClient
	vouchers     store.VoucherRepository
	reservations store.ReservationRepository
	campaigns    *storefake.InMemoryFlashSaleRepository
	holds        store.FlashHoldRepository
	voucherH     *rpc.VoucherHandler
	flashH       *rpc.FlashSaleHandler
}

func startRig(t *testing.T, o rigOpts) *rig {
	t.Helper()
	if o.listings == nil {
		o.listings = owned()
	}
	if o.audit == nil {
		o.audit = &recordingAudit{}
	}
	if o.vouchers == nil {
		o.vouchers = storefake.NewInMemoryVoucherRepository()
	}
	if o.reservations == nil {
		o.reservations = storefake.NewInMemoryReservationRepository()
	}
	if o.voucherEvent == nil {
		o.voucherEvent = noVoucherEvent
	}
	if o.logger == nil {
		o.logger = slog.New(slog.DiscardHandler)
	}
	gate := auditx.NewGate(o.audit, o.logger)

	campaigns := storefake.NewInMemoryFlashSaleRepository()
	memHolds := storefake.NewInMemoryFlashHoldRepository(campaigns)
	var holds store.FlashHoldRepository = memHolds
	if o.holds != nil {
		holds = o.holds(memHolds)
	}

	voucherSvc := voucher.New(o.vouchers, o.reservations, gate, o.voucherEvent, o.logger)
	flashSvc := flashsale.New(campaigns, holds, nil, o.listings, gate, noCampaignEvent, o.logger)
	subSvc := subscription.New(storefake.NewInMemorySubscriptionRepository(), o.logger)
	adSvc := sponsored.New(storefake.NewInMemoryAdCampaignRepository(), o.listings, gate, o.logger).WithLimits(1000, 5000)

	vh := rpc.NewVoucherHandler(voucherSvc, o.logger)
	fh := rpc.NewFlashSaleHandler(flashSvc, o.logger)
	sh := rpc.NewSubscriptionHandler(subSvc, o.logger)
	ah := rpc.NewSponsoredHandler(adSvc, o.logger)

	conn := serve(t, o.logger, vh, fh, sh, ah)
	return &rig{
		vc:           promotionv1.NewVoucherServiceClient(conn),
		fc:           promotionv1.NewFlashSaleServiceClient(conn),
		sub:          promotionv1.NewSubscriptionServiceClient(conn),
		ads:          promotionv1.NewSponsoredServiceClient(conn),
		vouchers:     o.vouchers,
		reservations: o.reservations,
		campaigns:    campaigns,
		holds:        holds,
		voucherH:     vh,
		flashH:       fh,
	}
}

func serve(t *testing.T, logger *slog.Logger, vh *rpc.VoucherHandler, fh *rpc.FlashSaleHandler, sh *rpc.SubscriptionHandler, ah *rpc.SponsoredHandler) *grpc.ClientConn {
	t.Helper()
	hl, err := healthx.New(healthx.DefaultInterval, logger)
	if err != nil {
		t.Fatalf("healthx.New: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Logger: logger, Health: hl})
	rpc.Register(srv, vh, fh, sh, ah)
	lis := bufconn.Listen(1024 * 1024)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func principalCtx(t *testing.T, id, typ string, scopes ...string) context.Context {
	t.Helper()
	md := metadata.New(map[string]string{
		"x-principal-id":     id,
		"x-principal-type":   typ,
		"x-principal-scopes": strings.Join(scopes, ","),
	})
	return metadata.NewOutgoingContext(t.Context(), md)
}

func authCtx(t *testing.T, id string, scopes ...string) context.Context {
	t.Helper()
	return principalCtx(t, id, "user", scopes...)
}

func adminCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "admin-1", "listing.write", "admin")
}

func sellerCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "seller-1", "listing.write")
}

func buyerCtx(t *testing.T) context.Context {
	t.Helper()
	return authCtx(t, "buyer-1", "listing.read")
}

func anonCtx(t *testing.T) context.Context {
	t.Helper()
	return principalCtx(t, "anonymous", "anonymous")
}

func orderSvcCtx(t *testing.T) context.Context {
	t.Helper()
	return principalCtx(t, "service-team-order", "service", "promotion.reserve")
}

func code(err error) codes.Code { return status.Code(err) }

func platformVoucher(code string, scope promotionv1.VoucherScope) *promotionv1.CreateVoucherRequest {
	return &promotionv1.CreateVoucherRequest{
		Code: code, Scope: scope, Quota: 5, DiscountValue: 10,
		DiscountType: promotionv1.DiscountType_DISCOUNT_TYPE_PERCENT,
	}
}

func newPostgres(t *testing.T) (*pgxpool.Pool, *outbox.Store) {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	if err := migrate.Up(t.Context(), dsn, sub); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ob, err := outbox.NewStore(pool, outboxTable)
	if err != nil {
		t.Fatalf("outbox store: %v", err)
	}
	return pool, ob
}

func outboxCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM `+outboxTable).Scan(&n); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return n
}
