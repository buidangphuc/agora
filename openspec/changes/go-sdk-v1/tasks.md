Code track only: every scenario gets a `not-testable` entry in `platform-core/FEATURES.yaml` (orchestrator, `## 7`). All paths below are under `platform-core/packages/go-sdk/` unless they start with `platform-core/`. Every packet: red test first, then code; `go build ./... && go vet ./... && go test ./...` in the SDK module; no new third-party module unless the packet says so; do not edit `go.mod`/`go.sum` except packet 5 (others: report a needed dependency instead).

## 1. auth, jwtx, idempotency (wave 1)

Owns: `pkg/auth/**`, `pkg/jwtx/**` (new), `pkg/idempotency/**` (new).

- [x] 1.1 Principal type constants, `RequireService`, `RequireUser`; tests `TestRequireServiceRejectsUser`, `TestRequireUserRejectsServiceAndAnonymous`. Model the claim fields on `team-identity/internal/token/jwt.go` and `team-gateway/internal/token/{jwt,jwks}.go` (read only).
- [x] 1.2 `jwtx.Claims`, `jwtx.JWKS`, `jwtx.JWK`; `TestClaimsJSONContract`. Use `github.com/golang-jwt/jwt/v5` `RegisteredClaims` only if already in the SDK module graph; otherwise plain fields with RFC 7519 names.
- [x] 1.3 `idempotency.MDKey`, `idempotency.Validate`; `TestValidateRule` (rule from `team-gateway/internal/edge/forward.go` and `team-order/internal/order/idempotency.go`).

## 2. grpcx rules (wave 1)

Owns: `pkg/grpcx/**`, `examples/**`, `platform-core/templates/_service-go/**`.

- [x] 2.1 `ErrorTable map[error]Rule`, `Rule{Code, Message}`, `Code(c)`; `TestStatusUsesRuleMessage`, `TestStatusKeepsErrorTextWithoutMessage`; update existing grpcx tests, `examples/hello` and the template (`.tmpl` sources) to the new table shape. `platform-core/tools/template_smoke_test.sh` passes.

## 3. config, pgx, pagination (wave 1)

Owns: `pkg/config/**`, `pkg/pgx/**`, `pkg/pagination/**` (new).

- [x] 3.1 `Runtime.IsStrict`, `PositiveDuration`, `keepempty`; `TestIsStrict`, `TestPositiveDuration`, `TestKeepEmpty`. `.env.example` drift check (`CheckEnvExample`) still works for `keepempty` fields.
- [x] 3.2 `pgx.OpenOrLocal`; `TestOpenOrLocalFallsBackOnlyLocally` (unreachable address, no container needed).
- [x] 3.3 `pagination`; `TestOffsetToken` plus a Clamp table test.

## 4. lifecycle, flags, resilience, upstream (wave 1)

Owns: `pkg/lifecycle/**`, `pkg/flags/**`, `pkg/resilience/**`, `pkg/upstream/**`.

- [x] 4.1 `lifecycle.Every`, `lifecycle.HTTP`; `TestEveryKeepsRunningAfterError`, `TestHTTPShutsDownGracefully`.
- [x] 4.2 `flags.NewOrDisabled`; `TestNewOrDisabledDegrades`.
- [x] 4.3 `RetryOptions.RetryIf`; `TestRetryIfOverridesDefault`.
- [x] 4.4 `upstream.WithDialOptions`; `TestDialOptionsApplied`.

## 5. messaging and sync (wave 1)

Owns: `pkg/consume/**`, `pkg/kafkax/**`, `go.mod`, `go.sum`, `platform-core/tools/sync-go-sdk.sh`, `platform-core/tools/sync-go-sdk_test.sh`.

