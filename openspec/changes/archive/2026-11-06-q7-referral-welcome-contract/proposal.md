## Why
The q7-q37-referral-welcome initiative attributes a signup to an inviter without a typed code, issues exactly one new-user voucher per account, pays the inviter a pending-then-released xu reward, and shows the buyer their own vouchers. Identity, order, referral, promotion and frontend slices all need the same wire contract first. This slice adds `RegisterRequest.referral_code` and the `UserRegistered` event, the referral verdict and reward-release events with their enums and fields, `VoucherKind` and the user-bound voucher fields, `ListMyVouchers`, and the internal `HasCompletedOrder` RPC, plus the proto copies consumers carry. Behaviour lives in `identity-user-registered`, `order-has-completed-order`, `referral-attribution-reward`, `promotion-new-user-vouchers`, `frontend-referral-invite` and `frontend-welcome-checklist`. This slice merges FIRST.

initiative: q7-q37-referral-welcome

## What changes
All edits are additive; no existing RPC, message, field or enum value changes or moves. Field names, numbers and enum values follow the Contracts section of `plans/q7-q37-referral-welcome/00-spec.md` (Locked). Where the spec names no number, this slice assigns the next free one (stated below and in design.md).

Canonical source: `platform-core/packages/proto/platform/...`. `team-referral/proto/platform/referral/v1/referral.proto` is a byte-identical consumer copy (it equals the source today), not a second source of truth; it is replaced, never edited by hand.

`platform/identity/v1/identity.proto`:
- `RegisterRequest.referral_code = 4` (string). Comment: accepted on the public `Register` RPC (no principal; the code grants no role or access); optional; an empty, unknown or self-owned code never fails `Register`.
- `AuthService` gains `rpc GetMyAccount(GetMyAccountRequest) returns (GetMyAccountResponse)`; `GetMyAccountRequest {}`; `GetMyAccountResponse { string user_id = 1; google.protobuf.Timestamp created_at = 2; }`. Comment: principal-scoped, the caller sees only their own account and no request field selects another user; routed at the gateway for any signed-in user (the frontend uses `created_at` for the 14-day welcome rule).
- `message UserRegistered { string user_id = 1; string referral_code = 2; google.protobuf.Timestamp registered_at = 3; }`. Comment: internal-only event (never exposed at the gateway); published on `identity.events`, key = user id, `EventEnvelope.type` "platform.identity.v1.UserRegistered", through the identity outbox in the same transaction as the user row, once per registration; `referral_code` is empty when none was supplied; consumers are team-referral and team-promotion and must be idempotent on the event id.

`platform/referral/v1/referral.proto`:
- `enum ReferralRejectReason { REFERRAL_REJECT_REASON_UNSPECIFIED = 0; REFERRAL_REJECT_REASON_UNKNOWN_CODE = 1; REFERRAL_REJECT_REASON_SELF_REFERRAL = 2; REFERRAL_REJECT_REASON_REFERRER_CAPPED = 3; }` (the spec lists the short names `UNKNOWN_CODE`, `SELF_REFERRAL`, `REFERRER_CAPPED`; buf lint requires the enum-name prefix, see design.md).
- `enum ReferralRewardStatus { REFERRAL_REWARD_STATUS_UNSPECIFIED = 0; REFERRAL_REWARD_STATUS_PENDING = 1; REFERRAL_REWARD_STATUS_RELEASED = 2; REFERRAL_REWARD_STATUS_VOID = 3; }` (short names `PENDING`, `RELEASED`, `VOID`).
- `message ReferralAttributed { referee_id = 1; referrer_id = 2; attributed_at = 3 (Timestamp) }`, `message ReferralRejected { referee_id = 1; ReferralRejectReason reason = 2 }`, `message ReferralRewardReleased { reward_id = 1; referrer_id = 2; referee_id = 3; int64 amount_xu = 4; released_at = 5 (Timestamp) }`. Comments: internal-only events (never exposed at the gateway); published on `referral.events`, key = referee user id, `EventEnvelope.type` "platform.referral.v1.<Message>", through the referral outbox; `ReferralAttributed` and `ReferralRejected` are mutually exclusive verdicts per referee; `ReferralRewardReleased` once per reward.
- `ReferralReward.status = 5` (`ReferralRewardStatus`), `ReferralReward.release_at = 6` (Timestamp). Comment: amounts are xu, not đ.
- `GetMyReferralResponse.pending_xu = 4`, `GetMyReferralResponse.released_xu = 5` (int64). Comment: `rewards_total` is kept and equals `released_xu`; all three are xu.
- `RedeemReferral` comment gains: refused with `FAILED_PRECONDITION` for an account older than `REFERRAL_REDEEM_MAX_ACCOUNT_AGE` (default 168h, configured in team-referral), an account with a completed order, a second redeem or a self-referral; it credits nothing immediately.

