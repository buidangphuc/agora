package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/buidangphuc/agora/platform-core/packages/go-sdk/pkg/auditx"
	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
	"github.com/buidangphuc/team-domain/internal/listing"
	"github.com/buidangphuc/team-domain/internal/rpc"
	"github.com/buidangphuc/team-domain/internal/store"
	"github.com/buidangphuc/team-domain/internal/store/storetest"
)

type handlerAuditCall struct {
	kind, actor, listing, seller, status string
	// storedStatus/existed snapshot the repository at call time, proving the
	// audit write happens before the change.
	storedTitle string
	existed     bool
}

type handlerAudit struct {
	repo  *storetest.InMemoryListingRepository
	calls []handlerAuditCall
	err   error
}

func (a *handlerAudit) snap(id string) (string, bool) {
	l, err := a.repo.Get(context.Background(), id)
	return l.Title, err == nil
}

func (a *handlerAudit) WriteAuditEvent(_ context.Context, ev auditx.Event) error {
	title, ok := a.snap(ev.TargetID)
	switch ev.Action {
	case "listing.admin_update":
		a.calls = append(a.calls, handlerAuditCall{"update", ev.ActorID, ev.TargetID, ev.Metadata["seller_id"], ev.Metadata["status"], title, ok})
	case "listing.admin_delete":
		a.calls = append(a.calls, handlerAuditCall{"delete", ev.ActorID, ev.TargetID, ev.Metadata["seller_id"], "", title, ok})
	}
	return a.err
}

type auditHandlerFixture struct {
	h     *rpc.ListingHandler
	repo  *storetest.InMemoryListingRepository
	txw   *storetest.InMemoryTxWriter
	audit *handlerAudit
}

func newAuditHandlerFixture(t *testing.T) *auditHandlerFixture {
	t.Helper()
	repo := storetest.NewInMemoryListingRepository(store.Listing{
		ID: "L-1", Title: "orig", Currency: "VND", Status: "published", SellerID: "seller-a", Stock: 1,
	})
	txw := storetest.NewInMemoryTxWriter(repo)
	fa := &handlerAudit{repo: repo}
	svc := listing.NewListingServiceWithOutbox(repo, txw).WithAudit(auditx.NewGate(fa, nil))
	return &auditHandlerFixture{h: rpc.NewListingHandler(svc, nil, nil), repo: repo, txw: txw, audit: fa}
}

func updReq(status listingv1.ListingStatus) *listingv1.UpdateListingRequest {
	return &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{
		Id: "L-1", Title: "new", Currency: "VND", Status: status, Stock: 1,
		SellerId: "forged-owner", // must never reach the audit event
	}}
}

func adminCtx() context.Context { return typedCtx("admin-1", "user", "listing.write", "admin") }
func ownerCtx() context.Context { return typedCtx("seller-a", "user", "listing.write") }
func otherCtx() context.Context { return typedCtx("seller-b", "user", "listing.write") }

func assertUnchanged(t *testing.T, f *auditHandlerFixture) {
	t.Helper()
	l, err := f.repo.Get(context.Background(), "L-1")
	if err != nil || l.Title != "orig" || l.Status != "published" {
		t.Fatalf("listing must be unchanged, got %+v, %v", l, err)
	}
	if n := len(f.txw.Rows()); n != 0 {
		t.Fatalf("no ListingChanged outbox row may be written, got %d", n)
	}
}

// Scenario: Listing audit: Admin update writes one audit event.
func TestUpdateListingAdminWritesAuditBeforeChange(t *testing.T) {
	f := newAuditHandlerFixture(t)
	if _, err := f.h.UpdateListing(adminCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED)); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	l, _ := f.repo.Get(context.Background(), "L-1")
	if l.Status != "published" {
		t.Fatalf("stored status = %q, want published (admin cannot set status)", l.Status)
	}
	if len(f.audit.calls) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(f.audit.calls))
	}
	c := f.audit.calls[0]
	if c.kind != "update" || c.actor != "admin-1" || c.listing != "L-1" || c.seller != "seller-a" || c.status != "published" {
		t.Errorf("audit call = %+v", c)
	}
	if c.storedTitle != "orig" {
		t.Errorf("audit must be written before the change; title at audit time = %q", c.storedTitle)
	}
}

// Scenario: Listing audit: Update without a status records the stored status.
func TestUpdateListingAdminAuditStatusFallsBackToStored(t *testing.T) {
	f := newAuditHandlerFixture(t)
	if _, err := f.h.UpdateListing(adminCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_UNSPECIFIED)); err != nil {
		t.Fatalf("UpdateListing: %v", err)
	}
	if len(f.audit.calls) != 1 || f.audit.calls[0].status != "published" {
		t.Fatalf("audit calls = %+v, want 1 with status published", f.audit.calls)
	}
}

// Scenario: Listing audit: Audit down refuses the admin update.
func TestUpdateListingAdminAuditDownUnavailableNoChange(t *testing.T) {
	for name, aerr := range map[string]error{
		"error":    errors.New("secret upstream detail"),
		"deadline": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			f := newAuditHandlerFixture(t)
			f.audit.err = aerr
			_, err := f.h.UpdateListing(adminCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED))
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (%v)", status.Code(err), err)
			}
			if got := status.Convert(err).Message(); got != auditx.Message {
				t.Errorf("message = %q, want %q", got, auditx.Message)
			}
			assertUnchanged(t, f)
		})
	}
}

