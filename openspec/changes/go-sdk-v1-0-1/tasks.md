Test command for the SDK unless noted: `cd platform-core/packages/go-sdk && go vet ./... && go test -short ./...` (testcontainers suites need Docker; `GO_SDK_REQUIRE_DOCKER=1 make test` runs them without skipping). Write each test red first. Scenario ids are the scenario titles of `specs/go-platform-sdk/spec.md`; the short tag in brackets is used below.

## 1. Code track

### 1.1 G1 module path, tagged releases, shared files
Write-set: `platform-core/packages/go-sdk/go.mod`, every `*.go` import under `platform-core/packages/go-sdk/**`, `platform-core/packages/go-sdk/release_test.go` (new), `platform-core/packages/go-sdk/CHANGELOG.md` (new), `platform-core/packages/go-sdk/.golangci.yml`, `platform-core/make/go-service.mk`, `platform-core/templates/_service-go/{go.mod,Makefile,Dockerfile,README.md}`, delete `platform-core/tools/{sync-go-sdk.sh,sync-go-sdk_test.sh,check-go-sdk-vendor.sh,check-go-sdk-vendor_test.sh}`, `.github/workflows/asdlc-gates.yml` and `scripts/asdlc/ci_local.sh` (drop the vendor check only).
- [ ] 1.1.1 Red tests `TestModulePathIsFetchable`, `TestReleaseVersionIsDocumented`, `TestTemplateRequiresTagWithoutReplace`, `TestVendoringToolingIsRemoved`, `TestSharedLintConfigIsServiceNeutral`, `TestServiceExclusionsArePathScoped`, `TestTemplateReferencesSharedMakeAndLint` in `release_test.go` (scenarios: Module path and imports use the fetchable path; Changelog names the release; The service template requires a tag and does not vendor; Vendoring tooling is gone; Shared lint file has no service-local block and one prefix; Service exclusions are path-scoped; The template references the shared files).
- [ ] 1.1.2 Rename the module to `github.com/buidangphuc/agora/platform-core/packages/go-sdk` in `go.mod` and all 52 files (`gofmt`, then `go build ./... && go vet ./...`); update `examples/hello` and the template imports.
- [ ] 1.1.3 `.golangci.yml`: remove the `BEGIN/END service-local` block, set goimports `local-prefixes: github.com/buidangphuc`, move any known service exclusion into `path: team-<svc>/...` rules; template Makefile `include ../platform-core/make/go-service.mk` and `golangci-lint run --config ../platform-core/packages/go-sdk/.golangci.yml`; delete the template copies of both files.
- [ ] 1.1.4 Delete the four vendoring scripts and tests (deleting a file absent on this branch, for example `check-go-sdk-vendor.sh`, is a no-op); remove their CI workflow and `ci_local.sh` references where present; template `go.mod` requires `go-sdk v1.0.1` without `replace`. Update `platform-core/tools/template_smoke_test.sh` to the MODIFIED scenario "A service scaffolded from the template builds without platform copies" (resolve the tag, no sync).
- [ ] 1.1.5 Run: `go test -run 'TestModulePathIsFetchable|TestReleaseVersionIsDocumented|TestTemplateRequiresTagWithoutReplace|TestVendoringToolingIsRemoved|TestSharedLintConfigIsServiceNeutral|TestServiceExclusionsArePathScoped|TestTemplateReferencesSharedMakeAndLint' .` then `make check`.

### 1.2 G2 no enable flags
Write-set: `platform-core/packages/go-sdk/pkg/config/{groups_test.go,config_test.go}`, `platform-core/packages/go-sdk/enableflags_test.go` (new). No production code change expected; fix any `*_ENABLED` the tests find.
- [ ] 1.2.1 Tests `TestNoDependencyEnableFlags`, `TestLoadIgnoresRemovedEnableFlags`, `TestNoEnableFlagInSDKSource` (scenarios: No config group declares a dependency enable variable; Removed flags are ignored, not honoured; No package source reads an enable flag).
- [ ] 1.2.2 Run `go test ./pkg/config/ -run 'TestNoDependencyEnableFlags|TestLoadIgnoresRemovedEnableFlags' && go test -run TestNoEnableFlagInSDKSource .`

