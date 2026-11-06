## REMOVED Requirements

### Requirement: Referral codes are minted once per user and redeemed at most once per referee
**Reason**: Redeeming no longer credits the referrer immediately; the reward is created pending on the invitee's first completed order. Replaced by the requirement "Referral codes are minted once per user and redeeming attributes the referee without crediting".
**Migration**: Scenario "Redeeming credits the referrer" is replaced by "Redeeming attributes the referee and credits nothing"; all other scenarios carry over under the new requirement.

## MODIFIED Requirements

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

## ADDED Requirements

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
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/referral/referral_reward.feature` scenario "A reward is released after the return window"

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
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/referral/referral_reward.feature` scenario "An invited signup is attributed and the first order creates a pending reward"
