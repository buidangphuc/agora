package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	analyticsv1 "github.com/buidangphuc/team-gateway/generated/platform/analytics/v1"
	commonv1 "github.com/buidangphuc/team-gateway/generated/platform/common/v1"
)

type mockAnalyticsPublisher struct {
	events []*analyticsv1.TrackingEvent
}

func (m *mockAnalyticsPublisher) PublishTrackingEvent(
	ctx context.Context,
	ev *analyticsv1.TrackingEvent,
	principal *commonv1.Principal,
	requestID string,
) error {
	m.events = append(m.events, ev)
	return nil
}

func (m *mockAnalyticsPublisher) Close() {}

func TestHandleTrackWithAttribution(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := HandleTrack(edge, pub, logger)

	beacon := trackBeacon{
		Type:         "view",
		ListingID:    "listing-123",
		SessionID:    "sess-abc",
		AnonymousID:  "anon-xyz",
		Path:         "/listing/listing-123",
		Referrer:     "/home",
		Position:     1,
		Query:        "sneakers",
		PlacementID:  "home_feed",
		ImpressionID: "imp-uuid-999",
		ModelVersion: "als_v1",
		Properties: map[string]string{
			"source": "widget",
		},
	}

	body, err := json.Marshal(beacon)
	if err != nil {
		t.Fatalf("marshal beacon: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rec.Code)
	}

	if len(pub.events) != 1 {
		t.Fatalf("expected 1 published event, got %d", len(pub.events))
	}

	ev := pub.events[0]
	if ev.GetPlacementId() != "home_feed" {
		t.Errorf("expected placement_id 'home_feed', got %q", ev.GetPlacementId())
	}
	if ev.GetImpressionId() != "imp-uuid-999" {
		t.Errorf("expected impression_id 'imp-uuid-999', got %q", ev.GetImpressionId())
	}
	if ev.GetModelVersion() != "als_v1" {
		t.Errorf("expected model_version 'als_v1', got %q", ev.GetModelVersion())
	}
	if ev.GetListingId() != "listing-123" {
		t.Errorf("expected listing_id 'listing-123', got %q", ev.GetListingId())
	}
}

func TestHandleTrackEcommerceBatchAndGA4Aliases(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := HandleTrack(edge, pub, logger)

	batch := []trackBeacon{
		{
			Type:          "view_item_list", // GA4 alias for impression
			ListingID:     "prod-1",
			SessionID:     "sess-1",
			AnonymousID:   "anon-1",
			Price:         50000,
			Currency:      "VND",
			ItemListID:    "search_results",
			Position:      1,
			EventGroupID:  "group-abc",
			ItemCategory:  "Fashion",
		},
		{
			Type:          "purchase",
			ListingID:     "prod-2",
			SessionID:     "sess-1",
			AnonymousID:   "anon-1",
			Value:         150000,
			Price:         100000,
			Quantity:      1,
			Currency:      "VND",
			TransactionID: "order-999",
			EventGroupID:  "group-abc",
			ShippingTier:  "SPX_EXPRESS",
			PaymentType:   "MOCK_WALLET",
		},
	}

	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rec.Code)
	}

	if len(pub.events) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(pub.events))
	}

	ev1 := pub.events[0]
	if ev1.GetEventType() != analyticsv1.EventType_EVENT_TYPE_IMPRESSION {
		t.Errorf("expected EVENT_TYPE_IMPRESSION, got %v", ev1.GetEventType())
	}
	if ev1.GetPrice() != 50000 || ev1.GetCurrency() != "VND" || ev1.GetEventGroupId() != "group-abc" {
		t.Errorf("mismatched ecommerce fields on ev1: %+v", ev1)
	}

	ev2 := pub.events[1]
	if ev2.GetEventType() != analyticsv1.EventType_EVENT_TYPE_PURCHASE {
		t.Errorf("expected EVENT_TYPE_PURCHASE, got %v", ev2.GetEventType())
	}
	if ev2.GetTransactionId() != "order-999" || ev2.GetShippingTier() != "SPX_EXPRESS" || ev2.GetPaymentType() != "MOCK_WALLET" {
		t.Errorf("mismatched ecommerce fields on ev2: %+v", ev2)
	}
}

func TestHandleTrackExceedsBatchLimit(t *testing.T) {
	edge := &Edge{
		publicScopes: []string{"public"},
	}
	pub := &mockAnalyticsPublisher{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := HandleTrack(edge, pub, logger)

	oversized := make([]trackBeacon, 101)
	for i := range oversized {
		oversized[i] = trackBeacon{
			Type:      "view",
			ListingID: "item",
		}
	}

	body, err := json.Marshal(oversized)
	if err != nil {
		t.Fatalf("marshal oversized: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/track", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for batch exceeding maxBatchItems, got %d", rec.Code)
	}
}
