package events_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-chat/generated/platform/events/v1"
	"github.com/buidangphuc/team-chat/internal/events"
	"github.com/buidangphuc/team-chat/internal/store"
)

func sampleMessage() store.ChatMessage {
	return store.ChatMessage{
		ID: "m1", ThreadID: "t1", SenderID: "seller-1", SenderName: "Shop One",
		RecipientID: "buyer-1", SellerID: "seller-1", Content: "s3cret-needle",
		CreatedAt: time.Unix(1700000000, 0), MessageType: 1, ListingID: "l1", Payload: `{"k":"v"}`,
	}
}

func principalCtx(id, requestID string) context.Context {
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Type: auth.TypeUser})
	return reqctx.WithRequestID(ctx, requestID)
}

func decode(t *testing.T, payload []byte) (*eventsv1.EventEnvelope, *chatv1.ChatMessage) {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(payload, &env); err != nil {
		t.Fatal(err)
	}
	var msg chatv1.ChatMessage
	if err := proto.Unmarshal(env.GetPayload(), &msg); err != nil {
		t.Fatal(err)
	}
	return &env, &msg
}

func TestEventIDIsDeterministicPerMessage(t *testing.T) {
	msg := sampleMessage()
	a, err := events.BuildMessageOutboxRow(principalCtx("u1", "r1"), msg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := events.BuildMessageOutboxRow(context.Background(), msg)
	if err != nil {
		t.Fatal(err)
	}
	if a.EventID == "" || a.EventID != b.EventID {
		t.Fatalf("event ids differ: %q vs %q", a.EventID, b.EventID)
	}
	if a.EventID != events.ChatMessageEventID("m1") {
		t.Fatalf("row event id %q != ChatMessageEventID", a.EventID)
	}
	if _, err := uuid.Parse(a.EventID); err != nil {
		t.Fatalf("event id is not a uuid: %v", err)
	}
	if events.ChatMessageEventID("m2") == a.EventID {
		t.Fatal("different messages must have different event ids")
	}
	other := msg
	other.ID = "m2"
	c, err := events.BuildMessageOutboxRow(context.Background(), other)
	if err != nil {
		t.Fatal(err)
	}
	if c.EventID == a.EventID {
		t.Fatal("different messages must produce different rows")
	}
}

func TestEnvelopeCarriesPrincipalAndRequestID(t *testing.T) {
	msg := sampleMessage()
	row, err := events.BuildMessageOutboxRow(principalCtx("u1", "r1"), msg)
	if err != nil {
		t.Fatal(err)
	}
	if row.RequestID != "r1" {
		t.Fatalf("row request id = %q, want r1", row.RequestID)
	}
	if row.AggregateID != "t1" || row.EventType != events.ChatMessageSentType {
		t.Fatalf("row = %+v", row)
	}
	env, _ := decode(t, row.Payload)
	if env.GetPrincipal().GetId() != "u1" {
		t.Fatalf("principal id = %q, want u1", env.GetPrincipal().GetId())
	}
	if env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		t.Fatalf("principal type = %v, want USER", env.GetPrincipal().GetType())
	}
	if env.GetRequestId() != "r1" {
		t.Fatalf("envelope request id = %q, want r1", env.GetRequestId())
	}
	if env.GetEventId() != row.EventID {
		t.Fatalf("envelope event id %q != row %q", env.GetEventId(), row.EventID)
	}
}

func TestEnvelopeWithoutCallerBuildsRowWithoutPrincipal(t *testing.T) {
	row, err := events.BuildMessageOutboxRow(context.Background(), sampleMessage())
	if err != nil {
		t.Fatal(err)
	}
	if row.RequestID != "" {
		t.Fatalf("row request id = %q, want empty", row.RequestID)
	}
	env, _ := decode(t, row.Payload)
	if env.GetPrincipal().GetId() != "" || env.GetRequestId() != "" {
		t.Fatalf("envelope = %+v", env)
	}
}

func TestEnvelopeCarriesNoMoreThanToday(t *testing.T) {
	msg := sampleMessage()
	row, err := events.BuildMessageOutboxRow(principalCtx("u1", "r1"), msg)
	if err != nil {
		t.Fatal(err)
	}
	_, got := decode(t, row.Payload)
	base := &chatv1.ChatMessage{
		Id: "m1", ThreadId: "t1", SenderId: "seller-1", SenderName: "Shop One",
		CreatedAt: timestamppb.New(msg.CreatedAt), MessageType: chatv1.MessageType(1),
		ListingId: "l1", Payload: `{"k":"v"}`, RecipientId: "buyer-1", SellerId: "seller-1",
	}
	withContent := proto.Clone(base).(*chatv1.ChatMessage)
	withContent.Content = "s3cret-needle"
	if !proto.Equal(got, base) && !proto.Equal(got, withContent) {
		t.Fatalf("payload = %+v", got)
	}
	if got.GetContent() == "" && bytes.Contains(row.Payload, []byte("s3cret-needle")) {
		t.Fatal("content leaked into the envelope although ChatMessage carries none")
	}
}
