package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/buidangphuc/team-domain/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-domain/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"

	"github.com/buidangphuc/team-domain/internal/config"
	"github.com/buidangphuc/team-domain/internal/grpcserver"
	"github.com/buidangphuc/team-domain/internal/handler"
	"github.com/buidangphuc/team-domain/internal/repository"
	"github.com/buidangphuc/team-domain/internal/service"
	"github.com/buidangphuc/team-domain/internal/storage"
)

// decodeChange unmarshals a stored outbox row's EventEnvelope bytes back into the
// inner ListingChanged, asserting the envelope's event_id matches the row id
// (the stable dedupe id the relayer re-delivers). It mirrors what a consumer
// sees on the wire, proving the bytes are a normal EventEnvelope.
func decodeChange(t *testing.T, row repository.OutboxRow) *listingv1.ListingChanged {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("unmarshal EventEnvelope: %v", err)
	}
	if env.GetEventId() != row.EventID {
		t.Fatalf("envelope event_id %q != row event_id %q (must be stable)", env.GetEventId(), row.EventID)
	}
	var ch listingv1.ListingChanged
	if err := proto.Unmarshal(env.GetPayload(), &ch); err != nil {
		t.Fatalf("unmarshal ListingChanged: %v", err)
	}
	return &ch
}

func testSettings() *config.Settings {
	s := &config.Settings{}
	s.Server.Port = 0
	s.Server.ReflectionEnabled = false
	s.Storage.Endpoint = "localhost:9000"
	s.Storage.Bucket = "listing-images"
	s.Storage.AccessKey = "testkey"
	s.Storage.SecretKey = "testsecret"
	s.Storage.PublicBaseURL = "http://localhost:9000/listing-images"
	return s
}

func startServer(t *testing.T, repo repository.ListingRepository) listingv1.ListingServiceClient {
	t.Helper()
	return startServerOutbox(t, repo, nil)
}

// startServerOutbox wires the handler with an optional in-memory transactional
// writer. When txw is non-nil, writes record their event in the (fake) outbox —
// exactly as production does with the Postgres writer — so a test can assert the
// enqueued rows. txw must wrap the same repo passed here.
func startServerOutbox(t *testing.T, repo repository.ListingRepository, txw *repository.InMemoryTxWriter) listingv1.ListingServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := testSettings()
	store := storage.NewS3Storage(cfg.Storage)
	catRepo := repository.NewInMemoryCategoryRepository()
	var svc *service.ListingService
	if txw != nil {
		svc = service.NewListingServiceWithOutbox(repo, txw)
	} else {
		svc = service.NewListingService(repo)
	}
	h := handler.NewListingHandler(svc, catRepo, store)
	srv := grpcserver.Build(cfg, h, nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		srv.Stop()
	})
	return listingv1.NewListingServiceClient(conn)
}

