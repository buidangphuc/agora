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
