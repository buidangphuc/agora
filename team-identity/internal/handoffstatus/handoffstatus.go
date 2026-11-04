package handoffstatus

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func InvalidCode() error {
	return status.Error(codes.Unauthenticated, "invalid or expired handoff code")
}

func AuthRequired() error {
	return status.Error(codes.Unauthenticated, "authentication required")
}

func UsersOnly() error {
	return status.Error(codes.PermissionDenied, "only users may mint a handoff code")
}

func NotConfigured() error {
	return status.Error(codes.Unavailable, "handoff is not configured")
}
