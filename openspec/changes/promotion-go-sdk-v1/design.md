## Context

team-promotion today: `cmd/server/main.go` (172 lines) builds everything inline; `internal/bootstrap` opens a pgx pool, Flipt and a franz-go producer behind enable flags; repositories exist in Postgres and in-memory form and `main` picks one; events are published after the insert by `internal/producer`; the audit client and the team-domain client live in `internal/upstream`; handlers call `interceptor.Require*` on a hand-rolled principal; migrations run from a compose job. Tests use in-memory repositories widely and skip Postgres tests without `PROMOTION_TEST_DATABASE_URL`.

Reference only: the older migration on `origin/feat/go-sdk-v1-e2e` kept the `migrate` Makefile target and the compose job. Main's behaviour wins; migrations run through the binary's `migrate up` subcommand (Round 2, decision 1), never at boot.

## Clarifications / ### Round 1 — 2026-10-23

- Target `github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`: require the tag; no `replace`, no `third_party`. (human, recipe)
- The Makefile includes `../platform-core/make/go-service.mk` and passes `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`; no copy of either file; `golangci-lint` is pinned as a Go `tool` directive like the SDK `go.mod`; revive doc-comment rules are off, so new code adds no doc comments. (human, recipe)
- ADR-0016 layout: `cmd/<bin>/main.go` calls `internal/app.Execute(ctx, args)` (Round 2 adds the `migrate up` subcommand; no args means `Run`); `internal/config`, `internal/rpc`, domain packages, `internal/store`, `internal/events` (the service publishes Kafka events). (human, recipe)
- Every dependency enable flag is deleted (`DATABASE_ENABLED`, `KAFKA_ENABLED`, `FEATURE_FLAGS_ENABLED`) and every runtime in-memory or no-op fallback (`NewInMemory*` in `main`, a nil producer, `featureflags.Disabled` after a failed boot, `Emitter` over a nil `Publisher`). Importing a dependency means using it; in-memory implementations stay only as test fakes. Kept flags: `OTEL_ENABLED`, `GRPC_REFLECTION_ENABLED` (off by default, refused in strict ENV). `SEED_ADMIN_ENABLED` does not apply to this service. (human, recipe)
- Mock means mock data served by the real service, through `pkg/mockdata` (`MOCK_DATA=<adapters>`, refused in strict ENV before validation). team-promotion has no mock switch today, so it declares no adapter. (human, recipe)
- Use the SDK for everything it covers: `config`, `logx`, `otelx`, `healthx` (readiness is the overall status, liveness is the separate `liveness` service, `probes.livenessService: liveness`), `lifecycle`, `grpcx` (error table including 23505 to `AlreadyExists`, `Canceled` and `DeadlineExceeded`), `auth`, `postgres.Open` (no `OpenOrLocal`), `migrate`, `outbox`, `upstream` (service principal), `auditx` (Writer interface, fail closed with "audit trail unavailable"), `flags`, `pagination`, `resilience`, `kafkax`. `consume` and `idempotency` and `jwtx` do not apply: the service has no consumer, no idempotency-key RPC and issues no token. These behaviour changes are accepted. (human, recipe)
- Every current requirement and every Q1-Q11/C4/privacy-hardening behaviour on this branch stays green; only requirements that describe removed flags, fallbacks or env vars are MODIFIED. (human, recipe)
- No DPO sign-off: basis is legitimate interest, retention is the platform log retention. (human, recipe)
- e2e is run once at the end, not per service; scenarios may still name e2e features in VERIFIED BY. `docker-compose.services.yaml` and `platform-gitops` edits are listed in `tasks.md` under "Deploy track" and done by change `deploy-go-sdk-v1`. (human, recipe)

## Decisions

### D1. Package layout and where code moves

