# ADR-0003 — Auth model (Principal + scopes)

**Status:** Accepted · **Date:** 2026-08-31
· **Amended by ADR-0006 (2026-09-02):** the **HS256 shared-secret signing clause
below is superseded** — `team-identity` now signs **RS256** with a private key and
publishes its public key(s) at a JWKS endpoint the gateway verifies against; there
is no shared `JWT_SECRET`. Everything else here (the `Principal` shape, the single
edge-verification point, the forwarded-principal / anti-spoof decision, and
services staying mechanism-agnostic) **remains authoritative.**

## Context

The platform is gRPC-first and polyglot, so auth must be expressible the same way
in every language and carried over gRPC metadata. The canonical identity is
`platform.common.v1.Principal{id, type, scopes}`. Phase 0 stubbed this with a
static shared-secret bearer; this ADR now specifies real login + authorization.

## Decision

- **Canonical identity** is `platform.common.v1.Principal` (`id`, `type`,
  `scopes`), shared by all languages. **Authorization** is a scope check
  (`principal.scopes ⊇ required`) → deny with stable `insufficient_scope`.
- **Identity service = team-identity (Go).** It owns users + credentials
  (bcrypt), exposes `platform.identity.v1.AuthService` (`Register`, `Login`), and
  issues a signed **JWT** whose claims carry `sub`, `name`, `type`, and
  `scopes`. Roles → scopes is a small in-code map (admin/seller/buyer), a seam to
  grow later. *(Signing was HS256 here; ADR-0006 changed it to **RS256 + JWKS** —
  the claims are unchanged.)*
- **Verification happens at the EDGE (Gateway).** The Gateway verifies the JWT
  (from the `authorization` bearer), resolves a Principal, and forwards it to
  upstream services as trusted metadata (`x-principal-id`, `x-principal-type`,
  `x-principal-scopes`). It builds that metadata fresh from verified claims and
  never forwards client-supplied `x-principal-*`, so it can't be spoofed. No
  token → an **anonymous** Principal with `PUBLIC_SCOPES` (browse + search), so
  read paths stay public and writes require a role. A token that is **present
  but invalid** (malformed, bad signature, unknown `kid`, expired) is answered
  `Unauthenticated` (HTTP 401, `WWW-Authenticate: Bearer error="invalid_token"`)
  on every route, public ones included (RFC 6750 §3.1) — it is never silently
  downgraded to anonymous. Clients must not send a token they know is expired.
- **Services stay mechanism-agnostic.** team-domain / team-search read the
  forwarded Principal from metadata and enforce `RequireScopes` — no JWT library,
  no credential parsing. This is the ADR's "services only ever see a resolved
  Principal".
- **Web sessions**: the frontend keeps the JWT in an **httpOnly cookie** and
  attaches it as the bearer on its server-side gateway calls (dual-auth: cookie
  for web SSR, bearer for native — same token).

## Open / future (scale later)

- **Zero-trust**: services verifying the JWT themselves (defense in depth) instead
  of trusting the edge — a drop-in, since the Principal shape is unchanged.
- Refresh/revoke, RS256/JWKS rotation, login rate-limit, per-permission ACLs
  (roles→scopes is flat today), OAuth/social.

## Consequences

- One identity shape across languages; one verification point (the edge).
- Swapping HS256→RS256 or edge-verify→zero-trust changes the interceptor + edge,
  not service logic (Rule: swap infra by config, not business code).

## Addendum (2026-10): session binding and event-fed revocation

Status: accepted. Change: `openspec/changes/session-revocation`.

The edge still verifies each JWT locally, once, against identity's JWKS — there is
**no per-request call to identity**. Revocation is added on top of that without
changing it:

- **Tokens are session-bound.** Every user token carries `sid`, the id of a row in
  identity's `sessions` table (service tokens carry none).
- **Identity publishes revocations.** `RevokeSession` marks the session revoked and,
  in the same DB transaction, inserts a `platform.identity.v1.SessionRevoked`
  (`session_id`, `user_id`, `expires_at`) row in `identity_outbox_events`; the
  relayer (same pattern as team-domain, ADR-0002/0005) produces it, wrapped in
  `platform.events.v1.EventEnvelope`, to Kafka topic **`identity.events`**, key =
  user id. Retention of the topic MUST be at least the access-token TTL.
- **The gateway keeps an in-memory denylist** of revoked `sid`s fed by that topic.
  The consumer does **not** join a consumer group: it assigns the topic's
  partitions itself and reads from the earliest offset on start, so *every* replica
  holds *every* revocation and a restart rebuilds the list. Entries are dropped once
  their `expires_at` passes (the token is expired by then anyway).
- **Enforcement.** A token whose `sid` is denylisted is answered 401 with
  `WWW-Authenticate: Bearer error="invalid_token"` on every route (same path and
  semantics as any other invalid bearer, RFC 6750 §3.1). Target latency from revoke
  to enforcement is under 5 s (relay poll + consume).
- **Availability trade-off.** If Kafka is unreachable the gateway still starts and
  serves; revocations are then not enforced (fail open, logged, and an
  up/lag gauge `gateway_revocation_consumer_up` / `_lag` makes it visible). Until the
  consumer has caught up after a start, a just-revoked token may pass for seconds.

### Trusted client context

The gateway builds `x-client-ip` and `x-client-user-agent` itself in `outgoing()`
(which rebuilds outbound metadata from scratch, so inbound `x-client-*` can never pass
through). The IP is the socket peer; `X-Forwarded-For` is honoured only when the
peer is a configured trusted proxy (`TRUSTED_PROXIES`: CIDRs, bare IPs or hostnames;
default empty). The user agent is the request's `User-Agent`, clipped to 256 bytes.
Services use these for audit fields (a session's device and IP) only — never for
authorization. When the frontend (Next.js server) calls the gateway on a browser's
behalf, it forwards the browser's `User-Agent` and an `X-Forwarded-For` it derives
itself, and the gateway trusts that `X-Forwarded-For` only because the frontend is
listed in `TRUSTED_PROXIES`.
