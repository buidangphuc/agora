## ADDED Requirements

### Requirement: The SDK is a tagged Go module that services require

The SDK module path SHALL be `github.com/buidangphuc/agora/platform-core/packages/go-sdk`, and every import inside the SDK, its examples and the service template SHALL use it. A release SHALL be a git tag `platform-core/packages/go-sdk/vX.Y.Z`. A service SHALL obtain the SDK with a `require` on a tag and SHALL NOT use a `replace` directive or a `third_party/go-sdk` directory for it. The SDK `README.md` and `CHANGELOG.md` SHALL name the release `v1.0.1` for this change. The repository SHALL NOT contain `platform-core/tools/sync-go-sdk.sh`, `platform-core/tools/check-go-sdk-vendor.sh` or their tests.

#### Scenario: Module path and imports use the fetchable path
- **WHEN** the SDK `go.mod` and every `.go` file under `platform-core/packages/go-sdk` are read
- **THEN** the module line is `github.com/buidangphuc/agora/platform-core/packages/go-sdk` and no file imports `github.com/buidangphuc/platform-core/`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestModulePathIsFetchable`

#### Scenario: Changelog names the release
- **WHEN** the first release heading of `CHANGELOG.md` and the version line of `README.md` are read
- **THEN** both are `v1.0.1`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestReleaseVersionIsDocumented`

#### Scenario: The service template requires a tag and does not vendor
- **WHEN** `platform-core/templates/_service-go/go.mod` is read
- **THEN** it has a `require` for the go-sdk module, does not contain a `replace` for it, and the template has no `third_party` directory
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestTemplateRequiresTagWithoutReplace`

#### Scenario: Vendoring tooling is gone
- **WHEN** the files `platform-core/tools/sync-go-sdk.sh`, `sync-go-sdk_test.sh`, `check-go-sdk-vendor.sh` and `check-go-sdk-vendor_test.sh` are looked up
- **THEN** none exists and no workflow or `ci_local.sh` references them
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestVendoringToolingIsRemoved`

### Requirement: Services share the make and lint files by reference

`platform-core/make/go-service.mk` and `platform-core/packages/go-sdk/.golangci.yml` SHALL be the only copies of the shared make rules and lint configuration. A service SHALL reference them by relative path (`include ../platform-core/make/go-service.mk`; `golangci-lint run --config ../platform-core/packages/go-sdk/.golangci.yml`) and SHALL NOT hold copies. The shared `.golangci.yml` SHALL NOT contain a `service-local` block; a service-specific exclusion SHALL be a rule scoped by `path: team-<service>/...`. The goimports `local-prefixes` SHALL be `github.com/buidangphuc`.

#### Scenario: Shared lint file has no service-local block and one prefix
- **WHEN** `.golangci.yml` is parsed
- **THEN** it has no `BEGIN service-local` marker, and `local-prefixes` equals `github.com/buidangphuc`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestSharedLintConfigIsServiceNeutral`

#### Scenario: Service exclusions are path-scoped
- **WHEN** every exclusion rule of `.golangci.yml` that names a service is read
- **THEN** each carries a `path` that starts with `team-`, and a rule without a `path` does not name a service
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestServiceExclusionsArePathScoped`

#### Scenario: The template references the shared files
- **WHEN** the template `Makefile` is read
- **THEN** it includes `../platform-core/make/go-service.mk`, passes the shared lint config path, and the template directory has no `.golangci.yml` and no copy of `go-service.mk`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestTemplateReferencesSharedMakeAndLint`

### Requirement: The SDK has no dependency enable flags

The SDK SHALL NOT read an environment variable that switches a dependency on or off. Postgres, Kafka, the outbox, feature flags and consumers SHALL be always on when a service uses their package: `config.Postgres.Validate()` SHALL reject an empty `DATABASE_URL`, `config.Kafka` SHALL require `KAFKA_BROKERS` when `Runtime.IsStrict()`, and no `config` group SHALL declare `DATABASE_ENABLED`, `KAFKA_ENABLED`, `OUTBOX_ENABLED`, `FEATURE_FLAGS_ENABLED` or a consumer enable variable. `OTEL_ENABLED` SHALL remain, as the exporter switch.

#### Scenario: No config group declares a dependency enable variable
- **WHEN** every `env` tag of every group in `pkg/config` is listed
- **THEN** the only tag ending in `_ENABLED` is `OTEL_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestNoDependencyEnableFlags`

#### Scenario: Removed flags are ignored, not honoured
- **WHEN** settings embedding `Runtime` and `Postgres` load with `ENV=local`, `DATABASE_ENABLED=false`, `KAFKA_ENABLED=false` and no `DATABASE_URL`
- **THEN** `Load` returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadIgnoresRemovedEnableFlags`

