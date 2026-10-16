package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	eventsv1 "github.com/buidangphuc/team-order/generated/platform/events/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	paymentv1 "github.com/buidangphuc/team-order/generated/platform/payment/v1"
	"github.com/buidangphuc/team-order/internal/events"
	"github.com/buidangphuc/team-order/internal/repository"
)

const reminderOffset = 2 * time.Hour

type reminderRowsFn func(t *testing.T, orderID string) []*orderv1.OrderPaymentReminder

func decodeReminderPayloads(t *testing.T, payloads [][]byte) []*orderv1.OrderPaymentReminder {
	t.Helper()
	var out []*orderv1.OrderPaymentReminder
	for _, p := range payloads {
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(p, &env); err != nil {
			t.Fatal(err)
		}
		if env.GetType() != events.OrderPaymentReminderEventType {
			continue
		}
		var ev orderv1.OrderPaymentReminder
		if err := proto.Unmarshal(env.GetPayload(), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, &ev)
	}
	return out
}

func newUnpaid(t *testing.T, repo repository.OrderRepository, method paymentv1.PaymentMethod, due *time.Time) repository.Order {
	t.Helper()
	o, err := repo.CreateOrder(context.Background(), repository.Order{BuyerID: "b", SellerID: "s", TotalAmount: 10,
		Currency: "VND", PaymentMethod: int32(method), PaymentDueAt: due,
		Items: []repository.OrderItem{{ListingID: "l", Quantity: 1, UnitPrice: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func ids(os []repository.Order) map[string]bool {
	m := map[string]bool{}
	for _, o := range os {
		m[o.ID] = true
	}
	return m
}

const momo = paymentv1.PaymentMethod_PAYMENT_METHOD_MOCK_MOMO

// runReminderContract: FindReminderDue selects exactly the Pending, online,
// unstamped orders whose window is open; MarkReminderSent is the once-only CAS
// that stamps the order and writes the OrderPaymentReminder row together.
// `due` is a deadline unique to this run so shared databases never collide.
func runReminderContract(t *testing.T, repo repository.OrderRepository, rows reminderRowsFn, due time.Time) {
	ctx := context.Background()
	find := func(now time.Time) map[string]bool {
		t.Helper()
		got, err := repo.FindReminderDue(ctx, now, reminderOffset, 100)
		if err != nil {
			t.Fatal(err)
		}
		return ids(got)
	}

	t.Run("window is inclusive at deadline minus offset and exclusive at the deadline", func(t *testing.T) { // Order expiry: Reminder boundary is inclusive at deadline minus offset
		d := due
		o := newUnpaid(t, repo, momo, &d)
		start := d.Add(-reminderOffset)
		if find(start.Add(-time.Second))[o.ID] {
			t.Fatal("one second before the window must not be due")
		}
		if !find(start)[o.ID] {
			t.Fatal("exactly deadline-minus-offset must be due")
		}
		if !find(d.Add(-time.Second))[o.ID] {
			t.Fatal("inside the window must be due")
		}
		if find(d)[o.ID] {
			t.Fatal("at the deadline the order is past the reminder window")
		}
		if find(d.Add(time.Second))[o.ID] {
			t.Fatal("past the deadline must never be due")
		}
	})

	t.Run("paid cod and deadline-less orders are never due", func(t *testing.T) { // Order expiry: A paid order is never reminded; A COD order is never reminded
		d := due.Add(24 * time.Hour)
		in := d.Add(-time.Hour)
		paid := newUnpaid(t, repo, momo, &d)
		if _, err := repo.UpdateOrderStatusFrom(ctx, paid.ID, repository.OrderStatusPaid,
			[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
			t.Fatal(err)
		}
		cod := newUnpaid(t, repo, paymentv1.PaymentMethod_PAYMENT_METHOD_COD, &d)
		noDue := newUnpaid(t, repo, momo, nil)
		live := newUnpaid(t, repo, momo, &d)
		got := find(in)
		if got[paid.ID] || got[cod.ID] || got[noDue.ID] {
			t.Fatalf("must not be due: paid=%v cod=%v noDue=%v", got[paid.ID], got[cod.ID], got[noDue.ID])
		}
		if !got[live.ID] {
			t.Fatal("the unpaid online order in the window must be due")
		}
	})

	t.Run("mark stamps the order and writes exactly one row, once", func(t *testing.T) { // Order expiry: Repeated sweeps never repeat the reminder
		d := due.Add(48 * time.Hour)
		o := newUnpaid(t, repo, momo, &d)
		now := d.Add(-time.Hour)
		got, err := repo.MarkReminderSent(ctx, o.ID, now)
		if err != nil || got.PaymentReminderSentAt == nil || got.Status != repository.OrderStatusPending {
			t.Fatalf("mark: %v %+v", err, got)
		}
		stored, _ := repo.GetOrder(ctx, o.ID)
		if stored.PaymentReminderSentAt == nil {
			t.Fatal("stamp must persist")
		}
		r := rows(t, o.ID)
		if len(r) != 1 || r[0].GetOrderId() != o.ID || r[0].GetPaymentDueAt() == nil ||
			!d.Equal(r[0].GetPaymentDueAt().AsTime()) || r[0].GetBuyerId() != "b" || r[0].GetTotalAmount() != 10 {
			t.Fatalf("reminder row: %+v", r)
		}
		if find(now)[o.ID] {
			t.Fatal("a stamped order is no longer due")
		}
		if _, err := repo.MarkReminderSent(ctx, o.ID, now); !errors.Is(err, repository.ErrStatusConflict) {
			t.Fatalf("second mark: want ErrStatusConflict, got %v", err)
		}
		if n := len(rows(t, o.ID)); n != 1 {
			t.Fatalf("still exactly one row, got %d", n)
		}
	})

	t.Run("mark refuses a non-pending order and writes nothing", func(t *testing.T) { // Order expiry: A paid order is never reminded
		d := due.Add(72 * time.Hour)
		o := newUnpaid(t, repo, momo, &d)
		if _, err := repo.UpdateOrderStatusFrom(ctx, o.ID, repository.OrderStatusPaid,
			[]repository.OrderStatus{repository.OrderStatusPending}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.MarkReminderSent(ctx, o.ID, d.Add(-time.Hour)); !errors.Is(err, repository.ErrStatusConflict) {
			t.Fatalf("want ErrStatusConflict, got %v", err)
		}
		if stored, _ := repo.GetOrder(ctx, o.ID); stored.PaymentReminderSentAt != nil {
			t.Fatal("a paid order must not be stamped")
		}
		if n := len(rows(t, o.ID)); n != 0 {
			t.Fatalf("want no row, got %d", n)
		}
	})

	t.Run("concurrent marks produce one winner and one row", func(t *testing.T) { // Order expiry: Repeated sweeps never repeat the reminder
		d := due.Add(96 * time.Hour)
		o := newUnpaid(t, repo, momo, &d)
		var wg sync.WaitGroup
		var mu sync.Mutex
		wins := 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := repo.MarkReminderSent(ctx, o.ID, d.Add(-time.Hour)); err == nil {
					mu.Lock()
					wins++
					mu.Unlock()
				} else if !errors.Is(err, repository.ErrStatusConflict) {
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		if wins != 1 {
			t.Fatalf("want exactly 1 winner, got %d", wins)
		}
		if n := len(rows(t, o.ID)); n != 1 {
			t.Fatalf("want exactly 1 row, got %d", n)
		}
	})
}

// Scenario: Order expiry: A reminder pass handles at most 100 orders per tick
func runFindReminderDueBatchBound(t *testing.T, repo repository.OrderRepository, total int, due time.Time) {
	ctx := context.Background()
	for i := 0; i < total; i++ {
		d := due
		newUnpaid(t, repo, momo, &d)
	}
	now := due.Add(-time.Hour)
	got, err := repo.FindReminderDue(ctx, now, reminderOffset, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 {
		t.Fatalf("want exactly the 100-row bound, got %d", len(got))
	}
	for _, o := range got {
		if _, err := repo.MarkReminderSent(ctx, o.ID, now); err != nil {
			t.Fatal(err)
		}
	}
	rest, err := repo.FindReminderDue(ctx, now, reminderOffset, 100)
	wantRest := total - 100
	if wantRest > 100 {
		wantRest = 100 // the limit caps the page
	}
	if err != nil || len(rest) != wantRest {
		t.Fatalf("remaining after a stamped batch: want %d, got %d (%v)", wantRest, len(rest), err)
	}
}

func inMemoryReminderRepo() (*repository.InMemoryOrderRepository, reminderRowsFn) {
	outbox := repository.NewInMemoryOutboxRepository()
	repo := repository.NewInMemoryOrderRepository(repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow), repository.WithInMemoryOutbox(outbox))
	return repo, func(t *testing.T, id string) []*orderv1.OrderPaymentReminder {
		var payloads [][]byte
		for _, r := range outbox.EnqueuedRows() {
			if r.AggregateID == id {
				payloads = append(payloads, r.Payload)
			}
		}
		return decodeReminderPayloads(t, payloads)
	}
}

func TestReminderContract_InMemory(t *testing.T) {
	repo, rows := inMemoryReminderRepo()
	runReminderContract(t, repo, rows, time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC))
}

func TestFindReminderDueBatchBound(t *testing.T) {
	repo, _ := inMemoryReminderRepo()
	runFindReminderDueBatchBound(t, repo, 250, time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC))
}

func uniqueDue() time.Time {
	// Bounded (< ~9.5 years of minutes, no int64 overflow) and minute-aligned, so it
	// survives Postgres TIMESTAMPTZ microsecond precision.
	off := time.Now().UnixMicro() % 5_000_000
	return time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(off) * time.Minute).Truncate(time.Microsecond)
}

func TestReminderContract_Postgres(t *testing.T) {
	pool := pgPool(t)
	ctx := context.Background()
	repo := repository.NewPostgresOrderRepository(pool, repository.WithPaidOutbox(events.BuildPaidOutboxRow),
		repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow))
	rows := func(t *testing.T, id string) []*orderv1.OrderPaymentReminder {
		r, err := pool.Query(ctx, `SELECT payload FROM order_outbox_events WHERE aggregate_id = $1`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var payloads [][]byte
		for r.Next() {
			var p []byte
			if err := r.Scan(&p); err != nil {
				t.Fatal(err)
			}
			payloads = append(payloads, p)
		}
		return decodeReminderPayloads(t, payloads)
	}
	runReminderContract(t, repo, rows, uniqueDue())
}

func TestFindReminderDueBatchBound_Postgres(t *testing.T) {
	pool := pgPool(t)
	repo := repository.NewPostgresOrderRepository(pool, repository.WithReminderOutbox(events.BuildPaymentReminderOutboxRow))
	runFindReminderDueBatchBound(t, repo, 105, uniqueDue())
}
