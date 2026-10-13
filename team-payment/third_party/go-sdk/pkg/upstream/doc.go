// Package upstream dials sibling gRPC services. The client carries the
// request id and either the caller's principal or a service principal, and
// every unary call is bounded so a dead upstream cannot consume the request
// budget.
package upstream
