## Context

ADR-0003 (Phase 0) made JWT trust a **symmetric HS256 shared secret**: `team-identity` signs and
`team-gateway` verifies with the *same* `JWT_SECRET`, seeded identically in
`docker-compose.services.yaml` (lines 13 + 144) and in Vault at `svc/shared/jwt` (readable by all
nine services, per `platform-gitops/platform/vault-config/vault-config.yaml`). Concretely:

- Issuer — `team-identity/internal/token/jwt.go:35`:
  `jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))`, driven by
  `team-identity/internal/service/auth.go:96` (`token.Sign(s.secret, …)`), with the secret coming
  from `team-identity/internal/config/config.go` `JWT.Secret` (`env:"JWT_SECRET"`, required in
  `Validate()`).
- Verifier — `team-gateway/internal/token/jwt.go:21-32`: `ParseWithClaims` with a keyfunc that
  rejects any non-HMAC method (`*jwt.SigningMethodHMAC`) and returns `[]byte(secret)`. Called from
  `team-gateway/internal/edge/forward.go:70-82` `resolve()` via `Edge.secret`
  (`team-gateway/internal/config/config.go` `Auth.JWTSecret`, required in `Validate()`).

The weakness: the verifier holds the signing key, so the gateway (and every service that can read
`svc/shared/jwt`) can mint tokens, and key rotation is a lock-step redeploy.

This change is **RS256 + JWKS with a clean cutover** and a deliberately **minimal blast radius**:
only the issuer and the edge verifier change. The forwarded-principal model (Rule 2) is preserved,
so no downstream service, no proto, and no frontend behavior changes. It reverses the AGENTS.md §9
"RS256/JWKS rotation" seam and therefore ships with **ADR-0006** extending/superseding ADR-0003.

Key observation that shapes the design: **team-identity is gRPC-only today** (`cmd/server/main.go`
starts a single gRPC listener on `:50053`; there is no HTTP surface). A JWKS document is served
over HTTP by convention and the gateway fetches it over HTTP, so identity must grow a small HTTP
listener for `/.well-known/jwks.json`.

## Decisions

- **Asymmetric split: identity signs, gateway only verifies with the public key.** `team-identity`
  loads an RSA private key (PEM) + a stable `kid` and signs with `jwt.SigningMethodRS256`, setting
  the `kid` in the JWT header. `team-gateway` holds **no** signing material; it fetches the public
  key(s) from identity's JWKS endpoint and verifies. This is the whole point of the change — the
  edge can no longer mint tokens.

- **JWKS published by the issuer over a new HTTP listener.** Because identity is gRPC-only, add a
  minimal `net/http` server (new `JWKS_HTTP_PORT`, e.g. `:50063`) started alongside the gRPC server
  in `cmd/server/main.go`, serving `GET /.well-known/jwks.json`. The document is a standard JWKS:
  a `keys` array of RSA public JWKs (`kty=RSA`, `use=sig`, `alg=RS256`, `kid`, `n`, `e`). The
  endpoint is **public and unauthenticated** — it exposes only public keys. It publishes an array
  (not one key) so rotation can pre-publish the next key.

- **Gateway fetches + caches the JWKS keyset, verifies by `kid`.** Replace
  `team-gateway/internal/token`'s HMAC keyfunc with an RS256 keyfunc that (a) requires
  `*jwt.SigningMethodRSA`, (b) reads the token header `kid`, (c) looks up the matching public key
  in the cached keyset. The keyset is fetched from `JWKS_URL` at startup and refreshed on a TTL
  (`JWKS_CACHE_TTL`, e.g. 5m); on a cache miss for an unknown `kid` the gateway may do one forced
  refresh (bounded) before rejecting, so a freshly rotated-in `kid` is honored without a redeploy.
  A token whose `kid` is missing from the current JWKS, or signed by a key not in it, is rejected
  → `resolve()` falls through to the **anonymous** principal exactly as an invalid HS256 token does
  today (behavior preserved: no token / bad token → `PUBLIC_SCOPES`).

