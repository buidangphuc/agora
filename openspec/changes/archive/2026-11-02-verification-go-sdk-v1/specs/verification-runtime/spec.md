## ADDED Requirements

### Requirement: team-verification boots on go-sdk with the ADR-0016 layout

team-verification SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at the tag `v1.0.2` with no `replace` directive and no `third_party` copy, and SHALL build `cmd/server/main.go` as a thin entry that loads `internal/config` settings and calls `internal/app.Run(ctx, settings)`. Settings SHALL be the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Audit` and `MockData`. `ENV`, `GRPC_PORT`, `DATABASE_URL` and `UPSTREAM_AUDIT_ADDR` SHALL be required in every `ENV` (no fallback mode when the audit connection is missing). `ENV` SHALL be required and SHALL be one of `local`, `test`, `staging`, `prod`; any other value (for example `dev`) makes `Load` return an error naming `ENV`. A strict ENV is any ENV other than `local` or `test`. Settings SHALL declare no dependency enable flag: the only boolean switches are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`. `.env.example` SHALL list exactly the declared keys. The binary SHALL NOT import the in-memory test fake.

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
- **THEN** it requires go-sdk `v1.0.2` and has no `replace` directive and no `third_party` directory exists
- **VERIFIED BY**: team-verification `internal/app` `TestGoModRequiresSDKTagWithoutReplace`

### Requirement: The database is mandatory and boot fails without it

`app.Run` SHALL open Postgres with `postgres.Open` in every `ENV`, including `local`, and SHALL return an error before the gRPC port opens when the database is unreachable. `app.Run` SHALL NOT apply migrations; the binary's `migrate up` subcommand (SDK `pkg/migrate`, embedded `migrations.Files`) applies them, run by the compose migrate service and the gitops PreSync Job. Readiness SHALL NOT depend on Kafka or any upstream. There SHALL be no in-memory repository on the runtime path. The Postgres store SHALL keep the semantics of the former repository: a review of a non-PENDING submission is `ErrAlreadyReviewed` and is final, an unknown id is `ErrNotFound`, and the newest submission per submitter is returned for status.

#### Scenario: An unreachable database fails boot locally
- **WHEN** `Run` starts with `ENV=local` and `DATABASE_URL` pointing at a closed port
- **THEN** `Run` returns an error within 10 s and the configured gRPC port accepts no connection
- **VERIFIED BY**: team-verification `internal/app` `TestRunFailsWhenDatabaseUnreachable`

#### Scenario: migrate up applies the schema and boot never migrates
- **WHEN** the binary runs `migrate up` against an empty Postgres, and separately `Run` starts against an unmigrated Postgres
- **THEN** `migrate up` creates the KYC tables and exits 0, and `Run` creates no table
- **VERIFIED BY**: team-verification `internal/app` `TestMigrateUpSubcommandAppliesSchema` and `TestRunDoesNotMigrate`

#### Scenario: Submit and review round trip on a real database
- **WHEN** user "u-1" submits KYC and an admin reviews it as approve against a migrated Postgres
- **THEN** the store returns VERIFIED with `reviewed_at` set and GetLatestByUser returns the same row
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresSubmitReviewRoundTrip`

#### Scenario: A decision is final in the store
- **WHEN** two reviews target the same PENDING submission, the second after the first
- **THEN** the first returns the new status and the second returns `ErrAlreadyReviewed` leaving the first decision stored
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresUpdateStatusIsFinal`

#### Scenario: Unknown submission in the store
- **WHEN** the store is asked for id "kyc_missing" and for the latest of a user who never submitted
- **THEN** both return `ErrNotFound`
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresNotFound`

#### Scenario: One user's status is not another's
- **WHEN** user "u-1" has a submission and the store is asked for the latest of user "u-2"
- **THEN** it returns `ErrNotFound` for "u-2" and the row of "u-1" is untouched
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresStatusIsPerUser`

#### Scenario: Status is the latest submission by created_at
- **WHEN** user "u-1" has two submissions created at t1 and t2 with t2 later than t1 and the store is asked for the latest of "u-1"
- **THEN** it returns the submission created at t2, whatever the insertion order
- **VERIFIED BY**: team-verification `internal/store` `TestPostgresLatestByCreatedAt`

#### Scenario: A user with no submission reports PENDING without a badge
- **WHEN** GetVerificationStatus runs for a user who never submitted
- **THEN** the response status is PENDING and the badge is not granted
- **VERIFIED BY**: team-verification `internal/verification` `TestGetStatusUnknownUserReturnsPending`

#### Scenario: Duplicate submission id is AlreadyExists
- **WHEN** the store inserts two submissions with the same id and the error goes through the rpc error table
- **THEN** the caller receives `AlreadyExists` and the message does not contain the id
- **VERIFIED BY**: team-verification `internal/rpc` `TestDuplicateIDIsAlreadyExists`

### Requirement: Health reflects the database and liveness does not