| Old | New | Notes |
|---|---|---|
| `cmd/server/main.go` | `cmd/server/main.go` | Only signals, `app.Execute(ctx, os.Args[1:])`, exit code. Binary name stays `server`. |
| `internal/config` | `internal/config` | `Settings` embeds `Runtime` and `GRPC`; named groups `Postgres`, `OTel`, `Kafka`, `Outbox`, `Flags`, `Audit`; service groups `Upstream` and `Ads` plus `Events` (`PROMOTION_EVENTS_TOPIC`). |
| `internal/bootstrap`, `internal/grpcserver`, `internal/interceptor` | `internal/app` | Composition root, `const service = "team-promotion"`; `Execute` dispatches no args to `Run` (loads `config.Load`) and `migrate up` to `Migrate` (loads only `ENV` and `DATABASE_URL`), anything else is an error. Interceptors come from `grpcx` and `auth`. |
| `internal/handler` | `internal/rpc` | Handlers: authorize, call domain, map error with `grpcx.Status` and the service `ErrorTable`. No business rules. |
| `internal/service` | `internal/voucher`, `internal/flashsale`, `internal/subscription`, `internal/sponsored`, `internal/listing` | Domain packages return sentinel errors and never import `codes`. `listing` holds the ownership check shared by `flashsale` and `sponsored`. The audit gate is passed in as an interface (`Do(ctx, auditx.Event, func(ctx) error) error`) that `*auditx.Gate` satisfies; the gate's own `Unavailable` status error passes through `grpcx.Status` unchanged. |
| `internal/repository` | `internal/store` | Postgres repositories, model types, repository interfaces, embedded `migrations/`. Domain packages import the model types and interfaces from `store`. |
| in-memory repositories | `internal/store/storefake` | Imported only from `_test.go` files; `TestFakesNotImportedByProductionCode` enforces it with `go list -deps ./cmd/server`. |
| `internal/producer`, `bootstrap/kafka.go` | `internal/events` | Builds the proto `EventEnvelope` and the `outbox.Row`. |
| `internal/upstream` | `internal/upstream` | Adapter implementing `auditx.Writer` over the generated audit client; the team-domain listing client over `upstream.NewClient`. |
| `internal/featureflags` | removed | `flags.Evaluator` from the SDK; the flag name constant `flash-sale-enabled` moves to `internal/flashsale`. |
| `migrations/` | `internal/store/migrations/` | Same file names `0001`..`0004`; golang-migrate keys on the version number, so a database already migrated by the compose job continues at 4. New `0005_outbox`. Embedded in the binary and applied only by `migrate up`. |

Test function names are preserved when a test moves (same `Test...` name, new package path). The guard is not limited to `openspec/specs`: task 1.1.0 captures the baseline of every `func Test` in `team-promotion` (`grep -rhE '^func Test' team-promotion --include=*_test.go | sort -u`) AND every `team-promotion ... internal/<pkg> Test...` citation in `openspec/specs/**` and in the non-archived `openspec/changes/*` except this one (`promotion-voucher-scope`, `q11-checkout-contract`, and any other that cites team-promotion). Task 1.8.2 fails on any baseline name that is missing after the move, except the names this change lists as deliberately replaced: `TestDeclaredEnvKeysNoDrift` and `TestAuditAddrDeclaredEnvKey` (both replaced by `TestEnvExampleInSync`). Deliberately replaced by the Round 2 decisions (upstream addresses required, 2 s SDK bound): `TestNewAuditClientEmptyAddrNil`, `TestCreateVoucherNilAuditUnavailable` (replaced by `TestCreateVoucherAuditDownUnavailable`), `TestCreateVoucherShopAdminNilAuditUnavailable`, `TestCreateVoucherPlatformSellerDeniedNilAudit` (the refusal stays covered by `TestSellerCannotCreatePlatformVoucher`) and the `TestWithTimeout*` tests of the removed timeout interceptor (replaced by `TestListingLookupTimeoutIsUnavailable` and `TestAuditClientTimeoutBounded`). `TestSellerPathsIgnoreAuditAvailability` and `TestCreateVoucherShopSellerWritesNoAudit` keep their name with a failing writer instead of a nil one; `TestCreateCampaignAdminNoOwnerLookup` and the `TestCreateCampaignAuditFailureFailsClosed` / `TestCreateAdCampaignAuditFailureFailsClosed` variants drop their nil-client cases. Tests that keep their name but change content are listed so the rewrite is intentional: `TestConfigLoadSettings` (new settings), `TestCreateVoucherEmitsEvent` and `TestCreateCampaignEmitsEvent` (assert the outbox builder output, not an `Emitter` call), `TestCreateCampaignOwnership` (the table cases "unset fails closed for seller" and "unset still allows admin" are deleted with `listing.ErrNotConfigured`, because `UPSTREAM_DOMAIN_ADDR` is required and the not-configured branch no longer exists), `TestFlashSaleKillSwitchSuppresses` (provider from `flags.NewWithProvider`), `TestInMemory*` (moved to `internal/store/storefake`, same names). Any other missing name is a failure.