- **`resolve()` and forwarded principal are unchanged.** Only how a token's signature is checked
  changes. `Edge` swaps its `secret string` field for a JWKS verifier handle; `NewEdge(...)` takes
  the verifier instead of the secret. The `x-principal-{id,type,scopes}` output in `outgoing()` is
  byte-for-byte the same, so downstream services and the proto are untouched (Rule 2, Rule 4).

- **Key rotation model (multi-key JWKS).** Identity signs with **one** current `kid`; the JWKS may
  list **several**. Rotation is: (1) add the new keypair to identity's key set and publish it in
  the JWKS (both old + new `kid` present); (2) once gateways have refreshed their cache, flip
  identity's *signing* key to the new `kid`; (3) later drop the retired key from the JWKS. Tokens
  signed by any currently-published `kid` verify throughout. For this change the runtime supports
  the mechanism (array of keys, sign-by-current-kid, verify-by-any-published-kid); operating a real
  rotation is an ops action, not code.

- **Clean cutover — delete the HS256 path, no dual acceptance window.** The HMAC signing/verifying
  code and every `JWT_SECRET` wiring are removed, not gated behind a flag. Rationale: this is a
  local/dev stack with **no live traffic**; there are no in-flight HS256 sessions worth preserving,
  so a transitional "accept HS256 OR RS256" verifier would be dead complexity. After deploy,
  existing cookies simply fail verification → anonymous → the user logs in once and receives an
  RS256 token. This is stated plainly in the proposal and ADR-0006.

- **Key provisioning: private key in Vault for identity only; gateway needs no secret.** The RSA
  keypair is generated by ops (out of band — e.g. `openssl genrsa`/a one-off Job); the **private**
  key PEM + `kid` are seeded into Vault at `svc/shared/jwt` as `JWT_PRIVATE_KEY` + `JWT_KID`
  (replacing `JWT_SECRET`), and the Vault policy is tightened so **only `team-identity`** reads
  that path (today every service's policy grants `read` on `svc/data/shared/jwt`). The gateway's
  `jwtShared` mount is dropped; it instead gets a plain `JWKS_URL` env (public data, no secret).
  In `docker-compose.services.yaml` the dev keypair is provided the same way env values are today
  (a dev PEM in the identity block; gateway gets `JWKS_URL=http://team-identity-svc:50063/.well-known/jwks.json`).

- **One ADR, extending ADR-0003.** ADR-0006 records the issuer/verifier asymmetric split, the JWKS
  distribution + `kid` rotation, and the explicit non-extension to zero-trust. ADR-0003's HS256
  clause is marked superseded-by-0006; its forwarded-principal decision stays authoritative. The
  ADR file itself is a planned task, not written here.

## Risks / Trade-offs

- **New HTTP surface on identity.** Identity was gRPC-only; adding an HTTP listener is new
  lifecycle + shutdown code. Mitigation: keep it minimal (one read-only handler, no auth, no
  business logic), started/stopped alongside the existing gRPC server in `main.go`; the gRPC
  `:50053` health/serving path is unchanged.
- **Gateway now has a startup dependency on identity's JWKS.** If identity's JWKS endpoint is
  unreachable at gateway boot, no key is cached and every token → anonymous. Mitigation: retry/lazy
  fetch (don't hard-fail boot), TTL refresh, and a forced refresh on unknown `kid`; anonymous
  read paths still work (matches today's "bad token → anonymous" degrade).
- **Clean cutover invalidates all existing sessions at once.** Acceptable and intended here (dev
  stack, re-login is free); would be unacceptable in prod, where a dual-accept window would be
  required — explicitly called out as a non-goal / prod-only future in ADR-0006.
- **Rotation is mechanism-only in this change.** We build multi-key JWKS + verify-by-any-`kid`, but
  an automated rotation schedule/keystore is not part of this change (ops runbook / later work).
- **Key material in Vault dev is a static PEM.** Same dev-grade posture as today's
  `dev-secret-change-me`; prod key generation/storage hardening is an ops concern, noted in the ADR.
- **RS256 verify is costlier than HMAC.** Negligible at this scale and offset by removing the
  shared-secret liability; the gateway caches parsed public keys rather than re-parsing per request.