#### Scenario: No package source reads an enable flag
- **WHEN** the string literals of every non-test `.go` file under `pkg/` are scanned for names ending in `_ENABLED`
- **THEN** the only match is `OTEL_ENABLED`
- **VERIFIED BY**: `platform-core/packages/go-sdk` `TestNoEnableFlagInSDKSource`

### Requirement: Mock data replaces only the outer adapter and never reaches a strict environment

The SDK SHALL provide the settings group `config.MockData` with variable `MOCK_DATA`, a comma-separated list of adapter names, and the package `pkg/mockdata`. `mockdata.New(cfg config.MockData, declared ...string) (*Set, error)` SHALL accept only names the service declared (compared case-insensitively after trimming, empty entries ignored) and SHALL return an error naming `MOCK_DATA` and the allowed names for an unknown name. `config.Load` SHALL refuse a non-empty `MOCK_DATA` when `Runtime.IsStrict()` with an error naming `MOCK_DATA`, from the same resolve step that runs before any group `Validate`, so no other validation error masks it. `Set.LogActive(logger)` SHALL log the active adapter names once per `Set` at boot and log nothing when none is active. `mockdata.Select(set, name, real, fixture)` SHALL return `fixture` when `name` is active and `real` otherwise. A mock adapter SHALL replace only the declared outermost adapter (payment provider, LLM backend, shipping or ETA provider); handlers, persistence, events and audit SHALL still run.

#### Scenario: Strict ENV refuses MOCK_DATA before other validation
- **WHEN** `ENV=prod`, `MOCK_DATA=payment-provider` and no `DATABASE_URL` is set for settings that embed `Runtime`, `Postgres` and `MockData`
- **THEN** `Load` returns an error naming `MOCK_DATA` and that error does not name `DATABASE_URL`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestLoadRefusesMockDataInStrictEnvBeforeValidation`

#### Scenario: Every strict ENV spelling is refused
- **WHEN** `MOCK_DATA=payment-provider` is set with `ENV` equal to `staging`, `stage`, `prod`, `production` and `PROD`
- **THEN** each `Load` returns an error naming `MOCK_DATA`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestMockDataRefusedForEveryStrictSpelling`

#### Scenario: Local and test ENV accept MOCK_DATA, unset and empty are accepted everywhere
- **WHEN** `MOCK_DATA=payment-provider` is set with `ENV=local` and `ENV=test`, and `MOCK_DATA` is unset or empty with `ENV=prod`
- **THEN** every `Load` succeeds
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestMockDataAllowedWhenLocalOrEmpty`

#### Scenario: Unknown adapter name is refused
- **WHEN** `mockdata.New` receives `MOCK_DATA=payment-provider,telepathy` and the declared names `payment-provider` and `eta-provider`
- **THEN** it returns an error naming `MOCK_DATA`, `telepathy` and the declared names
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestNewRejectsUnknownAdapter`

#### Scenario: Names are trimmed, de-duplicated and case-insensitive
- **WHEN** `mockdata.New` receives ` Payment-Provider ,payment-provider,,` with `payment-provider` declared
- **THEN** `Set.Active("payment-provider")` is true and `Set.Adapters()` is `["payment-provider"]`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestNewNormalisesNames`

#### Scenario: Active adapters are logged once
- **WHEN** `LogActive` is called twice on a `Set` with `payment-provider` active, and once on a `Set` with none
- **THEN** exactly one record is logged for the first set, listing `payment-provider`, and none for the second
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestLogActiveOncePerSet`

