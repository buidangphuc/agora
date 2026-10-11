package handler_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/handler"
	"github.com/buidangphuc/team-identity/internal/repository"
)

const driverText = "pq: password authentication failed for user postgres"

var errDriver = errors.New(driverText)

type failAddrRepo struct{ repository.AddressRepository }

func (failAddrRepo) List(context.Context, string) ([]repository.Address, error) {
	return nil, errDriver
}
func (failAddrRepo) Create(context.Context, repository.Address) (repository.Address, error) {
	return repository.Address{}, errDriver
}
func (failAddrRepo) Update(context.Context, repository.Address) (repository.Address, error) {
	return repository.Address{}, errDriver
}
func (failAddrRepo) Delete(context.Context, string, string) error { return errDriver }
func (failAddrRepo) SetDefault(context.Context, string, string) (repository.Address, error) {
	return repository.Address{}, errDriver
}

type failSessRepo struct{ repository.SessionRepository }

func (failSessRepo) ListSessions(context.Context, string) ([]repository.Session, error) {
	return nil, errDriver
}
func (failSessRepo) RevokeSession(context.Context, string, string, repository.EnqueueFn) error {
	return errDriver
}
func (failSessRepo) ListLoginHistory(context.Context, string, int, int) ([]repository.LoginEvent, int64, error) {
	return nil, 0, errDriver
}

func assertGenericInternal(t *testing.T, name string, err error) {
	t.Helper()
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("%s: code = %v, want Internal (err=%v)", name, st.Code(), err)
		return
	}
	if strings.Contains(st.Message(), "pq:") || strings.Contains(st.Message(), "postgres") {
		t.Errorf("%s: driver text leaked: %q", name, st.Message())
	}
}

func TestAddressHandlerInternalErrorsAreGeneric(t *testing.T) {
	h := handler.NewAddressHandler(failAddrRepo{}, nil)
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

func TestSessionHandlerInternalErrorsAreGeneric(t *testing.T) {
	h := handler.NewSessionHandler(failSessRepo{}, nil)
	ctx := principalContext("user-1")
	_, err := h.ListSessions(ctx, &identityv1.ListSessionsRequest{})
	assertGenericInternal(t, "list sessions", err)
	_, err = h.RevokeSession(ctx, &identityv1.RevokeSessionRequest{SessionId: "s"})
	assertGenericInternal(t, "revoke", err)
	_, err = h.ListLoginHistory(ctx, &identityv1.ListLoginHistoryRequest{})
	assertGenericInternal(t, "login history", err)
}
