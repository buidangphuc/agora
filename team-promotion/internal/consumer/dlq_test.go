package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/kafkax"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	"github.com/buidangphuc/team-promotion/internal/consumer"
)

const (
	identityDLQ = "identity.events.promotion.dlq"
	referralDLQ = "referral.events.promotion.dlq"
	dlqWait     = 60 * time.Second
)

func runConsumer(t *testing.T, brokers, group, topic, dlqTopic string, h consume.Handler) {
	t.Helper()
	cfg := config.Kafka{Brokers: brokers}
	client, err := kafkax.NewConsumerClient(cfg, group, []string{topic}, kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("consumer client: %v", err)
	}
	producer, err := kafkax.NewProducer(cfg)
	if err != nil {
		client.Close()
		t.Fatalf("dlq producer: %v", err)
	}
	c, err := consume.NewConsumer(client, h, producer, consume.Options{
		DLQTopic: dlqTopic, MaxAttempts: 3, BaseBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
	})
	if err != nil {
		client.Close()
		producer.Close()
		t.Fatalf("consumer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("consumer did not stop")
		}
		client.Close()
		producer.Close()
	})
}

func TestPoisonRecordToDLQ(t *testing.T) {
	brokers := testkit.Redpanda(t)
	e := newEnv(t)
	runConsumer(t, brokers, "team-promotion.new-user.test-identity", identityTopic, identityDLQ, consumer.NewIdentityHandler(e.pool, e.cfg))

	garbage := []byte{0xff, 0xff, 0xff, 0x01}
	testkit.Produce(t, brokers, identityTopic, "k-garbage", garbage)
	testkit.Produce(t, brokers, identityTopic, "k-empty", registeredRecord(t, "bad-empty", "", "").Value)
	testkit.Produce(t, brokers, identityTopic, "user-good", registeredRecord(t, "good-1", "user-good", "").Value)

	dead := testkit.ReadAll(t, brokers, identityDLQ, 2, dlqWait)
	for _, r := range dead {
		if r.Topic != identityDLQ {
			t.Fatalf("dead letter on %s", r.Topic)
		}
	}
	testkit.Eventually(t, dlqWait, func() bool { return len(newUserVouchers(t, e.pool, "user-good")) == 1 })
	if n := count(t, e.pool, `SELECT count(*) FROM vouchers WHERE kind = 2`); n != 1 {
		t.Fatalf("new-user vouchers = %d, want only the valid user's", n)
	}
}

func TestPoisonReferralRecordToDLQ(t *testing.T) {
	brokers := testkit.Redpanda(t)
	e := newEnv(t)
	e.mustRegister(t, "user-wait", "C")
	runConsumer(t, brokers, "team-promotion.new-user.test-referral", referralTopic, referralDLQ, consumer.NewReferralHandler(e.pool, e.cfg))

	garbage := []byte{0xff, 0xff, 0xff, 0x01}
	testkit.Produce(t, brokers, referralTopic, "k-garbage", rawEnvelope(t, "bad-a", typeAttributed, garbage))
	testkit.Produce(t, brokers, referralTopic, "k-garbage2", rawEnvelope(t, "bad-b", typeRejected, garbage))
	testkit.Produce(t, brokers, referralTopic, "k-empty", verdictRecord(t, "bad-c", typeAttributed, "").Value)
	testkit.Produce(t, brokers, referralTopic, "k-empty2", verdictRecord(t, "bad-d", typeRejected, "").Value)
	testkit.Produce(t, brokers, referralTopic, "user-wait", verdictRecord(t, "good-1", typeAttributed, "user-wait").Value)

	dead := testkit.ReadAll(t, brokers, referralDLQ, 4, dlqWait)
	for _, r := range dead {
		if r.Topic != referralDLQ {
			t.Fatalf("dead letter on %s", r.Topic)
		}
	}
	testkit.Eventually(t, dlqWait, func() bool { return len(newUserVouchers(t, e.pool, "user-wait")) == 1 })
	e.requireOne(t, "user-wait", 30000)
	if n := count(t, e.pool, `SELECT count(*) FROM vouchers WHERE kind = 2`); n != 1 {
		t.Fatalf("new-user vouchers = %d, want 1", n)
	}
}
