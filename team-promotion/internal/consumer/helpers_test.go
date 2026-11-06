package consumer_test

import (
	"context"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-promotion/generated/platform/identity/v1"
	referralv1 "github.com/buidangphuc/team-promotion/generated/platform/referral/v1"
	"github.com/buidangphuc/team-promotion/internal/consumer"
	"github.com/buidangphuc/team-promotion/internal/store"
)

var t0 = time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)

const (
	identityTopic = "identity.events"
	referralTopic = "referral.events"

	typeUserRegistered = "platform.identity.v1.UserRegistered"
	typeAttributed     = "platform.referral.v1.ReferralAttributed"
	typeRejected       = "platform.referral.v1.ReferralRejected"
	typeReleased       = "platform.referral.v1.ReferralRewardReleased"

	verdictTimeout = 30 * time.Minute
)

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	if err := migrate.Up(t.Context(), dsn, sub); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testConfig(now func() time.Time) consumer.Config {
	return consumer.Config{
		WelcomeAmount:  20000,
		InvitedAmount:  30000,
		MinSpend:       100000,
		VoucherTTL:     336 * time.Hour,
		VerdictTimeout: verdictTimeout,
		Now:            now,
	}
}

type env struct {
	pool     *pgxpool.Pool
	identity consume.Handler
	referral consume.Handler
	sweeper  *consumer.Sweeper
	cfg      consumer.Config
	clock    *time.Time
	seq      int
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := newPool(t)
	clock := t0
	cfg := testConfig(func() time.Time { return clock })
	return &env{
		pool:     pool,
		identity: consumer.NewIdentityHandler(pool, cfg),
		referral: consumer.NewReferralHandler(pool, cfg),
		sweeper:  consumer.NewSweeper(pool, cfg),
		cfg:      cfg,
		clock:    &clock,
	}
}

func (e *env) at(t time.Time) { *e.clock = t }

func envelope(t testing.TB, eventID, typ string, payload proto.Message) []byte {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		if body, err = proto.Marshal(payload); err != nil {
			t.Fatal(err)
		}
	}
	return rawEnvelope(t, eventID, typ, body)
}

func rawEnvelope(t testing.TB, eventID, typ string, payload []byte) []byte {
	t.Helper()
	b, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, OccurredAt: timestamppb.New(t0), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func registeredRecord(t testing.TB, eventID, user, code string) consume.Record {
	return consume.Record{
		Topic: identityTopic, Key: []byte(user),
		Value: envelope(t, eventID, typeUserRegistered, &identityv1.UserRegistered{UserId: user, ReferralCode: code, RegisteredAt: timestamppb.New(t0)}),
	}
}

func verdictRecord(t testing.TB, eventID, typ, referee string) consume.Record {
	var payload proto.Message
	switch typ {
	case typeAttributed:
		payload = &referralv1.ReferralAttributed{RefereeId: referee, ReferrerId: "inviter-1", AttributedAt: timestamppb.New(t0)}
	case typeRejected:
		payload = &referralv1.ReferralRejected{RefereeId: referee, Reason: referralv1.ReferralRejectReason_REFERRAL_REJECT_REASON_UNKNOWN_CODE}
	default:
		payload = &referralv1.ReferralRewardReleased{RewardId: "rw-1", ReferrerId: "inviter-1", RefereeId: referee, AmountXu: 10000}
	}
	return consume.Record{Topic: referralTopic, Key: []byte(referee), Value: envelope(t, eventID, typ, payload)}
}

func (e *env) registered(t testing.TB, eventID, user, code string) error {
	return e.identity(context.Background(), registeredRecord(t, eventID, user, code))
}

func (e *env) mustRegister(t testing.TB, user, code string) {
	t.Helper()
	e.seq++
	if err := e.registered(t, fmt.Sprintf("ur-%s-%d", user, e.seq), user, code); err != nil {
		t.Fatalf("UserRegistered(%s): %v", user, err)
	}
}

func (e *env) verdict(t testing.TB, typ, user string) error {
	e.seq++
	return e.referral(context.Background(), verdictRecord(t, fmt.Sprintf("vd-%s-%d", user, e.seq), typ, user))
}

func (e *env) mustVerdict(t testing.TB, typ, user string) {
	t.Helper()
	if err := e.verdict(t, typ, user); err != nil {
		t.Fatalf("%s(%s): %v", typ, user, err)
	}
}

type issued struct {
	Code         string
	Kind         int32
	Scope        int32
	DiscountType int32
	Discount     int64
	MinSpend     int64
	Quota        int64
	Used         int64
	PerUser      int32
	NewUserOnly  bool
	Active       bool
	StartsAt     time.Time
	EndsAt       time.Time
}

func newUserVouchers(t testing.TB, pool *pgxpool.Pool, owner string) []issued {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT code, kind, scope, discount_type, discount_value, min_spend, quota, used,
		per_user_limit, new_user_only, active, starts_at, ends_at
		FROM vouchers WHERE owner_user_id = $1 AND kind = 2 ORDER BY code`, owner)
	if err != nil {
		t.Fatalf("query vouchers: %v", err)
	}
	defer rows.Close()
	var out []issued
	for rows.Next() {
		var v issued
		if err := rows.Scan(&v.Code, &v.Kind, &v.Scope, &v.DiscountType, &v.Discount, &v.MinSpend, &v.Quota, &v.Used, &v.PerUser, &v.NewUserOnly, &v.Active, &v.StartsAt, &v.EndsAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func count(t testing.TB, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func (e *env) requireOne(t testing.TB, owner string, discount int64) issued {
	t.Helper()
	got := newUserVouchers(t, e.pool, owner)
	if len(got) != 1 {
		t.Fatalf("%s owns %d new-user vouchers, want 1: %+v", owner, len(got), got)
	}
	if got[0].Discount != discount {
		t.Fatalf("%s voucher discount = %d, want %d", owner, got[0].Discount, discount)
	}
	return got[0]
}

func (e *env) requireNone(t testing.TB, owner string) {
	t.Helper()
	if got := newUserVouchers(t, e.pool, owner); len(got) != 0 {
		t.Fatalf("%s owns %d new-user vouchers, want 0: %+v", owner, len(got), got)
	}
}

func isPermanent(err error) bool {
	want := fmt.Sprintf("%T", consume.Permanent(fmt.Errorf("x")))
	for ; err != nil; err = unwrap(err) {
		if fmt.Sprintf("%T", err) == want {
			return true
		}
	}
	return false
}

func unwrap(err error) error {
	u, ok := err.(interface{ Unwrap() error })
	if !ok {
		return nil
	}
	return u.Unwrap()
}
