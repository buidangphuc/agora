## Context

ADR-0003: the gateway verifies RS256 JWTs once against JWKS and forwards
`x-principal-*`; services never verify. `fix/sessions` added sessions and the `sid` claim.
team-domain, team-payment and team-order already use the transactional-outbox + relayer
pattern (with the batch-ordering fix); identity has none yet.

## Decisions

1. **Event-fed denylist, not a per-request check.** A call to identity per request would
   add latency and a hard dependency to every route. Revocations are rare, so identity
   publishes them and every gateway replica keeps them in memory. The window between
   revoke and enforcement is the relay plus consume latency (target < 5 s).
2. **Every replica must see every revocation**: the gateway consumer does not join a
   shared consumer group; it assigns the partitions itself and reads from the earliest
   offset on start, so a restart rebuilds the denylist. Topic retention ≥ the token TTL
   (the gateway drops entries past their `expires_at`).
3. **Outbox in identity** mirrors team-domain's (`identity_outbox_events`, the same claim
   query with the ordering fix, the `OUTBOX_*` env names). Revoke = update the session row
   + insert the outbox row in one transaction.
4. **Topic and key**: `identity.events`, key = user id, envelope
   `platform.events.v1.EventEnvelope` with type `platform.identity.v1.SessionRevoked`.
5. **Client context**: set in the gateway's `outgoing()`, which already rebuilds
   metadata from scratch, so an inbound `x-client-*` header can never pass through.
   `X-Forwarded-For` is honoured only when the peer is in `TRUSTED_PROXIES` (CIDR list,
   empty by default).

## Risks / Trade-offs

- Until the consumer catches up after a start, a just-revoked token may pass for a few
  seconds; the gateway does not delay readiness on it (documented).
- If Kafka is down, revocations are not enforced (fail-open for availability); a metric
  `gateway_revocation_consumer_lag` / up flag makes it visible.

## Open Questions

None blocking.
