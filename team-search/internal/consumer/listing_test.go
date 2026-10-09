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
	"github.com/buidangphuc/team-search/internal/taxonomy"
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

func (m *mockIndex) PurgeTombstones(context.Context, time.Time) (int64, error) { return 0, nil }
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

// D7: only a CREATED event carries a creation time (its occurred_at, millis).
func TestListingEventHandler_CreatedAtOnlyFromCreated(t *testing.T) {
	at := time.UnixMilli(1700000000123)
	for ct, want := range map[listingv1.ChangeType]bool{
		listingv1.ChangeType_CHANGE_TYPE_CREATED: true,
		listingv1.ChangeType_CHANGE_TYPE_UPDATED: false,
	} {
		idx := &mockIndex{}
		env := envelopeAt(t, "platform.listing.v1.ListingChanged", &listingv1.ListingChanged{
			Listing: &listingv1.Listing{Id: "l1"}, ChangeType: ct}, at)
		if err := consumer.ListingEventHandler(idx)(context.Background(), nil, env); err != nil {
			t.Fatal(err)
		}
		got := idx.lastUpsertDoc.CreatedAt
		if want && (got == nil || *got != at.UnixMilli()) {
			t.Errorf("%v: created_at = %v, want %d", ct, got, at.UnixMilli())
		}
		if !want && got != nil {
			t.Errorf("%v must not carry created_at, got %d", ct, *got)
		}
	}
}

func classified() *taxonomy.MockClassifier {
	return &taxonomy.MockClassifier{Result: taxonomy.Classification{
		FacetTags: []string{"connectivity:bluetooth-5-3"},
		SKUs: []index.SkuDoc{
			{VariantID: "v1", Stock: 3, InStock: true, Attrs: []string{"color:xanh-navy", "capacity:512gb"}},
		},
	}}
}

func tagChanged(variants ...*listingv1.Variant) *listingv1.ListingChanged {
	return &listingv1.ListingChanged{
		Listing: &listingv1.Listing{
			Id: "l-tag", Title: "Tai nghe Bluetooth 5.3", Description: "ANC", CategoryId: "cat-electronics",
			Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED, Variants: variants,
		},
		ChangeType: listingv1.ChangeType_CHANGE_TYPE_CREATED,
	}
}

func TestListingEventHandler_IndexesClassifiedTagsAndSkus(t *testing.T) {
	idx := &mockIndex{}
	cls := classified()
	h := consumer.NewListingEventHandler(idx, nil, cls)
	v := &listingv1.Variant{Id: "v1", Name: "Xanh Navy / 512GB", Sku: "SKU-1", Price: 100, Stock: 3}
	if err := h(context.Background(), nil, makeEnvelope(t, "platform.listing.v1.ListingChanged", tagChanged(v))); err != nil {
		t.Fatal(err)
	}
	if len(cls.Calls) != 1 || cls.Calls[0].Title != "Tai nghe Bluetooth 5.3" || cls.Calls[0].CategoryID != "cat-electronics" ||
		len(cls.Calls[0].Variants) != 1 || cls.Calls[0].Variants[0].Name != "Xanh Navy / 512GB" || cls.Calls[0].Variants[0].Stock != 3 {
		t.Errorf("classifier input = %+v", cls.Calls)
	}
	d := idx.lastUpsertDoc
	if len(d.FacetTags) != 1 || d.FacetTags[0] != "connectivity:bluetooth-5-3" || len(d.SKUs) != 1 || d.SKUs[0].Attrs[0] != "color:xanh-navy" || d.TagsPending {
		t.Errorf("doc = %+v", d)
	}
}

// A classifier outage must not fail the ingestion or block the listing: the doc is
// written (searchable by title) and marked tags_pending so the index keeps old tags.
func TestListingEventHandler_ClassifierOutageDoesNotFailIngestion(t *testing.T) {
	idx := &mockIndex{}
	cls := &taxonomy.MockClassifier{Err: errors.New("team-ai down")}
	h := consumer.NewListingEventHandler(idx, nil, cls)
	if err := h(context.Background(), nil, makeEnvelope(t, "platform.listing.v1.ListingChanged", tagChanged())); err != nil {
		t.Fatalf("handler failed on classifier outage: %v", err)
	}
	d := idx.lastUpsertDoc
	if d.ID != "l-tag" || d.Title == "" || !d.TagsPending || len(d.FacetTags) != 0 || len(d.SKUs) != 0 {
		t.Errorf("doc = %+v, want indexed with tags_pending and no tags", d)
	}
}

func TestListingEventHandler_NilClassifierIndexesWithoutTags(t *testing.T) {
	idx := &mockIndex{}
	if err := consumer.NewListingEventHandler(idx, nil, nil)(context.Background(), nil,
		makeEnvelope(t, "platform.listing.v1.ListingChanged", tagChanged())); err != nil {
		t.Fatal(err)
	}
	if d := idx.lastUpsertDoc; d.TagsPending || d.FacetTags != nil || d.SKUs != nil {
		t.Errorf("doc = %+v", d)
	}
}

func TestListingBaseInfoChanged_RefreshesSpuTags(t *testing.T) {
	idx := &mockIndex{}
	cls := classified()
	h := consumer.NewListingEventHandler(idx, nil, cls)
	ev := &listingv1.ListingBaseInfoChanged{ListingId: "l-tag", Title: "Tai nghe Bluetooth 5.3", CategoryId: "cat-electronics", SellerId: "s1",
		Status: listingv1.ListingStatus_LISTING_STATUS_PUBLISHED}
	if err := h(context.Background(), nil, makeEnvelope(t, "platform.listing.v1.ListingBaseInfoChanged", ev)); err != nil {
		t.Fatal(err)
	}
	if got, _ := idx.lastPartialDoc["facet_tags"].([]string); len(got) != 1 || got[0] != "connectivity:bluetooth-5-3" {
		t.Errorf("partial = %v", idx.lastPartialDoc)
	}
	if idx.lastPartialDoc["tags_pending"] != false {
		t.Errorf("tags_pending = %v", idx.lastPartialDoc["tags_pending"])
	}
	if _, has := idx.lastPartialDoc["skus"]; has {
		t.Error("a base-info event carries no variants and must not touch skus")
	}

	cls.Err = errors.New("down")
	if err := h(context.Background(), nil, makeEnvelope(t, "platform.listing.v1.ListingBaseInfoChanged", ev)); err != nil {
		t.Fatalf("outage failed the event: %v", err)
	}
	if idx.lastPartialDoc["tags_pending"] != true {
		t.Errorf("outage must set tags_pending: %v", idx.lastPartialDoc)
	}
	if _, has := idx.lastPartialDoc["facet_tags"]; has {
		t.Error("outage must leave stored facet_tags untouched")
	}
}
