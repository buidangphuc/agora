## Context

Facts (agora `feat/ui-system` @ 66a07d0f):
- `team-identity/internal/authz/scopes.go` `roleScopes[RoleAdmin]` ends with `"admin"`. There is a coverage test
  (`coverage_test.go`) that pins service-only scopes.
- team-order has no `RequireScopes`; its gate is `isAdminOrUser(principal, ownerIDs...)` in `handler/order.go`, which
  accepts the scopes `admin` or `order.admin`. `ForceFailSaga` calls it with no owner ids.
- The gateway authenticates a stream once, in `authInterceptor` (`edge/interceptors.go`). `Edge.verifyToken` checks the
  signature, `exp` and `sid` against `SessionRevocations` (`revocation.Denylist`, filled from `identity.events`
  `SessionRevoked`, in memory, fail open when Kafka is down). The resolved principal drops `exp` and `sid`.
- Streaming RPCs: only `platform.chat.v1.ChatService/StreamChat` (grep of `stream` in `platform-core/packages/proto`).

## Decisions

**D1. `order.admin` is issued to the admin role only.** Added to `roleScopes[RoleAdmin]`; a test pins that no other role
and no service principal holds it. It is a role scope, not service-only.

**D2. Two gates on `ForceFailSaga`, one gate on the other overrides.**
- `ForceFailSaga` needs `admin` AND `order.admin`, so the old gate stays and the new scope is added.
- `isAdminOrUser` is the "admin override" for the other RPCs. It now looks only for `order.admin`. Keeping bare `admin`
  there would leave the generic marker opening order data, which is what this change removes. Owner and seller id
  matches are unchanged.
- The gateway `adminProcedures` entry stays `admin`: the edge stays coarse; team-order is authoritative.
- A SERVICE principal holding `order.read` keeps `canViewOrder` unchanged.

**D3. Stream lifetime is enforced inside the existing stream auth interceptor.**
- `resolvedPrincipal` gains `exp` (time) and `sid`. `admit` fills them from the verified claims.
- For a stream with a non-anonymous principal, the interceptor derives a cancellable context and starts a watcher:
  - one timer fires at `exp`;
  - a ticker every `STREAM_REVOCATION_CHECK_SECONDS` calls `SessionRevocations.Revoked(sid)` (an in-memory map read, no
    call to identity).
- When either fires the watcher records the reason and cancels the context. The upstream call is built from that
  context, so the upstream stream is cancelled too.
- When the handler returns an error and the watcher fired, the interceptor replaces the error with
  `connect.CodeUnauthenticated` (`WWW-Authenticate: Bearer error="invalid_token"` metadata). A stream that already
  finished cleanly is not rewritten.
- Anonymous streams and tokens without `sid` (service tokens) get the `exp` timer only when they have an `exp`; no
  revocation poll without a `sid`.
- Revocation is as cheap as the denylist: cutoff latency is at most the check interval plus Kafka propagation. When the
  revocation consumer is down the denylist is empty, so only `exp` is enforced (existing fail-open, unchanged).
  Nothing new is invented: no call to identity, no new store.

**D4. No clock skew allowance.** The gateway's unary check uses the library default (no leeway); the stream timer uses the
same `exp`.

## Risks

- Admin tokens issued before the identity change lack `order.admin` (BREAKING above). Mitigation: re-login; tokens expire.
- A watcher goroutine per stream; it exits when the handler returns (context done), so no leak (unit test checks).
