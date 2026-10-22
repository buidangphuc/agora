## ADDED Requirements

### Requirement: Settings come from SDK groups and the service has no dependency enable flags

team-referral SHALL load its settings with `config.Load` over the SDK groups `Runtime`, `GRPC`, `Postgres` and `OTel`. `ENV`, `GRPC_PORT` and `DATABASE_URL` SHALL be required and have no default; `Load` SHALL return one error naming every one of them that is unset. The service SHALL NOT declare or read `DATABASE_ENABLED`, or any other `*_ENABLED` variable except `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`; reflection SHALL be off unless `GRPC_REFLECTION_ENABLED=true`. `team-referral/.env.example` SHALL contain exactly the keys declared by `Settings`.

#### Scenario: Missing database URL fails the load
- **WHEN** settings load with `ENV=local`, `GRPC_PORT=50062` and no `DATABASE_URL`
- **THEN** `Load` returns an error whose text names `DATABASE_URL`
- **VERIFIED BY**: `team-referral/internal/config` `TestLoadRequiresDatabaseURL`

#### Scenario: Missing ENV or port fails the load
- **WHEN** settings load with a valid `DATABASE_URL` and neither `ENV` nor `GRPC_PORT` set
- **THEN** `Load` returns one error that names both `ENV` and `GRPC_PORT`
- **VERIFIED BY**: `team-referral/internal/config` `TestLoadRequiresEnvAndPort`

#### Scenario: A removed enable flag is ignored, not honoured
- **WHEN** settings load with `ENV=local`, `DATABASE_ENABLED=false` and no `DATABASE_URL`
- **THEN** `Load` returns an error that names `DATABASE_URL`
- **VERIFIED BY**: `team-referral/internal/config` `TestLoadIgnoresRemovedEnableFlags`

#### Scenario: No enable flag is declared
- **WHEN** the env keys declared by `Settings` are listed
- **THEN** the only keys ending in `_ENABLED` are `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`
- **VERIFIED BY**: `team-referral/internal/config` `TestNoEnableFlagsDeclared`

#### Scenario: The env example matches the settings
- **WHEN** `.env.example` is compared with the keys declared by `Settings`
- **THEN** the two key sets are equal and `DATABASE_ENABLED` is absent from the file
- **VERIFIED BY**: `team-referral/internal/config` `TestEnvExampleInSync`

### Requirement: The service always uses Postgres and never serves from memory

team-referral SHALL open Postgres with `postgres.Open` during boot, in every `ENV` including `local` and `test`, and SHALL return an error from `app.Run` before opening the gRPC listener when the database cannot be reached. The production binary (`cmd/server` and `internal/app`) SHALL NOT import an in-memory store; the in-memory repository SHALL live under `internal/store/fake` and be imported by tests only. The readiness health service (empty name) SHALL report `NOT_SERVING` within 10 seconds (2 x `healthx.DefaultInterval` of 5 s) after the database stops answering and `SERVING` while it answers; the `liveness` health service SHALL stay `SERVING` regardless of the database. Migrations SHALL stay owned by the deploy migrate job: the service SHALL NOT apply them at boot.

