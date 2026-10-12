// Package auth reads the principal that the gateway forwards as
// x-principal-* metadata (ADR-0003) and offers exact-match authorization
// helpers. The interceptors only record the principal; handlers enforce access
// with RequirePrincipal and RequireScopes. No scope, including "admin", grants
// anything implicitly, and service-specific role helpers belong to the service.
//
// The metadata is trusted as-is: services must be reachable only from the
// gateway network (ADR-0010).
package auth
