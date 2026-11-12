package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
	"github.com/buidangphuc/team-search/internal/index"
	"github.com/buidangphuc/team-search/internal/rpc"
)

func newHandlerWithLogger(idx index.Index, logger *slog.Logger) (*rpc.Handler, error) {
	return rpc.NewHandler(idx, nil, newFakeSaved(), logger)
}

type stockLookupIndex struct {
	mockIndex
	mu      sync.Mutex
	stocks  map[string]int32
	err     error
	block   bool
	lookups int
	lastIDs []string
}

func (s *stockLookupIndex) ListingStocks(ctx context.Context, ids []string) (map[string]int32, error) {
	s.mu.Lock()
	s.lookups++
	s.lastIDs = append([]string(nil), ids...)
	s.mu.Unlock()
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	out := map[string]int32{}
	for _, id := range ids {
		if v, ok := s.stocks[id]; ok {
			out[id] = v
		}
	}
	return out, nil
}

func svcCtx() context.Context { return principalCtx("team-ai", "service", "search:read") }

func lookup(t *testing.T, idx *stockLookupIndex, ids ...string) (*searchv1.GetListingStockResponse, error) {
	t.Helper()
	h := newHandler(t, idx, newFakeSaved())
	return h.GetListingStock(svcCtx(), &searchv1.GetListingStockRequest{ListingIds: ids})
}

func wantEntries(t *testing.T, res *searchv1.GetListingStockResponse, want ...*int32) {
	t.Helper()
	got := res.GetListings()
	if len(got) != len(want) {
		t.Fatalf("%d entries, want %d: %v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		switch {
		case w == nil && g.Stock != nil:
			t.Errorf("entry %d (%s): stock %d, want absent", i, g.GetListingId(), g.GetStock())
		case w != nil && g.Stock == nil:
			t.Errorf("entry %d (%s): stock absent, want %d", i, g.GetListingId(), *w)
		case w != nil && g.GetStock() != *w:
			t.Errorf("entry %d (%s): stock %d, want %d", i, g.GetListingId(), g.GetStock(), *w)
		}
	}
}

func p32(v int32) *int32 { return &v }

func TestGetListingStockRequestOrder(t *testing.T) {
	idx := &stockLookupIndex{stocks: map[string]int32{"A": 5, "B": 0, "C": 12}}
	res, err := lookup(t, idx, "C", "A", "B")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries(t, res, p32(12), p32(5), p32(0))
	for i, id := range []string{"C", "A", "B"} {
		if res.GetListings()[i].GetListingId() != id {
			t.Errorf("entry %d id %q, want %q", i, res.GetListings()[i].GetListingId(), id)
		}
	}
}

func TestGetListingStockUnknownListing(t *testing.T) {
	idx := &stockLookupIndex{stocks: map[string]int32{"A": 5}}
	res, err := lookup(t, idx, "A", "U")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries(t, res, p32(5), nil)
	if res.GetListings()[1].GetListingId() != "U" {
		t.Errorf("unknown entry lost its id: %v", res.GetListings()[1])
	}
}

func TestGetListingStockRepeatedID(t *testing.T) {
	idx := &stockLookupIndex{stocks: map[string]int32{"A": 5}}
	res, err := lookup(t, idx, "A", "A")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries(t, res, p32(5), p32(5))
}

func TestGetListingStockTombstone(t *testing.T) {
	idx := &stockLookupIndex{stocks: map[string]int32{"live": 5}}
	res, err := lookup(t, idx, "dead", "live")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries(t, res, nil, p32(5))
}

func TestGetListingStockNoProjectedStock(t *testing.T) {
	idx := &stockLookupIndex{stocks: map[string]int32{"zero": 0}}
	res, err := lookup(t, idx, "legacy", "zero")
	if err != nil {
		t.Fatal(err)
	}
	wantEntries(t, res, nil, p32(0))
}