#### Scenario: Unreachable database refuses to boot in the local environment
- **WHEN** `app.Run` starts with `ENV=local` and `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error and nothing listens on the gRPC port
- **VERIFIED BY**: `team-referral/internal/app` `TestBootFailsWhenDatabaseUnreachableEvenLocally`

#### Scenario: Unreachable database refuses to boot outside local
- **WHEN** `app.Run` starts with `ENV=production` and `DATABASE_URL` pointing at a closed port
- **THEN** it returns an error and nothing listens on the gRPC port
- **VERIFIED BY**: `team-referral/internal/app` `TestBootFailsWhenDatabaseUnreachableOutsideLocal`

#### Scenario: A reachable database serves from Postgres
- **WHEN** `app.Run` starts against a Postgres with the migrations applied and a caller creates a referral code, then the process restarts on the same database and the same caller reads its referral
- **THEN** the second read returns the code created before the restart
- **VERIFIED BY**: `team-referral/internal/app` `TestServesFromPostgresAcrossRestart`

#### Scenario: Health turns NOT_SERVING when the database is lost
- **WHEN** the database of a running team-referral stops accepting connections
- **THEN** within 10 seconds `grpc.health.v1.Health/Check` for the empty service name returns `NOT_SERVING` while the service name `liveness` still returns `SERVING`
- **VERIFIED BY**: `team-referral/internal/app` `TestHealthReportsDatabaseDown`

#### Scenario: Production code cannot reach the in-memory repository
- **WHEN** the imports of the non-test files under `cmd/` and `internal/app` are listed
- **THEN** none of them is `internal/store/fake`, and no non-test file outside `internal/store/fake` defines an in-memory repository
- **VERIFIED BY**: `team-referral/internal/app` `TestProductionCodeDoesNotImportFake`

#### Scenario: Compose and gitops carry no removed flag
- **WHEN** the `team-referral` service in `docker-compose.services.yaml` and every `platform-gitops` values file for team-referral are read
- **THEN** none contains `DATABASE_ENABLED`, and each sets `ENV` and `GRPC_PORT`, and the baseline values set `probes.livenessService` to `liveness`
- **VERIFIED BY**: `team-referral/internal/config` `TestDeployFilesCarryNoRemovedFlag`

### Requirement: Every RPC acts only on the authenticated caller

Every `ReferralService` RPC SHALL resolve the caller with `auth.RequirePrincipal` from the gateway metadata (`x-principal-id`, `x-principal-type`) and SHALL NOT read an identity from the request body. A missing principal, the id `anonymous`, or the type `anonymous` SHALL return `Unauthenticated` with the message `authentication required`. Each RPC SHALL read or write only the caller's own rows; there is no admin or cross-user path and no scope gate.

#### Scenario: A call without a principal is rejected
- **WHEN** `CreateReferralCode`, `GetMyReferral`, `RedeemReferral` and `ListReferralRewards` are each called with no `x-principal-id` metadata
- **THEN** every call returns `Unauthenticated`
- **VERIFIED BY**: `team-referral/internal/rpc` `TestHandlerRejectsAnonymous`

#### Scenario: The gateway's anonymous principal is rejected
- **WHEN** a call carries `x-principal-id: anonymous` with `x-principal-type: anonymous`, or an id `u-1` with type `anonymous`, or the id `anonymous` with no type
- **THEN** each call returns `Unauthenticated` and no row is written
- **VERIFIED BY**: `team-referral/internal/rpc` `TestGatewayAnonymousIsUnauthenticated`

#### Scenario: A buyer reaches the service through the gateway
- **WHEN** an authenticated buyer creates a referral code through the gateway and then calls `GetMyReferral` and `ListReferralRewards`
- **THEN** `GetMyReferral` returns the same code and the rewards call succeeds
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/services/new_services.feature` scenario "Referral code creation is readable via GetMyReferral"

#### Scenario: One user cannot see another user's data
- **WHEN** user `bob` calls `GetMyReferral` and `ListReferralRewards` after user `alice` created a code and earned a reward
- **THEN** `bob` receives his own code, an invited count of 0 and an empty reward list, and `alice`'s code and reward are absent
- **VERIFIED BY**: `team-referral/internal/rpc` `TestHandlerScopesToPrincipal`

### Requirement: Referral codes are minted once per user and redeemed at most once per referee

`CreateReferralCode` SHALL return the caller's code, minting `REF` followed by 8 characters of a UUID when none exists, and SHALL return the same code on every later call. `GetMyReferral` SHALL return the caller's code (minted on first read), `invited_count` and `rewards_total` in minor units. `RedeemReferral` SHALL record the caller as referee of the code's owner and append a reward of `10000` minor units with reason `invitee_joined` to the referrer's ledger, and SHALL return `InvalidArgument` for an empty code, `NotFound` for an unknown code, and `FailedPrecondition` when the code is the caller's own or when the caller already redeemed a code. A failed redeem SHALL append no reward.

#### Scenario: Creating a code twice returns the same code
- **WHEN** a user calls `CreateReferralCode` twice
- **THEN** both responses carry the same code, which matches `^REF[0-9a-f]{8}$`
- **VERIFIED BY**: `team-referral/internal/referral` `TestCreateThenGetRoundtrip`

