# q7-referral-welcome-contract Specification

## Purpose
TBD - created by archiving change q7-referral-welcome-contract. Update Purpose after archive.

## Requirements

### Requirement: Identity carries the referral code and announces UserRegistered

`AuthService` SHALL gain `GetMyAccount(GetMyAccountRequest) returns (GetMyAccountResponse)` with `GetMyAccountRequest {}` (no fields) and `GetMyAccountResponse {user_id=1 (string), created_at=2 (google.protobuf.Timestamp)}`; its comment SHALL state it is principal-scoped (the caller sees only their own account, no request field selects a user) and routed at the gateway for any signed-in user. `RegisterRequest` in `platform/identity/v1/identity.proto` SHALL gain `string referral_code = 4`, accepted on the public Register RPC (no principal; the code grants no role or access); `username=1`, `password=2`, `role=3` SHALL be unchanged. The file SHALL define `message UserRegistered {user_id=1 (string), referral_code=2 (string), registered_at=3 (google.protobuf.Timestamp)}`. The comments SHALL state: `referral_code` is optional and never fails Register; `UserRegistered` is published on `identity.events`, key = user id, `EventEnvelope.type` "platform.identity.v1.UserRegistered", through the outbox in the same transaction as the user row, once per registration, with an empty `referral_code` when none was supplied; consumers are idempotent on the event id. `UserRegistered` and the referral events are internal-only and never exposed at the gateway.

#### Scenario: Q7 referral welcome contract: RegisterRequest carries an optional referral code
- **WHEN** the descriptor test reads `RegisterRequest`
- **THEN** fields 1 to 3 equal the committed expectations and `referral_code`=4 is a string
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: UserRegistered message is fixed
- **WHEN** the descriptor test reads `UserRegistered`
- **THEN** it has exactly `user_id`=1 string, `referral_code`=2 string, `registered_at`=3 `google.protobuf.Timestamp`
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: UserRegistered without a code round-trips
- **WHEN** a `UserRegistered` with an empty `referral_code` and a 26-character `user_id` is serialised and parsed
- **THEN** every field parses back equal and `referral_code` is the empty string (the field is absent on the wire)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Old readers skip the referral code
- **WHEN** bytes of a `RegisterRequest` carrying field 4 are parsed with a schema that knows only fields 1-3
- **THEN** parsing succeeds, fields 1-3 are intact and field 4 is skipped without error
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: UserRegistered semantics are documented
- **WHEN** a reader opens the comments of `RegisterRequest.referral_code` and `UserRegistered`
- **THEN** they contain "never fails", `identity.events`, "platform.identity.v1.UserRegistered", "same transaction" and "idempotent"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Missing never-fails note is caught
- **WHEN** the comment helper runs against a fixture `referral_code` comment lacking "never fails"
- **THEN** the helper reports failure (the test expects it), proving a missing note is caught
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: GetMyAccount exists on AuthService
- **WHEN** the descriptor test reads `AuthService`
- **THEN** it lists every existing RPC unchanged plus `GetMyAccount` with request `GetMyAccountRequest` (zero fields) and response `GetMyAccountResponse` (`user_id`=1 string, `created_at`=2 `google.protobuf.Timestamp`)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: GetMyAccount scope is documented
- **WHEN** a reader opens the `GetMyAccount` comment to learn whose account it returns and who may call it
- **THEN** it contains "principal-scoped", "own account", "no request field" and "any signed-in user"
- **VERIFIED BY**: comment-text assertion in `q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: GetMyAccount response round-trips
- **WHEN** a `GetMyAccountResponse` with a `user_id` and `created_at` 2026-11-06T00:00:00Z is serialised and parsed, and one with unset `created_at` is serialised and parsed
- **THEN** the first parses back equal and the second parses back with `created_at` unset
- **VERIFIED BY**: Go test `q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Missing principal-scope note is caught
- **WHEN** the comment helper runs against a fixture `GetMyAccount` comment lacking "principal-scoped"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: comment-text assertion in `q7_contract_compat_test.go` (tasks 2.1)

### Requirement: Referral gains verdict and reward-release events and reward fields

