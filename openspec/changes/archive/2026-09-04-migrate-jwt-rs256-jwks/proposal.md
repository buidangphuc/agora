## Why

Today the JWT trust model is a **shared HS256 secret** (ADR-0003, Phase 0): `team-identity`
signs with `JWT_SECRET` (`team-identity/internal/token/jwt.go` → `SignedString([]byte(secret))`)
and `team-gateway` verifies with the *same* secret (`team-gateway/internal/token/jwt.go`), the
two kept in sync by seeding one value everywhere (`docker-compose.services.yaml` and Vault
`svc/shared/jwt`, readable by all nine services). That symmetry means the verifier holds the
signing key: anyone who can read the gateway's secret can *mint* tokens, and rotating the key is
a lock-step redeploy of both services. AGENTS.md §9 lists "RS256/JWKS rotation" as a deliberately
unbuilt seam; this change builds it.

Move to **asymmetric RS256 + JWKS**: `team-identity` holds a private key and is the only minter;
`team-gateway` verifies with the *public* key it fetches from identity's `/.well-known/jwks.json`.
The verifier never holds signing material, and keys rotate by publishing a new `kid` in the JWKS
without redeploying the gateway. This reverses the §9 seam and therefore **requires a new ADR**
(ADR-0006) that extends/supersedes ADR-0003's auth model.

Scope is deliberately **minimal**: only the issuer (`team-identity`) and the edge verifier
(`team-gateway`) change. The forwarded-principal model (ADR-0003 Rule 2) is untouched — the eight
downstream services keep trusting the gateway's `x-principal-*` metadata and gain no JWT library.

## What Changes

- **team-identity** (issuer) — sign RS256 instead of HS256: load an RSA private key (PEM) + a
  `kid` from config (Vault-seeded), set `kid` in the JWT header, sign with `SigningMethodRS256`.
  Stand up a small HTTP listener (identity is gRPC-only today) that serves
  `GET /.well-known/jwks.json` publishing the current public key(s) as a JWKS keyset — supporting
  **multiple keys** so a rotated-in key is published before it is used to sign. Drop `JWT_SECRET`.
- **team-gateway** (edge verifier) — verify RS256 by matching the token's `kid` against a JWKS
  keyset fetched from `team-identity`'s JWKS URL and **cached** (with periodic/lazy refresh so a
  rotated-in `kid` is picked up). Reject any token whose `kid` is absent from the current JWKS or
  that is not signed by a published key. Drop the shared `JWT_SECRET`; add `JWKS_URL`
  (+ cache-TTL) config. The `resolve()` path and forwarded `x-principal-*` output are unchanged.
- **infra / gitops (platform-gitops + docker-compose.services.yaml)** — provision the RSA
  **private** key (PEM + `kid`) into Vault **for identity only** (`svc/shared/jwt` →
  `JWT_PRIVATE_KEY`,`JWT_KID`; tighten the Vault policy so only `team-identity` reads it, not all
  services). Remove `JWT_SECRET` from both services in compose and from the gateway's gitops env;
  give the gateway a plain `JWKS_URL` (no secret). Identity's JWKS HTTP port is exposed.
- **platform-core/docs/ADR** — add **ADR-0006** (RS256 + JWKS issuer/verifier split) extending/
  superseding ADR-0003's HS256 decision; mark ADR-0003's HS256 clause superseded and refresh the
  AGENTS.md auth summary + §9 seam list. (Planning lists this as a task; the ADR file is not
  written by this change.)
- **Clean cutover, no dual-path.** The HS256 shared-secret code path is **removed**, not kept
  alongside RS256. This is a local/dev stack with no live traffic — there is no acceptance window
  for old tokens; everyone simply logs in again once and gets an RS256 token. See Non-goals.
- **E2E (platform-e2e)** — assert login still works end-to-end and that the issued token is RS256
  and accepted by the gateway (reuse the existing `auth.login` capability; status: planned).

## Non-goals

- **Zero-trust / downstream self-verify is OUT of scope.** The eight downstream services keep
  trusting the gateway-forwarded `x-principal-*` metadata (ADR-0003 Rule 2 stays). Making services
  fetch JWKS and verify tokens themselves (defense-in-depth) is a noted follow-up, not this change.
- **No dual HS256+RS256 acceptance window.** We do not build a transitional verifier that accepts
  both algorithms; the HS256 path is deleted in the same change. (Justified: local/dev stack, no
  live sessions to preserve — re-login is free.)
- **No refresh/revoke tokens, no login rate-limit, no OAuth/social** — separate ADR-0003 futures.
- **No proto/contract change.** The `Principal{id,type,scopes}` shape and every RPC are unchanged;
  only the token's signing algorithm and key distribution change (no re-vendor/regenerate).
- **No new user-facing capability.** Login/register behavior is identical to the user; only the
  token's crypto changes.