### D2. Settings

`config.Load[Settings]()` replaces the reflection binder. Key table:

| Key | Change |
|---|---|
| `ENV` | required, no default (was `local`) |
| `GRPC_PORT` | required, no default (was 50061); image sets `ENV GRPC_PORT=50061` |
| `GRPC_HOST`, `LOG_LEVEL`, `LOG_JSON`, `DB_MAX_CONNS`, `DATABASE_URL`, `OTEL_*` | SDK groups, same names; `DATABASE_URL` required |
| `GRPC_REFLECTION_ENABLED` | default false (was true); refused unless ENV is `local` or `test` |
| `SHUTDOWN_GRACE_SECONDS` | renamed `SHUTDOWN_GRACE` (`10s`); new `SHUTDOWN_DRAIN_DELAY` (`0s`) |
| `DATABASE_ENABLED`, `KAFKA_ENABLED`, `FEATURE_FLAGS_ENABLED` | deleted |
| `KAFKA_BROKERS` | SDK `Kafka` group; required in staging and prod, localhost default otherwise |
| `FLIPT_ADDR`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS` | SDK `Flags` group; `FEATURE_FLAGS_EVAL_TIMEOUT` (`500ms`) |
| `OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK` | new, SDK `Outbox` group defaults |
| `UPSTREAM_AUDIT_ADDR` | SDK `Audit` group; required in every ENV (an empty value was a fallback: nil writer) |
| `UPSTREAM_DOMAIN_ADDR` | required in every ENV (a warning plus a runtime fail-closed branch was a fallback) |
| `UPSTREAM_CALL_TIMEOUT_SECONDS` | deleted: upstream calls use the SDK 2 s bound (Round 2, decision 12) |
| `MAX_AD_BID`, `MAX_AD_BUDGET`, `PROMOTION_EVENTS_TOPIC` | unchanged names and rules |
| `LOG_KEY_FP_SECRET` | not declared: it is read only by `consume`, which this service does not use |
| `MOCK_DATA` | not declared: no adapter exists |

`.env.example` lists every key once; `TestEnvExampleInSync` uses `config.CheckEnvExample`. `Migrate` settings are a subset type (`Runtime` plus `Postgres`), so the PreSync Job and the compose migrate service need only `ENV` and `DATABASE_URL`.

### D3. Database, migrations and tests

`postgres.Open(ctx, s.Postgres)` fails boot when the database is unreachable. Boot order of `Run`: logger, telemetry, `postgres.Open`, health with the `postgres` checker, flags, upstream clients, server, `lifecycle.Run`. `Run` never migrates. `app.Migrate(ctx, ms)` opens the pool, calls `migrate.Up(ctx, url, fs.Sub(store.Migrations, "migrations"))` and returns (a migration error or a dirty version is returned and the process exits non-zero); `server migrate up` is idempotent. A service that starts before `migrate up` ran fails on its first query; the deploy order (compose `depends_on` on the migrate service, Argo PreSync Job) guarantees the order, and the service does not add a schema-version check (not requested).

Migration `0005_outbox.up.sql` is the text returned by `outbox.SchemaSQL("promotion_outbox")`; `TestOutboxMigrationMatchesSDKSchema` compares the file with the function so the two cannot drift. The down file drops the table.

Tests that need Postgres use `testkit.SharedPostgres` and `testkit.Schema`; `make check` sets `GO_SDK_REQUIRE_DOCKER=1`, so they fail rather than skip when Docker is missing, and a test that reports SKIP fails the gate (task 1.9.5). `PROMOTION_TEST_DATABASE_URL`, when set, replaces the container as the server for `testkit.Schema` (so the existing local workflow keeps working); it is a test helper input only and is read nowhere in production code. Tests that exercise only domain logic keep using `storefake` and need no database.

### D4. Events through the outbox

Write path for `CreateVoucher` and `CreateCampaign`: audit gate (when required), then `store` runs one `postgres.WithTx`: insert the entity, call the event builder with the stored entity, `outbox.Store.EnqueueTx`. The repository method takes a builder `func(created Entity) (outbox.Row, error)` so the event carries the generated id. `events` builds the `outbox.Row`: `EventID` random 128-bit hex (as today), `AggregateType` `voucher` or `flash_sale`, `AggregateID` the entity id, `EventType` the existing `platform.promotion.v1.VoucherChanged` or `FlashSaleChanged`, `Payload` the proto-marshalled `EventEnvelope` (principal from `auth`, request id from `reqctx`, traceparent), `Topic` the setting `PROMOTION_EVENTS_TOPIC`. The envelope format on the wire is unchanged for consumers.

`app` registers the relayer as a `lifecycle` component (`outbox.NewRelayer(store, kafkax.NewProducer(s.Kafka), outbox.RelayerOptions{Topic, Config: s.Outbox, Logger})`), and a purge component using `lifecycle.Every(time.Hour, ...)` calling `Store.Purge(ctx, 7*24*time.Hour)`. The outbox table is new in `0005`: the old service published best effort and had no outbox, so there are no rows parked as `failed` to requeue and no old relayer that could run beside the new one; the first rollout therefore needs no scale-to-zero for correctness (Round 2, decision 4 matters to services whose old version already ran a relayer). Published rows are purged after 7 days by `Store.Purge` on the schedule above. Emission was best effort before; it is now at-least-once with the `event-id` header for consumer dedupe. Redemption RPCs (`ValidateAndReserve`, `Commit`, `Release`, flash holds) emit nothing, as today.

The producer is closed after the relayer stops (closer order through `lifecycle.Stack`).

### D5. Health

`healthx.New(healthx.DefaultInterval, logger)`; checker `postgres` from `postgres.Checker(pool)`. Readiness is Postgres only; no upstream check is registered because team-domain and team-audit outages already fail the calls that need them, closed. Kafka is not a readiness checker in any service (Round 2, decision 2): with the outbox, a broker outage does not stop writes, and taking every pod out of rotation for it would turn a delayed-event problem into an outage. Outbox backlog is visible through relayer ERROR logs and `Store.Stats`. Flipt is not a checker: `flags` fails open by design. `OnShutdown: health.Drain`. Chart value `probes.livenessService: liveness`.

### D6. Flags

`flags.NewOrDisabled(ctx, s.Flags, logger)` returns an evaluator and a closer; an unreachable Flipt makes evaluations return the caller default (`true` for `flash-sale-enabled`), logs a warning, and never blocks boot. This is the SDK's own failure mode, not a service fallback. Flipt becomes live in every cluster: the gitops value `FEATURE_FLAGS_ENABLED: "false"` is gone, so the kill-switch works everywhere (accepted, Round 2 decision 12). Tests use `flags.NewWithProvider` over the OpenFeature in-memory provider.

### D7. Authorization on SDK auth

`auth.UnaryServerInterceptor()` populates the principal. Handlers call `auth.RequirePrincipal`, `auth.RequireScopes`, `auth.RequireService(ctx, "promotion.reserve")`. The per-RPC rule stays exactly as today; the change widens nothing:

| RPC | Rule |
|---|---|
| `CreateVoucher` PLATFORM or unspecified scope | exact scope `admin` (helper `requireAdmin`) |
| `CreateVoucher` SHOP, `CreateCampaign` | `admin` or `listing.write` (helper `requireSeller`, was `interceptor.RequireSeller`) |
| `Subscribe`, `CreateAdCampaign` | exactly `listing.write` via `auth.RequireScopes(ctx, "listing.write")`; `admin` alone is refused; NOT `requireSeller` |
| `GetEntitlements` | any authenticated principal; reads its own seller; another seller only when principal type is `service`, else `PermissionDenied` |
| `ValidateAndReserve` | service principal with `promotion.reserve` (any reservation id), or any other authenticated principal only with `reservation_id` starting `preview:<principal id>:` and `buyer_id` forced to the principal id; otherwise `PermissionDenied`; service without the scope `PermissionDenied`; no principal `Unauthenticated`; empty reservation id `InvalidArgument` |
| `CommitReservation`, `ReleaseReservation`, flash hold RPCs | `auth.RequireService(ctx, "promotion.reserve")` |
| public reads | no principal |

Service-only RPCs keep the rule: principal type `service` and exact scope. Admin means the exact scope `admin`; `requireSeller` is a helper over `auth.Principal.HasScope`. The old `interceptor.PrincipalFromContext` use in the event builder becomes `auth.FromContext`; the envelope's `commonv1.Principal` is built from it. Messages of `PermissionDenied` become the SDK's `missing scope` and `principal type not allowed`; codes are unchanged.

Request id and call logging: the removed `interceptor/tracing.go` generated `x-request-id` when absent, logged `grpc.request` per call and installed the otelgrpc stats handler. `grpcx.NewServer` provides the same through `reqctx` (incoming `x-request-id` reused or a new uuid generated, stored in the context, logged per call) and installs otelgrpc itself, so `app` adds no interceptor for it. `events` reads the id with `reqctx` so it reaches the envelope. Event principal: `events` omits `principal` from the envelope when the context has none or its id is empty, as `producer.principalFromContext` does today.

### D8. Error table

`grpcx.NewErrorTable` rules, first match wins: `voucher.ErrInvalidVoucher` and the other invalid-input sentinels to `InvalidArgument`; `store.ErrNotFound`-style sentinels to `NotFound` with the existing short messages ("voucher not found", "campaign not found", "plan not found"); `listing.ErrNotFound` to `InvalidArgument` "listing not found"; `listing.ErrNotOwner` to `PermissionDenied` "listing belongs to another seller"; `listing.ErrLookupFailed` to `Unavailable` "listing lookup failed". SQLSTATE 23505 and `context.Canceled` and `context.DeadlineExceeded` come from the SDK defaults. Everything else is `Internal` "internal error" (the existing `TestInternalErrorIsGeneric` is kept in `rpc`).

### D9. Upstream clients

team-domain: `upstream.NewClient(addr, upstream.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))))`. The SDK forwards the caller's principal and request id but does not add scopes, so the listing adapter calls with `auth.WithPrincipal(ctx, p)` where `p` is the caller's principal plus `listing.read` when absent (team-domain's `GetListing` requires it and a seller role already holds it). The scenario "Listing lookup runs as the caller with listing.read" fixes the observable behaviour. team-audit: `auditx.Dial(s.Audit, "service-team-promotion", "audit.write")` returns the connection (nil for an empty address), the adapter implements `auditx.Writer` over `auditv1.AuditServiceClient`, `auditx.NewGate(writer, logger)` wraps it. `UPSTREAM_AUDIT_ADDR` is required, so the connection is never nil and there is no nil-writer mode; a failing or slow write makes the gate return `Unavailable` "audit trail unavailable". `auditx` bounds a write at 2 s and the SDK client bounds the listing call at 2 s; no service setting changes either bound. The service principal is `service-team-promotion`, type `service`, scope `audit.write`; the admin principal is never forwarded.

### D10. Pagination

`pagination.Clamp` is not adopted for vouchers and campaigns: it caps an oversized page at the limit, while the current rule sends an oversized or non-positive page size back to the default (50). The cursor stays the plain decimal offset (`"50"`), malformed or negative cursor still means offset 0, and `pagination.EncodeOffset` is not used, so no client sees a new cursor format. This is the "keep existing cursor formats" decision; behaviour is covered by the scenario "Page cursor is the plain offset".

### D11. Docker and lint

`Dockerfile` follows `platform-core/templates/go-service/Dockerfile` minus the `third_party` lines, `ARG BIN=server`, `ARG GRPC_PORT=50061`. The build downloads the SDK module by tag from the public repository `github.com/buidangphuc/agora`: no `GOPRIVATE`, no secret, no credential. `make check` comes from the shared mk file (gofmt, vet, lint, race tests with Docker required); the old local `check`, `check-env`, `test` and `migrate` targets are deleted.

## Risks

- The repository refactor touches every test. Mitigation: names preserved; tasks keep the package-scoped test command red-then-green per package.
- Outbox ordering: two concurrent creates of the same aggregate are not ordered. Creates use a fresh id, so each aggregate has exactly one row.
- A database already migrated by the compose job is at version 4; `migrate up` applies only 0005. A dirty version fails `migrate up` (intended).
- `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_AUDIT_ADDR` becoming required in every ENV breaks any deployment that left one empty; local compose and gitops already set them (the audit address is already required in staging and prod, and set in compose).
- A service started before `migrate up` fails on queries; mitigated by deploy order (compose dependency, PreSync Job).
- An audit outage now stalls only admin-audited creates, as before; with no nil-writer mode a missing address is a boot error, not a silent `Unavailable`.

## Open issues

- Assumption: `promotion-voucher-scope` code tasks 2 and 3 are already on this branch (their tests exist in `internal/service` and `internal/handler`); its remaining open tasks are rebased onto the new package paths (proposal, Ordering).
- Assumption: "required upstream addresses" (Round 2, decision 6) means peer services (team-domain, team-audit); `KAFKA_BROKERS` stays required in staging and prod only and `FLIPT_ADDR` keeps the SDK fail-open evaluator (decision of Round 1). The scenarios "Promotion audit upstream not configured" and "Shop voucher audit: audit not configured" live in `openspec/specs` and are removed by this change's MODIFIED requirements. Fallback scenarios of team-promotion that live only in other unarchived changes (none found by name search for the removed test names) archive first, then follow this change's removals.

## Clarifications / ### Round 2 — 2026-10-23

- Migrations: every service has a `migrate up` subcommand in its binary (SDK `pkg/migrate`, embedded migrations); the service never migrates at boot; compose keeps a `team-promotion-migrate` service running the service image with `migrate up`; gitops gets an Argo PreSync migrate Job per service (in `deploy-go-sdk-v1`). (human)
- Kafka is not a readiness check in any service; readiness is Postgres and required upstream checks only (for team-promotion: Postgres only). (human)
- The shared deploy-runtime requirement "Services refuse unsafe configuration in staging and production" (and any other shared deploy-runtime requirement) is MODIFIED only by `deploy-go-sdk-v1`; this change carries no copy and lists the needed service bullet under the Deploy track. (human)
- Outbox upgrade: the migration requeues rows parked as `failed`; the first rollout scales old pods to 0 before new pods start (Recreate once, never two relayers); published rows are purged after 7 days by the SDK `Store.Purge` on a schedule in the service. For team-promotion the table is new, so there is nothing to requeue and no old relayer; the purge schedule applies. (human)
- Raw gitops manifests get strict ENV overlays in `deploy-go-sdk-v1`; `LOG_KEY_FP_SECRET` is provisioned in Vault by the user. Not applicable to team-promotion (Helm chart, no consumer, `LOG_KEY_FP_SECRET` not declared). (human)
- Upstream addresses and clients the service uses are required in every ENV, no fallback modes; the fallback scenarios are MODIFIED or REMOVED where they live in `openspec/specs`. For team-promotion: `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_AUDIT_ADDR`. (human)
- Local compose sets `GRPC_REFLECTION_ENABLED=true` for every service (Deploy track line). (human)
- Accepted: the promotion Flipt kill-switch becomes live in every cluster; `UPSTREAM_CALL_TIMEOUT_SECONDS` is removed in favour of the SDK 2 s bound. (human)
- Docker-backed tests must run: a skip is a gate failure. (human)
- The agora repository is public: no `GOPRIVATE`, no credential, no `netrc` secret. (human)
- Not applicable to this service: analytics DLQ redaction, search tombstone keys, gateway renames, GA4 keys, order dispute consumer, payment DLQ shape, engagement page cap. (human)
