# referral-service Specification

## Purpose
TBD - created by archiving change referral-go-sdk-v1. Update Purpose after archive.

## Requirements

### Requirement: Settings come from SDK groups and the service has no dependency enable flags

team-referral SHALL load its settings with `config.Load` over the SDK groups `Runtime`, `GRPC`, `Postgres`, `OTel`, `Kafka` and `Outbox`, plus a `Referral` group. `ENV`, `GRPC_PORT` and `DATABASE_URL` SHALL be required and have no default; `Load` SHALL return one error naming every one of them that is unset. The `Referral` group SHALL read `REFERRAL_REWARD_XU` (default 10000), `REFERRAL_DAILY_CAP` (default 5), `REFERRAL_LIFETIME_CAP` (default 50), `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` (default 168h), `REFERRAL_RELEASE_SWEEP_INTERVAL` (default 1m), `REFERRAL_CAP_TIMEZONE` (default `Asia/Ho_Chi_Minh`), `IDENTITY_EVENTS_TOPIC` (default `identity.events`), `ORDER_EVENTS_TOPIC` (default `order.events`), `REFERRAL_EVENTS_TOPIC` (default `referral.events`), the consumer groups `REFERRAL_IDENTITY_CONSUMER_GROUP` and `REFERRAL_ORDER_CONSUMER_GROUP`, and the dead-letter topics `REFERRAL_IDENTITY_DLQ_TOPIC` and `REFERRAL_ORDER_DLQ_TOPIC`. `Load` SHALL return an error naming each of `REFERRAL_REWARD_XU`, `REFERRAL_DAILY_CAP`, `REFERRAL_LIFETIME_CAP`, `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` and `REFERRAL_RELEASE_SWEEP_INTERVAL` that is zero or negative or unparsable, and `REFERRAL_CAP_TIMEZONE` that is not a known IANA zone. The service SHALL NOT declare or read `DATABASE_ENABLED`, or any other `*_ENABLED` variable except `OTEL_ENABLED` and `GRPC_REFLECTION_ENABLED`; reflection SHALL be off unless `GRPC_REFLECTION_ENABLED=true`. `team-referral/.env.example` SHALL contain exactly the keys declared by `Settings`.

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

#### Scenario: Referral settings default to the locked values
- **WHEN** settings load with only `ENV`, `GRPC_PORT` and `DATABASE_URL` set
- **THEN** the reward is 10000, the daily cap 5, the lifetime cap 50, the redeem age limit 168h, the sweep interval 1m and the cap timezone `Asia/Ho_Chi_Minh`
- **VERIFIED BY**: `team-referral/internal/config` `TestReferralSettingsDefaults`

#### Scenario: A non-positive cap or an unknown timezone fails the load
- **WHEN** settings load with `REFERRAL_DAILY_CAP=0`, with `REFERRAL_RELEASE_SWEEP_INTERVAL=-1s`, and separately with `REFERRAL_CAP_TIMEZONE=Mars/Base`
- **THEN** each load returns an error whose text names the offending variable
- **VERIFIED BY**: `team-referral/internal/config` `TestReferralSettingsRejectInvalid`

#### Scenario: Referral settings can be overridden from the environment
- **WHEN** settings load with `REFERRAL_DAILY_CAP=2`, `REFERRAL_LIFETIME_CAP=3` and `REFERRAL_REWARD_XU=500`
- **THEN** the loaded caps are 2 and 3 and the reward is 500
- **VERIFIED BY**: `team-referral/internal/config` `TestReferralSettingsOverride`

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
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_ui.feature` scenario "A new user sees an empty ledger"

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

team-referral SHALL require `github.com/buidangphuc/agora/platform-core/packages/go-sdk` exactly once in `go.mod`, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version and no `-` suffix), with no `replace` directive and no `third_party` directory, and SHALL pin golangci-lint as a `tool` directive. `cmd/server/main.go` SHALL only load settings and call `app.Run(ctx, settings)`. `internal/app.Run` SHALL build, in order, the logger (`logx`), telemetry (`otelx`), health (`healthx`), the Postgres pool with its health check, the gRPC server (`grpcx.NewServer` with the SDK interceptor chain) and the lifecycle (`lifecycle.Run`), and SHALL close what it opened in reverse order on shutdown and on an early failure. The `Makefile` SHALL include `../platform-core/make/go-service.mk`, and the service SHALL NOT contain its own `.golangci.yml` or a copy of `go-service.mk`.

#### Scenario: The module requires the tag without a replace
- **WHEN** `team-referral/go.mod` is read
- **THEN** it requires go-sdk exactly once, at a version matching `^v\d+\.\d+\.\d+$`, has a golangci-lint `tool` line and contains no `replace` line, and `team-referral/third_party` does not exist
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
- **THEN** the call succeeds and the container is running, has not restarted and carries no removed flag; the service has no Docker healthcheck, and its gRPC health is verified by `TestHealthReportsDatabaseDown` and the other health scenarios
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/services/new_services.feature` scenario "The service answers on its port after the migration"

