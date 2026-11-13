package consumer_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"

	identityv1 "github.com/buidangphuc/team-promotion/generated/platform/identity/v1"
	"github.com/buidangphuc/team-promotion/internal/consumer"
)

const typeUserErased = "platform.identity.v1.UserErased"

func erasedRecord(t testing.TB, eventID, user string) consume.Record {
	return consume.Record{
		Topic: identityTopic, Key: []byte(user),
		Value: envelope(t, eventID, typeUserErased, &identityv1.UserErased{UserId: user, ErasedAt: timestamppb.New(t0)}),
	}
}

func seedOwnedVoucher(t *testing.T, e *env, id, code, owner string) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `INSERT INTO vouchers (id, code, scope, discount_type, discount_value, quota, kind, owner_user_id, active)
		VALUES ($1,$2,2,2,20000,1,2,$3,true)`, id, code, owner)
	if err != nil {
		t.Fatalf("seed voucher: %v", err)
	}
}

func voucherActive(t *testing.T, e *env, id string) bool {
	t.Helper()
	return count(t, e.pool, `SELECT count(*) FROM vouchers WHERE id = $1 AND active AND deactivated_at IS NULL`, id) == 1
}

func stateRows(t *testing.T, e *env, user string) int {
	t.Helper()
	return count(t, e.pool, `SELECT count(*) FROM new_user_voucher_state WHERE user_id = $1`, user)
}

func TestErasureConsumerParksMalformed(t *testing.T) {
	// PRO-5: handler level, then through a real consumer onto identity.events.promotion.dlq
	e := newEnv(t)
	seedOwnedVoucher(t, e, "v-u", "NU-U", "U")
	if err := e.identity(context.Background(), registeredRecord(t, "reg-u", "U", "")); err != nil {
		t.Fatalf("register: %v", err)
	}
	h := consumer.NewErasureHandler(e.pool, e.cfg)

	for name, rec := range map[string]consume.Record{
		"empty user_id": erasedRecord(t, "bad-empty", ""),
		"undecodable":   {Topic: identityTopic, Key: []byte("k"), Value: []byte{0xff, 0xff, 0xff, 0x01}},
		"bad payload":   {Topic: identityTopic, Key: []byte("k"), Value: rawEnvelope(t, "bad-payload", typeUserErased, []byte{0xff, 0xff, 0xff, 0x01})},
	} {
		err := h(context.Background(), rec)
		if err == nil || !isPermanent(err) {
			t.Errorf("PRO-5 %s: err = %v, want a permanent error", name, err)
		}
	}
	if !voucherActive(t, e, "v-u") || stateRows(t, e, "U") != 1 {
		t.Fatal("PRO-5 a malformed event changed rows")
	}

	brokers := testkit.Redpanda(t)
	runConsumer(t, brokers, "team-promotion.erasure.test", identityTopic, identityDLQ, h)
	testkit.Produce(t, brokers, identityTopic, "k-garbage", []byte{0xff, 0xff, 0xff, 0x01})
	testkit.Produce(t, brokers, identityTopic, "k-empty", erasedRecord(t, "bad-empty-2", "").Value)
	dead := testkit.ReadAll(t, brokers, identityDLQ, 2, dlqWait)
	for _, r := range dead {
		if r.Topic != identityDLQ {
			t.Fatalf("PRO-5 dead letter on %s", r.Topic)
		}
	}
	if !voucherActive(t, e, "v-u") || stateRows(t, e, "U") != 1 {
		t.Fatal("PRO-5 a parked record changed rows")
	}
}

