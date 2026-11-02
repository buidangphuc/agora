# Design

## Decisions

- One composition root, `internal/app.Run(ctx, settings)`, in the order of `examples/hello`: logger, telemetry, health, database, audit connection, handler, server, lifecycle. Everything opened goes on a `lifecycle.Stack`; closers run in reverse on shutdown and on an early failure. `cmd/server/main.go` stays the only binary (`BINARIES` default `server`), so the compose build and Dockerfile entrypoint keep their names.
- No database fallback. `examples/hello` tolerates an unreachable database when `IsLocal()`; this service does not: `postgres.Open` failure fails `Run` in every `ENV` (user rule: importing a dependency means using it). Local runs use the compose Postgres.
- In-memory store is a test fake only: `internal/store/storetest` (package imported by `_test.go` files). A test asserts `cmd/server`'s dependency graph does not contain it, so the fallback cannot creep back.
- `Settings` = `Runtime` + `GRPC` embedded; `Postgres`, `OTel`, `Audit`, `MockData` named groups. No service-specific env key remains (`UPSTREAM_CALL_TIMEOUT_SECONDS` dropped because `auditx.Gate` fixes the bound at 2 s, the value the spec already used as default).
- `MOCK_DATA` with zero adapters: the service stores references only, so there is no data source to swap. Declaring the group keeps the strict-ENV refusal uniform across services; `mockdata.New(cfg)` with no known names rejects any non-empty value at boot, which is the honest behaviour (a name nobody implements must not silently do nothing).
- Audit: `UPSTREAM_AUDIT_ADDR` is required in every ENV (human, round 2), so `auditx.Dial(s.Audit, "service-team-verification", "audit.write")` always returns a connection and the writer adapter in `internal/upstream` is never nil; the nil-writer fail-closed path and its scenarios are deleted. The step order of `kyc-audit` is unchanged: authenticate, `admin` scope, empty-id, load, self-review, decision, PENDING, then `Gate.Do(event, apply)`. `Gate.Do` returns the apply error unchanged, so a repository failure after the audit write is `Internal` and retryable (existing scenario).
- Event shape unchanged: `action=kyc.review`, `target_type=kyc_submission`, `target_id`, `actor_id` from the principal, metadata only `decision` and `user_id`.
- Auth: handlers use `auth.RequireUser` (SubmitKyc), `auth.RequirePrincipal` (status), `auth.RequireScopes("admin")` (review and other-user status). Codes are unchanged (Unauthenticated, PermissionDenied); message text may change (accepted). Two checks the SDK does not make stay in `internal/rpc`: (1) a type guard that returns `Unauthenticated` unless the principal type is `user`, `service` or `anonymous`. In go-sdk v1.0.2 `Principal.Type` is the raw `x-principal-type` string and `Anonymous()` is true only for an empty id, id `anonymous` or type `anonymous`, so a type such as `robot` passes `RequirePrincipal` and `RequireScopes`; the old `parsePrincipalType` mapped every unknown string to unspecified and `AuthenticatedPrincipal` refused it. Without the guard a `robot` principal holding `admin` would pass ReviewKyc and other-user status; the guard runs first in every handler. (2) GetVerificationStatus with an empty or own `user_id` requires a user principal (`PermissionDenied` for a service principal), because `auth.RequirePrincipal` accepts service principals. No principal behaviour is dropped.
- Errors: one `grpcx.ErrorTable` in `internal/rpc`: the four empty-field sentinels and `ErrInvalidReview` to `InvalidArgument`, `ErrNotFound` to `NotFound`, `ErrAlreadyReviewed` to `FailedPrecondition`; everything else is logged and sent as `Internal` "internal error". 23505 maps to `AlreadyExists` by the SDK default.
- Health: `health.Register("postgres", postgres.Checker(pool))`; deploy sets `probes.livenessService: liveness`; readiness uses the overall status.
- Pagination: none (no list RPC), nothing to keep.
- Test fakes: handler tests keep their fake audit writer (now implementing `auditx.Writer`) and `storetest` repo; the store has a real-Postgres suite on `testkit.SharedPostgres` + `Schema` with `migrations.Files`.
- Moved tests keep their names. Every kyc-audit scenario that cited `internal/handler` is in this delta (MODIFIED) and cites `internal/rpc`, so no stale package path survives archive. The old handler tests without a scenario (`TestHandlerSubmitReviewFlow`, `TestTwoUsersDoNotSeeEachOthersStatus`, `TestServicePrincipalCannotSubmit`, `TestAdminCanReviewSomeoneElse`, `TestReviewIsFinal`) have scenarios in `verification-runtime`; `TestReviewIsFinal` is mapped by "A review decision is final through the handler" and the store half by `TestPostgresUpdateStatusIsFinal`.
- Audit deadline: `auditx.Gate` uses a plain 2 s `WithTimeout` on ctx; the old 500 ms margin below the inbound deadline is dropped (accepted, see proposal).
- Migrations: the binary has a `migrate up` subcommand (SDK `pkg/migrate`, embedded `migrations.Files`); `app.Run` never migrates. Compose and gitops run it (deploy-go-sdk-v1).
- Readiness: Postgres checker only; there is no Kafka and no upstream readiness check.
- ENV: accepted set is `local`, `test`, `staging`, `prod`; strict means not `local` or `test`; an unrecognised value such as `dev` fails `Load` naming `ENV`.
- Listener: `GRPC_HOST` default empty, so the address is `:GRPC_PORT` (all interfaces) as before.
- The gateway passes only gRPC codes through; the message texts that change via the SDK ("audit trail unavailable", scope and principal messages) are not asserted by team-gateway or platform-e2e.
- Lint: the SDK `.golangci.yml` disables the revive doc-comment rules, so new code carries no doc comments. `golangci-lint` is pinned with a `tool` directive in `go.mod` like the SDK.