// principalCtx mimics the gateway: it forwards a resolved Principal (id + scopes)
// as trusted metadata.
func principalCtx(t *testing.T, scopes string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs(
		"x-principal-id", "u1",
		"x-principal-type", "user",
		"x-principal-scopes", scopes,
	)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

// principalCtxAs is principalCtx with an explicit principal type and id.
func principalCtxAs(t *testing.T, id, typ, scopes string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	md := metadata.Pairs(
		"x-principal-id", id,
		"x-principal-type", typ,
		"x-principal-scopes", scopes,
	)
	return metadata.NewOutgoingContext(ctx, md), cancel
}

func seedRepo() *repository.InMemoryListingRepository {
	return repository.NewInMemoryListingRepository(
		repository.Listing{ID: "a1", Title: "Alpha", Price: 100, Currency: "VND", Status: "published"},
		repository.Listing{ID: "b2", Title: "Bravo", Price: 200, Currency: "VND", Status: "draft"},
		repository.Listing{ID: "c3", Title: "Charlie", Price: 300, Currency: "VND", Status: "published"},
	)
}

func TestGetListing_Hit(t *testing.T) {
	client := startServer(t, seedRepo())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.GetListing(ctx, &listingv1.GetListingRequest{Id: "a1"})
	if err != nil {
		t.Fatalf("GetListing: %v", err)
	}
	if resp.GetListing().GetId() != "a1" || resp.GetListing().GetTitle() != "Alpha" {
		t.Fatalf("unexpected listing: %+v", resp.GetListing())
	}
}

func TestGetListing_NotFound(t *testing.T) {
	client := startServer(t, seedRepo())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	_, err := client.GetListing(ctx, &listingv1.GetListingRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestListListings_Page(t *testing.T) {
	client := startServer(t, seedRepo())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.ListListings(ctx, &listingv1.ListListingsRequest{
		Page: &commonv1.PageRequest{PageSize: 1},
	})
	if err != nil {
		t.Fatalf("ListListings: %v", err)
	}
	// Empty status means published: the seeded draft (b2) is excluded from the total.
	if len(resp.GetListings()) != 1 || resp.GetPage().GetTotal() != 2 {
		t.Fatalf("want 1 item of total 2 (published only), got %d/%d", len(resp.GetListings()), resp.GetPage().GetTotal())
	}
}

func TestGetListing_Unauthenticated(t *testing.T) {
	client := startServer(t, seedRepo())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// No forwarded principal at all.
	_, err := client.GetListing(ctx, &listingv1.GetListingRequest{Id: "a1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestGetListing_InsufficientScope(t *testing.T) {
	client := startServer(t, seedRepo())
	ctx, cancel := principalCtx(t, "other.scope")
	defer cancel()

	_, err := client.GetListing(ctx, &listingv1.GetListingRequest{Id: "a1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
}

func TestCreateListing_EnqueuesEvent(t *testing.T) {
	repo := repository.NewInMemoryListingRepository()
	txw := repository.NewInMemoryTxWriter(repo)
	client := startServerOutbox(t, repo, txw)
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	resp, err := client.CreateListing(ctx, &listingv1.CreateListingRequest{
		Listing: &listingv1.Listing{
			Title: "iPhone 15", Description: "new", Price: 20000000, Currency: "VND",
			Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
		},
	})
	if err != nil {
		t.Fatalf("CreateListing: %v", err)
	}
	if resp.GetListing().GetId() == "" {
		t.Fatal("expected server-assigned id")
	}
	if resp.GetListing().GetSellerId() != "u1" {
		t.Fatalf("expected seller_id from principal, got %q", resp.GetListing().GetSellerId())
	}
	// Exactly one outbox row, committed with the write, keyed by listing id and
	// carrying a byte-compatible ListingChanged(CREATED) envelope.
	rows := txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("want 1 enqueued outbox row, got %d", len(rows))
	}
	if rows[0].AggregateID != resp.GetListing().GetId() {
		t.Fatalf("outbox key %q != listing id %q", rows[0].AggregateID, resp.GetListing().GetId())
	}
	if ch := decodeChange(t, rows[0]); ch.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_CREATED {
		t.Fatalf("want CREATED change, got %v", ch.GetChangeType())
	}
}

func TestUpdateListing_EnqueuesEventAndNotFound(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(
		repository.Listing{ID: "x1", Title: "old", Price: 1, Currency: "VND", Status: "draft", SellerID: "u1"},
	)
	txw := repository.NewInMemoryTxWriter(repo)
	client := startServerOutbox(t, repo, txw)
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	_, err := client.UpdateListing(ctx, &listingv1.UpdateListingRequest{
		Listing: &listingv1.Listing{Id: "x1", Title: "new", Price: 2, Currency: "VND", Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
	})
	if err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	rows := txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("want 1 enqueued outbox row, got %d", len(rows))
	}
	if ch := decodeChange(t, rows[0]); ch.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_UPDATED {
		t.Fatalf("want UPDATED change, got %v", ch.GetChangeType())
	}
	// A NotFound update must not leave an outbox row behind.
	_, err = client.UpdateListing(ctx, &listingv1.UpdateListingRequest{
		Listing: &listingv1.Listing{Id: "missing", Title: "n"},
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound updating missing, got %v", err)
	}
	if got := len(txw.Rows()); got != 1 {
		t.Fatalf("want still 1 outbox row after failed update, got %d", got)
	}
}

func TestUpdateListing_NotOwner(t *testing.T) {
	// Listing owned by someone else; u1 tries to update it → PermissionDenied.
	repo := repository.NewInMemoryListingRepository(
		repository.Listing{ID: "y1", Title: "theirs", Currency: "VND", Status: "published", SellerID: "other-user"},
	)
	client := startServer(t, repo)
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	_, err := client.UpdateListing(ctx, &listingv1.UpdateListingRequest{
		Listing: &listingv1.Listing{Id: "y1", Title: "hijack", Currency: "VND"},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied editing another's listing, got %v", err)
	}
}

func TestListMyListings_OwnerOnly(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(
		repository.Listing{ID: "m1", Title: "mine1", Currency: "VND", Status: "published", SellerID: "u1"},
		repository.Listing{ID: "m2", Title: "mine2", Currency: "VND", Status: "draft", SellerID: "u1"},
		repository.Listing{ID: "o1", Title: "theirs", Currency: "VND", Status: "published", SellerID: "other"},
	)
	client := startServer(t, repo)
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	resp, err := client.ListMyListings(ctx, &listingv1.ListMyListingsRequest{})
	if err != nil {
		t.Fatalf("ListMyListings: %v", err)
	}
	if resp.GetPage().GetTotal() != 2 || len(resp.GetListings()) != 2 {
		t.Fatalf("want 2 of u1's listings, got total=%d n=%d", resp.GetPage().GetTotal(), len(resp.GetListings()))
	}
	for _, l := range resp.GetListings() {
		if l.GetSellerId() != "u1" {
			t.Fatalf("leaked another owner's listing: %+v", l)
		}
	}
}

func TestDeleteListing_OwnerAndNotOwner(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(
		repository.Listing{ID: "d1", Title: "mine", Currency: "VND", Status: "published", SellerID: "u1"},
		repository.Listing{ID: "d2", Title: "theirs", Currency: "VND", Status: "published", SellerID: "other"},
	)
	txw := repository.NewInMemoryTxWriter(repo)
	client := startServerOutbox(t, repo, txw)
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	// Deleting another owner's listing is denied — and enqueues nothing.
	_, err := client.DeleteListing(ctx, &listingv1.DeleteListingRequest{Id: "d2"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied deleting another's, got %v", err)
	}
	if got := len(txw.Rows()); got != 0 {
		t.Fatalf("want no outbox row on denied delete, got %d", got)
	}

	// Deleting own listing succeeds and enqueues a DELETED event.
	if _, err := client.DeleteListing(ctx, &listingv1.DeleteListingRequest{Id: "d1"}); err != nil {
		t.Fatalf("DeleteListing: %v", err)
	}
	rows := txw.Rows()
	if len(rows) != 1 {
		t.Fatalf("want 1 enqueued outbox row, got %d", len(rows))
	}
	if ch := decodeChange(t, rows[0]); ch.GetChangeType() != listingv1.ChangeType_CHANGE_TYPE_DELETED {
		t.Fatalf("want DELETED change, got %v", ch.GetChangeType())
	}
	// It is gone now.
	if _, err := client.GetListing(ctx, &listingv1.GetListingRequest{Id: "d1"}); status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound after delete, got %v", err)
	}
}

func TestCreateListing_RequiresWriteScope(t *testing.T) {
	// Buyer-level principal (read only) → CreateListing denied.
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	_, err := client.CreateListing(ctx, &listingv1.CreateListingRequest{
		Listing: &listingv1.Listing{Title: "x", Currency: "VND"},
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied without listing.write, got %v", err)
	}
}

func TestGetImageUploadUrl_Success(t *testing.T) {
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read,listing.write")
	defer cancel()

	resp, err := client.GetImageUploadUrl(ctx, &listingv1.GetImageUploadUrlRequest{
		ContentType: "image/jpeg",
		Filename:    "photo.jpg",
	})
	if err != nil {
		t.Fatalf("GetImageUploadUrl: %v", err)
	}
	if resp.GetUploadUrl() == "" {
		t.Fatal("expected non-empty upload_url")
	}
	if resp.GetImageKey() == "" {
		t.Fatal("expected non-empty image_key")
	}
	if resp.GetPublicUrl() == "" {
		t.Fatal("expected non-empty public_url")
	}
}

func TestGetImageUploadUrl_RequiresWriteScope(t *testing.T) {
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	_, err := client.GetImageUploadUrl(ctx, &listingv1.GetImageUploadUrlRequest{
		ContentType: "image/jpeg",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied without listing.write, got %v", err)
	}
}

func TestListCategories_Success(t *testing.T) {
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.ListCategories(ctx, &listingv1.ListCategoriesRequest{})
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(resp.GetCategories()) != 3 {
		t.Fatalf("want 3 seeded categories, got %d", len(resp.GetCategories()))
	}
	if resp.GetCategories()[0].GetId() != "cat-electronics" {
		t.Fatalf("expected first category cat-electronics, got %s", resp.GetCategories()[0].GetId())
	}
}

func TestGetCategory_Success(t *testing.T) {
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.GetCategory(ctx, &listingv1.GetCategoryRequest{Id: "cat-electronics"})
	if err != nil {
		t.Fatalf("GetCategory: %v", err)
	}
	if resp.GetCategory().GetName() != "Điện tử & Công nghệ" {
		t.Fatalf("unexpected category name: %s", resp.GetCategory().GetName())
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	client := startServer(t, repository.NewInMemoryListingRepository())
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	_, err := client.GetCategory(ctx, &listingv1.GetCategoryRequest{Id: "missing-cat"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestReserveStock_Success(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID:    "prod-1",
		Title: "Phone",
		Stock: 10,
	})
	client := startServer(t, repo)
	ctx, cancel := principalCtxAs(t, "service-team-order", "service", "listing.read,listing.write")
	defer cancel()

	resp, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{
		ListingId:     "prod-1",
		Quantity:      3,
		ReservationId: "r1",
	})
	if err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatalf("expected ReserveStock success, got message: %s", resp.GetMessage())
	}

	// Verify remaining stock
	got, _ := repo.Get(ctx, "prod-1")
	if got.Stock != 7 {
		t.Fatalf("want 7 remaining stock, got %d", got.Stock)
	}
}

func TestReserveStock_OutOfStock(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID:    "prod-1",
		Title: "Phone",
		Stock: 2,
	})
	client := startServer(t, repo)
	ctx, cancel := principalCtxAs(t, "service-team-order", "service", "listing.read,listing.write")
	defer cancel()

	resp, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{
		ListingId:     "prod-1",
		Quantity:      5,
		ReservationId: "r1",
	})
	if err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	if resp.GetSuccess() {
		t.Fatal("expected failure on out-of-stock reserve")
	}
}

func TestReleaseStock_Success(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{
		ID:    "prod-1",
		Title: "Phone",
		Stock: 10,
	})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 3, ReservationId: "r1"}); err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	// The request quantity is ignored: the stored 3 is restored, once.
	for i := 0; i < 2; i++ {
		resp, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ListingId: "prod-1", Quantity: 99, ReservationId: "r1"})
		if err != nil {
			t.Fatalf("ReleaseStock #%d: %v", i+1, err)
		}
		if !resp.GetSuccess() {
			t.Fatalf("expected ReleaseStock #%d success", i+1)
		}
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 10 {
		t.Fatalf("want 10 stock after release, got %d", got.Stock)
	}
	// Unknown id: successful no-op.
	if _, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ListingId: "prod-1", Quantity: 5, ReservationId: "never-reserved"}); err != nil {
		t.Fatalf("ReleaseStock unknown id: %v", err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 10 {
		t.Fatalf("unknown-id release changed stock to %d", got.Stock)
	}
}

// ReleaseStock without a reservation_id is INVALID_ARGUMENT and leaves stock
// unchanged (the blind stock = stock + quantity path is gone).
func TestReleaseStock_EmptyReservationIDRejected(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 7})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	_, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ListingId: "prod-1", Quantity: 3})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 7 {
		t.Fatalf("stock changed to %d", got.Stock)
	}
}

