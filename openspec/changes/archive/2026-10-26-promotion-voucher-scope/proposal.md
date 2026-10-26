## Why

Initiative `q11-checkout-coherence` (Locked 2026-10-16) makes team-order quote and charge a multi-shop checkout: each shop voucher discounts only its own shop's group, and one platform voucher is computed once on the selection subtotal and split across groups. To refuse a shop code sent in the platform slot (or the reverse; the legacy `voucher_code` is treated as the platform slot only, with no SHOP auto-routing probe, per decision M5), team-order must know what kind of voucher a code is without an extra `GetVoucher` round-trip. `ValidateAndReserveResponse` therefore echoes the voucher's `scope` and `seller_id` (added by `q11-checkout-contract` as fields 5 and 6). This slice is the team-promotion side: fill those two fields on every `ValidateAndReserve` outcome and pin the existing scope rules (a shop voucher only validates for its own seller, a platform voucher with empty `seller_id` validates against the subtotal sent) with tests, so team-order's `order-multishop-checkout` can rely on them.

initiative: q11-checkout-coherence

## What Changes

- **Service** (`team-promotion/internal/service/voucher.go`): `ReserveResult` gains `Scope int32` and `SellerID string`. `ValidateAndReserve` fills them from the stored voucher on every path where the voucher is known: a new valid hold, a rejection (expired, below `min_spend`, quota exhausted, not valid for this seller), an idempotent replay (looked up through the hold's `VoucherID` with `VoucherRepository.GetByID`, including a replay of a released hold). Where no voucher is known (empty code, unknown code, empty `reservation_id`) scope is `VOUCHER_SCOPE_UNSPECIFIED` and `seller_id` is empty. If the replayed hold's voucher can no longer be loaded (`ErrVoucherNotFound`) the replay keeps its stored outcome with scope unspecified; other repository errors stay `INTERNAL`.
- **Handler** (`internal/handler/voucher.go`): `ValidateAndReserve` copies `Scope` and `SellerId` into `ValidateAndReserveResponse` for both modes (service principal and the user preview namespace). Authorisation, preview-namespace rule and the idempotency key are unchanged.
- **Unchanged on purpose**: discount math, `validate` order and reason strings, quota counting, commit/release, `ListVouchers`, `CreateVoucher`, migrations (scope and seller are already stored on `vouchers`), the gateway, team-order. The platform voucher is validated by team-order as one call with empty `seller_id` on the whole selection subtotal; promotion needs no new rule for it because `min_spend` is already compared with `cart_subtotal` and a PLATFORM voucher ignores `seller_id`. Proration across groups is team-order's job.
- **Generated Go** from the proto copy the contract slice vendors under `team-promotion/proto/` (`make proto`, never hand-edited).
- **README** (`team-promotion/README.md`): one paragraph in the voucher section naming the two echoed fields.
- **platform-e2e**: feature `features/promo/voucher_scope.feature`, steps `step_definitions/voucher_scope_steps.py`, helper `support/voucher_scope_support.py` (service-principal and user-principal `ValidateAndReserve` through `grpcurl` from a throwaway container, same pattern as `flash_hold_support.py`; vouchers seeded through `VoucherService.CreateVoucher`, shop voucher as a seller, platform voucher as admin). `FEATURES.yaml` is written later by the e2e-author.

## NFR

- Latency: at most one extra primary-key read (`GetByID`) on the idempotent-replay path; the first-call path already loads the voucher. Non-gated target only: `ValidateAndReserve` p95 unchanged (under 50 ms on the local stack), which supports the initiative's `PreviewCheckout` p95 of 300 ms for up to 5 groups.
- Availability: no new upstream dependency. Behaviour of the existing audit and flag paths is unchanged.
- Limits: unchanged (`reservation_id` required; `cart_subtotal` int64 VND). Backward compatible: callers that ignore fields 5 and 6 see no change.

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `scope`, `seller_id` (response) | business data (public voucher attributes, already listed by `ListVouchers`) | let team-order route a voucher to a shop group or to the platform slot | contract performance (order processing) | not stored by this slice; echoed per call |
| `buyer_id` (request, existing) | personal identifier | quota and hold bookkeeping | contract performance | unchanged, stored on the hold as today |

No new personal data is collected or stored. Logs carry reservation ids and codes only, never buyer fields.

## Threat model

- **Assets:** voucher quota and discount integrity, the `promotion.reserve` service credential, preview-namespace isolation.
- **Abuse cases:**
  - A buyer applies shop A's voucher to shop B's group to get a discount from the wrong shop: `validate` refuses with "voucher not valid for this seller"; the response still reports the voucher's own scope and seller so team-order can show the right reason.
  - A buyer enters a shop code in the platform slot (or the reverse) to bypass `min_spend` or scope: team-order compares the echoed `scope` with the slot; promotion guarantees the echo is the stored scope, never a request value.
  - Enumeration of shop ids by guessing codes: the echoed `seller_id` is already public through `ListVouchers`, so nothing new is disclosed; unknown codes echo nothing.
  - A buyer consumes quota through the preview path: unchanged, preview holds stay in the caller's `preview:<id>:` namespace and cannot be committed by a user.
  - Replay with changed fields: the stored hold wins, so the replayed scope is the stored voucher's, never the new request's.
- **Authz rule (unchanged):** a SERVICE principal with scope `promotion.reserve` is trusted; any other authenticated principal is bound to itself and to its preview namespace (`PERMISSION_DENIED` outside it); anonymous is `UNAUTHENTICATED`; a service without the scope is `PERMISSION_DENIED`.
