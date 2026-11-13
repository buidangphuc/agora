## ADDED Requirements

### Requirement: The seller order page shows the settlement breakdown

`/seller/orders/[id]` SHALL show, for an order that has settled into the seller's wallet, a "Đối soát thanh toán" card
with the amount credited, the commission and its rate, the commission returned by refunds, the refunds deducted and
the net, each formatted as integer VND, using only the values returned by `PaymentService.GetOrderSettlement`. The
page SHALL request the settlement only after the existing ownership check passes. For an order that has not settled
the card SHALL say so, and a failed settlement call SHALL affect only the card.

#### Scenario: Breakdown of a settled order

- **WHEN** the owning seller opens `/seller/orders/[id]` of an order paid online with items_subtotal 300 000,
  shipping 30 000 and shop voucher 20 000 (credited 310 000, commission 14 000 at 500 bps)
- **THEN** the card shows "₫310.000" as credited, "₫14.000" as commission labelled "5%", "₫296.000" as net, and
  commission returned and refunds deducted as "₫0"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Breakdown of a settled order`

#### Scenario: Breakdown after a partial refund

- **WHEN** the owning seller opens the order page after a 40% partial return refund has been paid
  (commission_reversed 5 600, refund_deducted 124 000)
- **THEN** the card shows commission returned "₫5.600", refunds deducted "₫124.000" and net "₫177.600", equal to
  the response `net`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Breakdown after a partial refund`

#### Scenario: Zero commission is shown, not hidden

- **WHEN** the response has commission 0 and commission_rate_bps 0 (an order settled before commission existed)
- **THEN** the card shows commission "₫0" with rate "0%" and net equal to credited
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/OrderSettlementCard.test.tsx` `renders zero commission`

#### Scenario: Unsettled order shows the not-settled state

- **WHEN** the owning seller opens the page of a cash-on-delivery order, whose `GetOrderSettlement` returns NOT_FOUND
- **THEN** the card shows "Đơn chưa được đối soát vào ví" and no amount rows, and the rest of the order page renders
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Unsettled order shows the not-settled state`

#### Scenario: Settlement service failure degrades only the card

- **WHEN** `GetOrderSettlement` fails with UNAVAILABLE or exceeds the 2 s timeout while the order itself loads
- **THEN** the card shows an error alert "Không tải được đối soát thanh toán" with a "Thử lại" link, the order items,
  totals and tabs still render, and the page responds 200
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/orders/[id]/page.test.tsx` `settlement error degrades only the card`

#### Scenario: Another seller's order is not found and nothing is fetched

- **WHEN** a seller opens `/seller/orders/[id]` of an order that belongs to a different seller
- **THEN** the response is the not-found page and no `GetOrderSettlement` call is made
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Another seller's order is not found`

#### Scenario: Guest cannot see the breakdown

- **WHEN** a visitor without a session requests `/seller/orders/[id]`
- **THEN** the visitor is redirected to login and the response contains no settlement amounts
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Guest cannot see the breakdown`

#### Scenario: Large amounts keep integer precision

- **WHEN** the response holds credited 9 007 199 254 740 000 (below 2^53) and commission 450 359 962 737 000
- **THEN** the card renders both exactly as formatted integer VND with no fractional digit
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/payment.test.ts` `maps int64 settlement amounts without rounding`

### Requirement: The wallet ledger shows Vietnamese type labels

The `/seller/wallet` ledger Table SHALL show a Vietnamese label for each wallet transaction type: ORDER_SETTLEMENT
"Thanh toán đơn hàng", PAYOUT "Rút tiền", REFUND_DEDUCTION "Hoàn tiền cho người mua", COMMISSION "Phí hoa hồng",
COMMISSION_REVERSAL "Hoàn phí hoa hồng", SUBSCRIPTION_FEE "Phí gói người bán", AD_SPEND "Chi phí quảng cáo". A type
the UI does not know SHALL be shown as its raw string, and an empty type as "—".

#### Scenario: Commission rows are labelled

- **WHEN** the seller opens `/seller/wallet` after an order settled with commission 14 000
- **THEN** the ledger shows a row "Thanh toán đơn hàng" and a row "Phí hoa hồng" with amount "-₫14.000", and the text
  "COMMISSION" appears in no cell
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_fee_breakdown_ui.feature` `Commission rows are labelled`

#### Scenario: Every known type has a label

- **WHEN** the label function receives each of the seven known type strings
- **THEN** it returns the Vietnamese label listed above for each, and none returns the raw string
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ledgerLabels.test.ts` `labels every known type`

#### Scenario: Unknown or empty type falls back

- **WHEN** a ledger entry has type "FUTURE_TYPE" or an empty type
- **THEN** the cell shows "FUTURE_TYPE" or "—" respectively and the page does not error
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ledgerLabels.test.ts` `falls back for unknown and empty types`

#### Scenario: Ledger load failure keeps the existing error state

- **WHEN** `ListLedgerEntries` fails while the wallet page loads
- **THEN** the page shows "Không tải được lịch sử giao dịch" with "Thử lại" and no label is rendered from stale data
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/wallet/wallet.test.tsx` `ledger error keeps the retry state`

#### Scenario: Guest cannot see the ledger

- **WHEN** a visitor without a session requests `/seller/wallet`
- **THEN** the visitor is redirected to login and the response contains no ledger rows
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/wallet/wallet.test.tsx` `guest is redirected to login`
