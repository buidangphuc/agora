## 1. Code track (team-frontend-seller)

- [x] 1.1 Tests first: `src/lib/gateway/payment.test.ts` cases for `getOrderSettlement` mapping, NOT_FOUND to not-settled, UNAVAILABLE to error, int64 precision (Breakdown of a settled order, Unsettled order shows the not-settled state, Large amounts keep integer precision); confirm red
- [x] 1.2 Add `getOrderSettlement(orderId)` and `ViewOrderSettlement` to `src/lib/gateway/payment.ts` with a 2 s timeout; rate percent = bps / 100 (Breakdown of a settled order, Zero commission is shown, not hidden, Settlement service failure degrades only the card)
- [x] 1.3 Tests first: `OrderSettlementCard.test.tsx` for settled, partial refund, zero commission, not-settled, error (Breakdown of a settled order, Breakdown after a partial refund, Zero commission is shown, not hidden, Unsettled order shows the not-settled state)
- [x] 1.4 `src/features/seller/OrderSettlementCard.tsx` using `formatPrice`, Card and Descriptions components, fixed-height skeleton (Breakdown of a settled order, Settlement service failure degrades only the card)
- [x] 1.5 Wire the card into `src/app/seller/orders/[id]/page.tsx` after `isOwnedBy`, in parallel with existing reads; extend `page.test.tsx` (Another seller's order is not found and nothing is fetched, Settlement service failure degrades only the card)
- [x] 1.6 `src/features/seller/ledgerLabels.ts` + `ledgerLabels.test.ts`; use it in the `/seller/wallet` Type column; add `wallet.test.tsx` (Every known type has a label, Unknown or empty type falls back, Ledger load failure keeps the existing error state)
- [x] 1.7 `npm run check` in team-frontend-seller green; no new dependency

## 2. e2e track (platform-e2e)

- [x] 2.1 `tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` with scenarios Breakdown of a settled order, Breakdown after a partial refund, Unsettled order shows the not-settled state (COD order), Another seller's order is not found, Guest cannot see the breakdown, Commission rows are labelled; data comes from real order, payment and refund flows, no route fulfils
- [x] 2.2 Step definitions for the feature, reusing existing seller login, order creation and wallet steps
- [x] 2.3 FEATURES.yaml entry for team-frontend-seller (written by the e2e author)
- [x] 2.4 `openspec validate seller-fee-breakdown-ui --strict`; run the feature on a dev env after `payment-settlement-commission` is deployed
