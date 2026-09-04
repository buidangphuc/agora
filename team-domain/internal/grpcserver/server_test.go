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
		Page: &commonv1.PageRequest{PageSize: 2},
	})
	if err != nil {
		t.Fatalf("ListListings: %v", err)
	}
	if len(resp.GetListings()) != 2 || resp.GetPage().GetTotal() != 3 {
		t.Fatalf("want 2 items of total 3, got %d/%d", len(resp.GetListings()), resp.GetPage().GetTotal())
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
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{
		ListingId: "prod-1",
		Quantity:  3,
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
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.ReserveStock(ctx, &listingv1.ReserveStockRequest{
		ListingId: "prod-1",
		Quantity:  5,
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
		Stock: 7,
	})
	client := startServer(t, repo)
	ctx, cancel := principalCtx(t, "listing.read")
	defer cancel()

	resp, err := client.ReleaseStock(ctx, &listingv1.ReleaseStockRequest{
		ListingId: "prod-1",
		Quantity:  3,
	})
	if err != nil {
		t.Fatalf("ReleaseStock: %v", err)
	}
	if !resp.GetSuccess() {
		t.Fatal("expected ReleaseStock success")
	}

	got, _ := repo.Get(ctx, "prod-1")
	if got.Stock != 10 {
		t.Fatalf("want 10 stock after release, got %d", got.Stock)
	}
}