#### Scenario: Select swaps only the named adapter
- **WHEN** `Select` runs for `payment-provider` active and for `eta-provider` inactive
- **THEN** the first returns the fixture and the second returns the real adapter
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/mockdata` `TestSelectSwapsOnlyActiveAdapter`

### Requirement: Privileged actions are audit-first and fail closed

`pkg/auditx` SHALL provide `config.Audit` with variable `UPSTREAM_AUDIT_ADDR`, which `Load` SHALL require (non-empty) when `Runtime.IsStrict()` and leave empty otherwise. `auditx.Dial(cfg config.Audit, serviceID string, scopes ...string) (*grpc.ClientConn, error)` SHALL return a lazy connection whose calls carry a service principal (`upstream.AsService`), and SHALL return a nil connection and no error when the address is empty. The SDK SHALL define `auditx.Event{ActorID, Action, TargetType, TargetID string; Metadata map[string]string}` and the interface `auditx.Writer` with `WriteAuditEvent(ctx, Event) error`, which a service implements over the generated `platform.audit.v1.AuditService.WriteAuditEvent` client (design.md D2). `auditx.NewGate(w Writer, logger *slog.Logger) *Gate` and `Gate.Do(ctx, ev, action func(context.Context) error) error` SHALL bound the write by the earlier of the caller deadline and `upstream.DefaultTimeout` (2s), write the event first and call `action` only when the write succeeded. When `w` is nil or the write returns an error, `Do` SHALL NOT call `action`, SHALL log only `action`, `target_type` and, for a write error, its gRPC status code (`status.Code(err)`), never `target_id` or the error text, and SHALL return a gRPC status with code `Unavailable` and message `audit trail unavailable`, which never contains the cause. An error returned by `action` SHALL be returned unchanged.

#### Scenario: A successful audit write lets the action run once
- **WHEN** `Gate.Do` is called with a writer that succeeds
- **THEN** the writer saw the event before `action` ran and `action` ran exactly once
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoWritesBeforeAction`

#### Scenario: A failed audit write refuses the action
- **WHEN** `Gate.Do` is called with a writer that returns `errors.New("dial tcp 10.0.0.9: refused")`
- **THEN** `action` ran zero times, the code is `Unavailable`, the message is `audit trail unavailable` and does not contain `10.0.0.9`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedOnWriteError`

#### Scenario: A failed audit write logs no ids or error text
- **WHEN** `Gate.Do` is called with target id `listing-8f3a9c`, once with a writer that returns `status.Error(codes.Unavailable, "dial tcp 10.0.0.9: refused")` and once on a gate with a nil writer
- **THEN** both log records carry `action` and `target_type`, the first also carries code `Unavailable`, and neither contains `listing-8f3a9c`, `10.0.0.9` or the error text
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailureLogsNoTargetOrErrorText`

#### Scenario: A missing writer refuses the action
- **WHEN** `Gate.Do` is called on a gate built with a nil writer
- **THEN** `action` ran zero times and the status is `Unavailable` with message `audit trail unavailable`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedWithoutWriter`

#### Scenario: A timed-out audit write refuses the action
- **WHEN** the writer blocks until the context deadline of 50ms expires
- **THEN** `Do` returns `Unavailable` within 500ms and `action` ran zero times
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoFailsClosedOnTimeout`

#### Scenario: Without a caller deadline the write is bounded by 2s
- **WHEN** the context has no deadline and the writer blocks until its context ends
- **THEN** `Do` returns `Unavailable` within 2.5s and `action` ran zero times
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoBoundsWriteWithoutCallerDeadline`

#### Scenario: An action error is returned as is
- **WHEN** the audit write succeeds and `action` returns `status.Error(codes.PermissionDenied, "missing scope")`
- **THEN** `Do` returns that error with code `PermissionDenied`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDoReturnsActionErrorUnchanged`

#### Scenario: Strict ENV requires the audit address
- **WHEN** settings embedding `Runtime` and `config.Audit` load with `ENV=prod` and `UPSTREAM_AUDIT_ADDR` unset or empty, and again with `ENV=local`
- **THEN** the first two `Load` calls return an error naming `UPSTREAM_AUDIT_ADDR` and the `local` one succeeds with an empty address
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestStrictEnvRequiresAuditAddr`

#### Scenario: Dial attaches a service principal
- **WHEN** a client made with `auditx.Dial(cfg, "team-order", "audit.write")` calls a test server
- **THEN** the server sees principal type `service`, id `team-order` and scope `audit.write`, and a dial with an empty address returns a nil connection and no error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/auditx` `TestDialSendsServicePrincipal`

### Requirement: Kafka consumers log a keyed key fingerprint, never the raw key