#### Scenario: Redeeming credits the referrer
- **WHEN** `bob` redeems the code of `alice`
- **THEN** `alice`'s `invited_count` is 1, her `rewards_total` is 10000, and her newest reward has amount 10000 and reason `invitee_joined`
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemAttributesReferee`

#### Scenario: An empty code is rejected
- **WHEN** a user calls `RedeemReferral` with an empty code
- **THEN** the call returns `InvalidArgument` and no row is written
- **VERIFIED BY**: `team-referral/internal/rpc` `TestRedeemEmptyCodeIsInvalidArgument`

#### Scenario: An unknown code is not found
- **WHEN** a user calls `RedeemReferral` with the code `NOT-A-REAL-CODE`
- **THEN** the call returns `NotFound`, and the referral page shows an error toast with the server message
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_ui.feature` scenario "Redeeming an invalid code"

#### Scenario: Self-referral and a second redeem are refused
- **WHEN** `alice` redeems her own code, and separately `bob` redeems `alice`'s code and then `carol`'s code
- **THEN** the own-code call and the second call of `bob` return `FailedPrecondition`, and `alice` has exactly one reward
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRejectsSelfAndUnknown` and `TestRedeemRejectsSecondRedeem`

#### Scenario: The second redeem is refused by the database too
- **WHEN** two redeems for the same referee run against Postgres
- **THEN** the second returns the already-redeemed error and `referrals` holds one row for that referee
- **VERIFIED BY**: `team-referral/internal/store` `TestPostgresRedeemTwiceIsAlreadyRedeemed`

#### Scenario: An anonymous caller cannot redeem
- **WHEN** a call without a verified principal sends `RedeemReferral` with the valid code of `alice`
- **THEN** the call returns `Unauthenticated`, `alice`'s `invited_count` stays 0 and no reward is appended
- **VERIFIED BY**: `team-referral/internal/rpc` `TestAnonymousRedeemWritesNothing`

#### Scenario: Users are isolated in the ledger
- **WHEN** rewards exist for `alice` and `bob` and each lists and sums its own
- **THEN** each sees only its own rows and totals
- **VERIFIED BY**: `team-referral/internal/referral` `TestCrossUserIsolation`

### Requirement: Reward listing is newest first with a decimal row-offset cursor

`ListReferralRewards` SHALL return the caller's rewards newest first. The page size SHALL default to 20 and be clamped to at most 100 with `pagination.Clamp`. The cursor SHALL be a decimal row offset; a response with a further page SHALL carry the next offset and the last page SHALL carry an empty cursor. A malformed or negative cursor SHALL start at offset 0. `page.total` SHALL equal the number of rewards in the returned page. A storage error SHALL return `Internal` with the message `internal error` and no driver text.

#### Scenario: Pages walk the ledger
- **WHEN** a user with 5 rewards lists with page size 2, then with the returned cursor, then again
- **THEN** the pages hold 2, 2 and 1 rewards, the cursors are `2`, `4` and empty, and no reward repeats
- **VERIFIED BY**: `team-referral/internal/referral` `TestListRewardsPagination`

#### Scenario: Page size is clamped
- **WHEN** a user lists with page size 0 and with page size 1000 over 150 rewards
- **THEN** the pages hold 20 and 100 rewards
- **VERIFIED BY**: `team-referral/internal/referral` `TestListRewardsPageSizeClamp`

#### Scenario: A malformed cursor restarts at the first page
- **WHEN** a user lists with the cursor `abc` and with the cursor `-3`
- **THEN** both responses equal the first page and no error is returned
- **VERIFIED BY**: `team-referral/internal/referral` `TestMalformedCursorStartsAtZero`

#### Scenario: The page total is the page length
- **WHEN** a user with 5 rewards lists page size 2
- **THEN** `page.total` is 2
- **VERIFIED BY**: `team-referral/internal/rpc` `TestPageTotalIsPageLength`

#### Scenario: A storage failure does not leak its cause
- **WHEN** the store returns an error containing the text `pq: secret-host` during a call
- **THEN** the caller receives `Internal` with the message `internal error` and the text `secret-host` is absent from the response
- **VERIFIED BY**: `team-referral/internal/rpc` `TestMapErrHidesInternalCause`

#### Scenario: A new user sees an empty ledger
- **WHEN** a signed-in buyer with no rewards opens the referral page
- **THEN** the rewards area shows the Empty state "Chưa có phần thưởng nào."
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_ui.feature` scenario "No rewards yet"

