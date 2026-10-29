package rpc

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
	"github.com/buidangphuc/team-identity/internal/identity"
	"github.com/buidangphuc/team-identity/internal/store"
)

type UserLister interface {
	ListUsers(ctx context.Context, query, status, cursor string, limit int) ([]store.User, string, error)
}

type AuthHandler struct {
	identityv1.UnimplementedAuthServiceServer
}

func NewAuthHandler(_ *identity.AuthService) *AuthHandler { return &AuthHandler{} }

func (h *AuthHandler) WithLogger(_ *slog.Logger) *AuthHandler { return h }

func (h *AuthHandler) WithExposeResetToken(_ bool) *AuthHandler { return h }

type AddressHandler struct {
	identityv1.UnimplementedAddressServiceServer
}

func NewAddressHandler(_ store.AddressRepository, _ *slog.Logger) *AddressHandler {
	return &AddressHandler{}
}

type SessionHandler struct {
	identityv1.UnimplementedSessionServiceServer
}

func NewSessionHandler(_ store.SessionRepository, _ *slog.Logger) *SessionHandler {
	return &SessionHandler{}
}

func (h *SessionHandler) WithTokenTTL(_ time.Duration) *SessionHandler { return h }

type ProfileHandler struct {
	identityv1.UnimplementedPublicProfileServiceServer
}

func NewProfileHandler(_ store.ProfileReader, _ *slog.Logger) *ProfileHandler {
	return &ProfileHandler{}
}

type AccountAdminHandler struct {
	identityv1.UnimplementedAccountAdminServiceServer
}

func NewAccountAdminHandler(_ *identity.AuthService, _ UserLister, _ *slog.Logger) *AccountAdminHandler {
	return &AccountAdminHandler{}
}

type Handlers struct {
	Auth    *AuthHandler
	Address *AddressHandler
	Session *SessionHandler
	Profile *ProfileHandler
	Admin   *AccountAdminHandler
}

func Register(srv *grpc.Server, h Handlers) {
	identityv1.RegisterAuthServiceServer(srv, h.Auth)
	identityv1.RegisterAddressServiceServer(srv, h.Address)
	identityv1.RegisterSessionServiceServer(srv, h.Session)
	identityv1.RegisterPublicProfileServiceServer(srv, h.Profile)
	identityv1.RegisterAccountAdminServiceServer(srv, h.Admin)
}
