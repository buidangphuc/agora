package edge_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	listingv1 "github.com/buidangphuc/team-gateway/generated/platform/listing/v1"
	promotionv1 "github.com/buidangphuc/team-gateway/generated/platform/promotion/v1"
	"github.com/buidangphuc/team-gateway/internal/edge"
	"github.com/buidangphuc/team-gateway/internal/upstream"
)

const (
	createVoucherPath = "/platform.promotion.v1.VoucherService/CreateVoucher"
	batchStorefronts  = "/platform.listing.v1.ListingService/BatchGetStorefronts"
	rawUpstreamText   = "audit store connection refused at 10.0.0.7:5432"
)

func errWithInfo(t *testing.T, code codes.Code, msg, domain, reason string) error {
	t.Helper()
	st, err := status.New(code, msg).WithDetails(&errdetails.ErrorInfo{Domain: domain, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	return st.Err()
}

type voucherCreateUpstream struct {
	voucherAdminUpstream
	createErr error
}

func (v *voucherCreateUpstream) CreateVoucher(_ context.Context, _ *promotionv1.CreateVoucherRequest, _ ...grpc.CallOption) (*promotionv1.CreateVoucherResponse, error) {
	return nil, v.createErr
}

type listingDownUpstream struct {
	listingv1.ListingServiceClient
	err error
}

func (l *listingDownUpstream) BatchGetStorefronts(context.Context, *listingv1.BatchGetStorefrontsRequest, ...grpc.CallOption) (*listingv1.BatchGetStorefrontsResponse, error) {
	return nil, l.err
}

func callWith(t *testing.T, clients *upstream.Clients, path, body string, scopes []string) (int, string) {
	t.Helper()
	f := newCockpitFixture(t)
	srv := httptest.NewServer(edge.NewMux(clients, f.edge, nil, edge.CockpitConfig{}, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if scopes != nil {
		req.Header.Set("Authorization", "Bearer "+f.token(t, scopes...))
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, e := res.Body.Read(buf)
		sb.Write(buf[:n])
		if e != nil {
			break
		}
	}
	return res.StatusCode, sb.String()
}

// Scenario V19: duplicate create keeps already_exists through the gateway.
func TestV19_DuplicateCreateKeepsAlreadyExists(t *testing.T) {
	up := &voucherCreateUpstream{createErr: status.Error(codes.AlreadyExists, "voucher code already exists")}
	code, body := callWith(t, &upstream.Clients{Voucher: up}, createVoucherPath, `{"code":"DUP10"}`, []string{"admin"})
	if code != http.StatusConflict || !strings.Contains(body, "already_exists") {
		t.Fatalf("duplicate create = %d %s, want 409 already_exists", code, body)
	}
	if strings.Contains(body, `"internal"`) {
		t.Fatalf("duplicate create became internal: %s", body)
	}
}

// Scenario V23 / V23a: audit-down reason survives sanitisation, upstream text does not.
func TestV23_AuditUnavailableReasonKept_V23a_RawMessageDropped(t *testing.T) {
	for _, c := range append(voucherAdminCalls, struct{ name, path, body string }{"CreateVoucher", createVoucherPath, `{"code":"NEW10"}`}) {
		var clients *upstream.Clients
		err := errWithInfo(t, codes.Unavailable, rawUpstreamText, "promotion.platform", "AUDIT_UNAVAILABLE")
		if c.name == "CreateVoucher" {
			clients = &upstream.Clients{Voucher: &voucherCreateUpstream{createErr: err}}
		} else {
			clients = &upstream.Clients{Voucher: &voucherAdminUpstream{err: err}}
		}
		code, body := callWith(t, clients, c.path, c.body, []string{"admin", "promotion.admin"})
		if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
			t.Errorf("%s = %d %s, want 503 unavailable", c.name, code, body)
		}
		if !strings.Contains(body, "AUDIT_UNAVAILABLE") || !strings.Contains(body, "promotion.platform") {
			t.Errorf("%s lost ErrorInfo domain/reason: %s", c.name, body)
		}
		if !strings.Contains(body, "service unavailable") {
			t.Errorf("%s message is not the sanitized constant: %s", c.name, body)
		}
		if strings.Contains(body, rawUpstreamText) || strings.Contains(body, "10.0.0.7") {
			t.Errorf("%s leaked the upstream message: %s", c.name, body)
		}
		if strings.Contains(body, "PROMOTION_UNAVAILABLE") {
			t.Errorf("%s audit-down must not read as promotion down: %s", c.name, body)
		}
	}
}

// Scenario V24: promotion unreachable (plain Unavailable) reports PROMOTION_UNAVAILABLE.
func TestV24_PromotionUnreachableReportsPromotionUnavailable(t *testing.T) {
	for _, c := range append(voucherAdminCalls, struct{ name, path, body string }{"CreateVoucher", createVoucherPath, `{"code":"NEW10"}`}) {
		err := status.Error(codes.Unavailable, "connection error: dial tcp 10.0.0.9:9090: connection refused")
		var clients *upstream.Clients
		if c.name == "CreateVoucher" {
			clients = &upstream.Clients{Voucher: &voucherCreateUpstream{createErr: err}}
		} else {
			clients = &upstream.Clients{Voucher: &voucherAdminUpstream{err: err}}
		}
		code, body := callWith(t, clients, c.path, c.body, []string{"admin", "promotion.admin"})
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d %s, want 503", c.name, code, body)
		}
		if !strings.Contains(body, "PROMOTION_UNAVAILABLE") || !strings.Contains(body, "promotion.platform") {
			t.Errorf("%s missing PROMOTION_UNAVAILABLE ErrorInfo: %s", c.name, body)
		}
		if strings.Contains(body, "10.0.0.9") {
			t.Errorf("%s leaked upstream address: %s", c.name, body)
		}
	}
}

// Scenario V25: Unavailable without ErrorInfo from a non-promotion upstream stays generic.
func TestV25_NonPromotionUnavailableHasNoReason(t *testing.T) {
	up := &listingDownUpstream{err: status.Error(codes.Unavailable, "dial tcp 10.0.0.3: refused")}
	code, body := callWith(t, &upstream.Clients{Listing: up}, batchStorefronts, `{"seller_ids":["a"]}`, nil)
	if code != http.StatusServiceUnavailable || !strings.Contains(body, "unavailable") {
		t.Fatalf("listing down = %d %s, want 503 unavailable", code, body)
	}
	for _, bad := range []string{"PROMOTION_UNAVAILABLE", "AUDIT_UNAVAILABLE", "ErrorInfo", "reason"} {
		if strings.Contains(body, bad) {
			t.Errorf("non-promotion unavailable carries %q: %s", bad, body)
		}
	}
	if strings.Contains(body, "10.0.0.3") {
		t.Errorf("leaked upstream text: %s", body)
	}
}
