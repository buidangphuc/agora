## Why

Seller plans (initiative `monetization`, story 2) cost money, and the only wallet owner is `team-payment`. Today the
ledger knows three entry types (ORDER_SETTLEMENT, PAYOUT, REFUND_DEDUCTION; `team-payment/internal/store/ledger.go`,
type/sign CHECK in `migrations/0006_wallet_ledger_integrity.up.sql`) and has no way to debit a seller for a platform
fee. `team-promotion` (slice `promotion-plan-billing`) needs one service-to-service call that debits a seller's
withdrawable balance exactly once per billing period, with no card checkout.

initiative: monetization

## What Changes

- New `PaymentService.ChargeSellerFee` handler in `team-payment` (contract from `monetization-contract`):
  service-principal only, validates the request, books one `SUBSCRIPTION_FEE` debit of `-amount` for the seller, and is
  idempotent on `reference_id` (a replay returns the original entry with `created=false` and books nothing).
- The debit draws only on the withdrawable balance, the same rule as payouts: it runs under the per-seller advisory lock
  with the payout hold-back (`AppendDebit` semantics), so concurrent fee charges and payouts cannot overdraw. An amount
  above the withdrawable balance (insufficient balance, or proceeds still inside the hold window) is
  `FAILED_PRECONDITION` and nothing is booked.
- New ledger type `SUBSCRIPTION_FEE` (amount < 0, reference required, unique on `(type, reference_id)`): a new
  migration (next free number after 0017; `0018` unless `payment-settlement-commission` already took it) extends the
  `wallet_ledger_type_sign_chk` and `wallet_ledger_reference_required_chk` constraints. `AD_SPEND` is reserved in the
  enum and rejected: `SELLER_FEE_KIND_AD_SPEND` is `INVALID_ARGUMENT`, no `AD_SPEND` row is written (ad budget is a
  campaign counter in v1).
- The entry shows in the seller's `ListLedgerEntries` as type `SUBSCRIPTION_FEE` with a negative amount and lowers
  `GetWalletBalance`.
- `team-gateway` does NOT forward `ChargeSellerFee`; no frontend change in this slice.

## Capabilities

### New Capabilities
- `seller-fee-charge`: platform fees debited from a seller's withdrawable wallet balance, idempotent per reference.

### Modified Capabilities
none (the `seller-payout-holdback` and `seller-settlement-credit` requirements are unchanged; the fee reuses the
hold-back rule rather than altering it).

## Impact

- `team-payment`: `internal/rpc` (new handler + error mapping), `internal/payment` (service method), `internal/store`
  (`ledger.go` type constant, idempotent debit under the seller lock; hot file shared with
  `payment-settlement-commission`), `migrations/` (hot file, CHECK extension), `FEATURES.yaml` untouched here.
- `platform-e2e`: scenarios that can run through the gateway are limited; see design D5.
- Depends on `monetization-contract` for `ChargeSellerFee`, `SellerFeeKind` and `WALLET_TRANSACTION_TYPE_SUBSCRIPTION_FEE`.

## Non-goals

No card checkout, invoice, VAT, or bank movement. No `AD_SPEND` write. No plan logic (price, term, renewal: that is
`promotion-plan-billing`). No refund of a fee in v1. No gateway exposure.

## Non-functional requirements

- `ChargeSellerFee` p95 <= 50 ms locally (one transaction: lock, sum, insert), single seller.
- Idempotent: any number of calls with one `reference_id` book at most one entry, including concurrent calls.
- No overdraw: balance and withdrawable amount never go below the charged amount under concurrent fee charges and
  payouts for one seller. Ledger reconciles: balance = sum of entries, zero drift.
- Availability: stateless handler; failure is returned to the caller (never swallowed), the caller retries with the
  same `reference_id`.
- Money is integer VND (int64); `amount` is > 0 and the stored entry is `-amount`.

## Data classification

| field | class | purpose | consent / basis | retention |
|---|---|---|---|---|
| `seller_id` on the ledger row | personal (account identifier) | attribute the fee to the seller's wallet | existing ledger purpose (payouts and billing), contract performance | same as the ledger (financial record, no deletion in v1) |
| `amount`, `reference_id` (`sub:<subscription_id>:<period_start>`) | business / financial, not personal | bookkeeping and idempotency | contract performance | same as the ledger |

Logs and metrics never carry the raw `seller_id` or `reference_id` (sloglint forbidden-keys, key fingerprint helper
only).

## Threat model

- Assets: the seller's wallet balance and ledger integrity.
- Abuse cases: (1) a seller or buyer calls `ChargeSellerFee` directly to debit another seller or to forge a fee;
  (2) replay or double-delivery of a renewal charge debits twice; (3) concurrent charge and payout overdraw the wallet;
  (4) a fee is charged against funds still on hold; (5) a caller reuses a `reference_id` with a different amount to
  smuggle a different debit; (6) `AD_SPEND` is used to drain a wallet.
- Authz rule: only a principal of type `service` may call `ChargeSellerFee`; user (seller, buyer, admin) and anonymous
  principals get `PERMISSION_DENIED`. The gateway does not route it. The RPC never takes the seller from a user
  principal, so a seller cannot target any wallet.
- Mitigations: unique `(type, reference_id)` plus an in-transaction lookup (2, 5); per-seller advisory lock with
  withdrawable check (3, 4); kind allow-list (6).
