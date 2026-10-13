package rpc

import (
	"google.golang.org/grpc"

	identityv1 "github.com/buidangphuc/team-identity/generated/platform/identity/v1"
)

// Handlers are the gRPC services identity serves. A nil handler is not registered.
type Handlers struct {
	Auth    *AuthHandler
	Address *AddressHandler
	Session *SessionHandler
	Profile *ProfileHandler
}

// Register adds the handlers to srv.
func Register(srv *grpc.Server, h Handlers) {
	if h.Auth != nil {
		identityv1.RegisterAuthServiceServer(srv, h.Auth)
	}
	if h.Address != nil {
		identityv1.RegisterAddressServiceServer(srv, h.Address)
	}
	if h.Session != nil {
		identityv1.RegisterSessionServiceServer(srv, h.Session)
	}
	if h.Profile != nil {
		identityv1.RegisterPublicProfileServiceServer(srv, h.Profile)
	}
}
