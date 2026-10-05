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
   metadata from scratch, so an inbound `x-client-*` header can never pass through. The
   edge computes the values once per request in the auth interceptor (the only place
   the socket peer address is known) and carries them in the context.
   - `x-client-ip` is the socket peer. `X-Forwarded-For` is honoured only when the peer
     is in `TRUSTED_PROXIES` (CIDRs, bare IPs or hostnames; default empty), and then the
     rightmost entry that is not itself a trusted proxy is the client — entries to its
     left are client-controlled and ignored. An unparseable chain falls back to the peer.
   - `x-client-user-agent` is the request's `User-Agent`, clipped to 256 bytes. It needs
     no trust decision (a client can already set it freely; it is audit data).
   - **Frontend hop** (refinement): in the stack the gateway's peer for browser traffic is
     the Next.js server. On login and register only, the frontend forwards the browser's
     `User-Agent` and an `X-Forwarded-For` it derives itself: the entry
     `TRUSTED_PROXY_HOPS` places from the right (default 1), never the leftmost, validated
     as an IP. The gateway trusts that header only because the frontend is listed in
     `TRUSTED_PROXIES`. Compose sets `TRUSTED_PROXIES=team-frontend-svc`; hostnames are
     re-resolved every 30 s and an unresolvable one trusts nobody, so only the frontend
     container (not the whole docker network, which also holds the e2e runner) is trusted.
   - Residual: with no proxy in front of Next.js (the local stack) a browser that sends its
     own `X-Forwarded-For` to the frontend controls the IP recorded for its own session.
     Production puts a load balancer that overwrites/appends the header in front, and the
     value is audit-only, never authorization.

## Risks / Trade-offs

- Until the consumer catches up after a start, a just-revoked token may pass for a few
  seconds; the gateway does not delay readiness on it (documented).
- If Kafka is down, revocations are not enforced (fail-open for availability); a metric
  `gateway_revocation_consumer_lag` / up flag makes it visible.

## Open Questions

None blocking.
