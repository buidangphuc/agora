## Context

team-referral is the smallest Go service: one proto service, three tables, no Kafka. The reference migration is `origin/feat/go-sdk-v1-e2e:team-referral/` (read only); main's behaviour wins where they differ. Differences from that reference, decided here:

- No `postgres.OpenOrLocal` (removed from the SDK in v1.0.2) and no in-memory store in `app.Run` (the reference kept the local fallback; the owner decision of 2026-10-22 removes it).
- `ENV` has no default, so compose, `.env.example` and gitops must set it (the reference only covered compose).
- Settings embed `Runtime` and `GRPC`; `Postgres` and `OTel` are named fields (both have an `Enabled`/`URL` collision risk, as in the reference).

## Decisions

D1. Always Postgres. `app.Run` calls `postgres.Open`; failure returns before the listener opens. Local developers run the compose Postgres or a container; tests use `testkit` Postgres (Docker; `GO_SDK_REQUIRE_DOCKER=1` makes a skip a failure).

D2. Fake repository. `internal/store/fake.InMemoryReferralRepo` implements `store.ReferralRepository` for unit tests of `internal/referral` and `internal/rpc`. `TestProductionCodeDoesNotImportFake` guards against it returning to the binary.

D3. Cursor codec stays local to `internal/referral/pagination.go` (decimal offset, malformed resets to 0). `pagination.Clamp` replaces the hand-written clamp. Moving to the SDK's base64 token would invalidate tokens held by clients and turn a malformed token into an error.

D4. Error table (`grpcx.NewErrorTable`): `ErrEmptyUser` Unauthenticated, `ErrEmptyCode` InvalidArgument, `ErrCodeNotFound` NotFound, `ErrSelfReferral` and `ErrAlreadyRedeemed` FailedPrecondition, with the sentinel text as the message (same text as today). Everything else goes through `grpcx.Status`, which adds `AlreadyExists` for SQLSTATE 23505 and `Canceled`/`DeadlineExceeded`.

D5. Auth. `internal/interceptor` is deleted. `auth.UnaryServerInterceptor` (in `grpcx.NewServer`) stores the principal and `auth.RequirePrincipal` rejects missing and anonymous callers with `Unauthenticated`. `Principal.Anonymous()` covers the three anonymous cases that `TestInterceptorTreatsGatewayAnonymousAsAnonymous` tests today; that test is rewritten against the SDK chain (`TestGatewayAnonymousIsUnauthenticated`), not removed.

D6. Gitops. Baseline values add `env: {ENV: local, GRPC_PORT: "50062"}`; staging and prod overlays override `ENV`. The ApplicationSet that merges `envs/<env>/services/*.yaml` is assumed to pick up new overlay files by name, as it does for team-search; task 1.9 checks it.

D7. Migrations stay owned by the deploy migrate job. `migrations/embed.go` only serves tests (`migrate.Up` over `embed.FS`).

## Risks

- Vault secret `svc/team-referral` must contain `DATABASE_URL` before this ships; today an absent value silently used the localhost default. Human task 3.1.
- First image build needs network access to `proxy.golang.org`/GitHub for the SDK tag (public repo, no token).
- Existing unit tests keep their assertions; they move packages and switch to the fake repo. No assertion is loosened.

## Clarifications

### Round 1 — 2026-10-22

- Target SDK is v1.0.2, not v1.0.1. v1.0.2 removes `postgres.OpenOrLocal`, turns gRPC reflection off by default (`GRPC_REFLECTION_ENABLED`, refused when true in a strict ENV) and adds a `liveness` health service independent of the database; the chart gets opt-in `probes.livenessService`. (human)
- The service sets `probes.livenessService: liveness` in its gitops values. Readiness (empty service name) goes NOT_SERVING on database loss, liveness stays SERVING, so a database outage does not restart the pod. Reflection stays off unless `GRPC_REFLECTION_ENABLED=true` locally. (human)
- The OrderCompleted reward consumer is not in this change; it belongs to backlog Q7/Q37. (human)
- SDK behaviour changes are accepted: Canceled/DeadlineExceeded mapping, SQLSTATE 23505 to AlreadyExists, readiness NOT_SERVING, panic recovery. (human)
- Cursor format kept (decimal offset, malformed restarts at 0); only `pagination.Clamp` is adopted. (human)
- No DPO sign-off is needed (user rule); basis is legitimate interest, retention follows platform log retention. (human)
- `ENV` is set through gitops overlays as other services do (`envs/staging/services/*`, `envs/prod/services/*`). Sync the overlays before the image bump; rollback is a revert of the image tag. (human)
