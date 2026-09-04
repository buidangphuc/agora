# Tasks

Implementation status. `[x]` = done + verified this session; `[ ]` = deferred
(with a note). Verification constraints: Go is not on the host and generated proto
is gitignored, so full `go build ./...` cannot run locally — the isolated `token`
/ `config` / `service` packages were built + tested in a `golang:1.22` Docker
container; helm/kubectl were run for gitops.

## 1. Code — team-identity (issuer: RS256 sign + JWKS)
- [x] `internal/token/jwt.go`: `Sign` is now a `*Signer` method loading an RSA
      private key (`NewSigner(pem, kid)`), signing `jwt.SigningMethodRS256`, and
      stamping `kid` in the header. HS256 path + HMAC `Verify` removed.
- [x] `internal/config/config.go`: `JWT.Secret`/`JWT_SECRET` replaced by
      `JWT_PRIVATE_KEY` (PEM) + `JWT_KID` + `JWKS_HTTP_PORT` (default 50063).
      `Validate()` requires the private key + kid; `.env.example` updated (drift
      gate `TestEnvExampleInSync` green in Docker).
- [x] `internal/service/auth.go`: `AuthService` holds a `*token.Signer`;
      `NewAuthService(repo, signer, ttl)`; `issue()` calls `s.signer.Sign(...)`.
- [x] JWKS publisher `internal/token/jwks.go`: `BuildJWKS(...PublicKey)` emits a
      `keys[]` of RSA public JWKs (`kty/use/alg/kid/n/e`); supports multiple keys.
- [x] HTTP surface for JWKS in `cmd/server/main.go`: a minimal `net/http` server
      on `JWKS_HTTP_PORT` serving `GET /.well-known/jwks.json`, started alongside
      gRPC and drained (`Shutdown`) on signal. No auth, public keys only.
- [x] Unit tests (`internal/token/jwt_test.go`): RS256 token parses under the
      public key, header carries `alg=RS256` + `kid`, a foreign-key token fails,
      expired fails, and the JWKS carries the public JWK (n/e present, no private
      material). Built + green in Docker. NOTE: full `go test ./...` (incl.
      generated-proto packages) runs in CI.

## 2. Code — team-gateway (edge verifier: fetch/cache JWKS, verify by kid)
- [x] `internal/token/jwt.go`: HMAC keyfunc replaced by a `*Verifier` requiring
      `*jwt.SigningMethodRSA` (`WithValidMethods(["RS256"])`), reading `kid`, and
      resolving the key from the cached JWKS.
- [x] JWKS client `internal/token/jwks.go`: fetches `JWKS_URL`, parses RSA public
      JWKs, caches by kid, refreshes on TTL and on an unknown kid (one bounded
      forced refresh), fail-soft on fetch error (keeps prior cache; boot not
      hard-failed).
- [x] `internal/edge/forward.go`: `Edge.secret` → `verifier *token.Verifier`;
      `NewEdge(verifier, ...)`; `resolve()` calls `e.verifier.Verify(tok)`.
      Anonymous fallback + `x-principal-*` output unchanged.
      (`internal/edge/collector.go` `beaconPrincipal` updated the same way.)
- [x] `internal/config/config.go`: `Auth.JWTSecret`/`JWT_SECRET` removed; added
      `JWKS_URL` + `JWKS_CACHE_TTL` (default 300s); `Validate()` requires
      `JWKS_URL`; `.env.example` updated.
