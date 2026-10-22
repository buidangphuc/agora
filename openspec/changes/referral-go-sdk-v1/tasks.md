Test command for the service unless noted: `cd team-referral && make proto && go vet ./... && GO_SDK_REQUIRE_DOCKER=1 go test -race -count=1 ./...` (`generated/` is gitignored; Postgres tests use `testkit` and Docker, a skip is not a pass). Write each test red first. Scenario ids are the scenario titles in `specs/referral-service/spec.md`.

## 1. Code track (owner: team-referral)

Rules: tests move with their code, with every existing assertion kept (`TestHandlerRejectsAnonymous`, `TestHandlerScopesToPrincipal`, `TestMapErrHidesInternalCause`, `TestCreateThenGetRoundtrip`, `TestRedeemAttributesReferee`, `TestRedeemRejectsSelfAndUnknown`, `TestCrossUserIsolation`, `TestEmptyUserRejected`, `TestListRewardsPagination`; `TestInterceptorTreatsGatewayAnonymousAsAnonymous` becomes `TestGatewayAnonymousIsUnauthenticated`). Reference only: `git show origin/feat/go-sdk-v1-e2e:team-referral/<path>`.

### 1.1 Module and layout
Write-set: `team-referral/go.mod`, `team-referral/go.sum`, `team-referral/cmd/server/main.go`, `team-referral/internal/app/app.go`, `team-referral/internal/config/config.go`, `team-referral/migrations/embed.go`.
- [ ] 1.1.1 `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.2`; no `replace`, no `third_party` (scenarios: The module requires the tag without a replace).
- [ ] 1.1.2 `internal/config`: `Settings{Runtime; GRPC; Postgres; OTel}` and `Load()`; `cmd/server/main.go` = signals, `config.Load`, `app.Run` (scenarios: Missing database URL fails the load; Missing ENV or port fails the load; A removed enable flag is ignored, not honoured; No enable flag is declared).
- [ ] 1.1.3 `internal/app.Run`: logx, otelx, healthx, `postgres.Open` + `postgres.Checker` (readiness), `grpcx.NewServer`, `lifecycle.Run` with the `Stack` closers; delete the fallback in the old `cmd/server/main.go` (scenarios: Unreachable database refuses to boot in the local environment; Unreachable database refuses to boot outside local; A reachable database serves from Postgres; Health turns NOT_SERVING when the database is lost; A failed start releases what was opened; Context cancellation stops the server within the grace period).
- [ ] 1.1.4 Tests in `internal/app/app_test.go` and `internal/config/config_test.go` named in the scenarios above, plus `TestModuleRequiresSDKTagWithoutReplace`, `TestMakefileUsesSharedFiles` and `TestDeployFilesCarryNoRemovedFlag` (read `go.mod`, `Makefile`, `../docker-compose.services.yaml`, `../platform-gitops/**/team-referral.yaml`).
- Run: `go test ./internal/config ./internal/app -count=1`.

### 1.2 Store and fake
Write-set: `team-referral/internal/store/store.go` (moved from `internal/repository/referral.go`, Postgres part and the `ReferralRepository` interface), `team-referral/internal/store/fake/fake.go` (the in-memory repo), `team-referral/internal/store/store_test.go`; delete `team-referral/internal/repository/`.
- [ ] 1.2.1 Move without behaviour change; `InMemoryReferralRepo` only under `store/fake` (scenarios: Production code cannot reach the in-memory repository; The second redeem is refused by the database too).
- [ ] 1.2.2 `TestMigrationsApplyAndPostgresRoundtrip` and `TestPostgresRedeemTwiceIsAlreadyRedeemed` with `testkit` Postgres and `migrations.Files`.
- Run: `GO_SDK_REQUIRE_DOCKER=1 go test ./internal/store/... -count=1`.

