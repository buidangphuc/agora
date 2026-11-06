package rpc_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	commonv1 "github.com/buidangphuc/team-order/generated/platform/common/v1"
	orderv1 "github.com/buidangphuc/team-order/generated/platform/order/v1"
	"github.com/buidangphuc/team-order/internal/rpc"
	"github.com/buidangphuc/team-order/internal/store"
)

func (m *mockOrderServiceRepo) HasCompletedOrder(_ context.Context, buyerID string) (bool, error) {
	for _, o := range m.orders {
		if o.BuyerID == buyerID && o.Status == store.OrderStatusCompleted {
			return true, nil
		}
	}
	return false, nil
}

type q7Repo struct {
	mockOrderServiceRepo
	calls int
	err   error
}

func (r *q7Repo) HasCompletedOrder(ctx context.Context, buyerID string) (bool, error) {
	r.calls++
	if r.err != nil {
		return false, r.err
	}
	return r.mockOrderServiceRepo.HasCompletedOrder(ctx, buyerID)
}

func q7Handler(t *testing.T, repo *q7Repo, logger *slog.Logger) *rpc.OrderHandler {
	t.Helper()
	return newHandler(t, newSvc(t, repo, nil, nil, nil, nil, nil, nil), nil, logger)
}

func q7Orders() map[string]store.Order {
	return map[string]store.Order{
		"o_a1": {ID: "o_a1", BuyerID: "buyer_A", SellerID: "s1", Status: store.OrderStatusCompleted},
		"o_a2": {ID: "o_a2", BuyerID: "buyer_A", SellerID: "s1", Status: store.OrderStatusCancelled},
		"o_b1": {ID: "o_b1", BuyerID: "buyer_B", SellerID: "s1", Status: store.OrderStatusShipped},
	}
}

func q7Svc(scopes ...string) context.Context { return serviceCtx(scopes...) }

func TestQ7HasCompletedOrder_ServiceWithOrderReadGetsAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, buyer string
		want        bool
	}{
		{"Buyer with a completed order", "buyer_A", true},
		{"Buyer with only non-completed orders", "buyer_B", false},
		{"Buyer with no orders returns false with OK status", "buyer_C", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := q7Handler(t, &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: q7Orders()}}, nil)
			res, err := h.HasCompletedOrder(q7Svc("order.read"), &orderv1.HasCompletedOrderRequest{BuyerId: tc.buyer})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.GetHasCompleted() != tc.want {
				t.Fatalf("has_completed = %v, want %v", res.GetHasCompleted(), tc.want)
			}
		})
	}
}

func TestQ7HasCompletedOrder_StatusChangeIsVisibleOnTheNextCall(t *testing.T) {
	repo := &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: q7Orders()}}
	h := q7Handler(t, repo, nil)
	call := func() bool {
		res, err := h.HasCompletedOrder(q7Svc("order.read"), &orderv1.HasCompletedOrderRequest{BuyerId: "buyer_B"})
		if err != nil {
			t.Fatal(err)
		}
		return res.GetHasCompleted()
	}
	if call() {
		t.Fatal("want false before completion")
	}
	o := repo.orders["o_b1"]
	o.Status = store.OrderStatusCompleted
	repo.orders["o_b1"] = o
	if !call() {
		t.Fatal("want true after completion")
	}
}

func TestQ7HasCompletedOrder_EmptyBuyerIDIsInvalidArgument(t *testing.T) {
	repo := &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: q7Orders()}}
	h := q7Handler(t, repo, nil)
	res, err := h.HasCompletedOrder(q7Svc("order.read"), &orderv1.HasCompletedOrderRequest{BuyerId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument (err=%v)", status.Code(err), err)
	}
	if res != nil && res.GetHasCompleted() {
		t.Fatal("no has_completed value on failure")
	}
	if repo.calls != 0 {
		t.Fatalf("store queried %d times, want 0", repo.calls)
	}
}