## Clarifications

### Round 1 — 2026-10-23

- G-sdk [dependency] Target version: Which go-sdk version and how is it consumed? → **`go-sdk@v1.0.2` required by tag, no `replace`, no `third_party`; Makefile includes `../platform-core/make/go-service.mk` and the SDK `.golangci.yml`; no doc comments; `golangci-lint` pinned as a go tool** (human, recipe)
- G-layout [structure] Layout: Which structure? → **ADR-0016: `cmd/<bin>/main.go` calls `internal/app.Run(ctx, settings)`; `internal/config`, `internal/rpc`, domain package, `internal/store`** (human, recipe)
- G-flags [config] Enable flags: Which stay? → **Delete every dependency enable flag (`DATABASE_ENABLED` here) and every runtime in-memory or noop fallback; in-memory stays only as test fakes; kept flags are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV)** (human, recipe)
- G-mock [mock] Mock mode: What does mock mean? → **Mock data served by the real service through `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation); never a mock service. The in-memory "mock mode" is removed** (human, recipe)
- G-sdk-use [behaviour] SDK coverage: What does the service use from the SDK? → **config, logx, otelx, healthx (readiness overall, liveness independent, `probes.livenessService: liveness`), lifecycle, grpcx (error table incl. 23505 to AlreadyExists), auth, postgres.Open, migrate, auditx (fail closed "audit trail unavailable"); these behaviour changes are accepted** (human, recipe)
- G-keep [behaviour] Existing behaviour: What wins on conflict? → **Main's behaviour and every `kyc-audit`, privacy-hardening and FEATURES.yaml behaviour stay green; only requirements about removed flags, fallbacks and env vars are MODIFIED** (human, recipe)
- G-dpo [privacy] DPO sign-off → **Not needed: basis legitimate interest, platform log retention** (human, recipe)
- G-e2e [process] e2e timing → **Not run per service; once at the end. Compose and gitops edits are listed in tasks.md under the deploy track, not done here** (human, recipe)

### Round 2 — 2026-10-23

- R2-1 [migrations] How are migrations applied? → **Every service binary has a `migrate up` subcommand (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `team-verification-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (in deploy-go-sdk-v1)** (human)
- R2-2 [health] Is Kafka a readiness check? → **No, in no service; readiness is Postgres and required upstream checks only (here: Postgres only; this service has no Kafka)** (human)
- R2-3 [deploy-runtime] Who modifies shared deploy-runtime requirements? → **Only change deploy-go-sdk-v1 (for example "Services refuse unsafe configuration in staging and production"); this change carries no MODIFIED copy; the service bullet is listed under the Deploy track** (human)
- R2-4 [outbox] Outbox upgrade → **Not applicable to team-verification (no outbox); decided platform-wide for services that have one** (human)
- R2-5 [deploy] Raw gitops manifests and LOG_KEY_FP_SECRET → **Handled in deploy-go-sdk-v1; this service has no Kafka consumer and needs no `LOG_KEY_FP_SECRET`** (human)
- R2-6 [upstream] Fallback modes → **Upstream addresses and clients the service uses are required in every ENV: `UPSTREAM_AUDIT_ADDR` is required in local, test, staging and prod; the nil-audit-writer scenarios ("KYC audit: no audit upstream configured", "Empty audit address yields no client") are removed from `kyc-audit`** (human)
- R2-8 [compose] Local reflection → **Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track line)** (human)
- R2-12 [accepted] Accepted behaviour changes → **`UPSTREAM_CALL_TIMEOUT*` removed in favour of the SDK 2 s audit bound (already recorded in the proposal)** (human)
- R2-13 [tests] Docker-backed tests → **They must run: a skip is a gate failure (`GO_SDK_REQUIRE_DOCKER=1`)** (human)
- R2-gh [process] GOPRIVATE → **The agora repo is public; no GOPRIVATE or credentials are needed** (human)
