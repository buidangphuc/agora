// Package grpcx builds the gRPC server every service shares: otelgrpc stats,
// the interceptor chain (request id, recovery, logging, auth), the health
// service, and reflection when GRPC.ReflectionEnabled is set. It also serves the server as a lifecycle
// component and maps domain errors to gRPC status in one place.
package grpcx
