# ADR-0006 — RS256 + JWKS issuer/verifier split

**Status:** Accepted · **Date:** 2026-09-02 · **Extends/supersedes:** ADR-0003 (HS256 clause)

## Context

ADR-0003 made JWT trust a **symmetric HS256 shared secret**: `team-identity` signs
and `team-gateway` verifies with the *same* `JWT_SECRET`, seeded identically in
compose and in Vault at `svc/shared/jwt` (readable by every service). That symmetry
means the verifier holds the signing key — the gateway, and any service that could
read `svc/shared/jwt`, could **mint** tokens — and rotating the key is a lock-step
redeploy of both sides. ADR-0003 itself listed `RS256/JWKS rotation` as an open
seam. This ADR builds it.

An asymmetric split needs the issuer's **public** key to reach the verifier. The
convention is a JWKS document served over HTTP. `team-identity` was **gRPC-only**
(a single listener on `:50053`), so it must grow a small HTTP surface to publish it.

## Decision

- **Asymmetric split — identity signs, gateway only verifies.** `team-identity`
  loads an RSA **private** key (PEM) + a stable `kid`, signs each JWT with
  `RS256`, and stamps the `kid` in the JWT header. `team-gateway` holds **no**
  signing material; it verifies with the **public** key it fetches from identity.
  The edge can no longer mint tokens — the whole point of the change.

- **JWKS published by the issuer over a new HTTP listener.** Identity adds a
  minimal `net/http` server (`JWKS_HTTP_PORT`, default `:50063`), started
  alongside the gRPC server and drained with it, serving
  `GET /.well-known/jwks.json`. The document is a standard JWKS — a `keys` array
  of RSA public JWKs (`kty=RSA`, `use=sig`, `alg=RS256`, `kid`, `n`, `e`). It is
  **public and unauthenticated** (public keys only, never the private key) and
  publishes an **array** so a rotated-in key can be pre-published before it signs.

- **Gateway fetches + caches the JWKS, verifies by `kid`.** The edge verifier
  requires `RS256`, reads the token's `kid`, and matches it to a cached public
  key. The keyset is fetched from `JWKS_URL`, refreshed on a TTL
  (`JWKS_CACHE_TTL`, default 5m) and on an unknown `kid` (one bounded forced
  refresh) so a freshly rotated-in `kid` is honored without a redeploy. Fetch is
  fail-soft: a briefly unreachable JWKS at boot does not hard-fail the gateway
  (anonymous read paths still work), matching today's "bad token → anonymous".

- **Forwarded principal unchanged (ADR-0003 Rule 2 retained).** Only how a
  token's signature is checked changes. A token that verifies resolves to its
  `Principal`; no/invalid token → the **anonymous** principal with
  `PUBLIC_SCOPES`, exactly as before. The `x-principal-{id,type,scopes}` metadata
  the edge forwards is byte-for-byte identical, so **no downstream service, no
  proto, and no frontend behavior changes**. `Edge` swaps its `secret` field for
  a JWKS verifier handle; `NewEdge` takes the verifier.

- **Key rotation (multi-key JWKS).** Identity signs with **one** current `kid`;
  the JWKS may list several. Rotation is: (1) add the new keypair and publish it
  in the JWKS (old + new present); (2) once gateways have refreshed, flip
  identity's *signing* key to the new `kid`; (3) later drop the retired key.
  Tokens signed by any currently-published `kid` verify throughout. This change
  ships the **mechanism** (array of keys, sign-by-current, verify-by-any); an
  automated rotation schedule/keystore is ops work, not code.

- **Key provisioning — private key in Vault for identity only.** The RSA private
  key PEM + `kid` are seeded into Vault at `svc/shared/jwt`
  (`JWT_PRIVATE_KEY`, `JWT_KID`), replacing `JWT_SECRET`, and the per-service
  Vault policy is tightened so **only `team-identity`** reads that path (every
  other service's blanket `shared/jwt` read is removed). The gateway's `jwtShared`
  mount is dropped; it gets a plain `JWKS_URL` env (public data, no secret). Local
  dev uses a static dev keypair — same dev-grade posture as ADR-0003's
  `dev-secret-change-me`; real key generation/storage hardening is an ops concern.

- **Clean cutover — delete the HS256 path, no dual window.** The HMAC
  signing/verifying code and every `JWT_SECRET` wiring in the issuer and edge are
  **removed**, not gated behind a flag. Rationale: this is a local/dev stack with
  **no live traffic** — there are no in-flight HS256 sessions worth preserving, so
  a transitional "accept HS256 OR RS256" verifier would be dead complexity. After
  deploy, existing cookies fail verification → anonymous → the user logs in once
  and receives an RS256 token. **In production this would be unacceptable** — a
  dual-accept window would be required; that is explicitly a prod-only future, not
  this change.

## Non-goals

- **Zero-trust / downstream self-verify stays OUT.** The eight downstream services
  keep trusting the gateway-forwarded `x-principal-*` (ADR-0003 Rule 2). Making
  services fetch JWKS and verify tokens themselves (defense-in-depth) is a noted
  follow-up — a drop-in later, since the Principal shape is unchanged.
- **No dual HS256+RS256 acceptance window**, **no refresh/revoke**, **no login
  rate-limit**, **no OAuth/social**, **no proto/contract change** — all as in the
  proposal's Non-goals.

## Consequences

- The edge can no longer mint tokens; the verifier holds only public material.
- Keys rotate by publishing a new `kid` in the JWKS — no gateway redeploy.
- Identity grows a small, read-only HTTP surface (new lifecycle/shutdown code)
  next to its gRPC server; the gRPC health/serving path is unchanged.
- The gateway gains a soft startup dependency on identity's JWKS (mitigated by
  lazy/TTL refresh + fail-soft boot).
- Superseded: **ADR-0003's HS256 signing + shared-`JWT_SECRET` clause.** ADR-0003's
  Principal shape, edge-verification point, and forwarded-principal decision
  remain authoritative.
