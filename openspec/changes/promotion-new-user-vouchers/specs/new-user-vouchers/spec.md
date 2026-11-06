## ADDED Requirements

### Requirement: Each account receives exactly one VOUCHER_KIND_NEW_USER voucher

team-promotion SHALL consume `identity.events` (`platform.identity.v1.UserRegistered`) and `referral.events` (`platform.referral.v1.ReferralAttributed`, `ReferralRejected`) in consumer group `team-promotion.new-user`. For a user whose `UserRegistered.referral_code` is empty it SHALL issue the "Bạn mới" voucher; for a non-empty code it SHALL issue nothing until the referral verdict for that user is known, then issue "Bạn được mời" on `ReferralAttributed` and "Bạn mới" on `ReferralRejected`. When no verdict has arrived `NEW_USER_VERDICT_TIMEOUT` (default 30m) after the `UserRegistered` was consumed, a sweeper running every `NEW_USER_VERDICT_SWEEP_INTERVAL` (default 1m) SHALL issue "Bạn mới"; a `ReferralAttributed` that arrives after any voucher exists SHALL NOT change or add a voucher. The issued voucher SHALL have kind VOUCHER_KIND_NEW_USER, owner = the user, scope PLATFORM, discount type FIXED, quota 1, `per_user_limit` 1, `min_spend` = `NEW_USER_MIN_SPEND` (default 100000), `starts_at` = issue time and `ends_at` = issue time + `NEW_USER_VOUCHER_TTL` (default 336h). The discount SHALL be `NEW_USER_WELCOME_AMOUNT` (default 20000) for "Bạn mới" and `NEW_USER_INVITED_AMOUNT` (default 30000) for "Bạn được mời". The voucher code SHALL be generated, unique and not derivable from the user id in plain text. The state change, the voucher row and the processed-event row SHALL commit in one transaction. No `VoucherChanged` event SHALL be published for a VOUCHER_KIND_NEW_USER voucher.

#### Scenario: Registration without a referral code issues the welcome voucher
- **WHEN** `UserRegistered{user_id: U, referral_code: ""}` is consumed
- **THEN** within 10 s (the e2e polls every 500 ms and fails at 10 s) `ListMyVouchers` for U returns exactly one voucher with kind VOUCHER_KIND_NEW_USER, discount_value 20000, min_spend 100000, quota 1, per_user_limit 1, `ends_at - starts_at` = 336h
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/new_user_vouchers.feature` `Registration without a referral code issues the welcome voucher`; unit `team-promotion/internal/consumer/newuser_test.go` `TestEmptyCodeIssuesWelcome`

#### Scenario: Non-empty code waits for the verdict, then an attributed verdict issues the invited voucher
- **WHEN** `UserRegistered{user_id: U, referral_code: "C"}` is consumed and no voucher exists, then 5 s later `ReferralAttributed{referee_id: U}` is consumed
- **THEN** before the verdict U owns zero vouchers; after it U owns exactly one VOUCHER_KIND_NEW_USER voucher with discount_value 30000 and min_spend 100000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/new_user_vouchers.feature` `Invited signup waits for the verdict and gets the invited voucher`; unit `team-promotion/internal/consumer/newuser_test.go` `TestWaitsThenInvited`

#### Scenario: A rejected referral falls back to the welcome voucher
- **WHEN** `UserRegistered{user_id: U, referral_code: "C"}` and then `ReferralRejected{referee_id: U, reason: UNKNOWN_CODE}` are consumed
- **THEN** U owns exactly one VOUCHER_KIND_NEW_USER voucher with discount_value 20000
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/new_user_vouchers.feature` `A rejected referral falls back to the welcome voucher`; unit `team-promotion/internal/consumer/newuser_test.go` `TestRejectedGetsWelcome`

#### Scenario: A verdict that arrives before the registration event is used
- **WHEN** `ReferralAttributed{referee_id: U}` is consumed first and `UserRegistered{user_id: U, referral_code: "C"}` second
- **THEN** U owns exactly one VOUCHER_KIND_NEW_USER voucher with discount_value 30000 after the second event and none after the first
- **VERIFIED BY**: unit `team-promotion/internal/consumer/newuser_test.go` `TestVerdictBeforeRegistration`

#### Scenario: Redelivery and late verdicts issue nothing more
- **WHEN** the same `UserRegistered` is delivered twice, or a `ReferralRejected` for U arrives after U's voucher was issued
- **THEN** the count of vouchers with kind VOUCHER_KIND_NEW_USER and owner U stays 1 and the voucher discount_value is unchanged
- **VERIFIED BY**: unit `team-promotion/internal/consumer/newuser_test.go` `TestRedeliveryIdempotent`, `TestLateVerdictIgnored`; Postgres `team-promotion/internal/store/migration_test.go` `TestNewUserUniqueIndex`

#### Scenario: Missing verdict falls back to the welcome voucher after the timeout
- **WHEN** `UserRegistered{user_id: U, referral_code: "C"}` is consumed, no verdict arrives, and `NEW_USER_VERDICT_TIMEOUT` (30m; 2s in the e2e override) elapses, then `ReferralAttributed{referee_id: U}` arrives
- **THEN** U owns exactly one voucher with discount_value 20000 issued no later than timeout + 2 x sweep interval after the registration, and the late verdict changes nothing (still one voucher, 20000)
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/promotion/new_user_vouchers.feature` `Missing verdict falls back to the welcome voucher after the timeout`; unit `team-promotion/internal/consumer/newuser_test.go` `TestVerdictTimeoutFallback`, `TestLateAttributedDoesNotUpgrade`

