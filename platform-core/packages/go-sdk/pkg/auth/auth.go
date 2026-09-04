// Package auth provides Zero-Trust Principal extraction and scope gating for gRPC services.
package auth

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type principalKey struct{}

// Principal represents the authenticated caller resolved by team-gateway.
type Principal struct {
	ID     string
	Type   string // "user", "service", "anonymous"
	Scopes map[string]struct{}
}

// HasScope checks if the Principal possesses a required permission scope.
func (p *Principal) HasScope(scope string) bool {
	if p == nil || p.Scopes == nil {
		return false
	}
	// Admin scope bypasses all checks
	if _, ok := p.Scopes["admin"]; ok {
		return true
	}
	_, ok := p.Scopes[scope]
	return ok
}

// WithPrincipal attaches a Principal to a context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext extracts the Principal from context, returning nil if not present.
func FromContext(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey{}).(*Principal)
	return p
}

// ExtractFromMetadata extracts Principal from incoming gRPC metadata headers (x-principal-*).
func ExtractFromMetadata(ctx context.Context) *Principal {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}

	id := getFirst(md, "x-principal-id")
	pType := getFirst(md, "x-principal-type")
	scopesStr := getFirst(md, "x-principal-scopes")

	if id == "" && pType == "" {
		return nil
	}

	scopes := make(map[string]struct{})
	if scopesStr != "" {
		for _, s := range strings.Split(scopesStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				scopes[s] = struct{}{}
			}
		}
	}

	return &Principal{
		ID:     id,
		Type:   pType,
		Scopes: scopes,
	}
}

// RequireScopes validates that the caller has all required scopes, returning PermissionDenied if not.
func RequireScopes(ctx context.Context, requiredScopes ...string) error {
	p := FromContext(ctx)
	if p == nil {
		p = ExtractFromMetadata(ctx)
	}
	if p == nil {
		return status.Errorf(codes.Unauthenticated, "unauthenticated: missing principal")
	}

	for _, s := range requiredScopes {
		if !p.HasScope(s) {
			return status.Errorf(codes.PermissionDenied, "permission denied: missing required scope %q", s)
		}
	}
	return nil
}

func getFirst(md metadata.MD, key string) string {
	vals := md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}
