## Why

Initiative `monetization`: once team-payment books a 5% platform commission at settlement
(`payment-settlement-commission`), the seller must see what they earned. Today the seller order page
(`/seller/orders/[id]`) shows only the buyer-paid total, and the wallet page (`/seller/wallet`) prints raw ledger
type strings (`team-frontend-seller/src/app/seller/wallet/page.tsx:33`), so a COMMISSION row would read
"COMMISSION" next to a negative amount with no explanation.

initiative: monetization

## What Changes

- `src/lib/gateway/payment.ts` gains `getOrderSettlement(orderId)` over `PaymentService.GetOrderSettlement`
  (already routed by team-gateway in `payment-settlement-commission`), returning a view model
  `{ credited, commission, commissionReversed, refundDeducted, net, commissionRatePercent }` or a typed
  `not-settled` / `error` result. No other wrapper changes.
- The order detail page renders a "Đối soát thanh toán" card: gross credited ("Thực nhận trước phí"),
  commission with its rate, commission returned by refunds, refunds deducted, and net ("Thực nhận sau phí"),
  all through `formatPrice` (integer VND). The card is fetched only after the existing owner check passes.
- The wallet ledger Table shows Vietnamese labels for every `WalletTransactionType`
  (ORDER_SETTLEMENT, PAYOUT, REFUND_DEDUCTION, COMMISSION, COMMISSION_REVERSAL, SUBSCRIPTION_FEE, AD_SPEND);
  an unknown type falls back to the raw string.
- Out of scope: plan page and ads page (`seller-plan-ui`, `seller-ads-ui`), any commission computation (the UI only
  displays what the payment service returns), COD orders (never settle, so they show the not-settled state).

## Capabilities

### New Capabilities
none

### Modified Capabilities
- `ui-seller`: adds the settlement breakdown on the order detail page and Vietnamese ledger type labels.

## Impact

- `team-frontend-seller/src/lib/gateway/payment.ts` (+ unit test), `src/app/seller/orders/[id]/page.tsx`,
  new `src/features/seller/OrderSettlementCard.tsx`, `src/app/seller/wallet/page.tsx`,
  new `src/features/seller/ledgerLabels.ts`, `FEATURES.yaml` (written by the e2e author).
- `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` and its steps.
- Depends on `payment-settlement-commission` (RPC and gateway route); no proto change here.

## NFR

- The breakdown adds one server-side `GetOrderSettlement` call to the order detail render, issued in parallel with
  existing reads; p95 added latency <= 50 ms locally. A failed or slow (> 2 s timeout) call never blocks the page:
  the rest of the order renders and only the card shows an error state.
- Money is integer VND (int64 from proto converted with `Number`, safe below 2^53); no client-side recomputation of
  commission, rate or net. Display only.
- Zero layout shift: the card has a fixed skeleton height in `loading.tsx`-style Suspense fallback (existing rule
  `Seller pages follow the zero-layout-shift rules`).

## Data classification

- Order id and seller's own monetary amounts (credited, commission, refunds, net). Purpose: show the seller their own
  earnings. Basis: contract performance (marketplace terms), same as the existing wallet page. Retention: none added;
  values are fetched per request, never stored or cached by the frontend (`force-dynamic`, no client persistence).
- No buyer personal data is added to the page or logged. Never log raw seller or order ids (sloglint rule applies to
  server logs; the frontend logs only the outcome class).

## Threat model

- Assets: per-order commission and net (seller revenue data).
- Abuse cases: a seller reading another seller's order settlement by guessing an order id; a guest reading it; the UI
  trusting a tampered rate.
- Authz rule: the settlement is requested only after `loadSellerOrder` + `isOwnedBy` pass (order seller or
  `order.admin`); otherwise the page is `notFound()` and no payment call is made. Server-side, `GetOrderSettlement`
  returns NOT_FOUND for anyone else (defence in depth); the UI maps NOT_FOUND to the not-settled state, never to
  an error that leaks existence. The rate shown comes from the response (`commission_rate_bps`), not from config.
