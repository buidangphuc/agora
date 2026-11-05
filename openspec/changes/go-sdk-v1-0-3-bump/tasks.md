Test command base: `cd team-<svc> && go vet ./... && go test ./...`, then `make -C team-<svc> lint`. Run lint for one service at a time (parallel golangci-lint runs refuse to start). Postgres-backed tests skip without their `*_TEST_DATABASE_URL`; a skip is reported as a skip.

## 1. Code track

### 1.1 Bump the SDK
Write-set: `team-*/go.mod`, `team-*/go.sum` of analytics, audit, chat, domain, engagement, notification, order, payment, promotion, referral, search, sharing, verification.
- [x] 1.1.1 `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.3 && go mod tidy` in each of the 13 services; `go build ./...` passes in all 15.

### 1.2 Tests follow the tag rule
Write-set: `team-analytics/internal/app/layout_test.go`, `team-audit/internal/app/layout_test.go`, `team-chat/internal/config/layout_test.go`, `team-domain/internal/app/layout_test.go`, `team-engagement/internal/docs/module_test.go`, `team-gateway/internal/app/gomod_test.go`, `team-identity/internal/app/layout_test.go`, `team-notification/internal/app/module_test.go`, `team-order/internal/app/layout_test.go`, `team-payment/internal/app/module_test.go`, `team-referral/internal/config/config_test.go`, `team-referral/readme_test.go`, `team-search/internal/config/repo_test.go`, `team-sharing/internal/app/build_files_test.go`, `team-verification/internal/app/build_files_test.go`.
- [x] 1.2.1 Replace the literal version check with: exactly one require of the SDK module, version matching `^v\d+\.\d+\.\d+$`, no `replace` directive, golangci-lint `tool` line (scenarios: the go.mod scenario of each capability). Red check: compile each test binary, set the SDK require to `v1.0.4-0.20261105000000-abcdef123456` in `go.mod`, run the binary (fails in all 14), restore `go.mod` (passes).
- [x] 1.2.2 team-notification and team-payment `TestModuleCheckRejectsReplaceAndWrongVersion`: fixtures for a pseudo-version, a pre-release, two requires, a missing tool line, a `replace` and a `replace` block (scenario: A go.mod with a replace directive is rejected; A replace directive or a copied shared file fails the layout checks).

### 1.2a Call logs carry the fingerprint
Write-set: `team-sharing/internal/app/app_test.go`.
- [x] 1.2a.1 `TestCallLogsCarryNoLinkData` asserts `principal_fp` equals `logx.PrincipalFP("user-1")`, no `principal_id` key and no `user-1` in the output; `TestAnonymousCallIsLoggedAndServed` refuses `principal_id` and `principal_fp` (scenarios: Call logs carry no link data; An anonymous call is logged without a principal id). Red: the old assertion fails on `v1.0.3` (`no CreateShareLink record carries principal_id user-1`).

### 1.3 Docs
Write-set: `team-gateway/README.md`, `team-identity/README.md`, `team-referral/README.md`, `team-sharing/README.md`, `team-verification/README.md`, `team-analytics/FEATURES.yaml`, `team-audit/FEATURES.yaml`.
- [x] 1.3.1 READMEs describe the tag rule (currently `v1.0.3`); delete the identity and gateway Known gaps bullets about an untagged `v1.0.3` behind a `replace`.
- [x] 1.3.2 FEATURES.yaml: analytics acceptance text and audit entry id, title and acceptance follow the modified scenarios.

### 1.4 Logs
- [x] 1.4.1 Check no service log call passes a raw principal, user or contact id as an attribute (`git grep` over non-test, non-generated Go code; shared sloglint `forbidden-keys` in `make lint`).

### 1.5 Verify
- [x] 1.5.1 For each of the 15 services, sequentially: `make -C team-<svc> lint`, `go vet ./...`, `go test ./...`.
- [x] 1.5.2 `openspec validate go-sdk-v1-0-3-bump --strict`; `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/spec_lint.py go-sdk-v1-0-3-bump`; `bash scripts/asdlc/ci_local.sh specs repo features`.

## 2. e2e track
- [x] 2.1 [e2e] No user-facing behaviour changes: every modified scenario is unit-verified (`status: not-testable` entries in FEATURES.yaml stay as they are, with updated text for analytics and audit). (capability: analytics-service-runtime, audit-service-runtime)
