package events_test

import (
	"context"
	"testing"

	chatv1 "github.com/buidangphuc/team-chat/generated/platform/chat/v1"
	commonv1 "github.com/buidangphuc/team-chat/generated/platform/common/v1"
	"github.com/buidangphuc/team-chat/internal/events"
)

func TestNoopPublisher(t *testing.T) {
	pub := events.NoopPublisher{}
	ctx := context.Background()

	msg := &chatv1.ChatMessage{
		Id:       "msg-1",
		ThreadId: "thread-1",
		Content:  "test",
	}
	principal := &commonv1.Principal{
		Id:   "user-1",
		Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER,
	}

	err := pub.PublishMessageSent(ctx, msg, principal, "req-1")
	if err != nil {
		t.Fatalf("expected nil error from NoopPublisher, got %v", err)
	}
	pub.Close()
}

func TestNewKafkaPublisher_InvalidBrokers(t *testing.T) {
	// Creating kafka publisher with dummy address (does not connect immediately, verifies client initialization)
	pub, err := events.NewKafkaPublisher([]string{"127.0.0.1:9092"}, "chat.events")
	if err != nil {
		t.Fatalf("NewKafkaPublisher failed: %v", err)
	}
	if pub == nil {
		t.Fatal("expected non-nil publisher")
	}
	pub.Close()
}