### Requirement: Unique violations and cancellation map to their own status codes

A Postgres unique violation (SQLSTATE 23505) that reaches the handler SHALL map to `AlreadyExists` with the message `already exists` and no constraint detail. A cancelled request SHALL map to `Canceled` and an expired deadline to `DeadlineExceeded`. The redeem race on `referee_id` SHALL still return `FailedPrecondition` because the store maps it to the already-redeemed sentinel first.

#### Scenario: A unique violation is AlreadyExists
- **WHEN** a write fails because the store returns a `pgconn.PgError` with code `23505` for `CreateReferralCode`
- **THEN** the call returns `AlreadyExists` with the message `already exists`
- **VERIFIED BY**: `team-referral/internal/rpc` `TestUniqueViolationIsAlreadyExists`

#### Scenario: A cancelled request is Canceled
- **WHEN** a read fails because the store returns `context.Canceled` during `GetMyReferral`
- **THEN** the call returns `Canceled`, not `Internal`
- **VERIFIED BY**: `team-referral/internal/rpc` `TestCancelledRequestIsCanceled`

#### Scenario: The redeem race keeps FailedPrecondition
- **WHEN** the store reports the already-redeemed sentinel after a concurrent redeem
- **THEN** the call returns `FailedPrecondition`, not `AlreadyExists`
- **VERIFIED BY**: `team-referral/internal/rpc` `TestRedeemRaceIsFailedPrecondition`

### Requirement: The service follows the ADR-0016 layout and the shared build files

team-referral SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` at `v1.0.2` in `go.mod` with no `replace` directive and no `third_party` directory. `cmd/server/main.go` SHALL only load settings and call `app.Run(ctx, settings)`. `internal/app.Run` SHALL build, in order, the logger (`logx`), telemetry (`otelx`), health (`healthx`), the Postgres pool with its health check, the gRPC server (`grpcx.NewServer` with the SDK interceptor chain) and the lifecycle (`lifecycle.Run`), and SHALL close what it opened in reverse order on shutdown and on an early failure. The `Makefile` SHALL include `../platform-core/make/go-service.mk`, and the service SHALL NOT contain its own `.golangci.yml` or a copy of `go-service.mk`.

#### Scenario: The module requires the tag without a replace
- **WHEN** `team-referral/go.mod` is read
- **THEN** it requires `go-sdk v1.0.2` and contains no `replace` line, and `team-referral/third_party` does not exist
- **VERIFIED BY**: `team-referral/internal/config` `TestModuleRequiresSDKTagWithoutReplace`

#### Scenario: The Makefile uses the shared files
- **WHEN** `team-referral/Makefile` is read
- **THEN** it includes `../platform-core/make/go-service.mk`, sets `LINT_CONFIG` to `../platform-core/packages/go-sdk/.golangci.yml`, and `team-referral/.golangci.yml` and `team-referral/make/` do not exist
- **VERIFIED BY**: `team-referral/internal/config` `TestMakefileUsesSharedFiles`

#### Scenario: A failed start releases what was opened
- **WHEN** `app.Run` fails after the pool opened (for example the gRPC port is already bound)
- **THEN** it returns the error and the pool is closed, so a following `app.Run` on the same database starts
- **VERIFIED BY**: `team-referral/internal/app` `TestEarlyFailureClosesPool`

#### Scenario: Context cancellation stops the server within the grace period
- **WHEN** the context passed to `app.Run` is cancelled while a call is in flight and `SHUTDOWN_GRACE=2s`
- **THEN** `app.Run` returns within 3 seconds and the listener no longer accepts connections
- **VERIFIED BY**: `team-referral/internal/app` `TestShutdownWithinGrace`

#### Scenario: The service answers on its port after the migration
- **WHEN** the stack runs with the migrated image and a buyer creates a referral code through the gateway
- **THEN** the call succeeds and the container reports healthy
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/services/new_services.feature` scenario "Referral code creation is readable via GetMyReferral"
