package events_test

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/postgres"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-verification/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-verification/generated/platform/identity/v1"
	"github.com/buidangphuc/team-verification/internal/events"
	"github.com/buidangphuc/team-verification/internal/objects"
	"github.com/buidangphuc/team-verification/migrations"
)

var t0 = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

const userErasedType = "platform.identity.v1.UserErased"

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(context.Background(), url, migrations.Files); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := postgres.Open(context.Background(), config.Postgres{URL: url, MaxConns: 5})
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func envelope(t *testing.T, eventID, typ string, payload proto.Message) consume.Record {
	t.Helper()
	var pb []byte
	if payload != nil {
		var err error
		if pb, err = proto.Marshal(payload); err != nil {
			t.Fatal(err)
		}
	}
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, OccurredAt: timestamppb.New(t0), Payload: pb})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: "identity.events", Value: b}
}

func erased(t *testing.T, eventID, user string) consume.Record {
	t.Helper()
	return envelope(t, eventID, userErasedType, &identityv1.UserErased{UserId: user, ErasedAt: timestamppb.New(t0)})
}

func isPermanent(err error) bool {
	for ; err != nil; err = errors.Unwrap(err) {
		if reflect.TypeOf(err).Name() == "permanentError" {
			return true
		}
	}
	return false
}