func TestQ7HasCompletedOrder_CallerAuthorization(t *testing.T) {
	user := func(id string, scopes ...string) context.Context {
		return withPrincipal(context.Background(), &commonv1.Principal{
			Id: id, Type: commonv1.PrincipalType_PRINCIPAL_TYPE_USER, Scopes: scopes,
		})
	}
	anon := withPrincipal(context.Background(), &commonv1.Principal{Id: "anonymous", Type: commonv1.PrincipalType_PRINCIPAL_TYPE_ANONYMOUS})
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"Buyer principal refused", user("buyer_A", "order.read", "order.write"), codes.PermissionDenied},
		{"Seller principal refused", user("s1", "order.read"), codes.PermissionDenied},
		{"Admin refused", user("admin_1", "admin", "order.admin", "order.read"), codes.PermissionDenied},
		{"Service without the scope refused", q7Svc(), codes.PermissionDenied},
		{"Service with an unrelated scope refused", q7Svc("order.write"), codes.PermissionDenied},
		{"Service with order.admin only refused", q7Svc("order.admin"), codes.PermissionDenied},
		{"Unauthenticated call refused", context.Background(), codes.Unauthenticated},
		{"Anonymous principal refused", anon, codes.Unauthenticated},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: q7Orders()}}
			h := q7Handler(t, repo, nil)
			res, err := h.HasCompletedOrder(tc.ctx, &orderv1.HasCompletedOrderRequest{BuyerId: "buyer_A"})
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), tc.want, err)
			}
			if res != nil && res.GetHasCompleted() {
				t.Fatal("a refused caller must not learn has_completed")
			}
			if repo.calls != 0 {
				t.Fatalf("store queried %d times for a refused caller, want 0", repo.calls)
			}
		})
	}
}

func TestQ7HasCompletedOrder_StoreFailureIsNotReportedAsFalse(t *testing.T) {
	repo := &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: q7Orders()}, err: errors.New("pq: connection to 10.0.0.9 refused")}
	h := q7Handler(t, repo, nil)
	res, err := h.HasCompletedOrder(q7Svc("order.read"), &orderv1.HasCompletedOrderRequest{BuyerId: "buyer_B"})
	if c := status.Code(err); c != codes.Internal && c != codes.Unavailable {
		t.Fatalf("code = %v, want Internal or Unavailable (err=%v)", c, err)
	}
	if res != nil {
		t.Fatalf("response = %v, want nil on failure", res)
	}
	if msg := status.Convert(err).Message(); strings.Contains(msg, "pq:") || strings.Contains(msg, "10.0.0.9") {
		t.Fatalf("driver text leaked: %q", msg)
	}
}

func q7Forms(id string) []string {
	forms := []string{id, strings.ToUpper(id)}
	sums := [][]byte{}
	s256 := sha256.Sum256([]byte(id))
	s1 := sha1.Sum([]byte(id))
	m5 := md5.Sum([]byte(id))
	sums = append(sums, s256[:], s1[:], m5[:])
	for _, s := range sums {
		hx := hex.EncodeToString(s)
		forms = append(forms, hx[:8], hx[:12], hx[:16], hx,
			base64.StdEncoding.EncodeToString(s), base64.RawStdEncoding.EncodeToString(s),
			base64.URLEncoding.EncodeToString(s), base64.RawURLEncoding.EncodeToString(s))
	}
	forms = append(forms, hex.EncodeToString([]byte(id)), base64.StdEncoding.EncodeToString([]byte(id)), base64.RawURLEncoding.EncodeToString([]byte(id)))
	return forms
}

func TestQ7HasCompletedOrder_LogsCarryNoBuyerIDInAnyForm(t *testing.T) {
	const buyer = "buyer_zq7x_91f3"
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"failure", errors.New("db down")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			orders := q7Orders()
			orders["o_z"] = store.Order{ID: "o_z", BuyerID: buyer, SellerID: "s1", Status: store.OrderStatusCompleted}
			repo := &q7Repo{mockOrderServiceRepo: mockOrderServiceRepo{orders: orders}, err: tc.err}
			h := q7Handler(t, repo, logger)
			_, err := h.HasCompletedOrder(q7Svc("order.read"), &orderv1.HasCompletedOrderRequest{BuyerId: buyer})
			if (err != nil) != (tc.err != nil) {
				t.Fatalf("err = %v, want error=%v", err, tc.err != nil)
			}
			out := buf.String()
			if tc.err != nil && out == "" {
				t.Fatal("failure path must log through the handler logger for this check to mean anything")
			}
			for _, f := range q7Forms(buyer) {
				if strings.Contains(out, f) {
					t.Fatalf("log output contains buyer id form %q: %s", f, out)
				}
			}
		})
	}
}