- [x] 5.1 Port from commit `967bcc7a` (withdrawn v0.2.3; `git show 967bcc7a -- <path>`): `Dedupe.Processed`, the group-less `NewConsumerClient`, the sync `require` handling, the kadm dependency. Do not take its `openspec/` hunk. Tests `TestDedupeProcessedIsReadOnly`, `TestDedupeProcessedReturnsErrors`, `TestGroupLessConsumerClient`, `sync-go-sdk_test.sh`.
- [x] 5.2 `consume.NewBatchConsumer`, `BatchHandler`, `BatchOptions`; `TestBatchRetriesWithoutCommit`, `TestBatchPermanentGoesToDLQ`. Reuse the per-record consumer's DLQ header code. Read `team-analytics/internal/events/{listing,engagement,consumer}.go` (read only) for the loops it replaces.

## 6. testkit (wave 1)

Owns: `pkg/testkit/**`.

- [x] 6.1 `FreePort`, `Eventually`, `Recorder` (+`FailOn`); `TestRecorderFailOn`.
- [x] 6.2 `SharedPostgres`, `Schema`; `TestSchemaIsolation`.
- [x] 6.3 `Produce`, `ReadAll` over `Redpanda`; a round-trip test.

## 7. Close (orchestrator)

- [x] 7.1 Merge packets, `go mod tidy`, full SDK tests; SDK README table lists every package; ADR-0016 package list; FEATURES entries; `ci_local`.
- [ ] 7.2 Propose `go-sdk/v1.0.0` to the owner with the change list; tag only after approval.

## 8. Sync test fixture (wave 2)

Owns: `platform-core/tools/sync-go-sdk_test.sh`, `platform-core/tools/testdata/sync-fixture/**` (new).

- [x] 8.1 The sync test copies a minimal fixture service (`go.mod`, one `proto/` file with `buf.gen.yaml`, a `main.go` importing a stable SDK package) instead of `team-sharing`, so a breaking SDK change does not fail the test until a real service adopts it. Every existing assertion of the test is kept. Verify `bash platform-core/tools/sync-go-sdk_test.sh`.

## Review fixes

Findings and fixes are listed in `review.md` (ids B*, M*, m*). Every packet: a red test for each correctness finding it owns (named `Test<Behaviour>`), then the fix; replace `time.Sleep` synchronisation in the tests it owns; godoc for every changed exported identifier; SDK `go build ./... && go vet ./... && go test -race ./<owned pkgs>/...` and `go tool golangci-lint run ./...` pass. A packet may touch `examples/**` or `platform-core/templates/_service-go/**` only to keep them compiling against its own API change. `go.mod`/`go.sum`: only packet 13; others report a needed dependency.

## 9. config (wave 3)

Owns: `pkg/config/**`, `examples/hello/internal/config/**`, `examples/hello/.env.example`.

- [x] 9.1 B1: `ENV` required (no default); `IsLocal` true only for explicit `local`/`test`; tests for unset ENV.
- [x] 9.2 M8: `Load` calls `Validate()` on every nested/embedded group and joins the errors; test with `GRPC` and `Postgres` embedded together.

## 10. postgres pool (wave 3)

Owns: `pkg/pgx/**` (renamed in packet 15).

- [x] 10.1 M7 `WithTx` rolls back on panic (test with a panicking fn, pool not exhausted); m6 warm-up ctx; m7 nil logger.

## 11. outbox (wave 3)

Owns: `pkg/outbox/**`.