`platform/referral/v1/referral.proto` SHALL define `enum ReferralRejectReason {REFERRAL_REJECT_REASON_UNSPECIFIED=0, REFERRAL_REJECT_REASON_UNKNOWN_CODE=1, REFERRAL_REJECT_REASON_SELF_REFERRAL=2, REFERRAL_REJECT_REASON_REFERRER_CAPPED=3}`, `enum ReferralRewardStatus {REFERRAL_REWARD_STATUS_UNSPECIFIED=0, ..._PENDING=1, ..._RELEASED=2, ..._VOID=3}`, and `ReferralAttributed {referee_id=1, referrer_id=2 (string), attributed_at=3 (Timestamp)}`, `ReferralRejected {referee_id=1 (string), reason=2 (ReferralRejectReason)}`, `ReferralRewardReleased {reward_id=1, referrer_id=2, referee_id=3 (string), amount_xu=4 (int64), released_at=5 (Timestamp)}`. `ReferralReward` SHALL gain `status=5` (`ReferralRewardStatus`) and `release_at=6` (Timestamp) with fields 1-4 unchanged; `GetMyReferralResponse` SHALL gain `pending_xu=4` and `released_xu=5` (int64) with fields 1-3 unchanged. The comments SHALL state: the three events are internal-only (never exposed at the gateway) and go to `referral.events` through the referral outbox, key = referee user id, `EventEnvelope.type` "platform.referral.v1.<Message>"; `ReferralAttributed` and `ReferralRejected` are mutually exclusive verdicts per referee; `ReferralRewardReleased` is emitted once per reward; reward amounts are xu, not đ; `rewards_total` is kept and equals `released_xu`; `RedeemReferral` returns `FAILED_PRECONDITION` for an account older than `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` (default 168h, configured in team-referral), an account with a completed order, a second redeem or a self-referral, and credits nothing immediately.

