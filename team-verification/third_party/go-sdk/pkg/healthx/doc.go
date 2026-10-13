// Package healthx polls dependency checks in the background and reflects the
// result in the gRPC health service, with a sticky drain for graceful shutdown.
package healthx