#### Scenario: One tick before the timeout nothing is issued
- **WHEN** the sweeper runs while a pending row is 1 s younger than `NEW_USER_VERDICT_TIMEOUT`
- **THEN** no voucher exists for U; at the next tick after the timeout one exists
- **VERIFIED BY**: unit `team-promotion/internal/consumer/newuser_test.go` `TestSweeperTimeoutBoundary`

#### Scenario: A malformed event goes to the DLQ and does not block the partition
- **WHEN** a record on `identity.events` of type `platform.identity.v1.UserRegistered` carries an undecodable payload or an empty `user_id`, followed by a valid `UserRegistered`
- **THEN** the bad record is published to `identity.events.promotion.dlq` after 3 attempts, no voucher is created for it, and the valid event issues its voucher within 10 s
- **VERIFIED BY**: unit `team-promotion/internal/consumer/dlq_test.go` `TestPoisonRecordToDLQ`

#### Scenario: A malformed referral event goes to its DLQ
- **WHEN** a record on `referral.events` of type `platform.referral.v1.ReferralAttributed` or `ReferralRejected` carries an undecodable payload or an empty `referee_id`, followed by a valid verdict for a waiting user
- **THEN** the bad record is published to `referral.events.promotion.dlq` after 3 attempts, no state changes for it, and the valid verdict issues its voucher within 10 s
- **VERIFIED BY**: unit `team-promotion/internal/consumer/dlq_test.go` `TestPoisonReferralRecordToDLQ`

#### Scenario: A buyer cannot obtain a VOUCHER_KIND_NEW_USER voucher without a registration event
- **WHEN** a signed-in buyer whose `UserRegistered` was never consumed calls `ListMyVouchers`, and calls `CreateVoucher` with any code
- **THEN** `ListMyVouchers` returns zero vouchers and `CreateVoucher` (any scope) fails `PERMISSION_DENIED` for a buyer without `listing.write`; no RPC accepts a user id for issue
- **VERIFIED BY**: unit `team-promotion/internal/rpc/voucher_my_test.go` `TestNoSelfIssue`

#### Scenario: Events of other types on the topics are ignored
- **WHEN** `identity.events` carries a type other than `UserRegistered`, or `referral.events` carries `ReferralRewardReleased`
- **THEN** the record is acknowledged with no state change and no DLQ record
- **VERIFIED BY**: unit `team-promotion/internal/consumer/newuser_test.go` `TestIgnoresOtherTypes`

#### Scenario: Config boundary values
- **WHEN** the service boots with `NEW_USER_WELCOME_AMOUNT=0`, a negative amount, `NEW_USER_MIN_SPEND` negative, or `NEW_USER_VOUCHER_TTL`, `NEW_USER_VERDICT_TIMEOUT` or `VOUCHER_RESERVATION_TTL` of 0
- **THEN** startup fails with a message naming the variable; with the defaults it boots
- **VERIFIED BY**: unit `team-promotion/internal/config/config_test.go` `TestNewUserConfigValidate`

#### Scenario: Migration 0008 keeps existing vouchers unchanged
- **WHEN** migration 0008 runs on a database that holds vouchers created before it
- **THEN** each existing voucher has kind 1, empty `owner_user_id`, `per_user_limit` 0, `new_user_only` false, each existing `reserved` reservation has `expires_at` = `created_at` + 15m, and the down migration restores the 0007 schema
- **VERIFIED BY**: `team-promotion/internal/store/migration_test.go` `TestMigration0008UpDown`
