## Context

- `SSEHandler.ServeHTTP` (`internal/edge/realtime.go`) authorizes the room once in `authorizeRoom`, which resolves the
  principal (Authorization header or `session` cookie) and drops it; then it loops on broker messages, a 15 s
  heartbeat and `r.Context().Done()`.
- `Edge.watchStream(ctx, principal)` (`stream_lifetime.go`) already returns a context cancelled at `exp` or on
  denylist revocation, and `streamLifetime.ended()` tells the watcher cut the stream. `resolvedPrincipal` already
  carries `exp` and `sid`.

## Goals / Non-Goals

**Goals:** SSE ends at token expiry or session revocation, with a defined final event; reuse the watcher.
**Non-Goals:** see proposal.

## Decisions

**D1. Reuse `watchStream`.** `authorizeRoom` also returns the resolved principal (nil for public rooms). For a
non-nil principal the handler derives `ctx, life := edge.watchStream(r.Context(), p)` and `defer life.stop()`, and
selects on `ctx.Done()` instead of `r.Context().Done()`. The same check interval applies.

**D2. Final event.** When `ctx.Done()` fires and `life.ended()` is true, write
`event: unauthenticated` with data `{"code":"unauthenticated","reason":"token_expired"|"session_revoked"}` and flush,
then return. If instead the client went away (`r.Context().Err() != nil`), write nothing. The cause is exposed by a
small accessor on `streamLifetime` (`cause()`), no change to its behavior for Connect streams.

**D3. No event after the final one.** The handler returns right after writing it; the deferred `Unsubscribe` removes
and closes the broker channel, so the subscription is released. A message already queued is dropped, never written
after the cut.

**D4. Public rooms and anonymous: unchanged.** They resolve no principal and have no watcher.

**D5. Reconnect.** The browser's `EventSource` auto-reconnects; the reconnect re-runs `authorizeRoom`, which answers
401 for an expired or revoked token, so a cut client cannot silently resume.

## Risks / Trade-offs

- Cutoff latency is at most the check interval plus Kafka propagation (same as Connect streams).
- One watcher goroutine per authenticated SSE connection; it exits when the handler returns (unit test checks).
