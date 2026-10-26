package rpc_test

import (
	"context"
	"encoding/base64"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auth"
	commonv1 "github.com/buidangphuc/team-payment/generated/platform/common/v1"
	paymentv1 "github.com/buidangphuc/team-payment/generated/platform/payment/v1"
	"github.com/buidangphuc/team-payment/internal/store"
)

func seededLedger(t *testing.T, n int) (context.Context, func(*commonv1.PageRequest) (*paymentv1.ListLedgerEntriesResponse, error)) {
	t.Helper()
	st := newStores()
	h := newHandler(t, st.service(t, nil), permissiveAudit)
	for i := 1; i <= n; i++ {
		if _, err := st.ledger.AppendEntry(context.Background(), store.LedgerEntry{
			SellerID: "seller-1", Type: store.LedgerTypeOrderSettlement,
			Amount: int64(i), Status: store.LedgerStatusCompleted,
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := principalCtx("seller-1", auth.TypeUser, "listing.write")
	return ctx, func(p *commonv1.PageRequest) (*paymentv1.ListLedgerEntriesResponse, error) {
		return h.ListLedgerEntries(ctx, &paymentv1.ListLedgerEntriesRequest{Page: p})
	}
}

// Scenario: A cursor from the previous build still pages
func TestLedgerCursorFormatIsUnchanged(t *testing.T) {
	_, list := seededLedger(t, 130)
	legacy := base64.RawURLEncoding.EncodeToString([]byte("o:20"))
	res, err := list(&commonv1.PageRequest{Cursor: legacy, PageSize: 10})
	if err != nil {
		t.Fatalf("legacy cursor: %v", err)
	}
	if len(res.GetEntries()) != 10 || res.GetEntries()[0].GetAmount() != 110 {
		t.Fatalf("page must start at the 21st newest entry (amount 110): %v", res.GetEntries())
	}

	first, err := list(&commonv1.PageRequest{PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetPage().GetNextCursor() != legacy {
		t.Fatalf("next cursor = %q, want the unchanged format %q", first.GetPage().GetNextCursor(), legacy)
	}
	second, err := list(&commonv1.PageRequest{Cursor: first.GetPage().GetNextCursor(), PageSize: 20})
	if err != nil || second.GetEntries()[0].GetAmount() != 110 {
		t.Fatalf("second page: %v %v", second, err)
	}
}

// Scenario: The page size is clamped
func TestLedgerPageSizeClamped(t *testing.T) {
	_, list := seededLedger(t, 130)
	for _, c := range []struct {
		size int32
		want int
	}{{0, 20}, {500, 100}, {7, 7}} {
		res, err := list(&commonv1.PageRequest{PageSize: c.size})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.GetEntries()) != c.want {
			t.Fatalf("page size %d: got %d entries, want %d", c.size, len(res.GetEntries()), c.want)
		}
	}
}

// Scenario: A malformed cursor is rejected
func TestLedgerMalformedCursorInvalidArgument(t *testing.T) {
	_, list := seededLedger(t, 5)
	for _, cursor := range []string{"not-a-cursor", "%%%", base64.RawURLEncoding.EncodeToString([]byte("o:-3")), base64.RawURLEncoding.EncodeToString([]byte("x"))} {
		_, err := list(&commonv1.PageRequest{Cursor: cursor})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("cursor %q: want InvalidArgument, got %v", cursor, err)
		}
	}
}