// TestStockRPCs_RequireServicePrincipal: ReserveStock/ReleaseStock are internal
// (called by team-order) — only a service principal with listing.write passes.
func TestStockRPCs_RequireServicePrincipal(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		typ    string // "" = no principal metadata at all
		scopes string
		want   codes.Code
	}{
		{"service allowed", "service-team-order", "service", "listing.read,listing.write", codes.OK},
		{"service without listing.write denied", "service-team-order", "service", "listing.read", codes.PermissionDenied},
		{"user denied", "u1", "user", "listing.read", codes.PermissionDenied},
		{"seller user with listing.write denied", "seller-1", "user", "listing.read,listing.write", codes.PermissionDenied},
		{"anonymous principal denied", "anon", "anonymous", "listing.read", codes.PermissionDenied},
		{"no principal denied", "", "", "", codes.Unauthenticated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
			client := startServer(t, repo)
			var ctx context.Context
			var cancel context.CancelFunc
			if tc.typ == "" {
				ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
			} else {
				ctx, cancel = principalCtxAs(t, tc.id, tc.typ, tc.scopes)
			}
			defer cancel()

			_, rerr := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 1, ReservationId: "r1"})
			if got := status.Code(rerr); got != tc.want {
				t.Fatalf("ReserveStock: want %v, got %v (%v)", tc.want, got, rerr)
			}
			_, lerr := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ListingId: "prod-1", Quantity: 1, ReservationId: "r1"})
			if got := status.Code(lerr); got != tc.want {
				t.Fatalf("ReleaseStock: want %v, got %v (%v)", tc.want, got, lerr)
			}
			// Denied callers must not have moved stock; allowed: reserve 1 then release 1 = unchanged.
			if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 10 {
				t.Fatalf("stock changed: %d", got.Stock)
			}
		})
	}
}

func serviceCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return principalCtxAs(t, "service-team-order", "service", "listing.read,listing.write")
}

func TestCommitReservation_Outcomes(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 2, ReservationId: "r1"}); err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	for i := 0; i < 2; i++ { // the second call is the idempotent repeat
		if _, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "r1"}); err != nil {
			t.Fatalf("CommitReservation #%d: %v", i+1, err)
		}
	}
	// A committed reservation is not restored by the sweep.
	if n, err := repo.SweepExpiredReservations(ctx, time.Now().Add(service.DefaultReservationTTL+time.Minute)); err != nil || n != 0 {
		t.Fatalf("sweep of committed: n=%d err=%v, want 0/nil", n, err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 8 {
		t.Fatalf("stock after sweep = %d, want 8", got.Stock)
	}

	// A reservation swept (released) before the commit fails FAILED_PRECONDITION.
	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 1, ReservationId: "r2"}); err != nil {
		t.Fatalf("ReserveStock r2: %v", err)
	}
	if n, err := repo.SweepExpiredReservations(ctx, time.Now().Add(service.DefaultReservationTTL+time.Minute)); err != nil || n != 1 {
		t.Fatalf("sweep n=%d err=%v, want 1/nil", n, err)
	}
	if _, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "r2"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("released id: want FailedPrecondition, got %v", err)
	}
	if _, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "missing"}); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown id: want NotFound, got %v", err)
	}
	if _, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty id: want InvalidArgument, got %v", err)
	}
}