- [x] 11.1 B2: `Publisher` is `PublishRecord(ctx, *kgo.Record) error`; records carry headers `event_id`, `event_type`, `x-request-id` and the `traceparent` stored at enqueue; per-row nullable `topic` column (relayer topic is the default); use or drop `principal` (document which). Schema change is additive (`ADD COLUMN IF NOT EXISTS` in `SchemaSQL`, plus a note for services' migrations).
- [x] 11.2 M5 relay batch ctx shorter than the claim lock; M6 head-of-line per aggregate (test: a failing older row blocks newer rows of the same aggregate but not other aggregates); m9 reject empty EventID, `Purge(ctx, olderThan)`.

## 12. consume and kafkax (wave 3)

Owns: `pkg/consume/**`, `pkg/kafkax/**`.

- [ ] 12.1 M4 constructors return `(…, error)` and validate; m10 `NewDedupe` validates and returns an error, executor interface for `Seen`/`Processed`; m11 `Record` Partition/Timestamp/`Headers []Header`, DLQ source partition/offset/group headers.
- [ ] 12.2 M1 per-record ctx with trace and request id (span links for batches); M2 commit once per fetch with rebalance safety (test: revoke mid-fetch does not rewind or double-commit); M3 `BatchOptions.MaxAttempts` (default 5) then DLQ; M5 default `RecordDeliveryTimeout` 30s in `NewProducer`; m13 Close doc, `kafkax.Checker`.

## 13. grpcx, auth, idempotency (wave 3)

Owns: `pkg/grpcx/**`, `pkg/auth/**`, `pkg/idempotency/**`, `examples/hello/internal/rpc/**`, `platform-core/templates/_service-go/internal/rpc/**`, `go.mod`, `go.sum`.

- [ ] 13.1 M14 ordered error table; m3 default message is the sentinel text; M12 context errors map to Canceled/DeadlineExceeded without an ERROR log; m8 request-id validation.
- [ ] 13.2 m15 `type PrincipalType string` for `Principal.Type` and the constants; `idempotency.ErrInvalidKey`.

## 14. resilience, flags, lifecycle, healthx, upstream, testkit (wave 3)

Owns: `pkg/resilience/**`, `pkg/flags/**`, `pkg/lifecycle/**`, `pkg/healthx/**`, `pkg/upstream/**`, `pkg/testkit/**`.

- [ ] 14.1 M9 safe defaults and validation; M10 allow-list `Retryable`; M11 opaque `Policy`.
- [ ] 14.2 M13 evaluation context (targeting key from `auth.FromContext` by default, attributes optional); m12 rate-limited evaluation warning, `Close` shuts down only this client's provider.
- [ ] 14.3 m4 `Every` with options and interval validation, `healthx.New` rejects a non-positive interval; m18 one shared shutdown deadline; m14 default `round_robin` service config; m17 testkit Fatal in every caller under `GO_SDK_REQUIRE_DOCKER`, Ryuk documented.

## 15. renames, durations, docs and spec (wave 4)

Owns: everything under `platform-core/packages/go-sdk/**`, `platform-core/templates/_service-go/**`, `platform-core/docs/ADR/0016-go-service-convention-and-sdk.md`, `openspec/changes/go-sdk-v1/specs/**`.

- [ ] 15.1 m5 rename package `pkg/pgx` to `pkg/postgres`; m14 rename `upstream.Dial` to `upstream.NewClient`; update every SDK caller, examples and the template.
- [ ] 15.2 m1 every duration env var is a Go duration with a key without unit suffix (e.g. `SHUTDOWN_GRACE`, `OUTBOX_CLAIM_LOCK`, `FEATURE_FLAGS_EVAL_TIMEOUT`); list old → new keys in the SDK README "Upgrading to v1" section.
- [ ] 15.3 m16 evaluate moving testkit/tooling to its own module; do it only if `sync-go-sdk.sh`, `check-go-sdk-vendor.sh` and the template smoke test keep passing, else document the decision in the README.
- [ ] 15.4 m2 add MODIFIED/REMOVED requirement deltas in `specs/go-platform-sdk/spec.md` so the spec matches the code (reflection always on, no `DATABASE_ENABLED`, new signatures, ordering, headers, renames); README package table and an "Upgrading to v1" section; ADR-0016 snippets.

## 16. Close review fixes (orchestrator)

- [ ] 16.1 FEATURES entries for new scenarios; full SDK tests with `-race`, lint, template smoke, sync test, `ci_local`; re-run the staff review on the result.
