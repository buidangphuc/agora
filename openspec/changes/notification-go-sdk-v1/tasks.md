Test command base: `cd team-notification && go vet ./... && go test -race ./...` (Docker needed for `testkit.Postgres` and `testkit.Redpanda`; set `GO_SDK_REQUIRE_DOCKER=1`). Write each test red first. Scenario ids are the scenario titles in `specs/notification-service-runtime/spec.md`. Python for any repo script: `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python`. Do not commit.

## 1. Code track

### 1.1 Module, Makefile, tooling
Write-set: `team-notification/go.mod`, `team-notification/go.sum`, `team-notification/Makefile`, `team-notification/internal/app/module_test.go`, `team-notification/Dockerfile` (only if the build needs a change; expected none).
- [ ] 1.1.1 Red `TestModuleRequiresSDKTagWithoutReplace`, `TestModuleCheckRejectsReplaceAndWrongVersion`, `TestMakefileIncludesSharedGoServiceMk` (scenarios: The module requires the SDK tag without a replace; A go.mod with a replace directive is rejected; The Makefile reuses the shared targets). Then `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`, add the `tool` directive for golangci-lint at the version in the SDK `go.mod` (`go get -tool`), write the `Makefile` (`SERVICE := team-notification`, `GRPC_PORT := 50058`, `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, `include ../platform-core/make/go-service.mk`). No `replace`, no `third_party`, no copied `.golangci.yml`, no doc comments added.
- [ ] 1.1.2 Run `cd team-notification && go test ./internal/app -run 'TestModule|TestMakefile'`.

### 1.2 Settings
Write-set: `team-notification/internal/config/**`, `team-notification/.env.example`, `team-notification/README.md` (env table and Known gaps only).
- [ ] 1.2.1 Red tests `TestLoadRejectsMissingDatabaseURL`, `TestLoadRequiresKafkaBrokersAndKeyFPSecretInStrictEnv`, `TestNoDependencyEnableFlags`, `TestLoadRefusesReflectionInStrictEnv`, `TestLoadLowStockThreshold`, `TestLoadRequiresUpstreamAddrsInEveryEnv`, `TestConsumerSettingsDefaults`, `TestEnvExampleInSync` (scenarios: Upstream addresses are required in every ENV; A missing DATABASE_URL fails load; Kafka brokers and the key fingerprint secret are required in a strict ENV; No dependency enable flag exists; Reflection is refused in a strict ENV and off by default; The low-stock threshold boundaries; Consumer settings keep their names and defaults; .env.example matches Settings). Then `Settings` over the SDK groups plus the `Notification` group (design D2), `Validate`, `Load`; rewrite `.env.example` (set `GRPC_REFLECTION_ENABLED=false`; remove `DATABASE_ENABLED`, `KAFKA_ENABLED`, `SHUTDOWN_GRACE_SECONDS`; add `ENV`, `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `KAFKA_BROKERS`, `LOG_KEY_FP_SECRET=`). Move `TestTask5ReadmeDocumentsOrderWorkEventsAndLowStockThreshold` here and update the README (new env list, no mock-mode or `Unimplemented` text).
- [ ] 1.2.2 Run `cd team-notification && go test ./internal/config`.

### 1.3 Store and migrations
Write-set: `team-notification/internal/store/**`, `team-notification/internal/store/migrations/**` (moved from `team-notification/migrations/`, file contents unchanged), `team-notification/migrations` (deleted), `team-notification/internal/repository` (deleted).
- [ ] 1.3.1 Red Postgres tests on `testkit.Postgres` for the inbox, alert, prefs and last-seen queries, moved from `internal/repository` and `internal/consumer/*_pg_test.go` (`TestPostgresLastSeenStores`, `TestPostgresLowStockStoreDelete`) and `TestInboxQueriesP95At10kRows` (scenario: Listing stays fast at 10,000 rows per user); add `TestUniqueViolationMapsToAlreadyExists` support by exposing the raw pg error unchanged. Then the pgx store taking a `pgx.Tx`-or-pool `Querier` so consumers can run in a transaction; embed `migrations` as `store.Migrations`. In-memory repositories move to `_test.go` fakes in the packages that need them (scenarios: Notifications of a user are scoped; Subscribing twice returns the same subscription).
- [ ] 1.3.2 Run `cd team-notification && go test ./internal/store`.

### 1.4 Domain packages and error table
Write-set: `team-notification/internal/{inbox,alerts,prefs}/**`, `team-notification/internal/service` (deleted).
- [ ] 1.4.1 Move `internal/service` tests (`TestGetReturnsDefaultsWhenUnset`, `TestUpdateRoundtripAndIsolation`, `TestPrefsInputValidation`, `TestBucketDigestHelper`, `TestBundleDigest*`) to `internal/prefs`, with `prefs.ErrNilPrefs`; sentinel errors in `alerts`/`inbox`; no `grpc` import (scenarios: Preferences default and round-trip per user; A missing prefs payload is rejected). Run `go test ./internal/inbox ./internal/alerts ./internal/prefs`.

### 1.5 RPC transport
Write-set: `team-notification/internal/rpc/**`, `team-notification/internal/{handler,interceptor,grpcserver}` (deleted).
- [ ] 1.5.1 Red tests in `internal/rpc`: `TestUsersDoNotSeeEachOthersData`, `TestMarkAsReadOtherUsersNotificationIsNotFound`, `TestPerUserRPCsRequireAUserPrincipal`, `TestListNotificationsPageSizeClamp`, `TestMapErrsHideInternalCause`, `TestUniqueViolationMapsToAlreadyExists`, `TestSubscribeAlertIsIdempotent`, `TestSubscribeAlertValidationIsInvalidArgument`, `TestUnsubscribeForeignSubscriptionIsNoOp`, `TestPanicReturnsInternalAndServerKeepsServing`, `TestMarkAsReadEmptyIDMarksAllOfCallerOnly`, `TestResolverPrincipalDeniedOnPerUserRPCs` (scenarios: An empty id marks all of the caller's notifications read; The resolver principal cannot use the per-user RPCs; of requirements "Notification RPCs serve only the calling user's data" and "Alert and preference RPCs validate input and are always served"). Then `rpc.Handler`, `rpc.Register`, `auth.RequireUser`, `grpcx.Status` with `NewErrorTable`, `pagination.Clamp(size, 20, 100)`; no `Unimplemented` branch.
- [ ] 1.5.2 Run `cd team-notification && go test -race ./internal/rpc`.

### 1.6 Sender-name resolver
Write-set: `team-notification/internal/names/**`, `team-notification/internal/upstream` (deleted).
- [ ] 1.6.1 Red `TestResolve_SellerGetsShopName`, `TestResolverSendsReadOnlyServicePrincipal`, `TestResolverBoundedToTwoSeconds`, keep `TestResolve_BuyerGetsUserDisplayName`, `TestResolve_SellerWithoutShopFallsBackToProfile`, `TestResolve_LookupFailureYieldsEmpty` (scenarios: A seller reply names the shop; The lookup carries the read-only service principal; A slow upstream is cut at 2 seconds; Lookup failure still notifies). Then resolver over `upstream.NewClient` and `upstream.Bounded`.
- [ ] 1.6.2 Run `cd team-notification && go test -race ./internal/names`.

### 1.7 Event consumers
Write-set: `team-notification/internal/events/**`, `team-notification/internal/consumer` (deleted), `team-notification/internal/bootstrap` (deleted).
- [ ] 1.7.1 Move the consumer tests listed in design D9 into `internal/events` (names unchanged), run first red against the new package. Add `TestRedeliveredEventNotifiesOnce_Postgres`, `TestFailedWriteRollsBackLedgerAndRetryNotifies_Postgres`, `TestMalformedRecordGoesToDLQAndAdvances`, `TestTransientFailureRetriesBeforeCommit`, `TestConsumerLogsKeyFingerprintNotKey`, `TestOrder_DisabledPreferenceSkips`, `TestListingAlertsIgnorePrefs`, `TestEventsNeverCreateRowsForOtherUsers` (scenarios: An event for user A never creates a row for user B; A disabled order notification type is skipped; A listing alert ignores preferences; A redelivered event notifies once; A failed write rolls the ledger row back; A malformed record goes to the DLQ and the offset advances; A transient failure retries without committing; Logs show the key fingerprint, not the key; the other scenarios of "Notification rules keep their behaviour on the new wiring"). Then the three `consume.Handler`s: one `pgx.Tx` per record with `consume.Dedupe.Seen`, `consume.Permanent` for poison, `kafkax.Fingerprinter` from `LOG_KEY_FP_SECRET`; delete `runLoop`, `RecordReader`, `DeadLetterProducer`, `RunConfig` and every `InMemory*` outside `_test.go`.
- [ ] 1.7.2 Run `cd team-notification && GO_SDK_REQUIRE_DOCKER=1 go test -race ./internal/events`.

### 1.8 Composition root and main
Write-set: `team-notification/internal/app/**`, `team-notification/cmd/server/main.go`.
- [ ] 1.8.1 Red `TestRunFailsWhenDatabaseUnreachable`, `TestMigrateUpFailsOnDirtyMigration`, `TestRunDoesNotMigrate`, `TestMigrateThenRunServesAllRPCs`, `TestConsumerErrorStopsRun`, `TestNoRuntimeFallbacksInSource`, `TestDockerRequiredNeverSkips`, `TestReadinessFollowsPostgresLivenessDoesNot`, `TestReadinessIgnoresKafka`, `TestShutdownDrainsHealthFirst`, `TestReflectionOffByDefault`, `TestLivenessCheckLatencyUnder5ms`, `TestChatEventBecomesNotificationWithin30s` (scenarios: A failed migration fails the migrate subcommand; Boot never migrates; Migrate then boot serves every RPC; Docker-backed tests never skip; A broker outage does not flip readiness; Postgres open is bounded and fails fast; Liveness answers from memory; A chat message becomes a notification within 30 seconds; of requirements "Boot uses the real database and broker or fails" and "Readiness follows dependencies and liveness does not"; app tests start a broker with `testkit.Redpanda` and a database with `testkit.Postgres`, use package `app_test`, a port from `net.Listen("127.0.0.1:0")`, `logx.NewTo(io.Discard, ...)`). Then `app.Run` in the order of design D3, `app.Migrate` behind the `migrate up` subcommand in `main` (no migration inside `Run`); `const service = "team-notification"`.
- [ ] 1.8.2 Run `cd team-notification && GO_SDK_REQUIRE_DOCKER=1 go test -race ./internal/app`.

### 1.9 Whole-service gate (last code task)
Write-set: `team-notification/**` (fixes only), `openspec/changes/notification-go-sdk-v1/**`.
- [ ] 1.9.1 Run, in order: `cd team-notification && make check` (vet, golangci-lint with the SDK config, `GO_SDK_REQUIRE_DOCKER=1` race tests, env check), `go build ./...`, `docker build -t team-notification:local team-notification` from the worktree root. All must pass; `git grep -nE 'DATABASE_ENABLED|KAFKA_ENABLED|InMemory|Noop|mock mode|lookup disabled' -- team-notification ':!*_test.go'` returns nothing.
- [ ] 1.9.2 Run `openspec validate notification-go-sdk-v1 --strict` and `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/spec_lint.py notification-go-sdk-v1`.

## 2. e2e track
Write-set: `team-notification/FEATURES.yaml`.
- [x] 2.1 [e2e] (capability: notification-service-runtime) Update the existing entries `notification.center`, `notification.alerts`, `notification.alerts-delivery`, `notification.unread-badge` and the order-work and chat entries: keep their `covered_by` features, add `team-notification` restart-safety and readiness notes only where an entry's acceptance text mentions mock mode, `KAFKA_ENABLED` or in-memory state (remove that text). Add `notification.service-runtime` with `status: not-testable`, `covered_by` empty and a note that each scenario of `notification-service-runtime` names its Go test in `VERIFIED BY`. e2e runs once at the end of the migration; do not run it here.

## 3. Deploy track (done by change deploy-go-sdk-v1)
Not code tasks of this change. `deploy-go-sdk-v1` owns all of these edits and the shared deploy-runtime requirements; it merges together with this change. Exact edits, applied in the same merge as the service image.

`docker-compose.services.yaml`
- Keep the `team-notification-migrate:` service but run the service image (`team-notification`) with command `migrate up` instead of the migrate tool image; `team-notification` keeps `depends_on: team-notification-migrate: condition: service_completed_successfully` and `redpanda-init`.
- Add `GRPC_REFLECTION_ENABLED=true` to `team-notification` (local compose, every service).
- Delete the line `- KAFKA_ENABLED=true`.
- Keep `DATABASE_URL`, `KAFKA_BROKERS`, topic, DLQ and group lines, `UPSTREAM_*`, `GRPC_PORT=50058`, `ENV=local`. `UPSTREAM_DOMAIN_ADDR` and `UPSTREAM_IDENTITY_ADDR` stay set. `LOG_KEY_FP_SECRET` is not needed locally (the SDK fills its local default for `ENV=local`).

`platform-gitops/envs/services/team-notification.yaml` (baseline) and the inline block in `platform-gitops/argocd/apps/team-notification.yaml` (same edits in both)
- Delete `KAFKA_ENABLED: "true"`.
- Add `ENV: "local"` under `env:` (staging and prod override it below).
- Add at top level:
  ```
  probes:
    livenessService: liveness
  ```
- Add an Argo PreSync migrate Job running the service image with `migrate up` (shared chart value, per service).
- Both upstream addresses stay set in every ENV; the strict-ENV refusal of unsafe configuration (reflection on, missing required keys) is a bullet of the shared requirement "Services refuse unsafe configuration in staging and production", which only `deploy-go-sdk-v1` MODIFIES.
- Keep `secrets.enabled: true`; the Vault entry for team-notification must also provide `DATABASE_URL` (already) and the new `LOG_KEY_FP_SECRET` (provisioned in Vault by the user, human task; `platform-gitops/platform/vault-config/vault-config.yaml`, next to the other services that consume Kafka).

`platform-gitops/envs/staging/services/team-notification.yaml` (new file, same shape as `team-search.yaml`)
```
env:
  ENV: "staging"
```
`platform-gitops/envs/prod/services/team-notification.yaml` (new file)
```
env:
  ENV: "production"
```
Neither overlay sets `GRPC_REFLECTION_ENABLED`.