#### Scenario: Q7 referral welcome contract: Referral enums have fixed names and numbers
- **WHEN** the descriptor test reads `ReferralRejectReason` and `ReferralRewardStatus`
- **THEN** the values are exactly those listed with no other value
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Referral event messages have fixed fields
- **WHEN** the descriptor test reads `ReferralAttributed`, `ReferralRejected` and `ReferralRewardReleased`
- **THEN** each has exactly the listed fields, numbers and types
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Reward and summary additions are fixed
- **WHEN** the descriptor test reads `ReferralReward` and `GetMyReferralResponse`
- **THEN** `status`=5 (`ReferralRewardStatus`), `release_at`=6, `pending_xu`=4 and `released_xu`=5 (int64) exist and every previously existing field keeps its name and number
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Released event round-trips with a large amount
- **WHEN** a `ReferralRewardReleased` with `amount_xu` 9223372036854775807 is serialised and parsed
- **THEN** every field parses back equal, with no int32 truncation
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Rejected event round-trips and tolerates an unknown reason
- **WHEN** a `ReferralRejected` with reason `REFERRAL_REJECT_REASON_REFERRER_CAPPED` round-trips, and bytes whose `reason` varint is 99 are parsed
- **THEN** the first parses equal and the second parses with stored enum number 99 (open enum)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Old readers skip the reward additions
- **WHEN** bytes of a `ReferralReward` and a `GetMyReferralResponse` carrying the new fields are parsed with the old schemas
- **THEN** parsing succeeds, old fields are intact and the new fields are skipped without error
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Referral event semantics are documented
- **WHEN** a reader opens the comments of the three event messages, `ReferralReward`, `GetMyReferralResponse` and `RedeemReferral`
- **THEN** they contain `referral.events`, "key = referee", "platform.referral.v1.", "mutually exclusive", "once per reward", "xu", "equals released_xu", `FAILED_PRECONDITION`, "credits nothing", "168h" and "team-referral"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Referral events are documented as internal-only
- **WHEN** a reader opens the comments of the three referral event messages to learn whether an external caller or another user may receive them
- **THEN** they contain "internal-only" and "never exposed at the gateway", so no caller outside the platform is permitted to read them
- **VERIFIED BY**: comment-text assertion in `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Missing xu note is caught
- **WHEN** the comment helper runs against a fixture `GetMyReferralResponse` comment lacking "xu"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

### Requirement: Promotion gains voucher kind, user-bound fields and ListMyVouchers

`platform/promotion/v1/promotion.proto` SHALL define `enum VoucherKind {VOUCHER_KIND_UNSPECIFIED=0, VOUCHER_KIND_STANDARD=1, VOUCHER_KIND_NEW_USER=2}`. `Voucher` SHALL gain `owner_user_id=15` (string), `kind=16` (`VoucherKind`), `per_user_limit=17` (int32), `new_user_only=18` (bool) with fields 1-14 unchanged. `CreateVoucherRequest` SHALL gain `per_user_limit=10` (int32) and `new_user_only=11` (bool); `UpdateVoucherRequest` SHALL gain `optional int32 per_user_limit=4` and `optional bool new_user_only=5`; existing fields unchanged. `VoucherService` SHALL gain `ListMyVouchers(ListMyVouchersRequest) returns (ListMyVouchersResponse)` with `ListMyVouchersRequest {}` (no fields) and `ListMyVouchersResponse {vouchers=1 (repeated Voucher)}`. The comments SHALL state: `ListMyVouchers` principal is the signed-in buyer, returns only that buyer's own active unexpired user-bound vouchers, and the buyer id is never a request field; `owner_user_id` and `kind` are server-owned and not settable through Create or Update; `per_user_limit` 0 means global quota only; `ValidateAndReserve` refuses a user-bound voucher for any other buyer or after its single use with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE`, refuses per-user exhaustion with `valid=false` and reason `VOUCHER_PER_USER_LIMIT`, and refuses `new_user_only` when the buyer has a completed order with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE`; only an unavailable eligibility check answers gRPC `UNAVAILABLE`.

#### Scenario: Q7 referral welcome contract: VoucherKind has fixed names and numbers
- **WHEN** the descriptor test reads `VoucherKind`
- **THEN** the values are exactly `VOUCHER_KIND_UNSPECIFIED=0`, `VOUCHER_KIND_STANDARD=1`, `VOUCHER_KIND_NEW_USER=2`
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Voucher user-bound fields have fixed numbers
- **WHEN** the descriptor test reads `Voucher`, `CreateVoucherRequest` and `UpdateVoucherRequest`
- **THEN** `Voucher` 15-18, `CreateVoucherRequest` 10-11 and `UpdateVoucherRequest` 4-5 are exactly as listed (Update fields are proto3 optional), and all previously existing fields keep name and number
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: ListMyVouchers exists on VoucherService
- **WHEN** the descriptor test reads `VoucherService`
- **THEN** it lists every existing RPC unchanged plus `ListMyVouchers` with request `ListMyVouchersRequest` (zero fields) and response `ListMyVouchersResponse` (`vouchers`=1 repeated `Voucher`)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Voucher with user-bound fields round-trips
- **WHEN** a `Voucher` with `owner_user_id` set, `kind` NEW_USER, `per_user_limit` 1 and `new_user_only` true is serialised and parsed, and a `Voucher` with none of fields 15-18 is serialised and parsed
- **THEN** the first parses back equal and the second parses back with all four unset (empty owner, zero limit)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Old readers skip the voucher additions and tolerate an unknown kind
- **WHEN** `Voucher` bytes carrying fields 15-18 are parsed with a schema knowing only 1-14, and bytes with `kind` varint 99 are parsed with the new schema
- **THEN** the first parses with fields 1-14 intact and 15-18 skipped, the second parses with stored enum number 99
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Update distinguishes unset from zero
- **WHEN** an `UpdateVoucherRequest` with `per_user_limit` explicitly 0 and `new_user_only` explicitly false is serialised, and one with both unset is serialised
- **THEN** the first carries both fields on the wire with zero values and the second carries neither, so a consumer can tell clear from untouched
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: ListMyVouchers authz and scope are documented
- **WHEN** a reader opens the `ListMyVouchers` comment to learn whose vouchers it returns
- **THEN** it contains "signed-in buyer", "own", "active", "unexpired" and "never a request field"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Server-owned voucher fields are documented
- **WHEN** a reader opens the comments of `Voucher.owner_user_id`, `Voucher.kind` and `CreateVoucherRequest`
- **THEN** they contain "server-owned" and state that Create and Update cannot set them, so no caller can mint a bound voucher
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Voucher refusal reasons are documented
- **WHEN** a reader opens the `ValidateAndReserve` comment
- **THEN** it contains `VOUCHER_NOT_ELIGIBLE`, `VOUCHER_PER_USER_LIMIT`, "valid=false", `UNAVAILABLE`, "any other buyer", "single use" and "completed order"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Missing server-owned note is caught
- **WHEN** the comment helper runs against a fixture `owner_user_id` comment lacking "server-owned"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

### Requirement: Order exposes an internal HasCompletedOrder RPC

`OrderService` in `platform/order/v1/order.proto` SHALL gain `HasCompletedOrder(HasCompletedOrderRequest) returns (HasCompletedOrderResponse)` with `HasCompletedOrderRequest {buyer_id=1 (string)}` and `HasCompletedOrderResponse {has_completed=1 (bool)}`. The RPC comment SHALL state that it is internal service-to-service only (called by team-promotion for `new_user_only` vouchers) and NOT routed at the gateway. Existing RPCs, messages and fields SHALL NOT change.

#### Scenario: Q7 referral welcome contract: HasCompletedOrder RPC exists
- **WHEN** the descriptor test reads `OrderService`
- **THEN** it lists every existing RPC unchanged plus `HasCompletedOrder` with the listed request and response fields
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: HasCompletedOrder response round-trips both values
- **WHEN** `HasCompletedOrderResponse` with `has_completed` true and with false is serialised and parsed
- **THEN** true parses back true, and false parses back false (absent on the wire)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: HasCompletedOrder is documented as internal
- **WHEN** a reader opens the `HasCompletedOrder` comment to learn whether an external caller may reach it
- **THEN** it contains "internal", "service-to-service", "team-promotion" and "NOT routed at the gateway"
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

#### Scenario: Q7 referral welcome contract: Missing internal note is caught
- **WHEN** the comment helper runs against a fixture comment lacking "NOT routed"
- **THEN** the helper reports failure (the test expects it)
- **VERIFIED BY**: Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` (tasks 2.1)

