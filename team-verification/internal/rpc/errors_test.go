package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/config"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/grpcx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/healthx"
	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/logx"
	verificationv1 "github.com/buidangphuc/team-verification/generated/platform/verification/v1"
	"github.com/buidangphuc/team-verification/internal/rpc"
	"github.com/buidangphuc/team-verification/internal/store"
	"github.com/buidangphuc/team-verification/internal/store/storetest"
)

type faultRepo struct {
	store.KycRepository
	createErr   error
	createPanic bool
	created     int
}

func (r *faultRepo) Create(ctx context.Context, s *store.Submission) (*store.Submission, error) {
	r.created++
	if r.createPanic && r.created == 1 {
		panic("boom: nil pointer in repo")
	}
	if r.createErr != nil {
		return nil, r.createErr
	}
	return r.KycRepository.Create(ctx, s)
}

func newFault() *faultRepo { return &faultRepo{KycRepository: storetest.NewInMemoryKycRepo()} }

func TestMapErrHidesInternalCause(t *testing.T) {
	r := newFault()
	r.createErr = errors.New("pq: connection refused 10.0.0.5")
	_, err := buildHandler(t, r, &fakeAudit{}, quiet()).SubmitKyc(buyer("u"), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal || st.Message() != "internal error" {
		t.Fatalf("got %v %q", st.Code(), st.Message())
	}
}

// Scenario: Error table maps domain errors.
func TestErrorTableMapsDomainErrors(t *testing.T) {
	h := newHandler()
	id := submit(t, h, buyer("alice"))
	_, e1 := h.SubmitKyc(buyer("u"), &verificationv1.SubmitKycRequest{DocType: "", DocRef: "r"})
	_, e2 := h.SubmitKyc(buyer("u"), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: ""})
	_, e3 := h.ReviewKyc(admin("adm"), &verificationv1.ReviewKycRequest{Id: "", Decision: "approve"})
	_, e4 := h.ReviewKyc(admin("adm"), &verificationv1.ReviewKycRequest{Id: id, Decision: "maybe"})
	_, e5 := h.ReviewKyc(admin("adm"), &verificationv1.ReviewKycRequest{Id: "kyc_missing", Decision: "approve"})
	_, _ = h.ReviewKyc(admin("adm"), &verificationv1.ReviewKycRequest{Id: id, Decision: "approve"})
	_, e6 := h.ReviewKyc(admin("adm2"), &verificationv1.ReviewKycRequest{Id: id, Decision: "reject"})
	want := []struct {
		name string
		err  error
		code codes.Code
	}{
		{"empty doc_type", e1, codes.InvalidArgument},
		{"empty doc_ref", e2, codes.InvalidArgument},
		{"empty id", e3, codes.InvalidArgument},
		{"invalid decision", e4, codes.InvalidArgument},
		{"not found", e5, codes.NotFound},
		{"already reviewed", e6, codes.FailedPrecondition},
	}
	for _, w := range want {
		if got := status.Code(w.err); got != w.code {
			t.Errorf("%s: got %v want %v (%v)", w.name, got, w.code, w.err)
		}
	}
	_, e7 := h.GetVerificationStatus(buyer("u"), &verificationv1.GetVerificationStatusRequest{})
	if e7 != nil {
		t.Fatalf("unknown user status must be PENDING, got %v", e7)
	}
}

// Scenario: Duplicate id is AlreadyExists.
func TestDuplicateIDIsAlreadyExists(t *testing.T) {
	r := newFault()
	r.createErr = &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint kyc_pkey"}
	_, err := buildHandler(t, r, &fakeAudit{}, quiet()).SubmitKyc(buyer("u"), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"})
	wantCode(t, err, codes.AlreadyExists)
}

// Scenario: Panic returns Internal and keeps serving.
func TestPanicReturnsInternalAndKeepsServing(t *testing.T) {
	r := newFault()
	r.createPanic = true
	h := buildHandler(t, r, &fakeAudit{}, quiet())
	health, err := healthx.New(time.Second, quiet())
	if err != nil {
		t.Fatalf("healthx.New: %v", err)
	}
	srv := grpcx.NewServer(grpcx.ServerOptions{Config: config.GRPC{}, Logger: quiet(), Health: health})
	rpc.Register(srv, h)
	lis := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	client := verificationv1.NewVerificationServiceClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"x-principal-id", "u-1", "x-principal-type", "user", "x-principal-scopes", "listing.read")
	req := &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: "r"}

	_, err = client.SubmitKyc(ctx, req)
	wantCode(t, err, codes.Internal)
	wantMessage(t, err, "internal error")
	if _, err = client.SubmitKyc(ctx, req); err != nil {
		t.Fatalf("server stopped serving after panic: %v", err)
	}
}

// Scenario: Doc ref never logged.
func TestDocRefNeverLogged(t *testing.T) {
	const secret = "doc-ref-SECRET-9d41"
	var buf bytes.Buffer
	logger := logx.NewTo(&buf, config.Runtime{Env: "test", LogLevel: "debug", LogJSON: true}, "team-verification")
	r := newFault()
	r.createErr = errors.New("db down")
	h := buildHandler(t, r, &fakeAudit{}, logger)
	_, err := h.SubmitKyc(buyer("u-1"), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: secret})
	wantCode(t, err, codes.Internal)

	e := newEnv()
	e.h = buildHandler(t, e.repo, e.audit, logger)
	e.audit.err = errors.New("audit down")
	id, err := e.h.SubmitKyc(buyer("u-2"), &verificationv1.SubmitKycRequest{DocType: "national_id", DocRef: secret})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	_, _ = e.review(admin("adm-1"), id.GetId(), "approve")

	if !strings.Contains(buf.String(), "db down") {
		t.Fatalf("expected the failure to be logged, log = %q", buf.String())
	}
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("doc ref leaked into logs: %s", buf.String())
	}
}
