Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml` (orchestrator, `## 7`). All paths below are under `platform-core/packages/go-sdk/` unless they start with `platform-core/`. Every packet: red test first, then code; `go build ./... && go vet ./... && go test ./...` in the SDK module; no new third-party module unless the packet says so; do not edit `go.mod`/`go.sum` except packet 5 (others: report a needed dependency instead).

## 1. auth, jwtx, idempotency (wave 1)

Owns: `pkg/auth/**`, `pkg/jwtx/**` (new), `pkg/idempotency/**` (new).

- [ ] 1.1 Principal type constants, `RequireService`, `RequireUser`; tests `TestRequireServiceRejectsUser`, `TestRequireUserRejectsServiceAndAnonymous`. Model the claim fields on `team-identity/internal/token/jwt.go` and `team-gateway/internal/token/{jwt,jwks}.go` (read only).
- [ ] 1.2 `jwtx.Claims`, `jwtx.JWKS`, `jwtx.JWK`; `TestClaimsJSONContract`. Use `github.com/golang-jwt/jwt/v5` `RegisteredClaims` only if already in the SDK module graph; otherwise plain fields with RFC 7519 names.
- [ ] 1.3 `idempotency.MDKey`, `idempotency.Validate`; `TestValidateRule` (rule from `team-gateway/internal/edge/forward.go` and `team-order/internal/order/idempotency.go`).

## 2. grpcx rules (wave 1)

Owns: `pkg/grpcx/**`, `examples/**`, `platform-core/templates/_service-go/**`.

- [ ] 2.1 `ErrorTable map[error]Rule`, `Rule{Code, Message}`, `Code(c)`; `TestStatusUsesRuleMessage`, `TestStatusKeepsErrorTextWithoutMessage`; update existing grpcx tests, `examples/hello` and the template (`.tmpl` sources) to the new table shape. `platform-core/tools/template_smoke_test.sh` passes.

## 3. config, pgx, pagination (wave 1)

Owns: `pkg/config/**`, `pkg/pgx/**`, `pkg/pagination/**` (new).

- [ ] 3.1 `Runtime.IsStrict`, `PositiveDuration`, `keepempty`; `TestIsStrict`, `TestPositiveDuration`, `TestKeepEmpty`. `.env.example` drift check (`CheckEnvExample`) still works for `keepempty` fields.
- [ ] 3.2 `pgx.OpenOrLocal`; `TestOpenOrLocalFallsBackOnlyLocally` (unreachable address, no container needed).
- [ ] 3.3 `pagination`; `TestOffsetToken` plus a Clamp table test.

## 4. lifecycle, flags, resilience, upstream (wave 1)

Owns: `pkg/lifecycle/**`, `pkg/flags/**`, `pkg/resilience/**`, `pkg/upstream/**`.

- [ ] 4.1 `lifecycle.Every`, `lifecycle.HTTP`; `TestEveryKeepsRunningAfterError`, `TestHTTPShutsDownGracefully`.
- [ ] 4.2 `flags.NewOrDisabled`; `TestNewOrDisabledDegrades`.
- [ ] 4.3 `RetryOptions.RetryIf`; `TestRetryIfOverridesDefault`.
- [ ] 4.4 `upstream.WithDialOptions`; `TestDialOptionsApplied`.

## 5. messaging and sync (wave 1)

Owns: `pkg/consume/**`, `pkg/kafkax/**`, `go.mod`, `go.sum`, `platform-core/tools/sync-go-sdk.sh`, `platform-core/tools/sync-go-sdk_test.sh`.

- [ ] 5.1 Port from commit `967bcc7a` (withdrawn v0.2.3; `git show 967bcc7a -- <path>`): `Dedupe.Processed`, the group-less `NewConsumerClient`, the sync `require` handling, the kadm dependency. Do not take its `openspec/` hunk. Tests `TestDedupeProcessedIsReadOnly`, `TestDedupeProcessedReturnsErrors`, `TestGroupLessConsumerClient`, `sync-go-sdk_test.sh`.
- [ ] 5.2 `consume.NewBatchConsumer`, `BatchHandler`, `BatchOptions`; `TestBatchRetriesWithoutCommit`, `TestBatchPermanentGoesToDLQ`. Reuse the per-record consumer's DLQ header code. Read `team-analytics/internal/events/{listing,engagement,consumer}.go` (read only) for the loops it replaces.

## 6. testkit (wave 1)

Owns: `pkg/testkit/**`.

- [ ] 6.1 `FreePort`, `Eventually`, `Recorder` (+`FailOn`); `TestRecorderFailOn`.
- [ ] 6.2 `SharedPostgres`, `Schema`; `TestSchemaIsolation`.
- [ ] 6.3 `Produce`, `ReadAll` over `Redpanda`; a round-trip test.

## 7. Close (orchestrator)

- [ ] 7.1 Merge packets, `go mod tidy`, full SDK tests; SDK README table lists every package; ADR-0016 package list; FEATURES entries; `ci_local`.
- [ ] 7.2 Propose `go-sdk/v1.0.0` to the owner with the change list; tag only after approval.