### 1.3 G3 mockdata
Write-set: `platform-core/packages/go-sdk/pkg/config/{groups.go,groups_test.go,config.go}`, `platform-core/packages/go-sdk/pkg/mockdata/**` (new: `doc.go`, `mockdata.go`, `mockdata_test.go`).
- [ ] 1.3.1 Red tests in `pkg/config`: `TestLoadRefusesMockDataInStrictEnvBeforeValidation`, `TestMockDataRefusedForEveryStrictSpelling`, `TestMockDataAllowedWhenLocalOrEmpty`; then add `config.MockData` with `ResolveFor`.
- [ ] 1.3.2 Red tests in `pkg/mockdata`: `TestNewRejectsUnknownAdapter`, `TestNewNormalisesNames`, `TestLogActiveOncePerSet`, `TestSelectSwapsOnlyActiveAdapter`; then `New`, `Set.Active/Adapters/LogActive`, `Select`.
- [ ] 1.3.3 Run `go test ./pkg/config/ ./pkg/mockdata/`.

### 1.4 G4 auditx
Write-set: `platform-core/packages/go-sdk/pkg/config/{groups.go,groups_test.go}` (`Audit`), `platform-core/packages/go-sdk/pkg/auditx/**` (new: `doc.go`, `auditx.go`, `auditx_test.go`).
- [ ] 1.4.1 Red `TestStrictEnvRequiresAuditAddr` then `config.Audit` with `ResolveFor`.
- [ ] 1.4.2 Red `TestDoWritesBeforeAction`, `TestDoFailsClosedOnWriteError`, `TestDoFailsClosedWithoutWriter`, `TestDoFailsClosedOnTimeout`, `TestDoBoundsWriteWithoutCallerDeadline`, `TestDoReturnsActionErrorUnchanged`, `TestDialSendsServicePrincipal` (a bufconn or loopback test server); then `Event`, `Writer`, `Gate`, `Dial`, `Message` (design D2, D3).
- [ ] 1.4.3 Run `go test ./pkg/auditx/ ./pkg/config/ -run 'Audit|Do|Dial'`.

### 1.5 G5 key fingerprint
Write-set: `platform-core/packages/go-sdk/pkg/config/{groups.go,groups_test.go}` (`LogKeyFP`), `platform-core/packages/go-sdk/pkg/kafkax/{fingerprint.go,fingerprint_test.go}` (new), `platform-core/packages/go-sdk/pkg/consume/{consume.go,batch.go,redact.go,consume_test.go,batch_test.go}`.
- [ ] 1.5.1 Red `TestStrictEnvRequiresLogKeyFPSecret`, `TestFingerprintFormatStableAndKeyed`, `TestNewFingerprinterRejectsEmptySecret`; then `config.LogKeyFP`, `kafkax.NewFingerprinter`, `Fingerprinter.Of`.
- [ ] 1.5.2 Red `TestHandlerFailureLogsKeyFingerprint`, `TestDeadLetterLogsFingerprintKeepsDLQKey`, `TestDeadLetterFailureLogsFingerprintOffsetHeld`, `TestErrorTextIsScrubbedOfKey`, `TestBatchLogsFingerprintsOnly`, `TestNilFingerprinterUsesRandomSecret` (use the existing `fake_test.go` group client); then add `KeyFP` to `Options` and `BatchOptions`, a shared `redact` helper, `key_fp` on every consume log record, and scrubbed `dlq-error` header.
- [ ] 1.5.3 Run `go test ./pkg/kafkax/ ./pkg/consume/ ./pkg/config/` and `go test -race ./pkg/consume/`.