`config.LogKeyFP` SHALL read `LOG_KEY_FP_SECRET`; `Load` SHALL require it non-empty when `Runtime.IsStrict()` and otherwise fill a documented local default. `kafkax.NewFingerprinter(secret string) (*Fingerprinter, error)` SHALL reject an empty secret, and `Fingerprinter.Of(key []byte) string` SHALL return the first 12 lowercase hex characters of HMAC-SHA256 of `key` under the secret. `consume.Options` and `consume.BatchOptions` SHALL take an optional `KeyFP *kafkax.Fingerprinter`; when nil, a fingerprinter with a random per-process secret SHALL be used, never an unkeyed hash. Every log record that `consume` writes about a consumed record, a retry, a dead-lettering and a failed dead-letter write SHALL carry attribute `key_fp`, and SHALL NOT carry the raw key or the value in any attribute or message text. The record key bytes in the DLQ record and the offset and retry behaviour SHALL be unchanged. Wherever `consume` writes handler error text to a log record or to the `dlq-error` header, it SHALL replace every occurrence of the raw key text by `[key_fp:<fp>]`.

#### Scenario: Handler failure logs the fingerprint and not the key
- **WHEN** the handler fails on attempt 1 of 3 for key `seller-123` and the fingerprinter secret is `s3cret`
- **THEN** the log record carries `key_fp` equal to the first 12 hex of HMAC-SHA256(`s3cret`, `seller-123`) and the whole captured output does not contain `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestHandlerFailureLogsKeyFingerprint`

#### Scenario: Dead-letter logs carry the fingerprint and the DLQ key is unchanged
- **WHEN** the handler fails all attempts for key `seller-123` and the DLQ publish succeeds
- **THEN** the "record dead-lettered" log carries `key_fp` and no `key` attribute, and the DLQ record key equals the bytes `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestDeadLetterLogsFingerprintKeepsDLQKey`

#### Scenario: A failed dead-letter write still logs only the fingerprint
- **WHEN** the handler exhausts its attempts and the DLQ publish fails
- **THEN** the returned error and every log record do not contain `seller-123`, and the offset is not committed
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestDeadLetterFailureLogsFingerprintOffsetHeld`

#### Scenario: Error text embedding the key is scrubbed in logs and the DLQ header
- **WHEN** the handler returns `fmt.Errorf("user seller-123: %w", errBoom)` for key `seller-123` and the record is dead-lettered
- **THEN** the log `err` attribute and the `dlq-error` header equal `user [key_fp:<fp>]: boom` and contain no `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestErrorTextIsScrubbedOfKey`

#### Scenario: The batch consumer scrubs every key of the batch
- **WHEN** a batch handler fails with error text containing the keys `a-1` and `b-2` of two records
- **THEN** the logged text contains neither key and carries one `key_fp` per record
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestBatchLogsFingerprintsOnly`

#### Scenario: Fingerprints are 12 hex, stable per key and different across secrets
- **WHEN** `Of` runs twice for one key, once for another key (also a 200-byte key and a nil key), and once for the first key under a second secret
- **THEN** every result matches `^[0-9a-f]{12}$`, the first two are equal, the others differ, and the nil key does not panic
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestFingerprintFormatStableAndKeyed`

#### Scenario: An empty secret is refused
- **WHEN** `NewFingerprinter("")` runs
- **THEN** it returns an error
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/kafkax` `TestNewFingerprinterRejectsEmptySecret`

#### Scenario: Strict ENV requires the secret and local has a default
- **WHEN** settings embedding `Runtime` and `config.LogKeyFP` load with `ENV=prod` and `LOG_KEY_FP_SECRET` unset or empty, and with `ENV=local`
- **THEN** the first two `Load` calls return an error naming `LOG_KEY_FP_SECRET` and `local` succeeds with a non-empty secret
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/config` `TestStrictEnvRequiresLogKeyFPSecret`

#### Scenario: Without a configured fingerprinter the key is still not logged
- **WHEN** a consumer is built with `KeyFP` nil and the handler fails for key `seller-123`
- **THEN** the log carries a 12-hex `key_fp`, not an unkeyed SHA-256 of the key, and no `seller-123`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/consume` `TestNilFingerprinterUsesRandomSecret`

### Requirement: Unique violations map to AlreadyExists

`grpcx.Status` SHALL map an error whose chain contains a Postgres error with SQLSTATE `23505` (unique violation) to `codes.AlreadyExists` with the fixed message `already exists`. A matching `ErrorTable` rule SHALL take precedence over this mapping, an error that already carries a gRPC status SHALL pass through first, and any other SQLSTATE SHALL keep becoming `Internal` with `internal error`. The constraint name, column and row detail SHALL be logged and SHALL NOT reach the caller.

