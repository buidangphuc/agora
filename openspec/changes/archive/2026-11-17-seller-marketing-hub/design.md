## Context

team-frontend-seller reaches every backend through the gateway with a per-request client built from the `session`
cookie (`src/lib/gateway/promotion.ts`). It already holds `createVoucher` and the flash-sale read mappers but no
list/cancel calls and no page. Seller pages use RSC + server actions, `listParams.ts` URL state and `SellerPagination`.

## Decisions

- One page, two tabs through `?tab=`, not two routes: a single nav entry, one hot-file edit of `nav.tsx`. Only the
  active tab's RPC runs; each tab is wrapped in its own error boundary state so one outage does not blank the other.
- Voucher state is derived in the seller app from `active`, `starts_at`, `ends_at`, all already on the existing
  `Voucher` message (`bool active = 13`, `starts_at = 11`, `ends_at = 12` in `promotion.proto`; no contract change is
  needed): `active=false` -> Đã tắt; now < starts_at -> Sắp diễn ra; now > ends_at -> Đã kết thúc; otherwise Đang chạy.
  `deactivated_at` (field 14) is not read. Deactivated takes precedence over the time-based states.
- Flash sale create form picks one PUBLISHED listing through `ListMyListings(statuses=[PUBLISHED], query)` (provided by
  `domain-my-listings-filter`); it sends `variant_id` empty (listing-level sale) and an `idempotency_key` generated per
  form mount, so a double click creates one campaign. Price and cap rules stay in team-promotion; the UI only mirrors
  obvious ones (price > 0, cap >= 1, ends after starts) and shows server messages for the rest.
- Cancel is offered only for SCHEDULED rows. Deactivate and cancel ask for confirmation in a shared `ui` dialog.
- Mutations are server actions that call the gateway module, then `revalidatePath("/seller/marketing")`; the seller id is
  never a parameter.
- Buyer `/vouchers`: remove `VoucherManager` and `createVoucherAction`; `canManage` and its imports go with them.
  Deleted, not deprecated (stale code).

## Risks

- `promotion-seller-self-service` must be merged first; until then the gateway answers UNIMPLEMENTED/PERMISSION_DENIED
  and the tabs show the error state (no fallback to admin RPCs).
- `nav.tsx`, `promotion.ts`, `actions.ts`, `listingsPage.ts` are shared hot files; this slice is serialised against
  `seller-getting-started` and `seller-shop-customisation-ui`.
