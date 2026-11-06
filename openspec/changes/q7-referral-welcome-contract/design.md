# Design

## Clarifications
### Round 1 — 2026-11-06
- Canonical source is `platform-core/packages/proto/platform/referral/v1/referral.proto`; `team-referral/proto/platform/referral/v1/referral.proto` is a byte-identical copy today (checked with diff) and is replaced like every other copy.
- buf lint requires enum value names prefixed with the enum name, so the spec's short names (`UNKNOWN_CODE`, `PENDING`, `NEW_USER`, `STANDARD`) become `REFERRAL_REJECT_REASON_UNKNOWN_CODE`, `REFERRAL_REWARD_STATUS_PENDING`, `VOUCHER_KIND_NEW_USER` and so on. Numbers are exactly the spec's. Same convention as `CANCEL_REASON_*` in the q10 contract.
- The spec writes `PromotionService.ListMyVouchers`; the proto has no `PromotionService`, the voucher RPCs live in `VoucherService`. `ListMyVouchers` is added there.
- Field numbers the spec leaves open are the next free ones: `Voucher` 15-18, `CreateVoucherRequest` 10-11, `UpdateVoucherRequest` 4-5 (optional, so Update can tell "unset" from 0/false), `ReferralReward` 5-6, `GetMyReferralResponse` 4-5.
- `ListMyVouchers` has no pagination (spec: empty request); a buyer holds at most one `NEW_USER` voucher.
- Refusal reasons `VOUCHER_NOT_ELIGIBLE` and `VOUCHER_PER_USER_LIMIT` are `valid=false` + `reason` (clarification C3), the same shape as today's refusals; only an unavailable `HasCompletedOrder` answers gRPC `UNAVAILABLE` (fail closed). No `FAILED_PRECONDITION`/`ErrorInfo` for these two.
- `GetMyAccount` (C1) is added to `AuthService`: principal-scoped, response `{user_id=1, created_at=2}`, routed at the gateway for any signed-in user; team-identity implements it.
- `HasCompletedOrder` is internal: the proto cannot enforce it, so the comment states it and gateway non-routing is tested in order-has-completed-order.
- Test location: `platform-core/packages/proto/platform/referral/v1/q7_contract_compat_test.go`, dynamic-descriptor approach from the promotion flash-stock test.