// Scenario: Listing audit: Empty listing id writes no audit event.
func TestUpdateListingEmptyIDNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	_, err := f.h.UpdateListing(adminCtx(), &listingv1.UpdateListingRequest{Listing: &listingv1.Listing{Title: "x"}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Listing audit: Seller without admin scope cannot update another seller's listing.
func TestUpdateListingNonOwnerNonAdminDeniedNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	_, err := f.h.UpdateListing(otherCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
	assertUnchanged(t, f)
}

// Scenario: Listing audit: Caller without listing.write is denied before audit.
func TestUpdateListingMissingScopeNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	_, err := f.h.UpdateListing(typedCtx("admin-1", "user", "admin"), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Listing audit: Admin delete writes one audit event.
func TestDeleteListingAdminWritesAuditBeforeChange(t *testing.T) {
	f := newAuditHandlerFixture(t)
	if _, err := f.h.DeleteListing(adminCtx(), &listingv1.DeleteListingRequest{Id: "L-1"}); err != nil {
		t.Fatalf("DeleteListing: %v", err)
	}
	if _, err := f.repo.Get(context.Background(), "L-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("listing must be deleted, got %v", err)
	}
	if len(f.audit.calls) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(f.audit.calls))
	}
	c := f.audit.calls[0]
	if c.kind != "delete" || c.actor != "admin-1" || c.listing != "L-1" || c.seller != "seller-a" {
		t.Errorf("audit call = %+v", c)
	}
	if !c.existed {
		t.Error("audit must be written before the listing is deleted")
	}
}

// Scenario: Domain SDK: An admin delete is refused when audit is down.
func TestAdminDeleteAuditDownUnavailableNoChange(t *testing.T) {
	for name, aerr := range map[string]error{
		"error":    errors.New("secret upstream detail"),
		"deadline": context.DeadlineExceeded,
	} {
		t.Run(name, func(t *testing.T) {
			f := newAuditHandlerFixture(t)
			f.audit.err = aerr
			_, err := f.h.DeleteListing(adminCtx(), &listingv1.DeleteListingRequest{Id: "L-1"})
			if status.Code(err) != codes.Unavailable {
				t.Fatalf("code = %v, want Unavailable (%v)", status.Code(err), err)
			}
			if got := status.Convert(err).Message(); got != auditx.Message {
				t.Errorf("message = %q, want %q", got, auditx.Message)
			}
			assertUnchanged(t, f)
		})
	}
}

// Scenario: Listing audit: Empty id on delete writes no audit event.
func TestDeleteListingEmptyIDNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	_, err := f.h.DeleteListing(adminCtx(), &listingv1.DeleteListingRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Listing audit: Seller without admin scope cannot delete another seller's listing.
func TestDeleteListingNonOwnerNonAdminDeniedNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	_, err := f.h.DeleteListing(otherCtx(), &listingv1.DeleteListingRequest{Id: "L-1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
	assertUnchanged(t, f)
}

// Scenario: Listing audit: Owner update writes no audit event.
func TestUpdateListingOwnerNoAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	if _, err := f.h.UpdateListing(ownerCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_PUBLISHED)); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Domain SDK: A non-admin owner write does not touch audit.
func TestOwnerUpdateIndependentOfAudit(t *testing.T) {
	f := newAuditHandlerFixture(t)
	f.audit.err = errors.New("audit down")
	if _, err := f.h.UpdateListing(ownerCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_PUBLISHED)); err != nil {
		t.Fatalf("owner update: %v", err)
	}
	if _, err := f.h.DeleteListing(ownerCtx(), &listingv1.DeleteListingRequest{Id: "L-1"}); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
}

// Scenario: Domain SDK: The audit failure message hides the cause.
func TestAuditFailureHidesCause(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.7:50051: connect: connection refused")
	calls := map[string]func(f *auditHandlerFixture) error{
		"admin update": func(f *auditHandlerFixture) error {
			_, err := f.h.UpdateListing(adminCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED))
			return err
		},
		"admin delete": func(f *auditHandlerFixture) error {
			_, err := f.h.DeleteListing(adminCtx(), &listingv1.DeleteListingRequest{Id: "L-1"})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			f := newAuditHandlerFixture(t)
			f.audit.err = cause
			err := call(f)
			st := status.Convert(err)
			if st.Code() != codes.Unavailable || st.Message() != "audit trail unavailable" {
				t.Fatalf("got %v %q, want Unavailable %q", st.Code(), st.Message(), "audit trail unavailable")
			}
			if strings.Contains(err.Error(), "10.0.0.7") || strings.Contains(err.Error(), "refused") {
				t.Errorf("error text leaks the cause: %q", err.Error())
			}
			assertUnchanged(t, f)
		})
	}
}

// Scenario: Listing audit: Non-owner without admin is refused and unaudited while audit is down.
func TestNonOwnerNonAdminDeniedNotUnavailable(t *testing.T) {
	f := newAuditHandlerFixture(t)
	f.audit.err = errors.New("audit down")
	_, err := f.h.UpdateListing(otherCtx(), updReq(listingv1.ListingStatus_LISTING_STATUS_REJECTED))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %v, want PermissionDenied (not Unavailable)", status.Code(err))
	}
	if len(f.audit.calls) != 0 {
		t.Fatalf("audit calls = %+v, want none", f.audit.calls)
	}
	assertUnchanged(t, f)
}