// CommitReservation needs the same authority as the other stock RPCs: a service
// principal holding listing.write. A denied caller leaves the reservation active.
func TestCommitReservation_RequiresServicePrincipal(t *testing.T) {
	cases := []struct {
		name   string
		id     string
		typ    string // "" = no principal metadata at all
		scopes string
		want   codes.Code
	}{
		{"service without listing.write denied", "service-team-order", "service", "listing.read", codes.PermissionDenied},
		{"user denied", "u1", "user", "listing.read", codes.PermissionDenied},
		{"seller user with listing.write denied", "seller-1", "user", "listing.read,listing.write", codes.PermissionDenied},
		{"anonymous principal denied", "anon", "anonymous", "listing.read", codes.PermissionDenied},
		{"no principal denied", "", "", "", codes.Unauthenticated},
		{"service allowed", "service-team-order", "service", "listing.read,listing.write", codes.OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
			client := startServer(t, repo)
			sctx, scancel := serviceCtx(t)
			defer scancel()
			if _, err := client.ReserveStock(sctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 2, ReservationId: "r1"}); err != nil {
				t.Fatalf("ReserveStock: %v", err)
			}

			var ctx context.Context
			var cancel context.CancelFunc
			if tc.typ == "" {
				ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
			} else {
				ctx, cancel = principalCtxAs(t, tc.id, tc.typ, tc.scopes)
			}
			defer cancel()
			_, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "r1"})
			if got := status.Code(err); got != tc.want {
				t.Fatalf("CommitReservation: want %v, got %v (%v)", tc.want, got, err)
			}

			// Only an allowed commit protects the stock from the sweep.
			n, err := repo.SweepExpiredReservations(sctx, time.Now().Add(service.DefaultReservationTTL+time.Minute))
			if err != nil {
				t.Fatalf("sweep: %v", err)
			}
			wantSwept := 1
			if tc.want == codes.OK {
				wantSwept = 0
			}
			if n != wantSwept {
				t.Fatalf("sweep released %d, want %d", n, wantSwept)
			}
		})
	}
}

