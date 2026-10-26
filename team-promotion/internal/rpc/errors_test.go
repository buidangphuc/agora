package rpc_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/outbox"
	eventsv1 "github.com/buidangphuc/team-promotion/generated/platform/events/v1"
	promotionv1 "github.com/buidangphuc/team-promotion/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-promotion/internal/events"
	"github.com/buidangphuc/team-promotion/internal/store"
)

type failingGetVouchers struct {
	store.VoucherRepository
	err func(ctx context.Context) error
}

func (f failingGetVouchers) GetByCode(ctx context.Context, _ string) (store.Voucher, error) {
	return store.Voucher{}, f.err(ctx)
}

func TestInternalErrorIsGeneric(t *testing.T) {
	cause := errors.New("pq: connection refused to 10.0.0.5")
	r := startRig(t, rigOpts{vouchers: failingGetVouchers{
		err: func(context.Context) error { return cause },
	}})
	_, err := r.vc.GetVoucher(t.Context(), &promotionv1.GetVoucherRequest{Code: "ANY"})
	st := status.Convert(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q, want Internal \"internal error\"", st.Code(), st.Message())
	}
	if strings.Contains(st.Message(), "pq") || strings.Contains(st.Message(), "10.0.0.5") {
		t.Fatalf("message leaks internal detail: %q", st.Message())
	}
}

func TestDuplicateVoucherCodeIsAlreadyExists(t *testing.T) {
	pool, ob := newPostgres(t)
	r := startRig(t, rigOpts{
		vouchers:     store.NewPostgresVoucherRepository(pool, ob),
		reservations: store.NewPostgresReservationRepository(pool),
		voucherEvent: realVoucherEvent,
	})
	req := platformVoucher("DUP-1", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)
	if _, err := r.vc.CreateVoucher(adminCtx(t), req); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := r.vc.CreateVoucher(adminCtx(t), req)
	st := status.Convert(err)
	if st.Code() != codes.AlreadyExists {
		t.Fatalf("second create: got %v %q, want AlreadyExists", st.Code(), st.Message())
	}
	for _, leak := range []string{"DUP-1", "vouchers_code"} {
		if strings.Contains(st.Message(), leak) {
			t.Fatalf("message leaks %q: %q", leak, st.Message())
		}
	}
}

func TestCanceledAndDeadlineMapToCodes(t *testing.T) {
	r := startRig(t, rigOpts{vouchers: failingGetVouchers{
		err: func(ctx context.Context) error { return fmt.Errorf("get voucher: %w", ctx.Err()) },
	}})
	req := &promotionv1.GetVoucherRequest{Code: "ANY"}

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.voucherH.GetVoucher(canceled, req); code(err) != codes.Canceled {
		t.Errorf("canceled context: got %v, want Canceled (err=%v)", code(err), err)
	}

	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Hour))
	defer stop()
	if _, err := r.voucherH.GetVoucher(expired, req); code(err) != codes.DeadlineExceeded {
		t.Errorf("expired deadline: got %v, want DeadlineExceeded (err=%v)", code(err), err)
	}
}

func TestGetVoucherNotFoundDoesNotEchoCode(t *testing.T) {
	r := startRig(t, rigOpts{})
	_, err := r.vc.GetVoucher(t.Context(), &promotionv1.GetVoucherRequest{Code: "NOPE-9"})
	st := status.Convert(err)
	if st.Code() != codes.NotFound || st.Message() != "voucher not found" {
		t.Fatalf("got %v %q, want NotFound \"voucher not found\"", st.Code(), st.Message())
	}
}

func TestDomainPackagesDoNotImportGRPCCodes(t *testing.T) {
	for _, pkg := range []string{"voucher", "flashsale", "subscription", "sponsored", "listing"} {
		out, err := exec.CommandContext(t.Context(), "go", "list", "-f", `{{join .Imports "\n"}}`, "../"+pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		for _, imp := range strings.Fields(string(out)) {
			if imp == "google.golang.org/grpc/codes" {
				t.Errorf("internal/%s imports %s in non-test code", pkg, imp)
			}
		}
	}
}

func TestRefusedCallerWritesNoOutboxRow(t *testing.T) {
	pool, ob := newPostgres(t)
	r := startRig(t, rigOpts{
		vouchers:     store.NewPostgresVoucherRepository(pool, ob),
		reservations: store.NewPostgresReservationRepository(pool),
		voucherEvent: realVoucherEvent,
	})
	if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("OB-OK", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM)); err != nil {
		t.Fatalf("admin create: %v", err)
	}
	before := outboxCount(t, pool)
	if before != 1 {
		t.Fatalf("outbox rows after admin create = %d, want 1", before)
	}
	_, err := r.vc.CreateVoucher(sellerCtx(t), platformVoucher("OB-NO", promotionv1.VoucherScope_VOUCHER_SCOPE_PLATFORM))
	if code(err) != codes.PermissionDenied {
		t.Fatalf("seller platform voucher: got %v, want PermissionDenied", code(err))
	}
	if after := outboxCount(t, pool); after != before {
		t.Fatalf("outbox rows = %d, want unchanged %d", after, before)
	}
}

func realVoucherEvent(ctx context.Context, v store.Voucher) (outbox.Row, error) {
	return events.Voucher(ctx, "promotion.events", &promotionv1.Voucher{Id: v.ID, Code: v.Code})
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *lockedBuffer) requestIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var ids []string
	for line := range bytes.SplitSeq(b.buf.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if id, ok := rec["request_id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func envelopeRequestIDs(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT payload FROM `+outboxTable+` ORDER BY seq`)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var env eventsv1.EventEnvelope
		if err := proto.Unmarshal(payload, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		ids = append(ids, env.GetRequestId())
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return ids
}

func TestCallWithoutRequestIDGetsOneInEnvelope(t *testing.T) {
	pool, ob := newPostgres(t)
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := startRig(t, rigOpts{
		vouchers:     store.NewPostgresVoucherRepository(pool, ob),
		reservations: store.NewPostgresReservationRepository(pool),
		voucherEvent: realVoucherEvent,
		logger:       logger,
	})

	logs.reset()
	if _, err := r.vc.CreateVoucher(adminCtx(t), platformVoucher("RID-1", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Fatalf("create without request id: %v", err)
	}
	ids := envelopeRequestIDs(t, pool)
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("envelope request ids = %q, want one non-empty id", ids)
	}
	logged := logs.requestIDs()
	if len(logged) == 0 {
		t.Fatal("server logged no request_id for the call")
	}
	for _, id := range logged {
		if id != ids[0] {
			t.Fatalf("logged request id %q differs from envelope request id %q", id, ids[0])
		}
	}

	withID := metadata.AppendToOutgoingContext(adminCtx(t), "x-request-id", "req-9")
	if _, err := r.vc.CreateVoucher(withID, platformVoucher("RID-2", promotionv1.VoucherScope_VOUCHER_SCOPE_SHOP)); err != nil {
		t.Fatalf("create with request id: %v", err)
	}
	ids = envelopeRequestIDs(t, pool)
	if len(ids) != 2 || ids[1] != "req-9" {
		t.Fatalf("envelope request ids = %q, want second to be req-9", ids)
	}
}