func seedUser(t *testing.T, pool *pgxpool.Pool, objs *objects.Fake, user string) {
	t.Helper()
	ctx := context.Background()
	for i, s := range []struct{ id, status string }{{"kyc-v-" + user, "VERIFIED"}, {"kyc-r-" + user, "REJECTED"}} {
		key := "kyc/" + user + "/" + s.id
		if _, err := pool.Exec(ctx, `
			INSERT INTO kyc_submissions (id, user_id, doc_type, doc_ref, status, created_at)
			VALUES ($1, $2, 'id_card', $3, $4, $5)`, s.id, user, key, s.status, t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatalf("seed submission: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO kyc_documents (submission_id, doc_index, object_key, content_type, size_bytes)
			VALUES ($1, 0, $2, 'image/jpeg', 100)`, s.id, key); err != nil {
			t.Fatalf("seed document: %v", err)
		}
		objs.Put(key, "image/jpeg", 100)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO kyc_upload_grants (object_key, user_id, content_type, size_bytes, issued_at, expires_at)
		VALUES ($1, $2, 'image/jpeg', 100, $3, $4)`, "kyc/"+user+"/open", user, t0, t0.Add(15*time.Minute)); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	objs.Put("kyc/"+user+"/open", "image/jpeg", 100)
}

func rowCounts(t *testing.T, pool *pgxpool.Pool, user string) (subs, docs, grants int) {
	t.Helper()
	ctx := context.Background()
	q := func(sql string) int {
		var n int
		if err := pool.QueryRow(ctx, sql, user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	subs = q(`SELECT count(*) FROM kyc_submissions WHERE user_id=$1`)
	docs = q(`SELECT count(*) FROM kyc_documents d JOIN kyc_submissions s ON s.id=d.submission_id WHERE s.user_id=$1`)
	grants = q(`SELECT count(*) FROM kyc_upload_grants WHERE user_id=$1`)
	return
}

func newConsumer(pool *pgxpool.Pool, objs objects.Store) *events.UserErasedConsumer {
	return events.NewUserErasedConsumer(pool, objs, slog.New(slog.DiscardHandler))
}

func TestScenarioUserErasedRemovesTheUsersKYCData(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")

	if err := newConsumer(pool, objs).Handle(context.Background(), erased(t, "e-1", "u-1")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	for _, k := range []string{"kyc/u-1/kyc-v-u-1", "kyc/u-1/kyc-r-u-1", "kyc/u-1/open"} {
		if objs.Has(k) {
			t.Errorf("object %s still exists", k)
		}
	}
	if s, d, g := rowCounts(t, pool, "u-1"); s+d+g != 0 {
		t.Fatalf("rows left: submissions=%d documents=%d grants=%d", s, d, g)
	}
}

func TestScenarioDuplicateUserErasedIsHarmless(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")
	c := newConsumer(pool, objs)

	for i := 0; i < 2; i++ {
		if err := c.Handle(context.Background(), erased(t, "e-1", "u-1")); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	if err := c.Handle(context.Background(), erased(t, "e-2", "u-never-submitted")); err != nil {
		t.Fatalf("erasure of a user without data: %v", err)
	}
}

func TestScenarioErasureOfOneUserLeavesOthersUntouched(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")
	seedUser(t, pool, objs, "u-10")

	if err := newConsumer(pool, objs).Handle(context.Background(), erased(t, "e-1", "u-1")); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if s, d, g := rowCounts(t, pool, "u-10"); s != 2 || d != 2 || g != 1 {
		t.Fatalf("other user rows: submissions=%d documents=%d grants=%d, want 2/2/1", s, d, g)
	}
	for _, k := range []string{"kyc/u-10/kyc-v-u-10", "kyc/u-10/kyc-r-u-10", "kyc/u-10/open"} {
		if !objs.Has(k) {
			t.Errorf("other user's object %s was deleted", k)
		}
	}
}

func TestUserErasedStorageOutageRetriesThenDLQ(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")
	c := newConsumer(pool, objs)

	objs.SetError(errors.New("minio unavailable"))
	err := c.Handle(context.Background(), erased(t, "e-1", "u-1"))
	if err == nil {
		t.Fatal("Handle with storage down returned nil, the record would be acked and lost")
	}
	if isPermanent(err) {
		t.Fatalf("storage outage must be retryable before the DLQ, got permanent error %v", err)
	}
	if s, d, g := rowCounts(t, pool, "u-1"); s != 2 || d != 2 || g != 1 {
		t.Fatalf("rows deleted before the object delete succeeded: submissions=%d documents=%d grants=%d", s, d, g)
	}

	objs.SetError(nil)
	if err := c.Handle(context.Background(), erased(t, "e-1", "u-1")); err != nil {
		t.Fatalf("redelivery after recovery: %v", err)
	}
	if s, d, g := rowCounts(t, pool, "u-1"); s+d+g != 0 {
		t.Fatalf("rows left after recovery: %d/%d/%d", s, d, g)
	}
}

func TestUserErasedMalformedRecordsGoStraightToDLQ(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")
	c := newConsumer(pool, objs)

	for name, rec := range map[string]consume.Record{
		"garbage envelope": {Topic: "identity.events", Value: []byte{0xff, 0x01, 0x02}},
		"garbage payload":  {Topic: "identity.events", Value: mustEnv(t, userErasedType, []byte{0xff, 0xff})},
		"empty user id":    erased(t, "e-1", ""),
	} {
		err := c.Handle(context.Background(), rec)
		if err == nil || !isPermanent(err) {
			t.Errorf("%s: err = %v, want a permanent error", name, err)
		}
	}
	if s, _, _ := rowCounts(t, pool, "u-1"); s != 2 {
		t.Fatalf("malformed events changed data: submissions=%d", s)
	}
}

func TestUserErasedIgnoresOtherEventTypes(t *testing.T) {
	pool := migratedPool(t)
	objs := objects.NewFake()
	seedUser(t, pool, objs, "u-1")

	rec := envelope(t, "e-1", "platform.identity.v1.UserStatusChanged", &identityv1.UserStatusChanged{UserId: "u-1"})
	if err := newConsumer(pool, objs).Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if s, _, _ := rowCounts(t, pool, "u-1"); s != 2 {
		t.Fatalf("unrelated event deleted data: submissions=%d", s)
	}
}

func mustEnv(t *testing.T, typ string, payload []byte) []byte {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e", Type: typ, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
