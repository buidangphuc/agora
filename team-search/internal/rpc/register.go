package rpc

import (
	"google.golang.org/grpc"

	searchv1 "github.com/buidangphuc/team-search/generated/platform/search/v1"
)

// Register adds the SearchService handler to srv.
func Register(srv grpc.ServiceRegistrar, h *SearchHandler) {
	searchv1.RegisterSearchServiceServer(srv, h)
}
