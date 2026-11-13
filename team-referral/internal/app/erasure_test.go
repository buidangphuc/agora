package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/types/known/timestamppb"

	sdkconfig "github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-referral/generated/platform/identity/v1"
	"github.com/buidangphuc/team-referral/migrations"
)

const erasureDLQ = "identity.events.referral.dlq"

func hasRow(t *testing.T, pool *pgxpool.Pool, query string, args ...any) bool {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n > 0
}

func topicText(t *testing.T, brokers, topic string) string {
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
	// REF-1, REF-3, REF-6, REF-8: the app consumes UserErased from identity.events on its own group
	url := testkit.Postgres(t)
	brokers := testkit.Redpanda(t)
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for _, u := range []string{"U", "W"} {
		if _, err := pool.Exec(context.Background(), `INSERT INTO referral_codes (user_id, code) VALUES ($1, $2)`, u, "CODE-"+u); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), `INSERT INTO referral_accounts (user_id, registered_at) VALUES ($1, now())`, u); err != nil {
			t.Fatal(err)
		}
	}
	s := withBrokers(settings(testkit.FreePort(t), "local", sdkconfig.Postgres{URL: url}), brokers)
	s.Referral.ErasureGroup = "team-referral-erasure"
	s.Referral.ErasureDLQTopic = erasureDLQ
	r := startApp(t, s)
	waitServing(t, r)

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, "identity.events", "U", envelopeBytes(t, "evt-erase-u", "platform.identity.v1.UserErased",
			&identityv1.UserErased{UserId: "U", ErasedAt: timestamppb.Now()}))
		return !hasRow(t, pool, `SELECT 1 FROM referral_codes WHERE user_id = 'U'`)
	})
	if hasRow(t, pool, `SELECT 1 FROM referral_accounts WHERE user_id = 'U'`) {
		t.Error("REF-1 the account row of U remains")
	}
	if !hasRow(t, pool, `SELECT 1 FROM referral_codes WHERE user_id = 'W'`) || !hasRow(t, pool, `SELECT 1 FROM referral_accounts WHERE user_id = 'W'`) {
		t.Error("REF-3 another user's rows were deleted")
	}
	if strings.Contains(topicText(t, brokers, "referral.identity.dlq"), "evt-erase-u") {
		t.Error("REF-8 the identity consumer failed on UserErased and parked it")
	}

	testkit.Eventually(t, 90*time.Second, func() bool {
		testkit.Produce(t, brokers, "identity.events", "x", envelopeBytes(t, "evt-empty-user", "platform.identity.v1.UserErased", &identityv1.UserErased{}))
		return strings.Contains(topicText(t, brokers, erasureDLQ), "evt-empty-user")
	})
}