team-verification SHALL register a `postgres` health checker so the overall (readiness) status is SERVING only while the database answers, and SHALL keep the `liveness` health service SERVING while the database is down. On shutdown `Run` SHALL drain health before it stops the gRPC server and SHALL return nil within `SHUTDOWN_GRACE` when its context ends.

#### Scenario: Healthy database serves
- **WHEN** `Run` is up with a reachable database
- **THEN** the overall status and the `liveness` service are both SERVING within one health interval
- **VERIFIED BY**: team-verification `internal/app` `TestHealthServingWhenDatabaseUp`

#### Scenario: Readiness fails when the database is lost, liveness stays
- **WHEN** the database stops answering while `Run` is up
- **THEN** within one health interval the overall status is NOT_SERVING and the `liveness` service is still SERVING
- **VERIFIED BY**: team-verification `internal/app` `TestReadinessFollowsDatabaseLivenessIndependent`

#### Scenario: Context cancel shuts down within the grace
- **WHEN** the context passed to `Run` is cancelled with `SHUTDOWN_GRACE=2s`
- **THEN** `Run` returns nil within 2 s and the listener is closed
- **VERIFIED BY**: team-verification `internal/app` `TestRunShutsDownWithinGrace`

### Requirement: gRPC reflection and mock data are local-only and off by default

Reflection SHALL be served only when `GRPC_REFLECTION_ENABLED=true`, default false, and `Load` SHALL refuse a true value in a strict ENV. `MOCK_DATA` SHALL be refused by `Load` in a strict ENV before any group validation. team-verification has no mockable adapter, so a non-empty `MOCK_DATA` locally SHALL fail boot as an unknown adapter; no mock path skips persistence or audit.

#### Scenario: Reflection is off by default
- **WHEN** `Run` starts with `ENV=local` and no `GRPC_REFLECTION_ENABLED`
- **THEN** a reflection client call to the server returns `Unimplemented`
- **VERIFIED BY**: team-verification `internal/app` `TestReflectionOffByDefault`

#### Scenario: Reflection is refused in a strict environment
- **WHEN** config loads with `ENV=prod` and then `ENV=staging`, each with `GRPC_REFLECTION_ENABLED=true`
- **THEN** both return an error naming `GRPC_REFLECTION_ENABLED`; with `ENV=local` or `ENV=test` the value loads
- **VERIFIED BY**: team-verification `internal/config` `TestStrictEnvRefusesReflection`

#### Scenario: MOCK_DATA is refused in a strict environment before validation
- **WHEN** config loads with `ENV=staging`, `MOCK_DATA=kyc` and an empty `DATABASE_URL`
- **THEN** the returned error names `MOCK_DATA`
- **VERIFIED BY**: team-verification `internal/config` `TestStrictEnvRefusesMockDataFirst`

#### Scenario: An unknown mock adapter fails boot
- **WHEN** `Run` starts with `ENV=local` and `MOCK_DATA=kyc`
- **THEN** `Run` returns an error naming the unknown adapter before it opens the database
- **VERIFIED BY**: team-verification `internal/app` `TestUnknownMockAdapterFailsBoot`

### Requirement: Errors, auth and logs go through the SDK without leaking data

Handlers SHALL authorize with SDK `auth` and map errors with one `grpcx.ErrorTable`; a panicking handler SHALL return `Internal` and the server SHALL keep serving. An error that is not in the table SHALL be sent as `Internal` "internal error" with the cause only in the log. `doc_type` and `doc_ref` SHALL NOT appear in any log line or error string. Principal rules are unchanged, and the one check the SDK does not make stays in `internal/rpc`: a type guard that runs first in every handler and returns `Unauthenticated` when the principal type is anything other than `user`, `service` or `anonymous` (empty or unrecognised), whatever scopes it carries. The SDK takes the type as a raw string from `x-principal-type` and only treats an empty id, the id `anonymous` or the type `anonymous` as anonymous, so a forwarded type such as `robot` would otherwise pass `auth.RequirePrincipal` and `auth.RequireScopes`; the old interceptor mapped every unknown string to unspecified and refused it, and that fail-closed behaviour is kept. SubmitKyc needs a user principal, an anonymous, identity-less, typeless or unrecognised-type caller is `Unauthenticated`, a service principal is `PermissionDenied` for user-owned RPCs. GetVerificationStatus with an empty or own `user_id` needs a user principal (the type check stays in the handler, because `auth.RequirePrincipal` also accepts a service principal); another user's status needs the `admin` scope.

#### Scenario: Domain errors map to their codes
- **WHEN** the handler returns each of empty doc type, empty doc ref, empty id, invalid decision, unknown id and already reviewed
- **THEN** the codes are InvalidArgument for the first four, NotFound and FailedPrecondition for the last two
- **VERIFIED BY**: team-verification `internal/rpc` `TestErrorTableMapsDomainErrors`

