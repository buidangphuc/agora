## Why

Identity now records a session at login and stamps its id (`sid`) into the JWT, and users
can revoke sessions on `/account/security`. But revoking has no effect: the gateway
verifies tokens locally once (ADR-0003) and never learns about revocations, so a revoked
token works until it expires. Sessions also store an empty device and IP because the
gateway forwards no client context.

## What Changes

- platform-core: a `SessionRevoked` event message and the `identity.events` topic
  (additive proto; topic provisioned in compose / redpanda-init).
- team-identity: an outbox (`identity_outbox_events`) written in the same transaction as
  a revoke, and a relayer publishing to `identity.events`.
- team-gateway: an `identity.events` consumer feeding an in-memory `sid` denylist (replay
  from earliest on start, TTL = token expiry); the verifier rejects denylisted `sid`s with
  401. The gateway sets `x-client-ip` / `x-client-user-agent` on outgoing calls,
  overwriting inbound values; `X-Forwarded-For` is trusted only from configured proxies.
- ADR-0003 addendum: verify-once plus an event-fed revocation denylist.

Repos: platform-core, team-identity, team-gateway, platform-e2e (and compose).

## Capabilities

### Modified Capabilities
- `auth`: session-bound tokens, revocation enforcement, trusted client context.

## Non-goals

- No refresh tokens or token-TTL change.
- No per-request call from the gateway to identity.
- Login history (`RecordLogin`) is not wired here.
- `/api/track` beacons keep their lenient anonymous fallback.

## Impact

- New topic `identity.events` (retention must be ≥ the access-token TTL).
- Gateway memory: one entry per revoked, unexpired session.
- Gateway now consumes Kafka (consumer group per replica, or no group with manual
  offsets so every replica sees every revocation).
