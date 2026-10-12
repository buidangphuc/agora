package events

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/platform-core/packages/go-sdk/pkg/reqctx"

	commonv1 "github.com/buidangphuc/team-engagement/generated/platform/common/v1"
	engagementv1 "github.com/buidangphuc/team-engagement/generated/platform/engagement/v1"
	eventsv1 "github.com/buidangphuc/team-engagement/generated/platform/events/v1"
)

// The envelope carries the principal recorded with the fact, the payload's full
// proto name as type, the request id, and the payload untouched. (It used to be
// built by the relayer at publish time; the SDK relayer publishes rows verbatim,
// so it is built when the fact is written.)
func TestBuildEnvelopeCarriesPrincipalTypeAndPayload(t *testing.T) {
	at := time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)
	b, err := BuildEnvelope("e1", "platform.engagement.v1.FavoriteAdded", at,
		&commonv1.Principal{Id: "buyer-1", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER}, "r1", []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetEventId() != "e1" || env.GetType() != "platform.engagement.v1.FavoriteAdded" ||
		env.GetPrincipal().GetId() != "buyer-1" || env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
		env.GetRequestId() != "r1" || string(env.GetPayload()) != "a" || !env.GetOccurredAt().AsTime().Equal(at) {
		t.Fatalf("envelope = %v", &env)
	}
	// A fact written without a caller has no principal.
	b, err = BuildEnvelope("e2", "platform.engagement.v1.SellerFollowed", at, nil, "", []byte("b"))
	if err != nil {
		t.Fatal(err)
	}
	env.Reset()
	if err := proto.Unmarshal(b, &env); err != nil || env.GetPrincipal() != nil {
		t.Fatalf("envelope = %v err=%v, want no principal", &env, err)
	}
}

// FactRow takes the caller and request id from the context, keys the row by the
// partition key and publishes a fresh event id each time.
func TestFactRowBuildsTheFullEnvelopeFromContext(t *testing.T) {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: "buyer-1", Type: "user", Scopes: []string{"engagement:write"}})
	ctx = reqctx.WithRequestID(ctx, "req-7")
	fact := &engagementv1.FavoriteAdded{UserId: "buyer-1", ListingId: "L1"}

	row, err := FactRow(ctx, "Listing", "L1", fact)
	if err != nil {
		t.Fatal(err)
	}
	if row.AggregateType != "Listing" || row.AggregateID != "L1" || row.EventType != "platform.engagement.v1.FavoriteAdded" || row.RequestID != "req-7" {
		t.Fatalf("row = %+v", row)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(row.Payload, &env); err != nil {
		t.Fatal(err)
	}
	if env.GetEventId() != row.EventID || env.GetType() != row.EventType || env.GetRequestId() != "req-7" ||
		env.GetPrincipal().GetId() != "buyer-1" || env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER ||
		len(env.GetPrincipal().GetScopes()) != 0 {
		t.Fatalf("envelope = %v", &env)
	}
	var got engagementv1.FavoriteAdded
	if err := proto.Unmarshal(env.GetPayload(), &got); err != nil || !proto.Equal(&got, fact) {
		t.Fatalf("payload = %v err=%v", &got, err)
	}
	again, err := FactRow(ctx, "Listing", "L1", fact)
	if err != nil || again.EventID == row.EventID {
		t.Fatalf("event id must be fresh per fact: %v %v", again.EventID, err)
	}

	bare, err := FactRow(context.Background(), "Seller", "S1", &engagementv1.SellerFollowed{UserId: "u", SellerId: "S1"})
	if err != nil {
		t.Fatal(err)
	}
	env.Reset()
	if err := proto.Unmarshal(bare.Payload, &env); err != nil || env.GetPrincipal() != nil {
		t.Fatalf("no caller: envelope = %v err=%v", &env, err)
	}
}
