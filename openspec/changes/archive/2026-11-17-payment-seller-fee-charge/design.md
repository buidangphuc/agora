## Context

`team-payment` already has `LedgerRepository.AppendDebit` (per-seller advisory lock `lockSeller`, balance sum,
`CheckWithdrawable` against the hold-back, insert) used by payouts, and `insertLedgerOnce` unique on
`(type, reference_id)`. Errors map through `grpcx.Rule` in `internal/rpc/payment.go` (`ErrInsufficientBalance` and
`*FundsOnHoldError` already map to `FAILED_PRECONDITION`).

## Decisions

- D1 Idempotent debit in one transaction. A new store method (`AppendDebitOnce`) takes the seller lock, first looks up
  `(SUBSCRIPTION_FEE, reference_id)`; if found it returns that entry with `created=false` without checking funds
  (a replay after the balance dropped still succeeds as a replay); otherwise it runs the same
  balance/withdrawable check as `AppendDebit` and inserts. The unique index is the backstop. Why: a renewal retry must
  never fail or double-charge.
- D2 Reference reuse with different parameters. If the existing entry's seller or amount differs from the request,
  return `ALREADY_EXISTS` and book nothing (a key collision is a caller bug, not a replay).
- D3 Validation order: principal type (`PERMISSION_DENIED`), then `seller_id` non-empty, `amount > 0`, kind
  `SUBSCRIPTION` (`UNSPECIFIED` and `AD_SPEND` are `INVALID_ARGUMENT`), `reference_id` 1..64 chars (column is
  `VARCHAR(64)`; `sub:<uuid>:<unix seconds>` is 51 chars), all `INVALID_ARGUMENT`.
- D4 Migration. A new migration (number = next free after 0017, `0018` unless `payment-settlement-commission` landed
  first) drops and re-adds `wallet_ledger_type_sign_chk` and `wallet_ledger_reference_required_chk` and must carry
  every type already allowed at that point plus `SUBSCRIPTION_FEE AND amount < 0` (reference required). `AD_SPEND` is
  not allowed by the CHECK in v1. Down migration restores the previous constraint (fails if SUBSCRIPTION_FEE rows
  exist; documented in the file header). Hot file shared with `payment-settlement-commission`: build serialised.
- D5 Testability. `ChargeSellerFee` is service-only and the gateway does not expose it, so a pure e2e trigger does not
  exist in this slice. The observable effects (ledger row, balance, FAILED_PRECONDITION, idempotency) are verified by
  Docker-backed Go integration tests in `team-payment` (`GO_SDK_REQUIRE_DOCKER=1`) that call the gRPC handler with a
  service principal; the scenarios name them with `VERIFIED BY` and their FEATURES entries are `not-testable`. The
  end-to-end path (subscribe -> fee row visible in the seller's ledger through the gateway) is owned by
  `promotion-plan-billing`.
- D6 Wire type string. The ledger entry type is carried as the string `SUBSCRIPTION_FEE` like the other types; the
  entry mapper in `internal/rpc/wallet_ledger.go` must surface it (and the enum value 6) without error.