- [x] Wire the verifier in `cmd/gateway/main.go`: build `token.NewVerifier`,
      best-effort `Prime()` at startup (warn, don't fail), pass to `NewEdge`.
- [x] Unit tests (`internal/token/token_test.go`): valid RS256 accepted; unknown
      `kid` and same-kid-wrong-signature rejected; rotated-in `kid` accepted after
      refresh while the previous key still verifies; empty token rejected. Built +
      green in Docker (`internal/token`, `internal/config`). NOTE: `internal/edge`
      imports generated proto → compiles in CI, self-reviewed here.

## 3. Infra / gitops key provisioning (platform-gitops + docker-compose)
- [ ] `docker-compose.services.yaml`: **RESERVED to the orchestrator** (per scope
      boundary — not edited here). Exact env rewrite it must apply:
        · team-identity: **remove** `JWT_SECRET`; **add** `JWT_PRIVATE_KEY` (dev
          PEM — see scratchpad `jwt_dev_priv.pem`, same key seeded in vault-config),
          `JWT_KID=dev-2026`, `JWKS_HTTP_PORT=50063`; **expose** container port
          `50063`.
        · team-gateway: **remove** `JWT_SECRET`; **add**
          `JWKS_URL=http://team-identity-svc:50063/.well-known/jwks.json`
          (+ optional `JWKS_CACHE_TTL=300`).
- [x] `platform/vault-config/vault-config.yaml`: `svc/shared/jwt` now seeds
      `JWT_PRIVATE_KEY` (dev RSA PEM via heredoc + `key=@file`) + `JWT_KID=dev-2026`
      (was `JWT_SECRET`). Per-service policy loop no longer grants `shared/jwt`
      read to all services — only `team-identity` gets it. `kubectl apply
      --dry-run=client` clean.
- [x] `charts/service/templates/secrets.yaml`: `jwtShared` ExternalSecret block
      repointed from `JWT_SECRET` to `JWT_PRIVATE_KEY` + `JWT_KID` (still gated by
      `{{- if .Values.secrets.jwtShared }}`). `helm template` verified.
- [x] `envs/services/team-identity.yaml`: `secrets.jwtShared: true` kept (now the
      private key); `env.JWKS_HTTP_PORT: "50063"` added. NOTE: exposing 50063
      in-cluster needs a chart `deployment.yaml`/`service.yaml` port change (a
      generic multi-port seam) — **out of this change's gitops scope**; flagged in
      the file and the report. Local dev reaches it via compose.
- [x] `envs/services/team-gateway.yaml`: `secrets.jwtShared: false`;
      `env.JWKS_URL` added. `helm template` shows no JWT ExternalSecret data and
      the JWKS_URL env present; `kubectl --dry-run` clean.
- [x] Provide the RSA keypair (dev): generated with `openssl genrsa 2048`, kid
      `dev-2026`; private PEM embedded in vault-config + saved to the session
      scratchpad (`jwt_dev_priv.pem` / `jwt_dev_pub.pem`). NOTE: real-env keygen +
      storage hardening is ops, out of band.

## 4. ADR (platform-core/docs/ADR)
- [x] `platform-core/docs/ADR/0006-rs256-jwks-auth.md` written (asymmetric split,
      JWKS + kid rotation, clean cutover, zero-trust non-goal). Well-formed md.
- [x] ADR-0003 HS256 clause marked superseded-by-0006 (title amendment + inline
      note); forwarded-principal / edge-verification decision kept authoritative.
- [x] AGENTS.md refreshed: service table + "Auth" wiring line (HS256 → RS256/JWKS,
      no shared secret); §9 moves RS256/JWKS rotation to "built", keeps zero-trust
      self-verify (and automated key-rotation scheduling) deferred.

## E2E (platform-e2e) — DEFERRED (platform-e2e out of this session's scope boundary)
- [ ] Reuse `auth.login` capability — login still works end-to-end after cutover.
- [ ] Add acceptance asserting the issued token is RS256 (`alg=RS256`, has `kid`),
      accepted by the gateway, and `kid` matches `/.well-known/jwks.json`.
- [ ] Update `team-identity/FEATURES.yaml`; keep `status: planned` → `automated`.
- [ ] `make -C platform-e2e features-check` / `spec-check CHANGE=migrate-jwt-rs256-jwks`.

## Archive — DEFERRED
- [ ] Confirm the code tracks build in CI (`go build/test ./...`), gateway verifies
      RS256 end-to-end against identity's JWKS, no `JWT_SECRET` remains in service
      code / compose / gitops, ADR-0006 merged. (Code + gitops done; compose
      rewrite reserved; CI build + e2e pending.)
- [ ] `make -C platform-e2e features-check` green; scenarios automated.
- [ ] `openspec archive migrate-jwt-rs256-jwks`.
