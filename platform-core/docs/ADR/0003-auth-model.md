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
  token / an invalid token → an **anonymous** Principal with `PUBLIC_SCOPES`
  (browse + search), so read paths stay public and writes require a role.
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