func TestGetListingStockIDLimit(t *testing.T) {
	ids := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("L%d", i)
		}
		return out
	}
	idx := &stockLookupIndex{stocks: map[string]int32{}}
	res, err := lookup(t, idx, ids(10)...)
	if err != nil {
		t.Fatalf("10 ids: %v", err)
	}
	if len(res.GetListings()) != 10 {
		t.Fatalf("10 ids gave %d entries", len(res.GetListings()))
	}
	idx.lookups = 0
	_, err = lookup(t, idx, ids(11)...)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("11 ids: code %v (%v)", status.Code(err), err)
	}
	if idx.lookups != 0 {
		t.Error("11 ids reached the index")
	}
}

func TestGetListingStockInvalidRequest(t *testing.T) {
	for name, ids := range map[string][]string{"empty list": nil, "empty id": {"A", ""}} {
		t.Run(name, func(t *testing.T) {
			idx := &stockLookupIndex{stocks: map[string]int32{"A": 1}}
			res, err := lookup(t, idx, ids...)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("code %v (%v)", status.Code(err), err)
			}
			if len(res.GetListings()) != 0 {
				t.Errorf("entries returned: %v", res)
			}
			if idx.lookups != 0 {
				t.Error("invalid request reached the index")
			}
		})
	}
}

func TestGetListingStockPrincipals(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"no principal", principalCtx("", ""), codes.Unauthenticated},
		{"anonymous with search:read", principalCtx("anonymous", "anonymous", "search:read"), codes.Unauthenticated},
		{"user with search:read", principalCtx("buyer", "user", "search:read"), codes.PermissionDenied},
		{"service without search:read", principalCtx("svc", "service", "other:read"), codes.PermissionDenied},
		{"service with search:read", principalCtx("svc", "service", "search:read"), codes.OK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idx := &stockLookupIndex{stocks: map[string]int32{"A": 5}}
			h := newHandler(t, idx, newFakeSaved())
			res, err := h.GetListingStock(c.ctx, &searchv1.GetListingStockRequest{ListingIds: []string{"A"}})
			if status.Code(err) != c.want {
				t.Fatalf("code %v (%v), want %v", status.Code(err), err, c.want)
			}
			if c.want == codes.OK {
				wantEntries(t, res, p32(5))
				return
			}
			if len(res.GetListings()) != 0 {
				t.Errorf("refused call carried entries: %v", res)
			}
			if idx.lookups != 0 {
				t.Error("refused call reached the index")
			}
		})
	}
}

func TestGetListingStockIndexUnavailable(t *testing.T) {
	idx := &stockLookupIndex{err: errors.New("connection refused")}
	res, err := lookup(t, idx, "A")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code %v (%v)", status.Code(err), err)
	}
	if len(res.GetListings()) != 0 {
		t.Errorf("entries returned: %v", res)
	}
}

func TestGetListingStockDeadline(t *testing.T) {
	idx := &stockLookupIndex{block: true}
	start := time.Now()
	res, err := lookup(t, idx, "A")
	elapsed := time.Since(start)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code %v (%v)", status.Code(err), err)
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("took %v, want well within 400ms", elapsed)
	}
	if len(res.GetListings()) != 0 {
		t.Errorf("entries returned: %v", res)
	}
}

func TestGetListingStockLogsCountOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	idx := &stockLookupIndex{stocks: map[string]int32{"listing-AAA": 1, "listing-BBB": 2}}
	h, err := newHandlerWithLogger(idx, logger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetListingStock(svcCtx(), &searchv1.GetListingStockRequest{ListingIds: []string{"listing-AAA", "listing-BBB"}}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "listing-AAA") || strings.Contains(out, "listing-BBB") {
		t.Errorf("log holds an id value: %s", out)
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "GetListingStock") {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("%d lines name GetListingStock, want 1: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "2") {
		t.Errorf("line lacks the id count: %s", lines[0])
	}
}