### 1.3 Domain
Write-set: `team-referral/internal/referral/referral.go`, `team-referral/internal/referral/pagination.go`, `team-referral/internal/referral/referral_test.go` (from `internal/service/`); delete `team-referral/internal/service/`.
- [ ] 1.3.1 Move `ReferralService` as `referral.Service`; same sentinels, `RewardOnReferral = 10000`, `invitee_joined`; `decodePage` uses `pagination.Clamp`, cursor codec unchanged (scenarios: Creating a code twice returns the same code; Redeeming credits the referrer; Self-referral and a second redeem are refused; Users are isolated in the ledger; Pages walk the ledger; Page size is clamped; A malformed cursor restarts at the first page).
- [ ] 1.3.2 New tests `TestRedeemRejectsSecondRedeem`, `TestListRewardsPageSizeClamp`, `TestMalformedCursorStartsAtZero`.
- Run: `go test ./internal/referral -count=1`.

### 1.4 Transport
Write-set: `team-referral/internal/rpc/referral.go`, `team-referral/internal/rpc/referral_test.go`, `team-referral/internal/rpc/anonymous_test.go`, `team-referral/internal/rpc/errors_internal_test.go`; delete `team-referral/internal/handler/`, `team-referral/internal/interceptor/`, `team-referral/internal/grpcserver/`.
- [ ] 1.4.1 `rpc.Handler` with `auth.RequirePrincipal` and a `grpcx.ErrorTable` (design D4, D5); the response keeps `Page.Total = len(rewards)` (scenarios: A call without a principal is rejected; The gateway's anonymous principal is rejected; One user cannot see another user's data; An empty code is rejected; An anonymous caller cannot redeem; The page total is the page length; A storage failure does not leak its cause).
- [ ] 1.4.2 New tests `TestUniqueViolationIsAlreadyExists`, `TestCancelledRequestIsCanceled`, `TestRedeemRaceIsFailedPrecondition` (scenarios: A unique violation is AlreadyExists; A cancelled request is Canceled; The redeem race keeps FailedPrecondition).
- Run: `go test ./internal/rpc -count=1`.

### 1.5 Build files and env
Write-set: `team-referral/Makefile`, `team-referral/.env.example`, `team-referral/Dockerfile` (only if the build command must change; entrypoint stays `./cmd/server`, `ENV GRPC_PORT=50062` stays).
- [ ] 1.5.1 `Makefile`: `SERVICE := team-referral`, `GRPC_PORT := 50062`, `LINT_CONFIG := ../platform-core/packages/go-sdk/.golangci.yml`, `include ../platform-core/make/go-service.mk` (scenarios: The Makefile uses the shared files).
- [ ] 1.5.2 `.env.example`: delete `DATABASE_ENABLED`, `SHUTDOWN_GRACE_SECONDS`; keep `GRPC_REFLECTION_ENABLED=true` for local; add `SHUTDOWN_GRACE=10s`, `SHUTDOWN_DRAIN_DELAY=0s`; keep the rest; `DATABASE_URL` stays as the local example value (scenarios: The env example matches the settings).
- Run: `make check` (vet, shared lint, race tests with Docker, `TestEnvExampleInSync`).

### 1.6 Compose
Write-set: `docker-compose.services.yaml` (service `team-referral` only).
- [ ] 1.6.1 Delete `- DATABASE_ENABLED=true`; keep `DATABASE_URL`, `GRPC_PORT`, `ENV=local` (scenarios: Compose and gitops carry no removed flag).
- Run: `docker compose -f docker-compose.yaml -f docker-compose.services.yaml config -q`.

### 1.7 Gitops
Write-set: `platform-gitops/envs/services/team-referral.yaml`, `platform-gitops/argocd/apps/team-referral.yaml`, `platform-gitops/envs/staging/services/team-referral.yaml` (new), `platform-gitops/envs/prod/services/team-referral.yaml` (new).
- [ ] 1.7.1 Baseline and the mirrored inline block: add `env: {ENV: "local", GRPC_PORT: "50062"}` and `probes.livenessService: liveness`; overlays set `ENV: "staging"` and `ENV: "production"` in the shape of `envs/prod/services/team-search.yaml` (scenarios: Compose and gitops carry no removed flag).
- [ ] 1.7.2 Check the ApplicationSet merges `envs/<env>/services/<name>.yaml` for a service by file name; if it lists services explicitly, add team-referral there and record it in the PR.
- Run: `helm template team-referral platform-gitops/charts/service -f platform-gitops/envs/services/team-referral.yaml | grep -E 'name: (ENV|GRPC_PORT)|liveness'`.