### 1.6 G6 unique violation
Write-set: `platform-core/packages/go-sdk/pkg/grpcx/{status.go,grpcx_test.go}`, `platform-core/packages/go-sdk/go.mod` (direct `pgx` require stays).
- [ ] 1.6.1 Red `TestStatusMapsUniqueViolationToAlreadyExists`, `TestStatusUniqueViolationHidesDetail`, `TestStatusRuleBeatsUniqueViolation`, `TestStatusOtherPgErrorsStayInternal`, `TestStatusGRPCStatusBeatsUniqueViolation`; then the built-in step in `Status` (design D6).
- [ ] 1.6.2 Run `go test ./pkg/grpcx/`.

### 1.7 Docs
Write-set: `platform-core/packages/go-sdk/README.md`, `platform-core/packages/go-sdk/CHANGELOG.md`, `platform-core/docs/ADR/0018-go-sdk-tagged-module-distribution.md` (new), `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md` (status line and a pointer only).
- [ ] 1.7.1 README: version `v1.0.1`, new import path, `go get ...@v1.0.1`, packages `mockdata` and `auditx`, env vars `MOCK_DATA`, `UPSTREAM_AUDIT_ADDR`, `LOG_KEY_FP_SECRET`, the "no enable flags" rule, the "do not format ids into errors" rule, the shared make and lint reference; remove the sync section (`TestReleaseVersionIsDocumented`).
- [ ] 1.7.2 CHANGELOG: v1.0.1 entries G1 to G6.
- [ ] 1.7.3 ADR-0018 supersedes the distribution part of ADR-0016 (vendoring, `sync-go-sdk.sh`, `third_party`, `replace`, drift gate); states tag scheme, module path, no enable flags, shared make and lint files, and mock-data-not-mock-services. ADR-0016 gets "Superseded in part by ADR-0018" in its status and keeps its package rules.
- [ ] 1.7.4 Run `make check` in `platform-core/packages/go-sdk`, `scripts/asdlc/ci_local.sh`, then `openspec validate go-sdk-v1-0-1 --strict`.

## 2. e2e track

None of these has a user journey; each is `not-testable` with its unit test named, like the existing `go-sdk.*` entries.
- [ ] 2.1 [e2e] Add `platform-core/FEATURES.yaml` entries (`status: not-testable`, `tags: [go-sdk, unit]`, `notes` naming the VERIFIED BY test) for every scenario of the spec delta: module path and imports; changelog release; template requires tag; vendoring tooling removed; shared lint config; path-scoped exclusions; template references shared files; no enable variable in config; removed flags ignored; no enable flag in source; MOCK_DATA strict refusal, spelling, local acceptance, unknown name, normalisation, log once, select; audit gate order, write error, missing writer, timeout, action error, strict address, dial principal; key fingerprint failure log, DLQ log, DLQ failure, error scrubbing, batch, format, empty secret, strict secret, nil fingerprinter; unique violation mapping, detail hidden, rule precedence, other errors, gRPC status passthrough.
- [ ] 2.2 [e2e] Rewrite the FEATURES notes that still cite `sync-go-sdk_test.sh` or `check-go-sdk-vendor_test.sh` (around lines 804, 810, 825, 4349, 4560, 4691, 4700, 5081): mark the entries of the REMOVED requirement obsolete or point them at the new tests.
- [ ] 2.3 [e2e] Run the FEATURES lint/gate (`scripts/asdlc/ci_local.sh`) so every VERIFIED BY resolves to an existing test.

## 3. Human tasks (not for an agent)
- [ ] 3.1 After this PR merges to main, cut and push the tag `platform-core/packages/go-sdk/v1.0.1`.
- [ ] 3.2 After the tag exists, verify `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1` works from a clean Docker build (only the tag-to-proxy delay can fail it).
- [ ] 3.3 Provision `LOG_KEY_FP_SECRET` in Vault for staging and prod.
