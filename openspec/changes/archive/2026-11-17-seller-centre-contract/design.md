# Design

## Clarifications
- Names, numbers and comments follow the Contracts section of `plans/seller-centre/00-spec.md` (Locked 2026-11-13) verbatim.
- `ListMyCampaignsResponse.campaigns` and `ListMyShopVouchersResponse.vouchers` reuse the existing `FlashSaleCampaign` and `Voucher` messages; `statuses` empty means all states.
- Message numbers: `ListMyCampaignsRequest {statuses=1 (repeated CampaignStatus), page=2}`, `ListMyCampaignsResponse {campaigns=1, page=2}`, `CancelMyCampaignRequest {campaign_id=1}`, `CancelMyCampaignResponse {campaign=1}`, `ListMyShopVouchersRequest {page=1}`, `ListMyShopVouchersResponse {vouchers=1, page=2}`, `DeactivateMyShopVoucherRequest {voucher_id=1}`, `DeactivateMyShopVoucherResponse {voucher=1}`.
- `order.proto` needs `import "platform/common/v1/common.proto"` for `PageRequest`/`PageResponse`; every one of the 14 order copies already ships `common.proto` (checked), so no consumer needs a new file.
- `status_counts` is computed over all the seller's orders with `query` applied and `statuses` ignored, so tab counts stay stable while a tab is selected; comment states it.
- `statuses` wins over `status_filter` when both are set; unset `page` returns the full list with `page` and `status_counts` still populated (page.next_cursor empty).
- An unknown or malformed cursor is `INVALID_ARGUMENT` on both list RPCs; documented in comments.
- Bundle fields: `CartItem.bundle_discount` is the allocated pro-rata share for that line (int64 VND), `Cart.bundle_discount_total` the sum; both server-owned, never trusted from a request.
- Variant listings in a bundle: `AddBundleToCart` fails `FAILED_PRECONDITION` (Locked); `CreateBundle` rejection is domain-bundle-integrity, not a contract change.
- Test location: `platform-core/packages/proto/platform/{promotion,order,listing}/v1/`, using the descriptor-and-wire-codec approach of `order/v1/lifecycle_compat_test.go` (`buf build -o -#format=json`).
- Order field numbers used here: `Cart` 4, `CartItem` 10-11, `OrderItem` 9-10, `ListSellerOrders*` as listed. Other in-flight changes touching `order.proto` must not reuse these numbers; the test pins only these and existing fields, nothing above them.

## Decisions
- One contract slice carries every proto change of the initiative so consumers regenerate once.
- Seller RPCs are new RPCs, not relaxed admin RPCs, so admin edge policy and audit-first rule stay untouched.
- Non-breaking paging: unset `page` equals today's behaviour.
