## MODIFIED Requirements

### Requirement: team-verification boots on go-sdk with the ADR-0016 layout

team-verification SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` copy, SHALL pin golangci-lint as a `tool` directive, and SHALL build `cmd/server/main.go` as a thin entry that loads `internal/config` settings and calls `internal/app.Run(ctx, settings)`. Settings SHALL be the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Audit` and `MockData`. `ENV`, `GRPC_PORT`, `DATABASE_URL` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV` (no fallback mode when the audit connection is missing). `ENV` SHALL be required and SHALL be one of `local`, `test`, `staging`, `prod`; any other value (for example `dev`) makes `Load` return an error naming `ENV`. A strict ENV is any ENV other than `local` or `test`. Settings SHALL declare no dependency enable flag: the only boolean switches are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `.env.example` SHALL list exactly the declared keys. The binary SHALL NOT import the in-memory test fake.

#### Scenario: Settings declare no dependency enable flags
- **WHEN** the declared env keys of Settings are listed
- **THEN** no key other than `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED` ends in `_ENABLED`, and `DATABASE_ENABLED` and `UPSTREAM_CALL_TIMEOUT_SECONDS` are not declared
- **VERIFIED BY**: team-verification `internal/config` `TestNoDependencyEnableFlags`

#### Scenario: ENV is required
- **WHEN** config loads with `ENV` unset, and with `ENV=dev`, then with each of `local`, `test`, `staging` and `prod` plus the other required keys
- **THEN** the first two return an error naming `ENV` and the four accepted values load; `local` and `test` are not strict, `staging` and `prod` are strict
- **VERIFIED BY**: team-verification `internal/config` `TestLoadRequiresEnv`

#### Scenario: Defaults and overrides
- **WHEN** config loads with only `ENV=local`, `GRPC_PORT=50064`, `DATABASE_URL` and `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` set, then with `GRPC_HOST=127.0.0.1`
- **THEN** the first has that audit address, `DB_MAX_CONNS` 10, reflection off, `GRPC_HOST` empty (the listener binds all interfaces, `:50064`, so containers still accept traffic), `LOG_LEVEL` `info` and `LOG_JSON` true; the second binds `127.0.0.1:50064`
- **VERIFIED BY**: team-verification `internal/config` `TestLoadDefaultsAndOverrides`

#### Scenario: Docker-backed tests run, never skip
- **WHEN** the store and app test packages run with `GO_SDK_REQUIRE_DOCKER=1` and Docker is unavailable
- **THEN** the run fails and no Docker-backed test reports a skip
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresSubmitReviewRoundTrip` (run under `GO_SDK_REQUIRE_DOCKER=1`; the gate fails on any skip)

#### Scenario: Missing database url is rejected in every environment
- **WHEN** config loads with `ENV=local` and `DATABASE_URL` empty
- **THEN** `Load` returns an error naming `DATABASE_URL`
- **VERIFIED BY**: team-verification `internal/config` `TestLoadRequiresDatabaseURL`

#### Scenario: Env example matches the settings
- **WHEN** `.env.example` is compared with the Settings keys and its local values are passed to `Load`
- **THEN** every declared key is listed, no other key appears, and `Load` accepts the file as is (it carries a working local `DATABASE_URL`, because there is no code default)
- **VERIFIED BY**: team-verification `internal/config` `TestEnvExampleInSync`

#### Scenario: Audit address is required in every environment
- **WHEN** config loads with each of `ENV=local`, `test`, `staging` and `prod`, each with `UPSTREAM_AUDIT_ADDR` empty
- **THEN** all four return an error naming `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: team-verification `internal/config` `TestLoadRequiresAuditAddr`

#### Scenario: The runtime binary does not import the test fake
- **WHEN** the dependency graph of `./cmd/server` is listed
- **THEN** it contains no package named `storetest`
- **VERIFIED BY**: team-verification `internal/app` `TestRuntimeDoesNotImportStoreFake`

#### Scenario: go.mod uses the tag without replace
- **WHEN** `go.mod` is parsed
- **THEN** it requires go-sdk exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has a golangci-lint `tool` line and no `replace` directive, and no `third_party` directory exists
- **VERIFIED BY**: team-verification `internal/app` `TestGoModRequiresSDKTagWithoutReplace` and `TestGoModDeclaresLintTool`
