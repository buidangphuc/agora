## Why

A seller can create a shop voucher only on the buyer page `/vouchers` and has no seller-app page for flash sales:
no page calls any flash-sale RPC, and neither vouchers nor campaigns can be listed or stopped by their owner
(initiative `seller-centre`, PM review Q27). This slice adds one `/seller/marketing` hub in the seller app with a
"Voucher của shop" tab and a "Flash sale" tab, built on the seller-scoped RPCs delivered by
`seller-centre-contract` and `promotion-seller-self-service`, and removes the voucher create form from the buyer
page.

initiative: seller-centre

## What changes

- New route `/seller/marketing` (team-frontend-seller) with two URL-state tabs (`?tab=voucher|flashsale`, default
  `voucher`) and a nav entry "Marketing" in `SELLER_NAV` (`src/features/seller/nav.tsx`).
- Tab "Voucher của shop": cursor-paged list of the seller's shop vouchers (`VoucherService.ListMyShopVouchers`) with
  derived state (Đang chạy / Sắp diễn ra / Đã kết thúc / Đã tắt), used/quota; create form (`VoucherService.CreateVoucher`,
  scope SHOP); per-row "Tắt voucher" (`VoucherService.DeactivateMyShopVoucher`).
- Tab "Flash sale": cursor-paged list of the seller's campaigns (`FlashSaleService.ListMyCampaigns`) with status
  filter, status and sold/cap; create form for one of the seller's own PUBLISHED listings (picked through
  `ListingService.ListMyListings` with `statuses=[PUBLISHED]` + `query`, never walking the catalogue)
  (`FlashSaleService.CreateCampaign`); per-row "Huỷ" only for SCHEDULED campaigns (`FlashSaleService.CancelMyCampaign`).
- Seller-app gateway module `src/lib/gateway/promotion.ts` gains list/create/cancel/deactivate calls; server actions
  in `src/features/seller/actions.ts`.
- team-frontend: `/vouchers` no longer renders the create form; the `VoucherManager` component and its server
  action are deleted. The buyer voucher wallet and catalogue stay.
- No proto, backend or gateway change here (the state label reads `Voucher.active`, field 13, which already exists in `promotion.proto`); consumed RPCs are listed in `slices.yaml` `uses`.

## NFR

- `/seller/marketing` server render p95 <= 1 s locally with 200 vouchers and 200 campaigns (one page of 20 per tab is
  fetched; only the active tab's data is loaded).
- Page size default 20, max 100 (as the initiative); the listing picker returns <= 20 rows per query, debounced 300 ms.
- Deactivate / cancel / create round trip shows the refreshed row within 2 s locally.
- A failing dependency affects only its tab: the other tab and the rest of the seller app keep rendering. No
  `*_ENABLED` flag, no in-memory fallback.
- Seller UI follows the `fe-platform` skill (shared `ui`, tokens, `listParams.ts`, `SellerPagination`); viewport
  375 px usable.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| voucher code, discount, quota, validity | seller business content, public once active | shop promotion | contract | while the shop exists (owned by team-promotion) |
| flash sale listing, price, cap, window | seller business content, public once active | flash sale | contract | while the campaign exists (owned by team-promotion) |
| seller id | internal identifier | scoping | contract | never sent by the client; never logged raw |

The seller app stores nothing and logs no voucher code, listing title, search `query` or id. No buyer personal data
is read or shown on this page.

## Threat model

- Assets: shop vouchers and flash sales (money-affecting), seller identity.
- Abuse cases: (1) a seller deactivates or cancels another seller's voucher/campaign by forging an id; (2) a buyer or
  anonymous visitor opens `/seller/marketing` or posts the server actions; (3) a client-chosen seller id leaks into
  a request; (4) a seller cancels a running (ACTIVE) flash sale to dodge honouring the price; (5) a seller creates a
  campaign on a foreign listing.
- Authz rule: the page and actions require a signed-in principal with scope `listing.write` (otherwise the existing
  seller layout gate: `SellerOnboarding` for a signed-in user without the scope, login redirect for an anonymous visitor). Ownership is enforced by team-promotion (PERMISSION_DENIED for a foreign or
  unknown id, same answer for both); the UI never sends a seller id and shows the same "không có quyền hoặc không
  tồn tại" message for a denied row. The UI hides "Huỷ" for non-SCHEDULED rows, and the server answers
  FAILED_PRECONDITION if called anyway.
