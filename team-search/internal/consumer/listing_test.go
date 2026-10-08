package consumer_test

import (
	"context"
	"errors"
	"testing"
	"time"

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
	lastDeleteVer  int64
	deletes        int
	stockCalls     []stockCall
}

type stockCall struct {
	id      string
	stock   int32
	version int64
}

func (m *mockIndex) UpdateStock(ctx context.Context, id string, stock int32, version int64) error {
	m.stockCalls = append(m.stockCalls, stockCall{id, stock, version})
	return nil
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
func (m *mockIndex) Delete(ctx context.Context, id string, version int64) error {
	m.lastDeleteID = id
	m.lastDeleteVer = version
	m.deletes++
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

func envelopeAt(t *testing.T, eventType string, msg proto.Message, at time.Time) []byte {
	t.Helper()
	payload, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := &eventsv1.EventEnvelope{EventId: "evt", Type: eventType, Payload: payload}
	if !at.IsZero() {
		env.OccurredAt = timestamppb.New(at)
	}
	b, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

// D5: every delete path writes a tombstone carrying the delete event's version
// (occurred_at in ns), never a plain unversioned delete.
func TestListingEventHandler_DeletePathsCarryVersion(t *testing.T) {
	at := time.Unix(1700000000, 123)
	cases := map[string]struct {
		typ string
		msg proto.Message
	}{
		"ListingChanged DELETED": {"platform.listing.v1.ListingChanged", &listingv1.ListingChanged{
			Listing: &listingv1.Listing{Id: "l-del"}, ChangeType: listingv1.ChangeType_CHANGE_TYPE_DELETED}},
		"ListingBaseInfoChanged DELETED": {"platform.listing.v1.ListingBaseInfoChanged", &listingv1.ListingBaseInfoChanged{
			ListingId: "l-del", ChangeType: listingv1.ChangeType_CHANGE_TYPE_DELETED}},
		"ListingStatusChanged REJECTED": {"platform.listing.v1.ListingStatusChanged", &listingv1.ListingStatusChanged{
			ListingId: "l-del", Status: listingv1.ListingStatus_LISTING_STATUS_REJECTED}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			idx := &mockIndex{}
			if err := consumer.ListingEventHandler(idx)(context.Background(), nil, envelopeAt(t, tc.typ, tc.msg, at)); err != nil {
				t.Fatalf("handler: %v", err)
			}
			if idx.deletes != 1 || idx.lastDeleteID != "l-del" || idx.lastDeleteVer != at.UnixNano() {
				t.Errorf("want one Delete(l-del, %d), got %d x (%q, %d)", at.UnixNano(), idx.deletes, idx.lastDeleteID, idx.lastDeleteVer)
			}
			if idx.lastUpsertDoc.ID != "" || idx.lastPartialID != "" {
				t.Errorf("a delete must not upsert or partially update")
			}
		})
	}
}

// D2: a valid ListingStockChanged is projected with the envelope version; the
// listing id, stock and occurred_at are validated so a malformed event is an
// error (AD1 then retries and parks it), never a silent ack.
func TestListingEventHandler_StockChanged(t *testing.T) {
	at := time.Unix(1700000000, 5)
	const typ = "platform.listing.v1.ListingStockChanged"
	idx := &mockIndex{}
	h := consumer.ListingEventHandler(idx)
	if err := h(context.Background(), nil, envelopeAt(t, typ, &listingv1.ListingStockChanged{ListingId: "l1", Stock: 0}, at)); err != nil {
		t.Fatalf("valid stock event: %v", err)
	}
	if len(idx.stockCalls) != 1 || idx.stockCalls[0] != (stockCall{"l1", 0, at.UnixNano()}) {
		t.Fatalf("want UpdateStock(l1, 0, %d), got %+v", at.UnixNano(), idx.stockCalls)
	}
	bad := map[string][]byte{
		"empty id":            envelopeAt(t, typ, &listingv1.ListingStockChanged{Stock: 3}, at),
		"negative stock":      envelopeAt(t, typ, &listingv1.ListingStockChanged{ListingId: "l1", Stock: -1}, at),
		"missing occurred_at": envelopeAt(t, typ, &listingv1.ListingStockChanged{ListingId: "l1", Stock: 3}, time.Time{}),
	}
	for name, env := range bad {
		if err := h(context.Background(), nil, env); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(idx.stockCalls) != 1 {
		t.Errorf("a malformed stock event reached the index: %+v", idx.stockCalls)
	}
}

// D2: every ListingChanged carries Listing.stock into the upsert (the index
// decides under stock_version whether it wins).
func TestListingEventHandler_ListingChangedCarriesStock(t *testing.T) {
	idx := &mockIndex{}
	env := envelopeAt(t, "platform.listing.v1.ListingChanged", &listingv1.ListingChanged{
		Listing:    &listingv1.Listing{Id: "l1", Stock: 0, Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_UPDATED,
	}, time.Unix(1700000000, 0))
	if err := consumer.ListingEventHandler(idx)(context.Background(), nil, env); err != nil {
		t.Fatal(err)
	}
	if idx.lastUpsertDoc.Stock == nil || *idx.lastUpsertDoc.Stock != 0 {
		t.Errorf("want stock 0 carried (present), got %v", idx.lastUpsertDoc.Stock)
	}
}
