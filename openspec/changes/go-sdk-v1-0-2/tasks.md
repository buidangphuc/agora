Test command for the SDK: `cd platform-core/packages/go-sdk && go vet ./... && go test -short ./...`. Write each test red first. Scenario ids are the scenario titles of `specs/go-platform-sdk/spec.md`.

## 1. Code track

### 1.1 Reflection flag
Write-set: `platform-core/packages/go-sdk/pkg/config/{groups.go,groups_test.go,config_test.go}`, `platform-core/packages/go-sdk/pkg/grpcx/{grpcx.go,grpcx_test.go,doc.go}`, `platform-core/packages/go-sdk/enableflags_test.go`.
- [ ] 1.1.1 Red tests in `pkg/config`: `TestStrictEnvRefusesGRPCReflection`, `TestLocalEnvAcceptsGRPCReflection`, `TestGRPCReflectionDefaultsOff`, `TestStrictReflectionRefusalPrecedesValidation`, `TestReflectionWithoutRuntimeIsRefused` (scenarios: Reflection without a Runtime is refused; Reflection is refused in a strict environment; Reflection is accepted locally and defaults to off; A strict-env reflection refusal comes before group validation). Then add `GRPC.ReflectionEnabled` and `func (g *GRPC) ResolveFor(rt Runtime) error`; `Load` also refuses a true value when the settings struct holds no `Runtime`.
- [ ] 1.1.2 Red tests in `pkg/grpcx`: `TestReflectionOffByDefault`, `TestReflectionServedWhenEnabled` (scenarios: Reflection is off by default; Reflection is served locally when enabled; use a bufconn client with `grpc_reflection_v1`). Then register reflection only when `o.Config.ReflectionEnabled`; rename the old `TestReflectionAlwaysRegistered`.
- [ ] 1.1.3 (requirement "The SDK has no dependency enable flags", MODIFIED) Allow `GRPC_REFLECTION_ENABLED` next to `OTEL_ENABLED` in `TestNoDependencyEnableFlags` (`pkg/config`) and `TestNoEnableFlagInSDKSource` (root) so the `go-sdk-v1-0-1` scenarios keep passing; keep rejecting every other `*_ENABLED`.
- [ ] 1.1.4 Run `go test ./pkg/config/ ./pkg/grpcx/ && go test -run TestNoEnableFlagInSDKSource .`

### 1.2 Liveness health service
Write-set: `platform-core/packages/go-sdk/pkg/healthx/{healthx.go,healthx_test.go,doc.go}`.
- [ ] 1.2.1 Red tests `TestLivenessIgnoresCheckerFailure`, `TestLivenessServingBeforeRun`, `TestLivenessAnswersFastDuringHangingChecker`, `TestDrainSetsLivenessNotServing`, `TestUnknownHealthServiceNotFound` (scenarios: Liveness stays serving while a dependency is down; Liveness is serving from construction; A hanging checker does not delay liveness; Drain turns liveness off; An unknown health service name is still rejected). Then `New` sets `liveness` SERVING; export a `LivenessService = "liveness"` constant.
- [ ] 1.2.2 Run `go test -race ./pkg/healthx/`.

### 1.3 Remove OpenOrLocal
Write-set: `platform-core/packages/go-sdk/pkg/postgres/{postgres.go,postgres_test.go}`, `platform-core/packages/go-sdk/examples/**` (only if they call it).
- [ ] 1.3.1 Delete `OpenOrLocal`, `TestOpenOrLocalFallsBackOnlyLocally`, `TestOpenOrLocalNilLogger` and now-unused imports; keep `TestOpenFailsOnUnreachableDB` (requirement "Local-only database fallback" REMOVED). Run `go build ./... && go vet ./... && go test ./pkg/postgres/ -short`; `git grep OpenOrLocal` outside `openspec/changes/archive` and `FEATURES.yaml` returns nothing.

