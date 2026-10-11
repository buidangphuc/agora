## Why

`authz-residuals-2` made the gateway end Connect server streams (`ChatService/StreamChat`) when the bearer token
expires or its session is revoked. It left the plain-HTTP SSE route `/api/events/live` out (its Follow-ups). An
authenticated SSE client (`user:{id}`, `chat:*`, `ops:*` rooms) is checked once at open and then keeps receiving
notifications, chat messages or the admin order ticker for as long as the connection lives, long after the token
expired or the user signed that session out.

## What Changes

- **team-gateway:** an SSE connection to a room that requires a credential is watched with the existing stream
  lifetime watcher (`watchStream`). When the token's `exp` passes, or the session appears in the revocation denylist
  (polled every `STREAM_REVOCATION_CHECK_SECONDS`), the handler writes one final SSE event `unauthenticated`, then
  closes the response, which unsubscribes from the broker and so cancels the upstream subscription. No further
  event is written after it.
- Public rooms (`global`, `listing:*`) need no credential and stay unchanged; so does any request with no credential.
- No new config, no proto change, no new infra.

Repos/capability: `team-gateway`, capability `edge-stream-and-http-policy`.

## Non-goals

- Server-push of revocation (the gateway keeps the denylist it has).
- Cutting public-room SSE connections, even if the client also sent a token.
- A token with no `sid` cannot be revoked mid-stream (same as unary calls); it still ends at `exp`.
- Max stream duration, concurrent-stream caps, SSE reconnect policy in the browser client.

## Impact

- `team-gateway/internal/edge/realtime.go` and tests; `team-gateway/FEATURES.yaml`; e2e in `platform-e2e`.
- Browser clients see a final `event: unauthenticated` and must re-authenticate before reconnecting (a reconnect with
  the same token is refused with 401 at open).
