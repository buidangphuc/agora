package events_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
	"github.com/buidangphuc/team-engagement/internal/events"
)

func TestFactRowBuildsEnvelope(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "u-1", Type: auth.TypeUser, Scopes: []string{"engagement:write"}})
	ctx = reqctx.WithRequestID(ctx, "r-9")
	fact := &engagementv1.FavoriteAdded{UserId: "u-1", ListingId: "L1"}

	row, err := events.FactRow(ctx, "Listing", "L1", fact)
	if err != nil {
		t.Fatal(err)
	}
	if row.AggregateType != "Listing" || row.AggregateID != "L1" ||
		row.EventType != "platform.engagement.v1.FavoriteAdded" || row.RequestID != "r-9" || row.EventID == "" {
		t.Fatalf("row = %+v", row)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("row payload is not an EventEnvelope: %v", err)
	}
	if env.GetEventId() != row.EventID || env.GetType() != "platform.engagement.v1.FavoriteAdded" ||
		env.GetRequestId() != "r-9" || env.GetOccurredAt().AsTime().IsZero() ||
		env.GetPrincipal().GetId() != "u-1" || env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		t.Fatalf("envelope = %v", &env)
	}
	var got engagementv1.FavoriteAdded
	if err := proto.Unmarshal(env.GetPayload(), &got); err != nil || !proto.Equal(&got, fact) {
		t.Fatalf("payload = %v err=%v, want %v", &got, err, fact)
	}

	again, err := events.FactRow(ctx, "Listing", "L1", fact)
	if err != nil || again.EventID == row.EventID {
		t.Fatalf("event id must be fresh per fact: %q vs %q err=%v", again.EventID, row.EventID, err)
	}
}

func TestFactRowWithoutPrincipalHasNone(t *testing.T) {
	ctx := reqctx.WithRequestID(context.Background(), "r-3")

	row, err := events.FactRow(ctx, "Seller", "S1", &engagementv1.SellerFollowed{UserId: "u", SellerId: "S1"})
	if err != nil {
		t.Fatal(err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetPrincipal() != nil {
		t.Fatalf("principal = %v, want none", env.GetPrincipal())
	}
	if env.GetRequestId() != "r-3" || row.RequestID != "r-3" {
		t.Fatalf("request id = %q / %q, want r-3", env.GetRequestId(), row.RequestID)
	}
	if env.GetType() != "platform.engagement.v1.SellerFollowed" || row.AggregateType != "Seller" || row.AggregateID != "S1" {
		t.Fatalf("envelope = %v row = %+v", &env, row)
	}
}
