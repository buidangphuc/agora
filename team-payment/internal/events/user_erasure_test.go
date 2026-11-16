package events_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	eventsv1 "github.com/buidangphuc/team-payment/generated/platform/events/v1"
	identityv1 "github.com/buidangphuc/team-payment/generated/platform/identity/v1"
	"github.com/buidangphuc/team-payment/internal/events"
	"github.com/buidangphuc/team-payment/internal/store"
	"github.com/buidangphuc/team-payment/migrations"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
)

const (
	userErasedType     = "platform.identity.v1.UserErased"
	userRegisteredType = "platform.identity.v1.UserRegistered"
	statusChangedType  = "platform.identity.v1.UserStatusChanged"
	maskText           = "Đã xóa"
	rawNumber          = "0123456789"
	rawName            = "NGUYEN VAN A"
)

func quietLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

type rig struct {
	t    *testing.T
	pool *pgxpool.Pool
	w    *store.PostgresWalletRepository
}

func newRig(t *testing.T) *rig {
	t.Helper()
	dsn := testkit.Schema(t, testkit.SharedPostgres(t))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := migrate.Up(t.Context(), dsn, migrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &rig{t: t, pool: pool, w: store.NewPostgresWalletRepository(pool)}
}

func (r *rig) exec(sql string, args ...any) {
	r.t.Helper()
	if _, err := r.pool.Exec(context.Background(), sql, args...); err != nil {
		r.t.Fatalf("exec: %v", err)
	}
}

func (r *rig) seedPayouts(seller string) {
	r.t.Helper()
	r.exec(`INSERT INTO wallet_ledger (id, seller_id, type, amount, status, reference_id) VALUES ($1,$2,'ORDER_SETTLEMENT',400000,'COMPLETED',$3)`,
		"c-"+seller, seller, "pay-"+seller)
	for i, st := range []int{1, 3, 4} {
		r.exec(`INSERT INTO payout_requests (id, seller_id, amount, bank_code, account_number, account_name, status)
			VALUES ($1,$2,1000,'VCB',$3,$4,$5)`, "p-"+seller+string(rune('1'+i)), seller, rawNumber, rawName, st)
	}
}

func (r *rig) snapshot() string {
	r.t.Helper()
	var s string
	err := r.pool.QueryRow(context.Background(), `SELECT COALESCE(string_agg(x, '|' ORDER BY x), '') FROM (
		SELECT row_to_json(p)::text AS x FROM payout_requests p
		UNION ALL SELECT row_to_json(l)::text FROM wallet_ledger l
		UNION ALL SELECT row_to_json(t)::text FROM payment_transactions t) q`).Scan(&s)
	if err != nil {
		r.t.Fatal(err)
	}
	return s
}

func (r *rig) maskedCount(seller string) int {
	r.t.Helper()
	var n int
	if err := r.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM payout_requests WHERE seller_id=$1 AND account_number=$2 AND account_name=$2`, seller, maskText).Scan(&n); err != nil {
		r.t.Fatal(err)
	}
	return n
}

func envelope(t *testing.T, eventID, typ string, payload proto.Message) consume.Record {
	t.Helper()
	b, err := proto.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	v, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: eventID, Type: typ, Payload: b})
	if err != nil {
		t.Fatal(err)
	}
	return consume.Record{Topic: "identity.events", Key: []byte("k"), Value: v}
}

func userErased(t *testing.T, eventID, userID string) consume.Record {
	return envelope(t, eventID, userErasedType, &identityv1.UserErased{UserId: userID, ErasedAt: timestamppb.Now()})
}

func TestErasureConsumerMasksSellerPayouts(t *testing.T) {
	// PER-1, PER-2
	r := newRig(t)
	r.seedPayouts("S")
	r.seedPayouts("T")
	h := events.NewErasureConsumer(r.w, quietLogger())
	if err := h.Handle(context.Background(), userErased(t, "e-1", "S")); err != nil {
		t.Fatalf("PER-1 Handle: %v", err)
	}
	if got := r.maskedCount("S"); got != 3 {
		t.Fatalf("PER-1 masked S payouts = %d, want 3", got)
	}
	if got := r.maskedCount("T"); got != 0 {
		t.Fatalf("PER-2 masked T payouts = %d, want 0", got)
	}
}

func TestErasureConsumerRedeliveryChangesNothing(t *testing.T) {
	// PER-3
	r := newRig(t)
	r.seedPayouts("S")
	h := events.NewErasureConsumer(r.w, quietLogger())
	if err := h.Handle(context.Background(), userErased(t, "e-1", "S")); err != nil {
		t.Fatal(err)
	}
	first := r.snapshot()
	for _, id := range []string{"e-1", "e-2"} {
		if err := h.Handle(context.Background(), userErased(t, id, "S")); err != nil {
			t.Fatalf("PER-3 redelivery %s: %v", id, err)
		}
	}
	if again := r.snapshot(); again != first {
		t.Fatalf("PER-3 rows differ after redelivery\nfirst: %s\nagain: %s", first, again)
	}
}

func TestErasureConsumerUserWithoutPayoutsIsNoop(t *testing.T) {
	// PER-4
	r := newRig(t)
	r.seedPayouts("S")
	before := r.snapshot()
	h := events.NewErasureConsumer(r.w, quietLogger())
	if err := h.Handle(context.Background(), userErased(t, "e-1", "buyer-only")); err != nil {
		t.Fatalf("PER-4 Handle: %v", err)
	}
	if after := r.snapshot(); after != before {
		t.Fatal("PER-4 rows changed")
	}
}

func TestErasureConsumerRejectsMalformedWithoutApplying(t *testing.T) {
	// PER-5 (handler level; the DLQ routing is asserted in internal/app)
	r := newRig(t)
	r.seedPayouts("S")
	before := r.snapshot()
	h := events.NewErasureConsumer(r.w, quietLogger())

	bad := []consume.Record{
		userErased(t, "e-1", ""),
		{Topic: "identity.events", Value: []byte("\xff\xff not an envelope")},
	}
	badPayload, err := proto.Marshal(&eventsv1.EventEnvelope{EventId: "e-2", Type: userErasedType, Payload: []byte("\xff\xff")})
	if err != nil {
		t.Fatal(err)
	}
	bad = append(bad, consume.Record{Topic: "identity.events", Value: badPayload})
	for i, rec := range bad {
		if err := h.Handle(context.Background(), rec); err == nil {
			t.Fatalf("PER-5 malformed record %d was accepted", i)
		}
	}
	if after := r.snapshot(); after != before {
		t.Fatal("PER-5 rows changed")
	}
}

func TestErasureConsumerStoreErrorIsReturned(t *testing.T) {
	// PER-6: a store failure is an error so the consume loop retries it
	r := newRig(t)
	r.seedPayouts("S")
	h := events.NewErasureConsumer(r.w, quietLogger())
	r.pool.Close()
	if err := h.Handle(context.Background(), userErased(t, "e-1", "S")); err == nil {
		t.Fatal("PER-6 store failure was swallowed")
	}
}

func TestErasureConsumerIgnoresOtherEvents(t *testing.T) {
	// PER-7
	r := newRig(t)
	r.seedPayouts("S")
	before := r.snapshot()
	h := events.NewErasureConsumer(r.w, quietLogger())
	for _, rec := range []consume.Record{
		envelope(t, "e-1", userRegisteredType, &identityv1.UserRegistered{UserId: "S"}),
		envelope(t, "e-2", statusChangedType, &identityv1.UserStatusChanged{
			UserId: "S", Status: identityv1.UserStatus_USER_STATUS_BANNED, ChangedAt: timestamppb.Now(),
		}),
	} {
		if err := h.Handle(context.Background(), rec); err != nil {
			t.Fatalf("PER-7 Handle: %v", err)
		}
	}
	if after := r.snapshot(); after != before {
		t.Fatal("PER-7 rows changed")
	}
}

func TestErasureConsumerLogsNoRawPersonalData(t *testing.T) {
	// PER-1 logging check (tasks 1.15)
	r := newRig(t)
	const userID = "seller-raw-id-7f3a9c"
	r.seedPayouts(userID)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := events.NewErasureConsumer(r.w, logger)
	if err := h.Handle(context.Background(), userErased(t, "e-1", userID)); err != nil {
		t.Fatal(err)
	}
	_ = h.Handle(context.Background(), userErased(t, "e-2", ""))
	if buf.Len() == 0 {
		t.Fatal("handler logged nothing; expected counts and hashed ids")
	}
	for _, raw := range []string{userID, rawName, rawNumber} {
		if strings.Contains(buf.String(), raw) {
			t.Fatalf("captured log contains raw personal data %q:\n%s", raw, buf.String())
		}
	}
}
