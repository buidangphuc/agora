// Package rpc is the gRPC transport adapter: it implements the generated
// Service server, authorizes each call, maps between wire (protobuf) and
// domain types, and turns errors into gRPC statuses. Business rules live in
// internal/listing.
package rpc

import (
	"google.golang.org/grpc"

	listingv1 "github.com/buidangphuc/team-domain/generated/platform/listing/v1"
)

// Register attaches the Service handler to srv.
func Register(srv grpc.ServiceRegistrar, h *ListingHandler) {
	listingv1.RegisterListingServiceServer(srv, h)
}