#### Scenario: Internal cause is hidden
- **WHEN** the repository fails with an error whose text contains "password=secret"
- **THEN** the caller receives `Internal` with message "internal error" and the text does not appear in the response
- **VERIFIED BY**: team-verification `internal/rpc` `TestMapErrHidesInternalCause`

#### Scenario: A panic does not stop the server
- **WHEN** a handler panics and a second call follows
- **THEN** the first returns `Internal` and the second succeeds
- **VERIFIED BY**: team-verification `internal/rpc` `TestPanicReturnsInternalAndKeepsServing`

#### Scenario: Anonymous and service callers are refused
- **WHEN** an anonymous principal, no principal, and a service principal call SubmitKyc
- **THEN** the first two return `Unauthenticated` and the service principal returns `PermissionDenied`
- **VERIFIED BY**: team-verification `internal/rpc` `TestAnonymousAndNoPrincipalAreUnauthenticated`

#### Scenario: A principal with no type is refused
- **WHEN** a principal with a non-empty id, the `admin` scope and an unspecified type calls SubmitKyc, ReviewKyc and GetVerificationStatus for another user
- **THEN** all three return `Unauthenticated`, no audit write is made and no submission changes
- **VERIFIED BY**: team-verification `internal/rpc` `TestTypelessPrincipalUnauthenticated`

#### Scenario: A principal with an unrecognised type is refused
- **WHEN** a principal with a non-empty id, the `admin` scope and `x-principal-type=robot` calls SubmitKyc, ReviewKyc and GetVerificationStatus for another user
- **THEN** all three return `Unauthenticated`, no audit write is made and no submission changes
- **VERIFIED BY**: team-verification `internal/rpc` `TestUnknownTypePrincipalUnauthenticated`

#### Scenario: Service principal cannot read own status
- **WHEN** a service principal calls GetVerificationStatus with an empty `user_id`
- **THEN** the call returns `PermissionDenied`
- **VERIFIED BY**: team-verification `internal/rpc` `TestStatusServicePrincipalDenied`

#### Scenario: Non-admin cannot read another user's status
- **WHEN** user "u-2" without the `admin` scope calls GetVerificationStatus for "u-1"
- **THEN** the call returns `PermissionDenied` and the response carries no status
- **VERIFIED BY**: team-verification `internal/rpc` `TestStatusOtherUserNeedsAdmin`

#### Scenario: Admin can read another user's status
- **WHEN** an admin with the `admin` scope calls GetVerificationStatus for "u-1" who has a PENDING submission
- **THEN** the call returns PENDING for "u-1"
- **VERIFIED BY**: team-verification `internal/rpc` `TestStatusAdminReadsOtherUser`

#### Scenario: Anonymous caller cannot read a status
- **WHEN** an anonymous principal and a call with no principal request GetVerificationStatus
- **THEN** both return `Unauthenticated`
- **VERIFIED BY**: team-verification `internal/rpc` `TestStatusAnonymousUnauthenticated`

#### Scenario: A buyer cannot review
- **WHEN** a user without the `admin` scope calls ReviewKyc
- **THEN** the call returns `PermissionDenied`
- **VERIFIED BY**: team-verification `internal/rpc` `TestBuyerCannotReview`

#### Scenario: Submit then review through the handler
- **WHEN** a buyer submits KYC, an admin approves it, and the buyer reads the status
- **THEN** the status is VERIFIED with the badge granted
- **VERIFIED BY**: team-verification `internal/rpc` `TestHandlerSubmitReviewFlow`

#### Scenario: Two users do not see each other's status
- **WHEN** users "alice" and "bob" each submit and each reads their own status
- **THEN** each response carries only the caller's own submission
- **VERIFIED BY**: team-verification `internal/rpc` `TestTwoUsersDoNotSeeEachOthersStatus`

#### Scenario: A service principal cannot submit
- **WHEN** a service principal calls SubmitKyc
- **THEN** the call returns `PermissionDenied` and no submission is stored
- **VERIFIED BY**: team-verification `internal/rpc` `TestServicePrincipalCannotSubmit`

#### Scenario: An admin can review someone else
- **WHEN** an admin with the `admin` scope reviews a buyer's PENDING submission
- **THEN** the call succeeds and the status is VERIFIED
- **VERIFIED BY**: team-verification `internal/rpc` `TestAdminCanReviewSomeoneElse`

#### Scenario: A review decision is final through the handler
- **WHEN** admin "admin_1" approves a PENDING submission and admin "admin_2" then rejects it
- **THEN** the second call returns `FailedPrecondition` and the owner's status is still VERIFIED
- **VERIFIED BY**: team-verification `internal/rpc` `TestReviewIsFinal`

#### Scenario: Document references never reach the logs
- **WHEN** a SubmitKyc with doc_ref "secret-doc-ref" fails in the repository and the logger output is captured
- **THEN** the captured log contains no "secret-doc-ref" and no doc_type value
- **VERIFIED BY**: team-verification `internal/rpc` `TestDocRefNeverLogged`