### Requirement: Referral codes are minted once per user and redeeming attributes the referee without crediting

`CreateReferralCode` SHALL return the caller's code, minting `REF` followed by 8 characters of a UUID when none exists, and SHALL return the same code on every later call. `GetMyReferral` SHALL return the caller's code (minted on first read), `invited_count`, `pending_xu` (sum of the caller's `PENDING` rewards), `released_xu` (sum of the caller's `RELEASED` rewards) and `rewards_total` equal to `released_xu`. `RedeemReferral` SHALL attribute the caller as referee of the code's owner exactly as the `UserRegistered` consumer does (referral row with status `ATTRIBUTED`, outbox event `ReferralAttributed` in the same transaction) and SHALL NOT append any reward or credit anything. `RedeemReferral` SHALL return `InvalidArgument` for an empty code and `NotFound` for an unknown code, and SHALL return `FailedPrecondition` with a Vietnamese message when the code is the caller's own, when the caller already has a referral, when the code owner has reached the lifetime cap, or when the caller is not eligible: eligible means a stored registration time (from `UserRegistered`) younger than `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` and no stored `OrderCompleted` for the caller. A failed redeem SHALL write no row.

#### Scenario: Creating a code twice returns the same code
- **WHEN** a user calls `CreateReferralCode` twice
- **THEN** both responses carry the same code, which matches `^REF[0-9a-f]{8}$`
- **VERIFIED BY**: `team-referral/internal/referral` `TestCreateThenGetRoundtrip`

#### Scenario: Redeeming attributes the referee and credits nothing
- **WHEN** `bob`, registered 1 hour ago with no completed order, redeems the code of `alice`
- **THEN** `alice`'s `invited_count` is 1, `pending_xu` and `released_xu` are 0, her reward list is empty, the referral is `ATTRIBUTED` and one `ReferralAttributed` outbox row exists
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemAttributesWithoutCredit`

#### Scenario: An empty code is rejected
- **WHEN** a user calls `RedeemReferral` with an empty code
- **THEN** the call returns `InvalidArgument` and no row is written
- **VERIFIED BY**: `team-referral/internal/rpc` `TestRedeemEmptyCodeIsInvalidArgument`

#### Scenario: An unknown code is not found
- **WHEN** a user calls `RedeemReferral` with the code `NOT-A-REAL-CODE`
- **THEN** the call returns `NotFound`, and the referral page shows an error toast with the server message
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_ui.feature` scenario "An unknown code is not found"

#### Scenario: Self-referral and a second redeem are refused
- **WHEN** `alice` redeems her own code, and separately `bob` redeems `alice`'s code and then `carol`'s code
- **THEN** the own-code call and the second call of `bob` return `FailedPrecondition` with a Vietnamese message, and `alice` has no reward and one referral
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRejectsSelfAndUnknown` and `TestRedeemRejectsSecondRedeem`

#### Scenario: The second redeem is refused by the database too
- **WHEN** two redeems for the same referee run against Postgres
- **THEN** the second returns the already-redeemed error and `referrals` holds one row for that referee
- **VERIFIED BY**: `team-referral/internal/store` `TestPostgresRedeemTwiceIsAlreadyRedeemed`

#### Scenario: An anonymous caller cannot redeem
- **WHEN** a call without a verified principal sends `RedeemReferral` with the valid code of `alice`
- **THEN** the call returns `Unauthenticated`, `alice`'s `invited_count` stays 0 and no outbox row is written
- **VERIFIED BY**: `team-referral/internal/rpc` `TestAnonymousRedeemWritesNothing`

#### Scenario: Users are isolated in the ledger
- **WHEN** rewards exist for `alice` and `bob` and each lists and sums its own
- **THEN** each sees only its own rows and totals
- **VERIFIED BY**: `team-referral/internal/referral` `TestCrossUserIsolation`

#### Scenario: An account older than the limit cannot redeem
- **WHEN** `dave`, whose stored registration is 169 hours old with `REFERRAL_REDEEM_MAX_ACCOUNT_AGE=168h`, redeems a valid code
- **THEN** the call returns `FailedPrecondition` with a Vietnamese message and no row is written
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRefusesOldAccount`

