package events_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-referral/generated/platform/events/v1"
	referralv1 "github.com/buidangphuc/team-referral/generated/platform/referral/v1"
	"github.com/buidangphuc/team-referral/internal/events"
	"github.com/buidangphuc/team-referral/internal/store"
	"github.com/buidangphuc/team-referral/migrations"
)

var t0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

type flakyPublisher struct {
	down    atomic.Bool
	rec     *testkit.Recorder
	attempt atomic.Int32
}

func (f *flakyPublisher) PublishRecord(ctx context.Context, r *kgo.Record) error {
	f.attempt.Add(1)
	if f.down.Load() {
		return errors.New("broker unreachable")
	}
	return f.rec.PublishRecord(ctx, r)
}

func setup(t *testing.T) (*store.PostgresReferralRepo, *outbox.Store, func(pub outbox.Publisher) *outbox.Relayer) {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ob, err := outbox.NewStore(pool, "referral_outbox_events")
	if err != nil {
		t.Fatalf("outbox store over referral_outbox_events: %v", err)
	}
	mk := func(pub outbox.Publisher) *outbox.Relayer {
		r, err := outbox.NewRelayer(ob, pub, outbox.RelayerOptions{
			Topic:       "referral.events",
			Config:      config.Outbox{PollInterval: 50 * time.Millisecond, BatchSize: 10, ClaimLock: 10 * time.Second},
			BaseBackoff: time.Millisecond,
			MaxBackoff:  time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	repo := store.NewPostgresReferralRepo(pool)
	if _, err := repo.CreateCode(context.Background(), "alice", "REFAAAA1111"); err != nil {
		t.Fatal(err)
	}
	return repo, ob, mk
}

func attribute(t *testing.T, repo *store.PostgresReferralRepo, user string) {
	t.Helper()
	err := repo.ApplyUserRegistered(context.Background(), store.UserRegisteredInput{
		EventID: "ur-" + user, UserID: user, ReferralCode: "REFAAAA1111", RegisteredAt: t0, At: t0, LifetimeCap: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRelayerPublishesReferralEvents(t *testing.T) {
	repo, ob, mk := setup(t)
	attribute(t, repo, "bob")
	rec := testkit.NewRecorder()

	if _, err := mk(rec).RelayOnce(context.Background()); err != nil {
		t.Fatalf("RelayOnce: %v", err)
	}
	recs := rec.Records()
	if len(recs) != 1 {
		t.Fatalf("published %d records, want 1", len(recs))
	}
	got := recs[0]
	if got.Topic != "referral.events" || string(got.Key) != "bob" {
		t.Fatalf("record topic=%q key=%q, want referral.events keyed by the referee id", got.Topic, got.Key)
	}
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(got.Value, &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if env.GetType() != "platform.referral.v1.ReferralAttributed" {
		t.Fatalf("envelope type = %q", env.GetType())
	}
	if env.GetEventId() != events.StableEventID("ReferralAttributed", "bob") {
		t.Fatalf("event id = %q, want the stable id", env.GetEventId())
	}
	var payload referralv1.ReferralAttributed
	if err := proto.Unmarshal(env.GetPayload(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.GetRefereeId() != "bob" || payload.GetReferrerId() != "alice" {
		t.Fatalf("payload = %v", &payload)
	}
	st, err := ob.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending != 0 {
		t.Fatalf("outbox still has %d pending rows after publish", st.Pending)
	}
}

func TestOutboxSurvivesBrokerOutage(t *testing.T) {
	repo, _, mk := setup(t)
	pub := &flakyPublisher{rec: testkit.NewRecorder()}
	pub.down.Store(true)
	attribute(t, repo, "bob")

	relayer := mk(pub)
	for i := 0; i < 3; i++ {
		_, _ = relayer.RelayOnce(context.Background())
	}
	if pub.attempt.Load() == 0 {
		t.Fatal("relayer never tried to publish")
	}
	if n := len(pub.rec.Records()); n != 0 {
		t.Fatalf("%d records published while the broker was down", n)
	}
	if ref, err := repo.GetReferral(context.Background(), "bob"); err != nil || ref.Status != store.StatusAttributed {
		t.Fatalf("referral during outage = %+v, %v", ref, err)
	}

	pub.down.Store(false)
	testkit.Eventually(t, 5*time.Second, func() bool {
		_, _ = relayer.RelayOnce(context.Background())
		return len(pub.rec.Records()) == 1
	})
	if n := len(pub.rec.Records()); n != 1 {
		t.Fatalf("published %d records after recovery, want exactly 1", n)
	}
}

func TestStableEventIDIsDerivedFromMessageAndReferee(t *testing.T) {
	a := events.StableEventID("ReferralAttributed", "bob")
	if a == "" || a != events.StableEventID("ReferralAttributed", "bob") {
		t.Fatalf("id not stable: %q", a)
	}
	if a == events.StableEventID("ReferralRejected", "bob") || a == events.StableEventID("ReferralAttributed", "carol") {
		t.Fatal("id must differ per message name and per referee")
	}
}
