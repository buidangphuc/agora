// Package interceptor resolves the caller's identity for team-verification.
//
// Auth mechanism (bearer today, JWT/cookie later — see ADR-0003) is resolved at
// the edge/gateway, which forwards a resolved principal id in gRPC metadata.
// Services never re-authenticate; they only read the forwarded principal. This
// mirrors the platform convention and keeps handlers from guessing identity.
package interceptor

import (
	"context"

	"google.golang.org/grpc/metadata"
)

// principalMetadataKey is the lowercase gRPC metadata key the gateway forwards
// the resolved principal id under (gRPC normalizes metadata keys to lowercase).
const principalMetadataKey = "x-principal-id"

// demoUserID is the fallback identity used until the gateway forwards a real
// principal, so the mock KYC flow attaches to a stable demo user (matches
// team-notification's placeholder-user approach).
const demoUserID = "khach_hang_shopee"

// UserIDFromContext returns the caller's user id from forwarded metadata, or an
// empty string when the caller is anonymous / no principal was forwarded.
func UserIDFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(principalMetadataKey)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// UserIDOrDemo returns the forwarded principal id, falling back to the demo user
// when none is present. User-owned RPCs use this so local/dev calls without the
// gateway still resolve to a stable identity.
func UserIDOrDemo(ctx context.Context) string {
	if uid := UserIDFromContext(ctx); uid != "" {
		return uid
	}
	return demoUserID
}