#### Scenario: An account exactly at the age limit is refused and one just under is accepted
- **WHEN** two buyers registered 168h and 167h59m ago redeem a valid code with `REFERRAL_REDEEM_MAX_ACCOUNT_AGE=168h`
- **THEN** the first call returns `FailedPrecondition` and the second succeeds
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemAgeBoundary`

#### Scenario: An account with a completed order cannot redeem
- **WHEN** `erin`, registered 1 hour ago, has a stored `OrderCompleted` and redeems a valid code
- **THEN** the call returns `FailedPrecondition` with a Vietnamese message and no referral row is written
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRefusesAfterCompletedOrder`

#### Scenario: An account with no stored registration cannot redeem
- **WHEN** a buyer for whom no `UserRegistered` was ever consumed redeems a valid code
- **THEN** the call returns `FailedPrecondition` and no row is written
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRefusesUnknownRegistration`

#### Scenario: A code owner at the lifetime cap refuses a manual redeem
- **WHEN** `alice` has 50 non-void rewards and an eligible `bob` redeems her code with `REFERRAL_LIFETIME_CAP=50`
- **THEN** the call returns `FailedPrecondition` and no referral row is written
- **VERIFIED BY**: `team-referral/internal/referral` `TestRedeemRefusesCappedReferrer`

### Requirement: Signup attributes a referral from identity.events asynchronously

team-referral SHALL consume `identity.events` (envelope type `platform.identity.v1.UserRegistered`) with a durable consumer group and ignore every other envelope type. For each event it SHALL, in one transaction, record the processed event id, upsert the user's registration time (`registered_at`) and, when `referral_code` is non-empty, resolve the code. A code owned by another user whose owner is under the lifetime cap SHALL create one `referrals` row with status `ATTRIBUTED` and one outbox row `ReferralAttributed{referee_id, referrer_id, attributed_at}`. Otherwise it SHALL write one outbox row `ReferralRejected{referee_id, reason}` with `UNKNOWN_CODE`, `SELF_REFERRAL` or `REFERRER_CAPPED`. An event with an empty `referral_code` SHALL emit nothing. When the referee already has a referral row, the event SHALL change nothing except the processed event id and registration time, and SHALL emit no event. The consumers expose no RPC and the only identity they use is the event's `user_id`; no caller-supplied identity is trusted. Outbox rows SHALL be published to `referral.events` keyed by referee id with envelope type `platform.referral.v1.<Message>` and a stable event id derived from the message name and the referee id. A malformed event SHALL go to the dead-letter topic and SHALL NOT block later events.

#### Scenario: A valid code creates an attributed referral
- **WHEN** `UserRegistered{user_id: bob, referral_code: <alice's code>}` is consumed
- **THEN** `referrals` holds one row for `bob` with referrer `alice` and status `ATTRIBUTED`, and `referral.events` carries one `ReferralAttributed` keyed by `bob` within 10 seconds
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredAttributes`

#### Scenario: An unknown code is rejected with a verdict event
- **WHEN** `UserRegistered` carries the code `NOT-A-REAL-CODE`
- **THEN** no referral row is created and one `ReferralRejected` with reason `UNKNOWN_CODE` is emitted
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredUnknownCode`

#### Scenario: A self-referral is rejected with a verdict event
- **WHEN** `UserRegistered` carries the code owned by the registering user
- **THEN** no referral row is created and one `ReferralRejected` with reason `SELF_REFERRAL` is emitted
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredSelfReferral`

#### Scenario: A capped inviter is rejected with a verdict event
- **WHEN** `UserRegistered` carries the code of an inviter who has 50 non-void rewards with `REFERRAL_LIFETIME_CAP=50`
- **THEN** no referral row is created and one `ReferralRejected` with reason `REFERRER_CAPPED` is emitted
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredCappedReferrer`

#### Scenario: A signup without a code stores the registration and emits nothing
- **WHEN** `UserRegistered` with an empty `referral_code` is consumed
- **THEN** the user's `registered_at` is stored, no referral row exists and no outbox row is written
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredWithoutCode`

#### Scenario: A redelivered signup creates nothing twice
- **WHEN** the same `UserRegistered` event id is delivered twice
- **THEN** `referrals` holds one row for the referee, the outbox holds one row, and `referral.events` carries one message with that stable event id
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredRedelivery`

#### Scenario: A signup for a referee who already has a referral changes nothing
- **WHEN** `bob` already has a referral row and a `UserRegistered` with a new event id for `bob` carries another user's code
- **THEN** the referral row keeps its referrer and status, and no outbox row is written
- **VERIFIED BY**: `team-referral/internal/consumer` `TestUserRegisteredForAlreadyReferredUser`

#### Scenario: A malformed event is dead-lettered
- **WHEN** a record on `identity.events` has an envelope type `platform.identity.v1.UserRegistered` and an undecodable payload
- **THEN** the record is published to `REFERRAL_IDENTITY_DLQ_TOPIC`, no row is written and the next valid event is processed
- **VERIFIED BY**: `team-referral/internal/consumer` `TestMalformedUserRegisteredIsDeadLettered`

#### Scenario: Referral rows are committed atomically with the event
- **WHEN** the outbox insert fails while a valid `UserRegistered` is handled
- **THEN** the transaction rolls back, no referral row exists, and the event is retried
- **VERIFIED BY**: `team-referral/internal/store` `TestAttributionRollsBackWithOutbox`

### Requirement: The first completed order of an invitee creates a pending reward

team-referral SHALL consume `order.events` `platform.order.v1.OrderCompleted`. For every event it SHALL record in one transaction the processed event id and the buyer's first completed-order time. When the buyer's referral is `ATTRIBUTED` and the inviter has fewer than `REFERRAL_LIFETIME_CAP` rewards in status `PENDING` or `RELEASED`, it SHALL create one reward with status `PENDING`, amount `REFERRAL_REWARD_XU`, reason `invitee_first_order`, the order id, and `release_at` equal to `return_window_ends_at`, and set the referral to `QUALIFIED` in that transaction. When the inviter is at the lifetime cap it SHALL create no reward and set the referral to `CAPPED`. An `OrderCompleted` for a buyer with no referral, or whose referral is not `ATTRIBUTED`, SHALL change nothing but the recorded completed-order fact. The order id SHALL be unique across rewards. The check and the insert SHALL run while the inviter's code row is locked `FOR UPDATE`.

#### Scenario: The first order creates a pending reward
- **WHEN** `OrderCompleted{buyer: bob, order: o1, return_window_ends_at: T}` is consumed for an `ATTRIBUTED` referral of `alice`
- **THEN** one reward exists for `alice` with status `PENDING`, amount 10000, order `o1` and `release_at` `T`, the referral is `QUALIFIED`, and `alice`'s `pending_xu` is 10000 and `released_xu` 0
- **VERIFIED BY**: `team-referral/internal/consumer` `TestOrderCompletedCreatesPendingReward`

#### Scenario: A buyer with no referral is ignored
- **WHEN** `OrderCompleted` is consumed for a buyer with no referral row
- **THEN** no reward and no outbox row are written and the buyer's completed-order fact is stored
- **VERIFIED BY**: `team-referral/internal/consumer` `TestOrderCompletedWithoutReferral`

#### Scenario: A second order of a qualified invitee creates nothing
- **WHEN** `OrderCompleted` for order `o2` is consumed after `bob`'s referral is `QUALIFIED` for order `o1`
- **THEN** `alice` still has exactly one reward
- **VERIFIED BY**: `team-referral/internal/consumer` `TestSecondOrderDoesNotCreateSecondReward`

#### Scenario: A redelivered order event creates one reward
- **WHEN** the same `OrderCompleted` event id is delivered twice, and separately a different event id carries the same order id
- **THEN** exactly one reward exists for that order in both cases
- **VERIFIED BY**: `team-referral/internal/consumer` `TestOrderCompletedRedelivery`

#### Scenario: A capped inviter gets no pending reward
- **WHEN** `OrderCompleted` is consumed for the invitee of an inviter with 50 non-void rewards and `REFERRAL_LIFETIME_CAP=50`
- **THEN** no reward is created and the referral status is `CAPPED`
- **VERIFIED BY**: `team-referral/internal/consumer` `TestOrderCompletedAtLifetimeCap`

#### Scenario: Concurrent completions cannot exceed the lifetime cap
- **WHEN** two invitees of one inviter with 49 non-void rewards complete orders concurrently with `REFERRAL_LIFETIME_CAP=50`
- **THEN** exactly one reward is created and the other referral is `CAPPED`
- **VERIFIED BY**: `team-referral/internal/store` `TestLifetimeCapHoldsUnderConcurrency`

### Requirement: A return before release voids the reward

team-referral SHALL consume `order.events` `platform.order.v1.ReturnRequested`. When a reward linked to the event's order id is `PENDING`, it SHALL set it to `VOID` and set its referral back to `ATTRIBUTED` in one transaction, so a later `OrderCompleted` of the same invitee can qualify again. A `ReturnRequested` for an order with no reward, or whose reward is already `RELEASED` or `VOID`, SHALL change nothing. A void and a release of the same reward SHALL never both succeed: both transitions SHALL be conditional on the current status `PENDING`.

#### Scenario: A return voids a pending reward
- **WHEN** `ReturnRequested` for order `o1` is consumed while its reward is `PENDING`
- **THEN** the reward is `VOID`, the referral is `ATTRIBUTED`, and `alice`'s `pending_xu` is 0
- **VERIFIED BY**: `team-referral/internal/consumer` `TestReturnRequestedVoidsPendingReward`

#### Scenario: The next order qualifies again after a void
- **WHEN** the reward of `o1` was voided and `OrderCompleted` for a new order `o3` of `bob` is consumed
- **THEN** a new `PENDING` reward exists for `o3` and the referral is `QUALIFIED`
- **VERIFIED BY**: `team-referral/internal/consumer` `TestRequalifyAfterVoid`

#### Scenario: A return after release changes nothing
- **WHEN** `ReturnRequested` for order `o1` is consumed after its reward was `RELEASED`
- **THEN** the reward stays `RELEASED` and `alice`'s `released_xu` is unchanged
- **VERIFIED BY**: `team-referral/internal/consumer` `TestReturnAfterReleaseIgnored`

#### Scenario: A return for an unrelated order is ignored
- **WHEN** `ReturnRequested` is consumed for an order that has no reward
- **THEN** no row changes and no event is written
- **VERIFIED BY**: `team-referral/internal/consumer` `TestReturnRequestedUnknownOrder`

#### Scenario: A release and a void racing on one reward end in exactly one state
- **WHEN** the sweeper release and a `ReturnRequested` for the same pending reward run concurrently
- **THEN** the reward ends `RELEASED` with a `ReferralRewardReleased` outbox row, or `VOID` with none, never both
- **VERIFIED BY**: `team-referral/internal/store` `TestReleaseAndVoidAreMutuallyExclusive`

### Requirement: A sweeper releases due rewards under daily and lifetime caps

Every `REFERRAL_RELEASE_SWEEP_INTERVAL` team-referral SHALL select `PENDING` rewards with `release_at <= now`, oldest `release_at` first, and for each one in a transaction that locks the inviter's code row it SHALL release the reward when the inviter has fewer than `REFERRAL_DAILY_CAP` rewards whose `released_at` falls on the current calendar day in `REFERRAL_CAP_TIMEZONE`, and, as defence in depth (it matters only when `REFERRAL_LIFETIME_CAP` is lowered by configuration after rewards were created, since qualify time already enforces the cap), fewer than `REFERRAL_LIFETIME_CAP` `RELEASED` rewards. Releasing SHALL set the reward to `RELEASED` with `released_at = now`, the referral to `REWARDED`, and write an outbox row `ReferralRewardReleased{reward_id, referrer_id, referee_id, amount_xu, released_at}`. A due reward whose inviter is at the daily cap SHALL stay `PENDING` and be retried on the next sweep until the day changes. The reward amount SHALL be released in xu as stored and never converted. Several sweeper instances SHALL be safe: a reward is released at most once.

#### Scenario: A due reward is released and announced
- **WHEN** a `PENDING` reward of 10000 has `release_at` 10 seconds ago and a sweep runs
- **THEN** the reward is `RELEASED` with `released_at` set, the referral is `REWARDED`, `released_xu` of the inviter is 10000, and `referral.events` carries one `ReferralRewardReleased` with `amount_xu` 10000
- **VERIFIED BY**: `team-referral/internal/referral` `TestSweepReleasesDueReward`

#### Scenario: A reward whose window has not ended stays pending
- **WHEN** a sweep runs while a `PENDING` reward has `release_at` one hour in the future
- **THEN** the reward stays `PENDING` and no outbox row is written
- **VERIFIED BY**: `team-referral/internal/referral` `TestSweepKeepsFutureReward`

#### Scenario: The daily cap defers the sixth release to the next day
- **WHEN** an inviter has 6 due `PENDING` rewards, `REFERRAL_DAILY_CAP=5`, and a sweep runs, then a sweep runs after midnight in `Asia/Ho_Chi_Minh`
- **THEN** after the first sweep 5 are `RELEASED` and 1 is `PENDING`, and after the second sweep the sixth is `RELEASED`
- **VERIFIED BY**: `team-referral/internal/referral` `TestSweepDailyCapDefersToNextDay`

#### Scenario: The day boundary follows the configured timezone
- **WHEN** 5 rewards were released at 16:59 UTC (23:59 in `Asia/Ho_Chi_Minh`) and a sixth is due at 17:01 UTC (00:01 the next day there)
- **THEN** the sixth is released at the 17:01 UTC sweep
- **VERIFIED BY**: `team-referral/internal/referral` `TestDailyCapUsesConfiguredTimezone`

#### Scenario: A lowered lifetime cap stops releases
- **WHEN** an inviter has 3 `RELEASED` rewards and a due `PENDING` reward, and the service now runs with `REFERRAL_LIFETIME_CAP=3`
- **THEN** the sweep releases nothing for that inviter
- **VERIFIED BY**: `team-referral/internal/referral` `TestSweepLifetimeCap`

#### Scenario: Two sweepers release a reward once
- **WHEN** two sweeps run concurrently over one due reward
- **THEN** exactly one `ReferralRewardReleased` outbox row exists for it
- **VERIFIED BY**: `team-referral/internal/store` `TestConcurrentSweepReleasesOnce`

#### Scenario: A failed release leaves the reward pending
- **WHEN** the outbox insert fails while the sweeper releases a due reward
- **THEN** the transaction rolls back, the reward stays `PENDING`, the referral stays `QUALIFIED`, and the next sweep retries it
- **VERIFIED BY**: `team-referral/internal/store` `TestReleaseRollsBackWithOutbox`

#### Scenario: A sweep releases within twice the interval
- **WHEN** the stack runs with `REFERRAL_RELEASE_SWEEP_INTERVAL=1s` and `RETURN_WINDOW=3s`, and an invitee's first order completes
- **THEN** the inviter's reward is `RELEASED` within `RETURN_WINDOW` plus 2 seconds and `released_xu` shows 10000 through `GetMyReferral`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/referral/referral_reward.feature` scenario "A sweep releases within twice the interval"

### Requirement: The reward ledger and summary show state in xu

`ListReferralRewards` SHALL return the caller's rewards in status `PENDING` or `RELEASED`, newest first, each with `status`, `amount` in xu and `release_at`; `VOID` rewards SHALL be omitted. Rewards that existed before migration `002` (credited immediately) SHALL become `RELEASED` with `released_at` and `release_at` equal to their `created_at`, and their referrals `REWARDED`. `GetMyReferral.pending_xu` and `released_xu` SHALL be computed from the caller's own rewards only. The summary SHALL NOT be derived from rows of another user.

#### Scenario: Pending and released totals are separate
- **WHEN** `alice` has one `PENDING` reward of 10000 and two `RELEASED` rewards of 10000 and calls `GetMyReferral`
- **THEN** `pending_xu` is 10000, `released_xu` is 20000 and `rewards_total` is 20000
- **VERIFIED BY**: `team-referral/internal/referral` `TestSummarySplitsPendingAndReleased`

#### Scenario: A void reward is not listed
- **WHEN** `alice` has one `VOID`, one `PENDING` and one `RELEASED` reward and lists rewards
- **THEN** the response holds two rewards, with statuses `PENDING` and `RELEASED`, and no `VOID`
- **VERIFIED BY**: `team-referral/internal/referral` `TestListRewardsOmitsVoid`

#### Scenario: Another user's rewards never appear in the summary
- **WHEN** `bob` calls `GetMyReferral` and `ListReferralRewards` while `alice` has pending and released rewards
- **THEN** `bob` sees `pending_xu` 0, `released_xu` 0 and an empty list
- **VERIFIED BY**: `team-referral/internal/rpc` `TestSummaryScopedToPrincipal`

#### Scenario: Legacy rewards migrate as released
- **WHEN** migration `002` runs over a database that holds a reward and a referral created by migration `001` code
- **THEN** the reward is `RELEASED` with `released_at` equal to its `created_at` and the referral is `REWARDED`
- **VERIFIED BY**: `team-referral/internal/store` `TestMigration002BackfillsLegacyRewards`

### Requirement: The service publishes through the outbox and runs consumers and the sweeper under the lifecycle

`internal/app.Run` SHALL, after the Postgres pool opens, start under `lifecycle.Run` the go-sdk outbox relayer over `referral_outbox_events` (`OUTBOX_POLL_INTERVAL`, `OUTBOX_BATCH_SIZE`, `OUTBOX_CLAIM_LOCK`), a `kafkax` consumer per source topic (`identity.events`, `order.events`) with its own group and dead-letter topic, and the reward sweeper, and SHALL stop all of them on shutdown within `SHUTDOWN_GRACE`. The service SHALL return an error from `app.Run` before opening the gRPC listener when the broker cannot be configured, SHALL NOT offer an in-memory or no-op substitute for Kafka or the outbox, and SHALL write `referral.events` only through the outbox in the same transaction as the state change. Logs of consumers, relayer and sweeper SHALL carry only `principal_fp` / `key_fp` fingerprints and SHALL NOT contain a raw user id, referral code or order id.

#### Scenario: An outbox row is published to referral.events
- **WHEN** a `ReferralAttributed` outbox row is committed
- **THEN** within 5 seconds a record with envelope type `platform.referral.v1.ReferralAttributed` and key equal to the referee id is on `referral.events` and the row is marked published
- **VERIFIED BY**: `team-referral/internal/events` `TestRelayerPublishesReferralEvents`

#### Scenario: Shutdown stops consumers and the sweeper
- **WHEN** the context passed to `app.Run` is cancelled with `SHUTDOWN_GRACE=2s`
- **THEN** `app.Run` returns within 3 seconds and no consumer or sweeper goroutine remains
- **VERIFIED BY**: `team-referral/internal/app` `TestShutdownStopsWorkers`

#### Scenario: An unreachable broker does not lose events
- **WHEN** Kafka is unreachable while a referral is attributed
- **THEN** the outbox row stays unpublished, the referral row exists, and the row is published once Kafka is back
- **VERIFIED BY**: `team-referral/internal/events` `TestOutboxSurvivesBrokerOutage`

#### Scenario: Logs carry no raw ids or codes
- **WHEN** a signup is attributed, an order qualifies a reward and the sweeper releases it, with log capture on
- **THEN** the captured logs contain none of the user ids, the referral code or the order id, and contain `principal_fp` or `key_fp` fields
- **VERIFIED BY**: `team-referral/internal/consumer` `TestLogsContainNoRawIdentifiers`

#### Scenario: The deploy files carry the Kafka and referral settings
- **WHEN** the `team-referral` service in `docker-compose.services.yaml` and `platform-gitops/envs/services/team-referral.yaml` are read
- **THEN** each sets `KAFKA_BROKERS`, `REFERRAL_EVENTS_TOPIC` and the `OUTBOX_*` keys, compose lists `redpanda-init` in `depends_on` and creates `referral.events` and both referral DLQ topics, and no file contains a removed `*_ENABLED` flag
- **VERIFIED BY**: `team-referral/internal/config` `TestDeployFilesCarryKafkaSettings`

#### Scenario: Invite attribution works end to end through the chain
- **WHEN** a visitor registers through the gateway with `referral_code` of `alice`, and then completes a first order
- **THEN** `ReferralAttributed` is observed on `referral.events` within 10 seconds of registering, and `alice`'s `GetMyReferral` shows `pending_xu` 10000 after the order completes
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/referral/referral_reward.feature` scenario "Invite attribution works end to end through the chain"
