package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type outboxRow struct {
	eventID, aggregateType, aggregateID, eventType string
}

func outboxRows(t *testing.T, pool *pgxpool.Pool) []outboxRow {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT event_id, aggregate_type, aggregate_id, event_type FROM `+outboxTable+` ORDER BY seq`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	var out []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.eventID, &r.aggregateType, &r.aggregateID, &r.eventType); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("outbox rows: %v", err)
	}
	return out
}

func TestCreateVoucherEnqueuesOutboxRow(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))

	var built store.Voucher
	v, err := repo.Create(t.Context(), store.Voucher{Code: "OB10", Scope: 2, DiscountType: 1, DiscountValue: 10, Quota: 5}, func(created store.Voucher) (outbox.Row, error) {
		built = created
		return voucherEvent(created)
	})
	if err != nil {
		t.Fatalf("create voucher: %v", err)
	}
	if v.ID == "" || built.ID != v.ID {
		t.Fatalf("builder saw id %q, stored id %q: the event must carry the generated id", built.ID, v.ID)
	}
	rows := outboxRows(t, pool)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	want := outboxRow{eventID: "ev-voucher-" + v.ID, aggregateType: "voucher", aggregateID: v.ID, eventType: voucherChanged}
	if rows[0] != want {
		t.Fatalf("outbox row = %+v, want %+v", rows[0], want)
	}
	var status string
	var payload []byte
	var topic string
	if err := pool.QueryRow(t.Context(), `SELECT status, payload, topic FROM `+outboxTable).Scan(&status, &payload, &topic); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if status != "pending" || string(payload) != "voucher-payload" || topic != "promotion.events" {
		t.Fatalf("row status=%q payload=%q topic=%q, want pending voucher-payload promotion.events", status, payload, topic)
	}
}

func TestCreateCampaignEnqueuesFlashSaleChanged(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresFlashSaleRepository(pool, newOutbox(t, pool))
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	c, err := repo.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: "l-ob", SalePrice: 100, StockCap: 20, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, campaignEvent)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	rows := outboxRows(t, pool)
	if len(rows) != 1 {
		t.Fatalf("outbox rows = %d, want 1", len(rows))
	}
	want := outboxRow{eventID: "ev-campaign-" + c.ID, aggregateType: "flash_sale", aggregateID: c.ID, eventType: flashSaleChanged}
	if rows[0] != want {
		t.Fatalf("outbox row = %+v, want %+v", rows[0], want)
	}
}

func TestVoucherInsertFailureLeavesNoOutboxRow(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))

	if _, err := repo.Create(t.Context(), store.Voucher{Code: "DUP"}, voucherEvent); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if n := outboxCount(t, pool); n != 1 {
		t.Fatalf("outbox rows after first create = %d, want 1", n)
	}
	if _, err := repo.Create(t.Context(), store.Voucher{Code: "DUP"}, voucherEvent); err == nil {
		t.Fatal("second create with the same code succeeded, want an error")
	}
	if n := outboxCount(t, pool); n != 1 {
		t.Fatalf("outbox rows after failed insert = %d, want unchanged 1", n)
	}
}

func TestVoucherBuilderFailureLeavesNoVoucherAndNoOutboxRow(t *testing.T) {
	pool := newTestPool(t)
	repo := store.NewPostgresVoucherRepository(pool, newOutbox(t, pool))
	boom := errors.New("boom")

	_, err := repo.Create(t.Context(), store.Voucher{Code: "NOEVT"}, func(store.Voucher) (outbox.Row, error) {
		return outbox.Row{}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("create err = %v, want the builder error", err)
	}
	if _, err := repo.GetByCode(t.Context(), "NOEVT"); !errors.Is(err, store.ErrVoucherNotFound) {
		t.Fatalf("voucher committed without its event: %v", err)
	}
	if n := outboxCount(t, pool); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}

func TestRedemptionWritesNoOutboxRow(t *testing.T) {
	pool := newTestPool(t)
	ob := newOutbox(t, pool)
	vouchers := store.NewPostgresVoucherRepository(pool, ob)
	reservations := store.NewPostgresReservationRepository(pool)
	campaigns := store.NewPostgresFlashSaleRepository(pool, ob)
	holds := store.NewPostgresFlashHoldRepository(pool)
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	v, err := vouchers.Create(t.Context(), store.Voucher{Code: "RED10", Quota: 5}, voucherEvent)
	if err != nil {
		t.Fatalf("create voucher: %v", err)
	}
	c, err := campaigns.Create(t.Context(), store.FlashSaleCampaign{
		ListingID: "l-red", SalePrice: 1, StockCap: 10, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
	}, campaignEvent)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	before := outboxCount(t, pool)
	if before != 2 {
		t.Fatalf("outbox rows after seeding = %d, want 2", before)
	}

	if _, created, err := reservations.Create(t.Context(), store.Reservation{ReservationID: "r1", VoucherID: v.ID, BuyerID: "b1", DiscountAmount: 5}); err != nil || !created {
		t.Fatalf("reserve voucher: created=%v err=%v", created, err)
	}
	if err := reservations.UpdateStatus(t.Context(), "r1", store.ReservationCommitted); err != nil {
		t.Fatalf("commit reservation: %v", err)
	}
	if err := vouchers.IncrementUsed(t.Context(), v.ID, 1); err != nil {
		t.Fatalf("increment used: %v", err)
	}
	if err := reservations.UpdateStatus(t.Context(), "r1", store.ReservationReleased); err != nil {
		t.Fatalf("release reservation: %v", err)
	}
	if _, created, err := holds.Reserve(t.Context(), store.FlashHold{ReservationID: "h1", CampaignID: c.ID, ListingID: "l-red", Quantity: 2}, now); err != nil || !created {
		t.Fatalf("reserve hold: created=%v err=%v", created, err)
	}
	if ok, err := holds.Commit(t.Context(), "h1", now); err != nil || !ok {
		t.Fatalf("commit hold: ok=%v err=%v", ok, err)
	}
	if _, created, err := holds.Reserve(t.Context(), store.FlashHold{ReservationID: "h2", CampaignID: c.ID, ListingID: "l-red", Quantity: 1}, now); err != nil || !created {
		t.Fatalf("reserve second hold: created=%v err=%v", created, err)
	}
	if found, err := holds.Release(t.Context(), "h2", now); err != nil || !found {
		t.Fatalf("release hold: found=%v err=%v", found, err)
	}

	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows after redemption calls = %d, want unchanged %d", after, before)
	}
}
