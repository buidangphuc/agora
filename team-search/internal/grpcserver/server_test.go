package grpcserver_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"

	"github.com/buidangphuc/team-search/internal/config"
	"github.com/buidangphuc/team-search/internal/grpcserver"
	"github.com/buidangphuc/team-search/internal/handler"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/repository"
)

// fakeIndex is an in-memory index.Index for tests (no OpenSearch needed).
type fakeIndex struct{ docs []index.ListingDoc }

func (f *fakeIndex) EnsureIndex(context.Context) error { return nil }
func (f *fakeIndex) Upsert(_ context.Context, d index.ListingDoc) error {
	f.docs = append(f.docs, d)
	return nil
}
func (f *fakeIndex) PartialUpdate(_ context.Context, id string, partialDoc map[string]interface{}) error {
	for i, d := range f.docs {
		if d.ID == id {
			if price, ok := partialDoc["price"].(int64); ok {
				d.Price = price
			}
			if status, ok := partialDoc["status"].(string); ok {
				d.Status = status
			}
			f.docs[i] = d
			break
		}
	}
	return nil
}
func (f *fakeIndex) Delete(context.Context, string) error { return nil }

func (f *fakeIndex) Search(_ context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	var matched []index.Hit
	for _, d := range f.docs {
		if query != "" && !strings.Contains(strings.ToLower(d.Title+" "+d.Description), strings.ToLower(query)) {
			continue
		}
		if s, ok := filters["status"]; ok && d.Status != s {
			continue
		}
		matched = append(matched, index.Hit{ListingID: d.ID, Score: 1})
	}
	total := int64(len(matched))
	if from > len(matched) {
		from = len(matched)
	}
	end := from + size
	if end > len(matched) {
		end = len(matched)
	}
	return index.SearchResult{Hits: matched[from:end], Total: total}, nil
}

func (f *fakeIndex) Suggest(_ context.Context, prefix string, limit int) ([]string, error) {
	var out []string
	for _, d := range f.docs {
		if strings.HasPrefix(strings.ToLower(d.Title), strings.ToLower(prefix)) {
			out = append(out, d.Title)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func testSettings() *config.Settings {
	s := &config.Settings{}
	s.Server.Port = 0
	s.Server.ReflectionEnabled = false
	return s
}

func startServer(t *testing.T, idx index.Index) searchv1.SearchServiceClient {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := grpcserver.Build(testSettings(), handler.NewSearchHandler(idx, repository.NewInMemorySavedSearchRepository()), nil, logger)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(lis) }()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(); srv.Stop() })
	return searchv1.NewSearchServiceClient(conn)
}

// principalCtx mimics the gateway forwarding a resolved Principal.
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

func seeded() *fakeIndex {
	return &fakeIndex{docs: []index.ListingDoc{
		{ID: "a1", Title: "iPhone 15 Pro", Description: "phone", Status: "published", Currency: "VND"},
		{ID: "b2", Title: "iPad Air", Description: "tablet", Status: "draft", Currency: "VND"},
		{ID: "c3", Title: "iPhone 14", Description: "phone", Status: "published", Currency: "VND"},
	}}
}

func TestSearchListings_FreeTextAndFilter(t *testing.T) {
	client := startServer(t, seeded())
	ctx, cancel := principalCtx(t, "search:read")
	defer cancel()

	resp, err := client.SearchListings(ctx, &searchv1.SearchListingsRequest{
		Query:   "iphone",
		Filters: map[string]string{"status": "published"},
	})
	if err != nil {
		t.Fatalf("SearchListings: %v", err)
	}
	if resp.GetPage().GetTotal() != 2 {
		t.Fatalf("want 2 published iphones, got total=%d hits=%d", resp.GetPage().GetTotal(), len(resp.GetHits()))
	}
	for _, h := range resp.GetHits() {
		if h.GetListingId() != "a1" && h.GetListingId() != "c3" {
			t.Fatalf("unexpected hit %q", h.GetListingId())
		}
	}
}

func TestSuggest(t *testing.T) {
	client := startServer(t, seeded())
	ctx, cancel := principalCtx(t, "search:read")
	defer cancel()

	resp, err := client.Suggest(ctx, &searchv1.SuggestRequest{Query: "ipho", Limit: 5})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(resp.GetSuggestions()) != 2 {
		t.Fatalf("want 2 suggestions for 'ipho', got %v", resp.GetSuggestions())
	}
}

func TestSearch_Unauthenticated(t *testing.T) {
	client := startServer(t, seeded())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := client.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("want Unauthenticated, got %v", err)
	}
}

func TestSearch_InsufficientScope(t *testing.T) {
	client := startServer(t, seeded())
	ctx, cancel := principalCtx(t, "other.scope")
	defer cancel()

	_, err := client.SearchListings(ctx, &searchv1.SearchListingsRequest{Query: "x"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("want PermissionDenied, got %v", err)
	}
}
