package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/rpc"
	"github.com/buidangphuc/team-identity/internal/store"
)

const driverText = "pq: password authentication failed for user postgres password=secret"

var errDriver = errors.New(driverText)

type failAddrRepo struct{ store.AddressRepository }

func (failAddrRepo) List(context.Context, string) ([]store.Address, error) {
	return nil, errDriver
}

func (failAddrRepo) Create(context.Context, store.Address) (store.Address, error) {
	return store.Address{}, errDriver
}

func (failAddrRepo) Update(context.Context, store.Address) (store.Address, error) {
	return store.Address{}, errDriver
}
func (failAddrRepo) Delete(context.Context, string, string) error { return errDriver }
func (failAddrRepo) SetDefault(context.Context, string, string) (store.Address, error) {
	return store.Address{}, errDriver
}

type failSessRepo struct{ store.SessionRepository }

func (failSessRepo) ListSessions(context.Context, string) ([]store.Session, error) {
	return nil, errDriver
}

func (failSessRepo) RevokeSession(context.Context, string, string, store.EnqueueFn) error {
	return errDriver
}

func (failSessRepo) ListLoginHistory(context.Context, string, int, int) ([]store.LoginEvent, int64, error) {
	return nil, 0, errDriver
}

func assertGenericInternal(t *testing.T, name string, err error) {
	t.Helper()
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("%s: code = %v, want Internal (err=%v)", name, st.Code(), err)
		return
	}
	if st.Message() != "internal error" {
		t.Errorf("%s: message = %q, want %q", name, st.Message(), "internal error")
	}
	if strings.Contains(st.Message(), "pq:") || strings.Contains(st.Message(), "postgres") || strings.Contains(st.Message(), "password=secret") {
		t.Errorf("%s: driver text leaked: %q", name, st.Message())
	}
}

// Scenario: A storage failure is generic and not leaked
func TestAddressHandlerInternalErrorsAreGeneric(t *testing.T) {
	h := rpc.NewAddressHandler(failAddrRepo{}, nil)
	ctx := principalContext("user-1")
	_, err := h.ListAddresses(ctx, &identityv1.ListAddressesRequest{})
	assertGenericInternal(t, "list", err)
	_, err = h.CreateAddress(ctx, &identityv1.CreateAddressRequest{RecipientName: "a", Phone: "1", Street: "s", City: "c"})
	assertGenericInternal(t, "create", err)
	_, err = h.UpdateAddress(ctx, &identityv1.UpdateAddressRequest{Id: "x", RecipientName: "a", Phone: "1", Street: "s", City: "c"})
	assertGenericInternal(t, "update", err)
	_, err = h.DeleteAddress(ctx, &identityv1.DeleteAddressRequest{Id: "x"})
	assertGenericInternal(t, "delete", err)
	_, err = h.SetDefaultAddress(ctx, &identityv1.SetDefaultAddressRequest{Id: "x"})
	assertGenericInternal(t, "set default", err)
}

// Scenario: A storage failure is generic and not leaked
func TestSessionHandlerInternalErrorsAreGeneric(t *testing.T) {
	h := rpc.NewSessionHandler(failSessRepo{}, nil)
	ctx := principalContext("user-1")
	_, err := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	assertGenericInternal(t, "list sessions", err)
	_, err = h.RevokeSession(ctx, &identityv1.RevokeSessionRequest{SessionId: "s"})
	assertGenericInternal(t, "revoke", err)
	_, err = h.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{})
	assertGenericInternal(t, "login history", err)
}