### 1.8 README
Write-set: `team-referral/README.md`.
- [ ] 1.8.1 Update: Configuration table equals the `Settings` variables (`ENV`, `LOG_LEVEL`, `LOG_JSON`, `GRPC_HOST`, `GRPC_PORT`, `SHUTDOWN_GRACE`, `SHUTDOWN_DRAIN_DELAY`, `DATABASE_URL`, `DB_MAX_CONNS`, `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`) with required/default columns; layout section (ADR-0016) and SDK tag; Build/test table uses `make check`; delete the "In-memory fallback", "inert variables", "No graceful drain" and "Most .env.example variables are inert" lines; Contract, Events ("None") and Data sections stay; keep every documented variable referenced in a source file of the repo so `scripts/repo_doctor.py` passes (scenarios: The env example matches the settings).
- Run: `python3 scripts/repo_doctor.py` (no finding for `team-referral`).

### 1.9 Final gates
- [ ] 1.9.1 `cd team-referral && make check`; `docker build -t team-referral:local team-referral` (needs `make proto` output in the context); `scripts/asdlc/ci_local.sh`; `openspec validate referral-go-sdk-v1 --strict`.

## 2. e2e track (owner: team-referral FEATURES.yaml; platform-e2e files exist, no new feature file)

- [ ] 2.1 [e2e] Re-run the existing referral journeys on the mainx stack against the migrated image: `platform-e2e/tests/e2e/features/services/new_services.feature` scenario "Referral code creation is readable via GetMyReferral" (steps `platform-e2e/tests/e2e/step_definitions/test_new_services.py`, client `platform-e2e/src/api/services/referral_service.py`); `platform-e2e/tests/e2e/features/frontend/account_ui.feature` scenarios "Redeeming an invalid code" and "No rewards yet" (steps `d1_account_steps.py`); `platform-e2e/tests/e2e/features/tracking/loop_events.feature` scenario "Loop events: redeeming a referral code emits referral_redeem without the code"; `platform-e2e/tests/e2e/features/ops/repo_doctor.feature` (README env table row for team-referral). Start only `team-referral`, its migrate job, Postgres, `team-gateway`, `team-identity` and the frontend as needed (scenarios: A buyer reaches the service through the gateway; An unknown code is not found; A new user sees an empty ledger; The service answers on its port after the migration).
- [ ] 2.2 [e2e] Create `team-referral/FEATURES.yaml` (`repo: team-referral`, schema `platform-e2e/schemas/features.schema.json`). Entries with `status: automated` and `covered_by` pointing at the e2e scenarios of 2.1 for: A buyer reaches the service through the gateway; An unknown code is not found; A new user sees an empty ledger; The service answers on its port after the migration. Entries `status: not-testable`, `tags: [referral-service, unit]`, `notes` citing the VERIFIED BY test, for every other scenario of the spec (settings and enable-flag scenarios, always-Postgres scenarios, authz, redeem rules, pagination, status-code mapping, layout scenarios). The existing `account.referral` entry in `team-frontend/FEATURES.yaml` is unchanged.
- [ ] 2.3 [e2e] `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=referral-go-sdk-v1` pass; full-suite referral-related scenarios stay green against the baseline in memory `reference-local-e2e-stack`.

## 3. Human tasks (not for an agent)
- [ ] 3.1 Confirm the Vault secret `svc/team-referral` (staging, prod) holds `DATABASE_URL`; the old code fell back to a localhost URL when it was absent, the new code refuses to boot.
- [ ] 3.2 Rollout order: sync the gitops ENV overlays (`envs/staging|prod/services/team-referral.yaml`) before bumping the image tag. Rollback = revert the image tag.
