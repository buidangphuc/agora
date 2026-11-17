package consumer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/consume"

	identityv1 "github.com/buidangphuc/team-referral/generated/platform/identity/v1"
	"github.com/buidangphuc/team-referral/internal/consumer"
	"github.com/buidangphuc/team-referral/internal/store"
)

const typeUserErased = "platform.identity.v1.UserErased"

func (e *env) erasure() consume.Handler { return consumer.NewErasureHandler(e.repo, e.cfg) }

func erasedRecord(t testing.TB, eventID, user string) consume.Record {
	return consume.Record{
		Topic: "identity.events", Key: []byte(user),
		Value: envelope(t, eventID, typeUserErased, &identityv1.UserErased{UserId: user, ErasedAt: timestamppb.New(t0)}),
	}
}

func (e *env) seedUser(t *testing.T, user string) string {
	t.Helper()
	code, err := e.repo.CreateCode(context.Background(), user, "CODE-"+user)
	if err != nil {
		t.Fatal(err)
	}
	e.mustRegister(t, user, "")
	e.mustComplete(t, "first-"+user, user, t0.Add(48*time.Hour))
	return code
}

func (e *env) hasIdentity(user string) (code, account, firstOrder bool) {
	ctx := context.Background()
	_, err := e.repo.GetCodeByUser(ctx, user)
	code = err == nil
	_, err = e.repo.RegisteredAt(ctx, user)
	account = err == nil
	firstOrder, _ = e.repo.HasCompletedOrder(ctx, user)
	return
}

func TestErasureConsumerDeletesIdentityKeepsLedger(t *testing.T) {
	// REF-1, REF-2, REF-3, REF-4, REF-5
	e := newEnv(t)
	ctx := context.Background()
	codeU := e.seedUser(t, "U")
	e.seedUser(t, "W")
	e.mustRegister(t, "r1", codeU)
	e.mustRegister(t, "r2", codeU)
	e.mustComplete(t, "o-r1", "r1", t0.Add(48*time.Hour))
	referrals, _ := e.repo.CountReferralsByReferrer(ctx, "U")
	totals, _ := e.repo.RewardTotals(ctx, "U")
	if referrals != 2 || totals.PendingXU == 0 {
		t.Fatalf("seed: referrals=%d totals=%+v", referrals, totals)
	}
	rewardsBefore, _ := e.repo.ListRewards(ctx, "U", 50, 0)

	h := e.erasure()
	if err := h(ctx, erasedRecord(t, "ev-1", "U")); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if c, a, o := e.hasIdentity("U"); c || a || o {
		t.Errorf("REF-1 U rows remain: code=%v account=%v first-order=%v", c, a, o)
	}
	if n, _ := e.repo.CountReferralsByReferrer(ctx, "U"); n != referrals {
		t.Errorf("REF-1 attributions = %d, want %d kept", n, referrals)
	}
	if got, _ := e.repo.RewardTotals(ctx, "U"); got != totals {
		t.Errorf("REF-1 reward totals = %+v, want %+v kept", got, totals)
	}
	if got, _ := e.repo.ListRewards(ctx, "U", 50, 0); len(got) != len(rewardsBefore) {
		t.Errorf("REF-1 reward rows = %d, want %d", len(got), len(rewardsBefore))
	}
	if c, a, o := e.hasIdentity("W"); !c || !a || !o {
		t.Errorf("REF-3 W rows changed: code=%v account=%v first-order=%v", c, a, o)
	}
	if _, err := e.repo.OwnerOfCode(ctx, codeU); !errors.Is(err, store.ErrCodeNotFound) {
		t.Errorf("REF-2 OwnerOfCode(%s) = %v, want ErrCodeNotFound", codeU, err)
	}

	for _, id := range []string{"ev-1", "ev-2"} {
		if err := h(ctx, erasedRecord(t, id, "U")); err != nil {
			t.Errorf("REF-4 redelivery %s: %v", id, err)
		}
	}
	if err := h(ctx, erasedRecord(t, "ev-none", "nobody")); err != nil {
		t.Errorf("REF-5 user without referral data: %v", err)
	}
}

func TestErasureConsumerParksMalformed(t *testing.T) {
	// REF-6
	e := newEnv(t)
	e.seedUser(t, "U")
	h := e.erasure()
	for name, rec := range map[string]consume.Record{
		"empty user_id": erasedRecord(t, "bad-empty", ""),
		"undecodable":   {Topic: "identity.events", Key: []byte("k"), Value: []byte{0xff, 0xff, 0xff, 0x01}},
		"bad payload":   {Topic: "identity.events", Key: []byte("k"), Value: rawEnvelope(t, "bad-payload", typeUserErased, []byte{0xff, 0xff, 0xff, 0x01})},
	} {
		if err := h(context.Background(), rec); err == nil || !isPermanent(err) {
			t.Errorf("REF-6 %s: err = %v, want a permanent error", name, err)
		}
	}
	if c, a, o := e.hasIdentity("U"); !c || !a || !o {
		t.Error("REF-6 a malformed event changed rows")
	}
}

type failingRepo struct {
	store.ReferralRepository
	err error
}

func (f failingRepo) EraseUser(context.Context, string) error { return f.err }

func TestErasureConsumerRetriesStoreError(t *testing.T) {
	// REF-7
	e := newEnv(t)
	boom := errors.New("db down")
	h := consumer.NewErasureHandler(failingRepo{ReferralRepository: e.repo, err: boom}, e.cfg)
	err := h(context.Background(), erasedRecord(t, "ev-1", "U"))
	if !errors.Is(err, boom) || isPermanent(err) {
		t.Fatalf("REF-7 err = %v, want the store error, retryable", err)
	}
}

func TestErasureConsumerIgnoresOtherEvents(t *testing.T) {
	// REF-8: the erasure consumer skips other identity events
	e := newEnv(t)
	if err := e.erasure()(context.Background(), consume.Record{Topic: "identity.events", Key: []byte("newbie"), Value: envelope(t, "reg-1", typeUserRegistered, &identityv1.UserRegistered{UserId: "newbie", RegisteredAt: timestamppb.New(t0)})}); err != nil {
		t.Fatalf("erasure consumer on UserRegistered: %v", err)
	}
	if _, err := e.repo.RegisteredAt(context.Background(), "newbie"); err == nil {
		t.Fatal("the erasure consumer wrote a registration")
	}
}

func TestIdentityConsumerSkipsUserErased(t *testing.T) {
	// REF-8
	e := newEnv(t)
	e.seedUser(t, "U")
	if err := e.identity(context.Background(), erasedRecord(t, "ev-1", "U")); err != nil {
		t.Fatalf("identity consumer on UserErased: %v", err)
	}
	if err := e.identity(context.Background(), erasedRecord(t, "ev-2", "ghost")); err != nil {
		t.Fatalf("identity consumer on UserErased: %v", err)
	}
	if _, err := e.repo.RegisteredAt(context.Background(), "ghost"); err == nil {
		t.Error("REF-8 a referral_accounts row was written for the erased user")
	}
	if c, a, o := e.hasIdentity("U"); !c || !a || !o {
		t.Errorf("REF-8 the identity consumer erased rows: %v %v %v", c, a, o)
	}
	if err := e.order(context.Background(), erasedRecord(t, "ev-3", "U")); err != nil {
		t.Fatalf("order consumer on UserErased: %v", err)
	}
}
