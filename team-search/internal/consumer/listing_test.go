package consumer_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/consumer"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/retrieval"
)

type mockIndex struct {
	lastUpsertDoc  index.ListingDoc
	lastPartialDoc map[string]interface{}
	lastPartialID  string
	lastDeleteID   string
}

func (m *mockIndex) EnsureIndex(ctx context.Context) error { return nil }
func (m *mockIndex) Upsert(ctx context.Context, doc index.ListingDoc) error {
	m.lastUpsertDoc = doc
	return nil
}
func (m *mockIndex) PartialUpdate(ctx context.Context, id string, partialDoc map[string]interface{}) error {
	m.lastPartialID = id
	m.lastPartialDoc = partialDoc
	return nil
}
func (m *mockIndex) Delete(ctx context.Context, id string) error {
	m.lastDeleteID = id
	return nil
}
func (m *mockIndex) Search(ctx context.Context, query string, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	return index.SearchResult{}, nil
}
func (m *mockIndex) SearchVector(ctx context.Context, vector []float32, filters map[string]string, categoryID string, minPrice, maxPrice int64, minRating int32, sortBy searchv1.SortBy, from, size int) (index.SearchResult, error) {
	return index.SearchResult{}, nil
}
func (m *mockIndex) Suggest(ctx context.Context, prefix string, limit int) ([]string, error) {
	return nil, nil
}

func makeEnvelope(t *testing.T, eventType string, msg proto.Message) []byte {
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := &eventsv1.EventEnvelope{
		EventId:    "evt-1",
		Type:       eventType,
		OccurredAt: timestamppb.Now(),
		Payload:    payload,
	}
	envBytes, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return envBytes
}

func TestListingEventHandler_VectorizesPublishedListing(t *testing.T) {
	idx := &mockIndex{}
	embed := &retrieval.MockEmbedClient{Dim: 4}
	handler := consumer.ListingEventHandlerWithEmbedder(idx, embed)

	changed := &listingv1.ListingChanged{
		Listing: &listingv1.Listing{
			Id:          "l-100",
			Title:       "Giày chạy bộ",
			Description: "Chất liệu êm ái",
			Price:       500000,
			Status:      listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
		},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
	}

	envBytes := makeEnvelope(t, "platform.listing.v1.ListingChanged", changed)
	err := handler(context.Background(), []byte("l-100"), envBytes)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if idx.lastUpsertDoc.ID != "l-100" {
		t.Errorf("expected upsert doc ID l-100, got %s", idx.lastUpsertDoc.ID)
	}
	if len(idx.lastUpsertDoc.Embedding) != 4 {
		t.Errorf("expected 4-dim embedding vector, got %d", len(idx.lastUpsertDoc.Embedding))
	}
	if idx.lastUpsertDoc.VectorPending {
		t.Errorf("expected VectorPending to be false")
	}
}

func TestListingEventHandler_EmbeddingFailureMarksVectorPending(t *testing.T) {
	idx := &mockIndex{}
	embed := &retrieval.MockEmbedClient{Err: errors.New("model server down")}
	handler := consumer.ListingEventHandlerWithEmbedder(idx, embed)

	changed := &listingv1.ListingChanged{
		Listing: &listingv1.Listing{
			Id:          "l-101",
			Title:       "Áo khoác",
			Description: "Màu đen",
			Price:       300000,
			Status:      listingv1.ListingStatus_LISTING_STATUS_PUBLISHED,
		},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
	}

	envBytes := makeEnvelope(t, "platform.listing.v1.ListingChanged", changed)
	err := handler(context.Background(), []byte("l-101"), envBytes)
	if err != nil {
		t.Fatalf("handler should fail-open (no error to DLQ), got: %v", err)
	}

	if idx.lastUpsertDoc.ID != "l-101" {
		t.Errorf("expected upsert doc ID l-101, got %s", idx.lastUpsertDoc.ID)
	}
	if !idx.lastUpsertDoc.VectorPending {
		t.Errorf("expected VectorPending = true when embedder fails")
	}
}

func TestListingEventHandler_PricingChangePreservesEmbedding(t *testing.T) {
	idx := &mockIndex{}
	embed := &retrieval.MockEmbedClient{Dim: 4}
	handler := consumer.ListingEventHandlerWithEmbedder(idx, embed)

	pricing := &listingv1.ListingPricingChanged{
		ListingId:     "l-102",
		OriginalPrice: 450000,
		Currency:      "VND",
	}

	envBytes := makeEnvelope(t, "platform.listing.v1.ListingPricingChanged", pricing)
	err := handler(context.Background(), []byte("l-102"), envBytes)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if idx.lastPartialID != "l-102" {
		t.Errorf("expected partial update ID l-102, got %s", idx.lastPartialID)
	}
	// Partial update must only touch price/currency/version, not overwrite embedding
	if _, ok := idx.lastPartialDoc["embedding"]; ok {
		t.Errorf("pricing change must not touch embedding field")
	}
	if idx.lastPartialDoc["price"] != int64(450000) {
		t.Errorf("expected price 450000, got %v", idx.lastPartialDoc["price"])
	}
}