func TestErasureConsumerErasesAndRetries(t *testing.T) {
	// PRO-1, PRO-3, PRO-6 at consumer level
	e := newEnv(t)
	seedOwnedVoucher(t, e, "v-u", "NU-U", "U")
	e.mustRegister(t, "U", "")
	h := consumer.NewErasureHandler(e.pool, e.cfg)

	if _, err := e.pool.Exec(context.Background(), `CREATE FUNCTION erasure_fail() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'injected failure'; END $$ LANGUAGE plpgsql;
CREATE TRIGGER erasure_fail BEFORE DELETE ON new_user_voucher_state FOR EACH ROW EXECUTE FUNCTION erasure_fail();`); err != nil {
		t.Fatal(err)
	}
	err := h(context.Background(), erasedRecord(t, "ev-1", "U"))
	if err == nil || isPermanent(err) {
		t.Fatalf("PRO-6 err = %v, want a retryable error", err)
	}
	if !voucherActive(t, e, "v-u") || stateRows(t, e, "U") != 1 {
		t.Fatal("PRO-6 partial erasure after a store error")
	}
	if _, err := e.pool.Exec(context.Background(), `DROP TRIGGER erasure_fail ON new_user_voucher_state`); err != nil {
		t.Fatal(err)
	}

	e.at(t0.Add(time.Hour))
	if err := h(context.Background(), erasedRecord(t, "ev-1", "U")); err != nil {
		t.Fatalf("PRO-6 retry: %v", err)
	}
	if voucherActive(t, e, "v-u") || stateRows(t, e, "U") != 0 {
		t.Fatal("PRO-1 voucher still active or state row remains")
	}
	var first time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT deactivated_at FROM vouchers WHERE id = 'v-u'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	e.at(t0.Add(5 * time.Hour))
	for _, id := range []string{"ev-1", "ev-2"} {
		if err := h(context.Background(), erasedRecord(t, id, "U")); err != nil {
			t.Fatalf("PRO-3 redelivery %s: %v", id, err)
		}
	}
	var again time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT deactivated_at FROM vouchers WHERE id = 'v-u'`).Scan(&again); err != nil {
		t.Fatal(err)
	}
	if !again.Equal(first) {
		t.Errorf("PRO-3 deactivated_at = %v after redelivery, want first value %v", again, first)
	}
}

func TestIdentityConsumerUnchangedByErasure(t *testing.T) {
	// PRO-7: the erasure consumer ignores UserRegistered, the identity consumer still issues state
	e := newEnv(t)
	seedOwnedVoucher(t, e, "v-w", "NU-W", "W")
	h := consumer.NewErasureHandler(e.pool, e.cfg)

	rec := registeredRecord(t, "reg-new", "newbie", "")
	if err := h(context.Background(), rec); err != nil {
		t.Fatalf("PRO-7 erasure consumer on UserRegistered: %v", err)
	}
	if stateRows(t, e, "newbie") != 0 || !voucherActive(t, e, "v-w") {
		t.Fatal("PRO-7 the erasure consumer acted on UserRegistered")
	}
	if err := e.identity(context.Background(), rec); err != nil {
		t.Fatalf("PRO-7 identity consumer: %v", err)
	}
	if stateRows(t, e, "newbie") != 1 {
		t.Fatal("PRO-7 the identity consumer no longer writes the state row")
	}
	if err := e.identity(context.Background(), erasedRecord(t, "er-1", "W")); err != nil {
		t.Fatalf("PRO-7 identity consumer on UserErased: %v", err)
	}
	if !voucherActive(t, e, "v-w") {
		t.Fatal("PRO-7 the identity consumer erased on UserErased")
	}
}

func TestErasureOnlyFromIdentityEvents(t *testing.T) {
	// PRO-9: a UserErased envelope produced to promotion.events drives nothing
	e := newEnv(t)
	seedOwnedVoucher(t, e, "v-a", "NU-A", "A")
	seedOwnedVoucher(t, e, "v-b", "NU-B", "B")
	brokers := testkit.Redpanda(t)
	runConsumer(t, brokers, "team-promotion.erasure.only-identity", identityTopic, identityDLQ, consumer.NewErasureHandler(e.pool, e.cfg))

	testkit.Produce(t, brokers, "promotion.events", "B", erasedRecord(t, "forged-b", "B").Value)
	testkit.Produce(t, brokers, identityTopic, "A", erasedRecord(t, "real-a", "A").Value)
	testkit.Eventually(t, dlqWait, func() bool { return !voucherActive(t, e, "v-a") })
	if !voucherActive(t, e, "v-b") {
		t.Fatal("PRO-9 an envelope on promotion.events erased a user")
	}
}