### Requirement: Contract is additive, lint-clean and generated in every language

All changes SHALL be additive: no existing field number, name, type or enum value changes. `make -C platform-core lint-proto breaking` SHALL pass against the base commit before the slice commit, and `make proto` SHALL expose the new symbols in the Go, Python, JVM and TS outputs.

#### Scenario: Q7 referral welcome contract: Change is backward compatible
- **WHEN** `make -C platform-core breaking` runs against the base commit before the slice commit
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core breaking` (tasks 1.5)

#### Scenario: Q7 referral welcome contract: Lint passes
- **WHEN** `make -C platform-core lint-proto` runs
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto` (tasks 1.5)

#### Scenario: Q7 referral welcome contract: Wire-breaking edit is rejected
- **WHEN** `buf breaking` runs on a scratch copy of the proto with one new field renumbered and one existing field retyped
- **THEN** it exits non-zero for both (the mutation is discarded afterwards)
- **VERIFIED BY**: one-off mutation run of `make -C platform-core breaking` (tasks 1.5)

#### Scenario: Q7 referral welcome contract: New contract in every generated language
- **WHEN** `make -C platform-core proto` has run
- **THEN** each of the Go, Python, JVM and TS outputs contains `UserRegistered`, `ReferralAttributed`, `ReferralRewardReleased`, `VoucherKind`, `ListMyVouchers` and `HasCompletedOrder`
- **VERIFIED BY**: grep over generated outputs after `make -C platform-core proto` (tasks 2.2)

### Requirement: Consumer copies match the source

The four changed protos SHALL be copied byte-identical into every repo listed in the proposal, including the new copies for team-referral (identity, order, payment, events) and team-promotion (identity, referral, order, payment); copies SHALL lint with `buf lint`. A missing or drifted copy SHALL fail the check.

#### Scenario: Q7 referral welcome contract: Copies match the source
- **WHEN** the `cmp` loop runs over every (source, copy) pair in the proposal list
- **THEN** every pair is identical and every pair file exists
- **VERIFIED BY**: `platform-core/packages/proto/q7_consumer_copies_test.sh` (tasks 3.1)

#### Scenario: Q7 referral welcome contract: Drifted or missing copy is rejected
- **WHEN** the `cmp` loop runs on a scratch tree with one byte changed in one copy and another copy deleted
- **THEN** it exits non-zero and names both
- **VERIFIED BY**: `q7_consumer_copies_test.sh` self-test on a scratch tree (tasks 3.1)

#### Scenario: Q7 referral welcome contract: Consumer copies lint
- **WHEN** `buf lint` runs in the proto directory of each consumer that has a `buf.yaml`
- **THEN** it exits 0 in each
- **VERIFIED BY**: `q7_consumer_copies_test.sh` (tasks 3.2)

#### Scenario: Q7 referral welcome contract: Referral has one canonical source
- **WHEN** the check compares `team-referral/proto/platform/referral/v1/referral.proto` with `platform-core/packages/proto/platform/referral/v1/referral.proto`
- **THEN** they are byte-identical, so the copy is not a second source
- **VERIFIED BY**: `q7_consumer_copies_test.sh` (tasks 3.1)

#### Scenario: Q7 referral welcome contract: Consumer protos build
- **WHEN** `buf build` runs in `team-referral/proto` and in `team-promotion/proto` after the new copies are in place
- **THEN** it exits 0 in both (every import resolves)
- **VERIFIED BY**: `platform-core/packages/proto/q7_consumer_copies_test.sh` (tasks 3.2)