// ReserveStock without a reservation_id is INVALID_ARGUMENT with stock unchanged:
// there is no ledger-less decrement.
func TestReserveStock_EmptyReservationIDRejected(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	_, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 3})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("want InvalidArgument, got %v", err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 10 {
		t.Fatalf("stock changed to %d", got.Stock)
	}
}

// reserve -> release -> reserve again under the same id fails FAILED_PRECONDITION
// and leaves stock unchanged (the caller must use a new id).
func TestReserveStock_ReleasedIDFailsPrecondition(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	req := &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 3, ReservationId: "r1"}
	if _, err := client.ReserveStock(ctx, req); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ReservationId: "r1"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, err := client.ReserveStock(ctx, req); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("re-reserve of released id: want FailedPrecondition, got %v", err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 10 {
		t.Fatalf("stock = %d, want 10", got.Stock)
	}
}

// With the real builder wired, a reserve and a release each announce a decodable
// ListingStockChanged envelope carrying the post-change stock; a commit and a
// repeated release announce nothing.
func TestStockEvents_WireEnvelope(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10}).
		WithStockEvents(handler.NewStockEventBuilder())
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()

	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 3, ReservationId: "r1"}); err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	if _, err := client.CommitReservation(ctx, &listingv1.CommitReservationRequest{ReservationId: "r1"}); err != nil {
		t.Fatalf("CommitReservation: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{ReservationId: "r1"}); err != nil {
			t.Fatalf("ReleaseStock: %v", err)
		}
	}
	rows := repo.StockEventRows()
	if len(rows) != 2 {
		t.Fatalf("want 2 stock events, got %d", len(rows))
	}
	for i, want := range []int32{7, 10} {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(rows[i].Payload, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetType() != "platform.listing.v1.ListingStockChanged" || env.GetEventId() != rows[i].EventID || env.GetOccurredAt() == nil {
			t.Fatalf("bad envelope: %+v", &env)
		}
		if env.GetPrincipal().GetId() != "service-team-order" {
			t.Errorf("principal = %q, want service-team-order", env.GetPrincipal().GetId())
		}
		var ev listingv1.ListingStockChanged
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatalf("unmarshal ListingStockChanged: %v", err)
		}
		if ev.GetListingId() != "prod-1" || ev.GetStock() != want || rows[i].AggregateID != "prod-1" {
			t.Errorf("event %d = %+v, want listing prod-1 stock %d", i, &ev, want)
		}
	}
}

func TestReserveStock_MismatchedRetryIsFailedPrecondition(t *testing.T) {
	repo := repository.NewInMemoryListingRepository(repository.Listing{ID: "prod-1", Title: "Phone", Stock: 10})
	client := startServer(t, repo)
	ctx, cancel := serviceCtx(t)
	defer cancel()
	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 2, ReservationId: "r1"}); err != nil {
		t.Fatalf("ReserveStock: %v", err)
	}
	if _, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 2, ReservationId: "r1"}); err != nil {
		t.Fatalf("identical retry must succeed: %v", err)
	}
	_, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{ListingId: "prod-1", Quantity: 5, ReservationId: "r1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("mismatched retry: %v, want FailedPrecondition", err)
	}
	if got, _ := repo.Get(ctx, "prod-1"); got.Stock != 8 {
		t.Fatalf("stock = %d, want 8", got.Stock)
	}
}
