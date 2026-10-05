package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buidangphuc/team-chat/internal/repository"
	"github.com/buidangphuc/team-chat/internal/service"
)

// A failure while building/writing the event aborts the whole send: neither the
// message nor an outbox row is stored.
func TestSendMessageOutboxFailureStoresNothing(t *testing.T) {
	outbox := repository.NewInMemoryOutboxRepository()
	repo := repository.NewInMemoryChatRepository(
		repository.WithMessageOutbox(func(context.Context, repository.ChatMessage) (repository.OutboxRow, error) {
			return repository.OutboxRow{}, errors.New("boom")
		}),
		repository.WithInMemoryOutbox(outbox),
	)
	svc := service.NewChatService(repo, nil)
	th, err := svc.GetOrCreateThread(context.Background(), "buyer-1", "seller-1", "l1", "t", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendMessage(context.Background(), th.ID, "buyer-1", "b", "hello"); err == nil {
		t.Fatal("expected the send to fail")
	}
	msgs, total, _ := svc.GetThreadMessages(context.Background(), th.ID, "buyer-1", 1, 50)
	if len(msgs) != 0 || total != 0 || len(outbox.EnqueuedRows()) != 0 {
		t.Fatalf("stored message=%d total=%d outbox=%d, want all 0", len(msgs), total, len(outbox.EnqueuedRows()))
	}
}

// The recipient handed to the outbox builder is the participant who did not send.
func TestSendMessagePassesRecipientToOutbox(t *testing.T) {
	var got []string
	repo := repository.NewInMemoryChatRepository(
		repository.WithMessageOutbox(func(_ context.Context, m repository.ChatMessage) (repository.OutboxRow, error) {
			got = append(got, m.SenderID+">"+m.RecipientID)
			return repository.OutboxRow{EventID: m.ID, AggregateID: m.ThreadID, Payload: []byte("x")}, nil
		}),
		repository.WithInMemoryOutbox(repository.NewInMemoryOutboxRepository()),
	)
	svc := service.NewChatService(repo, nil)
	th, _ := svc.GetOrCreateThread(context.Background(), "buyer-1", "seller-1", "l1", "t", "")
	_, _ = svc.SendMessage(context.Background(), th.ID, "buyer-1", "b", "q")
	_, _ = svc.SendMessage(context.Background(), th.ID, "seller-1", "s", "a")
	if len(got) != 2 || got[0] != "buyer-1>seller-1" || got[1] != "seller-1>buyer-1" {
		t.Fatalf("sender>recipient = %v", got)
	}
}