### 1.4 Chart opt-in
Write-set: `platform-gitops/charts/service/templates/_container.tpl`, `platform-gitops/charts/service/values.yaml`, `platform-gitops/tests/golden/team-verification.pre-liveness.yaml` (new), `platform-gitops/README.md` (one line on the value), `platform-e2e/tests/e2e/features/ops/gitops_render.feature`, `platform-e2e/tests/e2e/step_definitions/pear_authz_gitops_steps.py` (`_render` accepts extra `--set` args).
- [ ] 1.4.0 BEFORE editing `_container.tpl`, capture the golden: `cd platform-gitops && helm template team-verification charts/service --namespace marketplace` with the same values layering as the step (envs/<env>/values.yaml then envs/services/team-verification.yaml) into `tests/golden/team-verification.pre-liveness.yaml`.
- [ ] 1.4.1 Red: add the four scenarios "A gRPC service with the value set probes liveness by name", "The default render is unchanged", "An HTTP service ignores the value", "The canary Rollout renders the same liveness probe" to `gitops_render.feature` with steps (scenarios of the same titles). Run them red against the unchanged chart.
- [ ] 1.4.2 Add `probes.livenessService: ""` to `values.yaml` with a comment; in `_container.tpl` render `service: {{ . }}` under the gRPC `livenessProbe.grpc` only when non-empty. Run `cd platform-e2e && .venv/bin/python -m pytest tests/e2e/step_definitions/test_pear_authz_gitops.py` (needs `helm`).

### 1.5 Docs
Write-set: `platform-core/packages/go-sdk/README.md`, `platform-core/packages/go-sdk/CHANGELOG.md`, `platform-core/packages/go-sdk/release_test.go`.
- [ ] 1.5.1 Update `TestReleaseVersionIsDocumented` expectation to `v1.0.2` first (red), then README version line and `go get ...@v1.0.2`; remove `OpenOrLocal` from the `postgres` row and the Configuration note; `grpcx` row says reflection is off unless `GRPC_REFLECTION_ENABLED=true` (refused when `ENV` is staging or prod); `healthx` row names the `liveness` service; add the line "set `probes.livenessService: liveness` in the service's gitops values". Fix the sentence that says `GRPC_REFLECTION_ENABLED` does not exist.
- [ ] 1.5.2 CHANGELOG `v1.0.2`: Removed `OpenOrLocal`; Added `GRPC_REFLECTION_ENABLED`, `liveness` health service; Changed reflection default.
- [ ] 1.5.3 Run `make check` in `platform-core/packages/go-sdk`, then `openspec validate go-sdk-v1-0-2 --strict`.

## 2. e2e track
- [ ] 2.1 [e2e] In `platform-core/FEATURES.yaml` add `status: not-testable`, `tags: [go-sdk, unit]` entries (notes name the VERIFIED BY test) for: reflection refused in strict ENV, accepted locally and off by default, refusal precedes validation, reflection off by default, reflection served when enabled, liveness stays serving, liveness from construction, drain turns liveness off, unknown service not found. Replace the entry `go-sdk.reflection-is-always-on`. Mark the two `OpenOrLocal` entries (`go-sdk.an-unreachable-database-fails-boot-outside-local` and the nil-logger one) obsolete or delete them (REMOVED requirement). (capability: go-platform-sdk)
- [ ] 2.2 [e2e] In `platform-gitops/FEATURES.yaml` add four entries `status: automated`, `covered_by: ops/gitops_render.feature::<title>` for the chart scenarios. (capability: go-platform-sdk)
- [ ] 2.3 [e2e] Run `scripts/asdlc/ci_local.sh` so every VERIFIED BY resolves. (capability: go-platform-sdk)

## 3. Human tasks (not for an agent)
- [ ] 3.1 After this PR merges to main, cut and push tag `platform-core/packages/go-sdk/v1.0.2`.
- [ ] 3.2 Each service-migration change sets `probes.livenessService: liveness` in its `platform-gitops/envs/services/<svc>.yaml` and never sets `GRPC_REFLECTION_ENABLED` in staging or prod.
