package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-promotion/generated/platform/identity/v1"
)

const (
	identityTopic = "identity.events"
	erasureDLQ    = "identity.events.promotion.dlq"
)

func erasedEnvelope(t *testing.T, eventID, user string) []byte {
	t.Helper()
	body, err := proto.Marshal(&identityv1.UserErased{UserId: user, ErasedAt: timestamppb.Now()})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: "platform.identity.v1.UserErased", Payload: body})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func seedOwned(t *testing.T, url, id, owner string) {
	t.Helper()
	c := dial(t, url)
	if _, err := c.Exec(t.Context(), `INSERT INTO vouchers (id, code, scope, discount_type, discount_value, quota, kind, owner_user_id, active)
		VALUES ($1,$2,2,2,20000,1,2,$3,true)`, id, "NU-"+owner, owner); err != nil {
		t.Fatalf("seed voucher: %v", err)
	}
	if _, err := c.Exec(t.Context(), `INSERT INTO new_user_voucher_state (user_id, registered, has_code, verdict, registered_at, issued_at, updated_at)
		VALUES ($1,true,false,'',now(),now(),now())`, owner); err != nil {
		t.Fatalf("seed state: %v", err)
	}
}

func erased(t *testing.T, pool *pgxpool.Pool, id, owner string) bool {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM vouchers WHERE id = $1 AND NOT active AND deactivated_at IS NOT NULL) +
		(SELECT count(*) FROM new_user_voucher_state WHERE user_id = $2) * 10`, id, owner).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func dlqText(t *testing.T, brokers, topic string) string {
	t.Helper()
	client, err := kgo.NewClient(kgo.SeedBrokers(brokers), kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), kgo.AllowAutoTopicCreation())
	if err != nil {
		t.Fatalf("read client: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var b strings.Builder
	for ctx.Err() == nil {
		client.PollFetches(ctx).EachRecord(func(r *kgo.Record) { b.Write(r.Value) })
	}
	return b.String()
}

func TestErasureConsumerWiredOnIdentityTopic(t *testing.T) {
	// PRO-1, PRO-2, PRO-5: the app consumes UserErased from identity.events on its own group
	url := migratedURL(t)
	brokers := testkit.Redpanda(t)
	seedOwned(t, url, "v-a", "A")
	seedOwned(t, url, "v-b", "B")
	pool := openPool(t, url)
	r := start(t, promoSettings(testkit.FreePort(t), url, brokers))
	waitServing(t, r)

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "A", erasedEnvelope(t, "evt-erase-a", "A"))
		return erased(t, pool, "v-a", "A")
	})
	if erased(t, pool, "v-b", "B") {
		t.Fatal("PRO-2 another user's voucher or state was erased")
	}

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, identityTopic, "x", erasedEnvelope(t, "evt-empty-user", ""))
		return strings.Contains(dlqText(t, brokers, erasureDLQ), "evt-empty-user")
	})
	if erased(t, pool, "v-b", "B") {
		t.Fatal("PRO-5 a malformed event erased a user")
	}
}