#### Scenario: A unique violation becomes AlreadyExists
- **WHEN** `grpcx.Status` receives `fmt.Errorf("insert voucher: %w", &pgconn.PgError{Code: "23505", ConstraintName: "vouchers_code_key"})`
- **THEN** the code is `AlreadyExists` and the message is `already exists`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusMapsUniqueViolationToAlreadyExists`

#### Scenario: Constraint detail does not reach the caller
- **WHEN** the `PgError` carries `Detail: "Key (code)=(SAVE10) already exists."`
- **THEN** the status message does not contain `SAVE10` or `vouchers_code_key`, and the log record contains the constraint name
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusUniqueViolationHidesDetail`

#### Scenario: A service rule wins over the built-in mapping
- **WHEN** the error wraps both a table sentinel with `Rule{Code: FailedPrecondition}` and a 23505 `PgError`
- **THEN** the code is `FailedPrecondition`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusRuleBeatsUniqueViolation`

#### Scenario: Other database errors stay internal
- **WHEN** the error wraps a `PgError` with SQLSTATE `23503` (foreign key violation)
- **THEN** the code is `Internal` and the message is `internal error`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusOtherPgErrorsStayInternal`

#### Scenario: An existing gRPC status passes through before the mapping
- **WHEN** the error is `status.Error(codes.PermissionDenied, "missing scope")`
- **THEN** the code stays `PermissionDenied` and the message stays `missing scope`
- **VERIFIED BY**: `platform-core/packages/go-sdk/pkg/grpcx` `TestStatusGRPCStatusBeatsUniqueViolation`

## MODIFIED Requirements

### Requirement: The Go service template follows ADR-0016

`platform-core/templates/_service-go` SHALL mirror `examples/hello`: the layout, `internal/app` wiring, a `Makefile` that includes `../platform-core/make/go-service.mk`, `.env.example`, and a README with the rename and `go get` steps. Its `go.mod` SHALL `require` the SDK at a tag, without a `replace` and without a `third_party` directory. It SHALL NOT contain its own interceptors, platform code, `.golangci.yml` or copy of `go-service.mk`.

#### Scenario: A service scaffolded from the template builds without platform copies
- **WHEN** the template is copied to a scratch dir, renamed, and its SDK `require` resolved to the current tag
- **THEN** `go build ./...` and `go vet ./...` pass and no `internal/interceptor`, `internal/grpcserver` or `third_party` exists
- **VERIFIED BY**: `platform-core/tools/template_smoke_test.sh`

## REMOVED Requirements

### Requirement: Services vendor a pinned SDK copy
**Reason**: The module is now fetchable (`github.com/buidangphuc/agora/platform-core/packages/go-sdk`) and released by tag, so copying it into `third_party/go-sdk` and rewriting `go.mod` with a `replace` has no purpose.
**Migration**: A service runs `go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`, deletes `third_party/go-sdk` and the `replace`, and references the shared make and lint files (requirement "Services share the make and lint files by reference"). `sync-go-sdk.sh` and `check-go-sdk-vendor.sh` are deleted with their tests.

### Requirement: Sync prepares a buildable, correctly linted service
**Reason**: It describes `sync-go-sdk.sh` behaviour (buf generate, goimports rewrite); the script is removed because services no longer vendor the SDK.
**Migration**: Services `require` a tag (`go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`) and reference the shared make and lint files (requirement "Services share the make and lint files by reference"); the script and its test are deleted.

### Requirement: Sync keeps service-local lint overrides
**Reason**: The `service-local` block no longer exists: service exclusions are path-scoped rules in the one shared `.golangci.yml`.
**Migration**: Services `require` a tag (`go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`) and reference the shared make and lint files (requirement "Services share the make and lint files by reference"); the script and its test are deleted.

### Requirement: Vendored SDK copies match their tag
**Reason**: There are no vendored copies, so no drift check; `check-go-sdk-vendor.sh` is removed.
**Migration**: Services `require` a tag (`go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`) and reference the shared make and lint files (requirement "Services share the make and lint files by reference"); the script and its test are deleted.

### Requirement: Sync keeps the go-sdk requirement
**Reason**: No sync step and no `replace`; the service `go.mod` holds a plain `require` on a tag.
**Migration**: Services `require` a tag (`go get github.com/buidangphuc/agora/platform-core/packages/go-sdk@v1.0.1`) and reference the shared make and lint files (requirement "Services share the make and lint files by reference"); the script and its test are deleted.
