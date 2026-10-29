package events_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-search/generated/platform/events/v1"
	listingv1 "github.com/buidangphuc/team-search/generated/platform/listing/v1"
	"github.com/buidangphuc/team-search/internal/events"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/taxonomy"
)

type stockOnlyIndex struct {
	index.Index
	calls int
}

func (s *stockOnlyIndex) UpdateStock(context.Context, string, int32, int64) error {
	s.calls++
	return nil
}

// Scenario "A malformed stock event is retried and parked" (handler level):
// the handler returns an error for a malformed ListingStockChanged and never
// applies it; consume then retries and parks it (asserted in 1.8).
func TestListingEventHandler_MalformedStockEventReturnsError(t *testing.T) {
	const typ = "platform.listing.v1.ListingStockChanged"
	at := time.Unix(1700000000, 0)
	cases := map[string]*listingv1.ListingStockChanged{
		"empty listing_id": {Stock: 3},
		"negative stock":   {ListingId: "l1", Stock: -1},
	}
	t.Run("valid stock event is applied", func(t *testing.T) {
		idx := &stockOnlyIndex{}
		h := events.NewListingEventHandler(idx, nil, classified())
		ok := &listingv1.ListingStockChanged{ListingId: "l1", Stock: 3}
		if err := h(context.Background(), rec(envelopeAt(t, typ, ok, at))); err != nil {
			t.Fatal(err)
		}
		if idx.calls != 1 {
			t.Errorf("want 1 UpdateStock, got %d", idx.calls)
		}
	})
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			idx := &stockOnlyIndex{}
			h := events.NewListingEventHandler(idx, nil, classified())
			if err := h(context.Background(), rec(envelopeAt(t, typ, msg, at))); err == nil {
				t.Fatal("expected an error so consume retries and parks the record")
			}
			if idx.calls != 0 {
				t.Errorf("malformed stock event was applied %d times", idx.calls)
			}
		})
	}
	t.Run("missing occurred_at", func(t *testing.T) {
		payload, _ := proto.Marshal(&listingv1.ListingStockChanged{ListingId: "l1", Stock: 3})
		value, _ := proto.Marshal(&eventsv1.EventEnvelope{Type: typ, Payload: payload})
		idx := &stockOnlyIndex{}
		if err := events.NewListingEventHandler(idx, nil, classified())(context.Background(), rec(value)); err == nil {
			t.Fatal("expected an error")
		}
		if idx.calls != 0 {
			t.Errorf("applied %d times", idx.calls)
		}
	})
	t.Run("undecodable envelope", func(t *testing.T) {
		idx := &stockOnlyIndex{}
		if err := events.NewListingEventHandler(idx, nil, classified())(context.Background(), rec([]byte{0xff, 0xff, 0xff})); err == nil {
			t.Fatal("expected an error")
		}
	})
}

type captureHandler struct {
	slog.Handler
	recs *[]slog.Record
}

func (c captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (c captureHandler) Handle(_ context.Context, r slog.Record) error {
	*c.recs = append(*c.recs, r)
	return nil
}

// Scenario "A classifier outage does not fail ingestion": the degradation is
// logged with listing_id as a structured attribute, never inside the message.
func TestListingEventHandler_ClassifierOutageLogsListingIDAsAttribute(t *testing.T) {
	var recs []slog.Record
	prev := slog.Default()
	slog.SetDefault(slog.New(captureHandler{Handler: slog.NewTextHandler(io.Discard, nil), recs: &recs}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	idx := &mockIndex{}
	cls := &taxonomy.MockClassifier{Err: errors.New("team-ai down")}
	if err := events.NewListingEventHandler(idx, nil, cls)(context.Background(),
		rec(makeEnvelope(t, "platform.listing.v1.ListingChanged", tagChanged()))); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range recs {
		if r.Level < slog.LevelWarn {
			continue
		}
		if strings.Contains(r.Message, "l-tag") {
			t.Errorf("listing id must not be inside the message: %q", r.Message)
		}
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "listing_id" && a.Value.String() == "l-tag" {
				found = true
			}
			if strings.Contains(a.Value.String(), "team-ai down") && strings.Contains(a.Value.String(), "l-tag") {
				t.Errorf("listing id inside an error string: %q", a.Value.String())
			}
			return true
		})
	}
	if !found {
		t.Errorf("no warn record with listing_id attribute, got %d records", len(recs))
	}
}