`platform/promotion/v1/promotion.proto`:
- `enum VoucherKind { VOUCHER_KIND_UNSPECIFIED = 0; VOUCHER_KIND_STANDARD = 1; VOUCHER_KIND_NEW_USER = 2; }` (short names `STANDARD`, `NEW_USER`).
- `Voucher` gains `owner_user_id = 15` (string, empty = not user-bound), `kind = 16` (`VoucherKind`), `per_user_limit = 17` (int32, 0 = global quota only), `new_user_only = 18` (bool).
- `CreateVoucherRequest` gains `per_user_limit = 10` (int32), `new_user_only = 11` (bool). `UpdateVoucherRequest` gains `optional int32 per_user_limit = 4`, `optional bool new_user_only = 5`. Comments: admin-only like the existing fields; `owner_user_id` and `kind` are server-owned (never settable through Create or Update).
- `VoucherService` gains `rpc ListMyVouchers(ListMyVouchersRequest) returns (ListMyVouchersResponse)`; `ListMyVouchersRequest {}`; `ListMyVouchersResponse { repeated Voucher vouchers = 1; }`. Comment: principal = the signed-in buyer; returns only that buyer's own active, unexpired user-bound vouchers; the buyer id comes from the token and is never a request field.
- `ValidateAndReserve` comment gains: a user-bound voucher used by any other buyer or after its single use, a per-user-limit exhaustion, and a `new_user_only` voucher for a buyer with a completed order are refused with `valid=false` and reason `VOUCHER_NOT_ELIGIBLE` (first and third) or `VOUCHER_PER_USER_LIMIT` (second), the same shape as today's refusals; only an unavailable eligibility check answers gRPC `UNAVAILABLE` (fail closed).
- The spec says `PromotionService`; the proto's service is `VoucherService` (see OPEN_ISSUES). `ListMyVouchers` goes into `VoucherService`.

`platform/order/v1/order.proto`:
- `OrderService` gains `rpc HasCompletedOrder(HasCompletedOrderRequest) returns (HasCompletedOrderResponse)`; `HasCompletedOrderRequest { string buyer_id = 1; }`, `HasCompletedOrderResponse { bool has_completed = 1; }`. Comment: internal, service-to-service only (called by team-promotion for `new_user_only`), NOT routed at the gateway.

Copies (byte-identical to the source; consumers regenerate in their own slices and never edit a copy):
- `identity.proto` into every existing copy: team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-identity, team-notification, team-order, team-payment, team-search.
- `order.proto` into every existing copy: team-analytics, team-chat, team-domain, team-engagement, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-notification, team-order, team-payment, team-search.
- `promotion.proto` into: team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-order, team-promotion.
- `referral.proto` into: team-analytics, team-frontend, team-frontend-admin, team-frontend-seller, team-gateway, team-referral.
- New copies needed so the consumers compile: team-referral gets `identity/v1/identity.proto`, `order/v1/order.proto`, `payment/v1/payment.proto` and `events/v1/events.proto` (it consumes `identity.events` and `order.events`; `order.proto` imports identity and payment); team-promotion gets `identity/v1/identity.proto`, `referral/v1/referral.proto`, `order/v1/order.proto` and `payment/v1/payment.proto` (it consumes `identity.events` and `referral.events` and calls `HasCompletedOrder`).

