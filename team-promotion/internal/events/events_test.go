package events_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/migrate"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/reqctx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/testkit"
	commonv1 "github.com/buidangphuc/team-promotion/generated/platform/common/v1"
	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/events"
	"github.com/buidangphuc/team-promotion/internal/store"
)

const (
	topic = "promotion.events"
	table = "promotion_outbox"
)

func adminCtx(base context.Context, id, reqID string) context.Context {
	ctx := auth.WithPrincipal(base, auth.Principal{ID: id, Type: auth.TypeUser, Scopes: []string{"admin"}})
	return reqctx.WithRequestID(ctx, reqID)
}

func envelopeOf(t *testing.T, raw []byte) *eventsv1.EventEnvelope {
	t.Helper()
	var env eventsv1.EventEnvelope
	if err := proto.Unmarshal(raw, &env); err != nil {
		t.Fatalf("payload is not an EventEnvelope: %v", err)
	}
	return &env
}

func TestEventTypeConstants(t *testing.T) {
	if events.TypeVoucherChanged != "platform.promotion.v1.VoucherChanged" ||
		events.TypeFlashSaleChanged != "platform.promotion.v1.FlashSaleChanged" {
		t.Fatalf("constants = %q %q", events.TypeVoucherChanged, events.TypeFlashSaleChanged)
	}
}

// Scenario: The envelope carries identity and ids
func TestEnvelopeCarriesPrincipalAndRequestID(t *testing.T) {
	ctx := adminCtx(t.Context(), "u-admin", "req-1")
	v := &promotionv1.Voucher{Id: "V1", Code: "SAVE10"}

	row, err := events.Voucher(ctx, topic, v)
	if err != nil {
		t.Fatal(err)
	}
	if row.AggregateType != "voucher" || row.AggregateID != "V1" ||
		row.EventType != events.TypeVoucherChanged || row.Topic != topic ||
		row.RequestID != "req-1" || row.EventID == "" {
		t.Fatalf("row = %+v", row)
	}
	env := envelopeOf(t, row.Payload)
	if env.GetEventId() != row.EventID || env.GetType() != events.TypeVoucherChanged ||
		env.GetRequestId() != "req-1" || env.GetOccurredAt().AsTime().IsZero() ||
		env.GetPrincipal().GetId() != "u-admin" ||
		env.GetPrincipal().GetType() != commonv1.PrincipalType_PRINCIPAL_TYPE_USER {
		t.Fatalf("envelope = %v", env)
	}
	var got promotionv1.VoucherChanged
	if err := proto.Unmarshal(env.GetPayload(), &got); err != nil || !proto.Equal(got.GetVoucher(), v) {
		t.Fatalf("payload = %v err=%v", &got, err)
	}

	again, err := events.Voucher(ctx, topic, v)
	if err != nil || again.EventID == row.EventID {
		t.Fatalf("event id must be fresh: %q vs %q err=%v", again.EventID, row.EventID, err)
	}

	c := &promotionv1.FlashSaleCampaign{Id: "C1"}
	frow, err := events.FlashSale(ctx, topic, c)
	if err != nil {
		t.Fatal(err)
	}
	if frow.AggregateType != "flash_sale" || frow.AggregateID != "C1" ||
		frow.EventType != events.TypeFlashSaleChanged || frow.Topic != topic || frow.RequestID != "req-1" {
		t.Fatalf("flash row = %+v", frow)
	}
	fenv := envelopeOf(t, frow.Payload)
	var fgot promotionv1.FlashSaleChanged
	if err := proto.Unmarshal(fenv.GetPayload(), &fgot); err != nil || !proto.Equal(fgot.GetCampaign(), c) ||
		fenv.GetPrincipal().GetId() != "u-admin" || fenv.GetType() != events.TypeFlashSaleChanged ||
		fenv.GetEventId() != frow.EventID {
		t.Fatalf("flash envelope = %v err=%v", fenv, err)
	}
}

// Scenario: An envelope without a principal omits it
func TestEnvelopeWithoutPrincipalOmitsIt(t *testing.T) {
	base := t.Context()
	emptyID := auth.WithPrincipal(base, auth.Principal{ID: "", Type: auth.TypeUser})
	for name, ctx := range map[string]context.Context{"none": base, "empty id": emptyID} {
		v := &promotionv1.Voucher{Id: "V1"}
		row, err := events.Voucher(ctx, topic, v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		env := envelopeOf(t, row.Payload)
		if env.GetPrincipal() != nil {
			t.Fatalf("%s: principal = %v, want nil", name, env.GetPrincipal())
		}
		var got promotionv1.VoucherChanged
		if err := proto.Unmarshal(env.GetPayload(), &got); err != nil || !proto.Equal(got.GetVoucher(), v) {
			t.Fatalf("%s: payload = %v err=%v", name, &got, err)
		}
		frow, err := events.FlashSale(ctx, topic, &promotionv1.FlashSaleCampaign{Id: "C1"})
		if err != nil || envelopeOf(t, frow.Payload).GetPrincipal() != nil {
			t.Fatalf("%s: flash sale envelope keeps a principal, err=%v", name, err)
		}
	}
}

// Scenario: The relayed record is keyed by the entity id
func TestRelayedRecordKeyAndHeaders(t *testing.T) {
	sub, err := fs.Sub(store.Migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	url := testkit.Schema(t, testkit.SharedPostgres(t))
	if err := migrate.Up(t.Context(), url, sub); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ob, err := outbox.NewStore(pool, table)
	if err != nil {
		t.Fatal(err)
	}

	row, err := events.Voucher(adminCtx(t.Context(), "u-admin", "req-1"), topic, &promotionv1.Voucher{Id: "V1"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if err := ob.EnqueueTx(t.Context(), tx, row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	rec := testkit.NewRecorder()
	relayer, err := outbox.NewRelayer(ob, rec, outbox.RelayerOptions{
		Topic: topic, Config: config.Outbox{BatchSize: 10},
		BaseBackoff: time.Nanosecond, MaxBackoff: 2 * time.Nanosecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := relayer.RelayOnce(t.Context()); err != nil {
		t.Fatal(err)
	}

	recs := rec.Records()
	if len(recs) != 1 {
		t.Fatalf("records = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.Topic != topic || string(r.Key) != "V1" {
		t.Fatalf("topic %q key %q, want %s / V1", r.Topic, r.Key, topic)
	}
	hdr := map[string]string{}
	for _, h := range r.Headers {
		hdr[h.Key] = string(h.Value)
	}
	if hdr["event-id"] != row.EventID || hdr["event-type"] != events.TypeVoucherChanged {
		t.Fatalf("headers = %v", hdr)
	}
	if envelopeOf(t, r.Value).GetEventId() != row.EventID {
		t.Fatal("record value is not the enqueued envelope")
	}
}