`make proto` for team-referral and team-promotion must build with the new copies in place (`buf build` check below).

A committed Go test `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go` pins the contract (same dynamic-descriptor approach as `promotion/v1/flashstock_compat_test.go`); it reads the identity, referral, promotion and order protos from sibling directories through one `buf build`.

Out of scope: any runtime behaviour (outbox rows, consumers, sweeper, caps, voucher issue, UI), gateway routes for `ListMyVouchers` and for `RegisterRequest.referral_code` pass-through (owned by promotion-new-user-vouchers and identity-user-registered), the `HasCompletedOrder` implementation (order-has-completed-order), renumbering any existing field.

## NFR
- Additive only: `buf breaking` passes with baseline HEAD after the slice commit (the pre-commit run compares against the base from main; the mutation check runs after the commit, against HEAD). Consumers unaware of the additions keep working: unknown fields are skipped and unknown enum values parse as numbers.
- No new service, port or dependency. Contract-only: no runtime latency or availability impact. Initiative runtime NFRs (register latency unchanged, voucher within 10 s, sweeper within 2 x interval) are owned by the behaviour slices.
- Limits fixed by the contract: xu and money are int64; timestamps are UTC `google.protobuf.Timestamp`; `per_user_limit` is int32 and 0 means unlimited per user.
- Compat: old bytes of existing messages are unchanged; a new `Voucher` parses in an old schema (fields 15-18 skipped).

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `RegisterRequest.referral_code`, `UserRegistered.referral_code` | pseudonymous code (identifies the inviter indirectly) | attribute the signup to the inviter | contract performance (the invite programme the user opted into by signing up through the link); DPO sign-off is NEEDS_HUMAN | kept with the referral row in team-referral; the identity outbox row follows the topic retention |
| `UserRegistered.user_id`, `registered_at` | pseudonymous id, timestamp (existing data) | route the new-user voucher and referral consumers | contract performance | `identity.events` topic retention |
| `referee_id`/`referrer_id` in `referral.events` | pseudonymous ids | reward accounting, voucher verdict | contract performance | topic retention (key = referee id) |
| `amount_xu`, `pending_xu`, `released_xu`, reward `status`, `release_at` | business data (mock ledger, no real money) | show and release rewards | contract | lives with the reward row |
| `Voucher.owner_user_id` | pseudonymous id | bind a voucher to one buyer | contract performance | until voucher expiry plus the order retention policy |
| `kind`, `per_user_limit`, `new_user_only` | non-personal config | eligibility rules | n/a | lives with the voucher |
| `HasCompletedOrder.buyer_id` | pseudonymous id (internal RPC) | new-user eligibility | contract | not stored by the callee |

No new class of personal data: no name, address, phone or payment instrument is added. Logs in consumers use `principal_fp`/`key_fp` only, never raw ids or codes.

## Threat model
- Assets: voucher integrity (a user-bound voucher must not be spent by someone else); reward integrity (no double or farmed rewards, caps); the invite attribution (a code must not break signup); privacy of who invited whom.
- Abuse cases:
  - A client sets `owner_user_id` or `kind` through Create/Update to mint a bound voucher (the contract marks both server-owned; enforcement in promotion-new-user-vouchers).
  - A caller reads another buyer's vouchers (`ListMyVouchers` has no buyer field; the principal comes from the token).
  - An external caller reaches `HasCompletedOrder` through the edge to probe whether any user ever ordered (documented internal, not routed; the gateway-absence test lives in order-has-completed-order).
  - A crafted `referral_code` makes registration fail or enumerates inviters (documented: never fails Register; the verdict is only on the internal topic).
  - A consumer copy drifts from the source (mitigated by the `cmp` check); a renumbered field reinterprets old data (mitigated by `buf breaking` and the pinned descriptor test).
- Authz rule: `ListMyVouchers` is callable by any authenticated buyer for their own vouchers only; `HasCompletedOrder` is service-to-service only; Create/Update voucher stay admin-only. This slice documents the rules; enforcement belongs to the owning slices.
