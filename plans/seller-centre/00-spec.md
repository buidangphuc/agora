# 00-spec — Seller Centre: shop customisation, marketing hub, buyable bundles, ops at scale

initiative: seller-centre

## Goal
A seller runs the whole shop from the seller app (`team-frontend-seller`, :3001): a new seller
is guided through first steps, brands the shop page (banner, tagline, featured products,
theme) and buyers see it; shop vouchers and flash sales are created, listed and stopped from one
`/seller/marketing` hub; a combo shown on `/shop/[id]` can really be bought at its combo price;
and order/listing lists are paged and filtered server-side with bulk ship and batch print, so a
shop with thousands of orders stays usable.

Source: PM review 2026-10-14 (Q21 seller#6, Q22 seller#9, Q27 seller#7, Q35 seller#10),
re-verified against `origin/main` 06c44c17 on 2026-11-13:
- Q21 mostly done: `AuthService.BecomeSeller` (platform-core/packages/proto/platform/identity/v1/identity.proto:57-60),
  seller-app onboarding screen + `acceptSellerTerms` (team-frontend-seller/src/features/seller/onboarding.ts:37-75,
  archived change `2026-11-05-seller-onboarding-ui`). Left: no first-steps guidance after the grant.
- Q22 backend done, UI missing: proto `Storefront` has banner_url/tagline/featured_listing_ids/theme
  (listing.proto:181-193); team-domain stores them but validates only slug + display_name
  (team-domain/internal/listing/storefront.go:46-60); `/seller/shop` edits only the name
  (team-frontend-seller/src/features/seller/ShopProfileForm.tsx:19-73); the buyer shop page renders banner
  and tagline (team-frontend/src/features/shop/ShopHeaderCard.tsx:68-80) but not featured items or theme.
- Q27 missing: a seller may call `CreateCampaign` and shop-scope `CreateVoucher`
  (team-promotion/internal/rpc/flashsale.go:31-61, voucher.go:31-50) but cannot list or stop them:
  `ListCampaigns`, `CancelCampaign`, `DeactivateVoucher` are admin-only at the edge
  (team-gateway/internal/edge/policy.go:42-46); `flash_sale_campaigns` has no seller column
  (team-promotion/internal/store/migrations/0001_promotion.up.sql:42-53). No seller-app page calls any
  flash-sale RPC; the shop-voucher form still lives on the buyer page `/vouchers`
  (team-frontend/src/app/(shop)/vouchers/page.tsx:28-33). Bundles show a price on `/shop/[id]`
  (page.tsx:58-80) but `CartItem`/`OrderItem` have no bundle reference (order.proto:33-43, 379-388), and
  `CreateBundle` checks neither listing ownership nor price (team-domain/internal/listing/bundle.go:31-45).
- Q35 partial: listings are cursor-paged (team-frontend-seller/src/features/seller/listingsPage.ts:14-27) but
  `ListMyListingsRequest` has no status/query filter (listing.proto:368-370) and pickers walk up to 10
  pages (`getAllListings`, listingsPage.ts:33-46); `ListSellerOrdersRequest` has only `status_filter`
  (order.proto:499-505), so `/seller/orders` and `/seller` load every order and filter in Next.js
  (team-frontend-seller/src/app/seller/orders/(list)/page.tsx:46-59). No bulk ship; print is per order
  (`PrintButton`, orders/[id]/page.tsx:58).

## User stories
1. **First steps (Q21).** Given a seller just granted the role, when they open `/seller`, then a
   "Bắt đầu bán hàng" card lists steps with done/not-done state: shop name, shop banner, first
   listing, first voucher or flash sale; each links to its page; the card hides once all are done.
   Edge: a step whose data source is down shows "chưa rõ" and never blocks the dashboard.
   There is no KYC step; the card covers only the shop, listing and marketing steps above.
2. **Brand the shop (Q22).** Given a seller on `/seller/shop`, when they upload a banner, set a
   tagline, pick up to N featured listings and a theme and save, then `/shop/{id}` on the buyer app
   shows the banner, tagline, featured listings first (in the chosen order) and the theme.
   Edge: a featured listing that is not the seller's or not found → INVALID_ARGUMENT, nothing saved.
   Edge: a featured listing later unpublished/deleted is skipped on the shop page, not an error.
   Edge: saving one field never clears the others (the form sends the full current storefront).
   N is a fixed 6 per shop for now; the plan `featured_listings` entitlement will later cap featured ids and override 6.
   Theme keys are `default`, `dark` and `festive`; each changes only the shop page header colour and accent colour.
3. **Marketing hub (Q27).** Given a seller on `/seller/marketing`, when they open the
   "Voucher của shop" tab, then they see their shop vouchers (active, scheduled, ended, deactivated)
   with used/quota, can create one and can deactivate one of their own. On the "Flash sale" tab they
   see their campaigns with status and sold/cap, create one for their own listing, and cancel one.
   Edge: deactivating or cancelling another seller's item → PERMISSION_DENIED (same answer as not found).
   Edge: the buyer `/vouchers` page no longer shows a create form.
   A seller may cancel only a SCHEDULED flash sale; cancelling an ACTIVE one → FAILED_PRECONDITION.
4. **Buy a combo (Q27).** Given a published bundle of listings L1..Ln on `/shop/{id}`, when a buyer
   clicks "Thêm combo vào giỏ", then the cart holds one line per listing tagged with the bundle and
   the bundle discount (sum of unit prices − bundle price) is shown; at checkout the order total
   reflects the bundle price and the seller's order shows the bundle on its lines.
   Edge: any bundle listing unpublished, out of stock or no longer owned by the bundle seller →
   the add fails with FAILED_PRECONDITION and the shop page shows the combo as unavailable.
   Edge: removing or changing the quantity of one bundle line drops the bundle tag from all its
   lines (regular prices apply) and the cart says so.
   Buyable bundles are built in this initiative (`AddBundleToCart`), with the bundle discount applied proportionally across the bundle lines.
   Bundles containing a variant listing are refused: `CreateBundle` rejects them and `AddBundleToCart` fails for any existing one.
5. **Orders at scale (Q35).** Given a seller with 5,000 orders, when they open `/seller/orders`
   with a status tab, a search query and a page, then only that page is fetched from team-order and
   tab counts come from the same response.
   Edge: an unknown cursor → INVALID_ARGUMENT, the page shows the retry state.
6. **Listings at scale (Q35).** Given a seller with 2,000 listings, when they filter `/seller` by
   status or title, or search in the ads/bundle/featured pickers, then the filter runs in
   team-domain (`ListMyListings` status + query) and no page walks the whole catalogue.
7. **Bulk ship + batch print (Q35).** Given several PAID orders on the "Chờ gửi" tab, when the
   seller selects up to 50, enters an optional tracking number per order and confirms, then each
   order moves to SHIPPED and a per-order result list shows success or the reason it failed;
   "In phiếu hàng loạt" prints one packing slip per selected order.
   Edge: one order cancelled meanwhile → only that row fails; the others ship.
   Bulk price/stock editing (inline or CSV) is out of scope and deferred to the next initiative.

## Non-goals
- No change to `BecomeSeller` or the onboarding screen (done, `identity-seller-onboarding`, `seller-onboarding-ui`).
- No plan entitlement enforcement, ad economics or sponsored slots (other PM-review items).
- No new flash-sale or voucher pricing rules; seller paths reuse existing validation.
- No admin-app change; admin flash-sale/voucher RPCs and their audit stay as they are.
- No staging/prod work (gitops values, Vault, runbooks) — local first.

## Architecture decisions
- New seller-scoped RPCs instead of opening admin RPCs at the edge. `ListMyCampaigns`,
  `CancelMyCampaign`, `ListMyShopVouchers`, `DeactivateMyShopVoucher` take the seller from the
  principal (never a request field), require `listing.write`, and answer PERMISSION_DENIED for a
  foreign or unknown id. Why: admin RPCs keep their edge policy and audit-first rule (AGENTS.md §3.6);
  seller calls are not admin mutations and are not audited.
- team-promotion stores `seller_id` on `flash_sale_campaigns` (migration, default `''`), set on every
  create from the listing owner returned by team-domain; a one-shot idempotent
  `backfill-campaign-sellers` subcommand fills old rows via `ListingService.GetListing`. Why: list
  by seller without calling team-domain per request (DB-per-service, AGENTS.md §3.3).
- Storefront validation lives in team-domain `StorefrontService.Upsert`: tagline ≤ 120 runes, no
  control chars; banner_url must be an object URL from the listing-images bucket
  (`GetImageUploadUrl`) or empty; featured ids ≤ cap, unique, owned by the caller and existing;
  theme in a closed set. Why: the owning service is authoritative; the UI only mirrors it.
- Bundles become purchasable in team-order: `CartService.AddBundleToCart` reads the bundle with
  `ListingService.GetBundle`, checks every listing (owner = bundle seller, PUBLISHED, stock), adds
  one cart line per listing with `bundle_id`, and allocates the discount pro rata to line prices
  (rounding remainder on the last line). Checkout re-reads the bundle; a changed or invalid bundle
  drops the tag and the preview shows regular prices before payment. Returns refund the allocated
  line price. Why: the order is the only price authority; no cross-DB join.
- team-domain `CreateBundle` refuses listings not owned by the caller, fewer than 2 listings, and a
  bundle price ≥ the sum of current listing prices. Existing bad bundles are shown unavailable, not deleted.
- `ListSellerOrders` and `ListMyListings` get optional paging/filter fields. An unset `page` keeps
  today's full-list behaviour so existing callers do not change; the seller app moves to paged
  calls. Search is server-side (order id prefix / buyer-visible item title for orders; title
  substring for listings). Why: non-breaking proto (AGENTS.md §6a).
- Bulk ship is a seller-app loop over the existing `OrderService.UpdateOrderStatus` (≤ 4 in flight,
  ≤ 50 orders) with a per-order result; no bulk RPC. Why: per-order ownership and saga rules stay in
  team-order; partial failure is natural per order.
- One platform-core contract slice carries every proto change and is built first.
- Mock data, not mock services: e2e seeds bundles, campaigns, vouchers and orders through the real
  services (gateway API seeding / `seed-marketplace.sh`); no fake containers, no route fulfils.
- No `*_ENABLED` flags or in-memory fallbacks for any new path; a down dependency fails the call.
- No LLM use in this initiative (if one is added later it goes through team-ai with mock default).
- Images stay non-root; no new image is introduced.

## Contracts
```proto
// platform/promotion/v1/promotion.proto — add to FlashSaleService
// Seller: campaigns on the caller's own listings (seller from the principal; needs listing.write).
rpc ListMyCampaigns(ListMyCampaignsRequest) returns (ListMyCampaignsResponse);
// Seller: cancels the caller's own campaign; foreign/unknown id → PERMISSION_DENIED. Not audited.
rpc CancelMyCampaign(CancelMyCampaignRequest) returns (CancelMyCampaignResponse);

// add to VoucherService
// Seller: the caller's shop-scope vouchers of any state (seller from the principal).
rpc ListMyShopVouchers(ListMyShopVouchersRequest) returns (ListMyShopVouchersResponse);
// Seller: deactivates the caller's own shop voucher; idempotent; foreign/unknown → PERMISSION_DENIED.
rpc DeactivateMyShopVoucher(DeactivateMyShopVoucherRequest) returns (DeactivateMyShopVoucherResponse);

message FlashSaleCampaign { /* existing 1..10 */ string seller_id = 11; } // server-owned
message ListMyCampaignsRequest { repeated CampaignStatus statuses = 1; platform.common.v1.PageRequest page = 2; }
message ListMyCampaignsResponse { repeated FlashSaleCampaign campaigns = 1; platform.common.v1.PageResponse page = 2; }
message CancelMyCampaignRequest { string campaign_id = 1; }
message CancelMyCampaignResponse { FlashSaleCampaign campaign = 1; }
message ListMyShopVouchersRequest { platform.common.v1.PageRequest page = 1; }
message ListMyShopVouchersResponse { repeated Voucher vouchers = 1; platform.common.v1.PageResponse page = 2; }
message DeactivateMyShopVoucherRequest { string voucher_id = 1; }
message DeactivateMyShopVoucherResponse { Voucher voucher = 1; }

// platform/order/v1/order.proto — add to CartService
// Adds every listing of a bundle as cart lines tagged bundle_id. FAILED_PRECONDITION when a
// listing is unpublished, out of stock or not owned by the bundle seller; NOT_FOUND for no bundle.
rpc AddBundleToCart(AddBundleToCartRequest) returns (AddBundleToCartResponse);
message AddBundleToCartRequest { string bundle_id = 1; int32 quantity = 2; } // quantity ≥ 1
message AddBundleToCartResponse { Cart cart = 1; }
message CartItem  { /* existing 1..9 */ string bundle_id = 10; int64 bundle_discount = 11; }
message Cart      { /* existing 1..3 */ int64 bundle_discount_total = 4; }
message OrderItem { /* existing 1..8 */ string bundle_id = 9; int64 bundle_discount = 10; }

// ListSellerOrders: optional paging and search; unset page = today's full list.
message ListSellerOrdersRequest {
  OrderStatus status_filter = 1;
  platform.common.v1.PageRequest page = 2;
  string query = 3;                       // order id prefix or item title substring; ≤ 100 chars
  repeated OrderStatus statuses = 4;      // OR-filter; wins over status_filter when set
}
message OrderStatusCount { OrderStatus status = 1; int64 count = 2; }
message ListSellerOrdersResponse {
  repeated Order orders = 1;
  platform.common.v1.PageResponse page = 2;
  repeated OrderStatusCount status_counts = 3;  // over all the seller's orders, query applied
}

// platform/listing/v1/listing.proto — ListMyListings filters
message ListMyListingsRequest {
  platform.common.v1.PageRequest page = 1;
  repeated ListingStatus statuses = 2;    // empty = all
  string query = 3;                       // title substring, case-insensitive; ≤ 100 chars
}
// Storefront: comments document the new validation (tagline ≤ 120, banner from upload bucket,
// featured ids owned + capped, theme closed set). No field change.
```
Existing contracts consumed unchanged: `ListingService.UpsertStorefront`, `ListingService.GetStorefront`,
`ListingService.GetImageUploadUrl`, `ListingService.GetBundle`, `ListingService.ListBundlesBySeller`,
`ListingService.CreateBundle`, `ListingService.GetListing`, `FlashSaleService.CreateCampaign`,
`VoucherService.CreateVoucher`, `OrderService.UpdateOrderStatus`, `OrderService.GetOrder`,
`OrderService.PreviewCheckout`, `OrderService.CreateOrder`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| contracts | seller-centre-contract | platform-core | – | provides: FlashSaleService.ListMyCampaigns, FlashSaleService.CancelMyCampaign, VoucherService.ListMyShopVouchers, VoucherService.DeactivateMyShopVoucher, CartService.AddBundleToCart | platform-core |
| storefront fields validated and owned | domain-storefront-customisation | team-domain | – | uses: ListingService.UpsertStorefront | team-domain, platform-e2e |
| seller brands the shop | seller-shop-customisation-ui | team-frontend-seller | domain-storefront-customisation, domain-my-listings-filter | uses: ListingService.UpsertStorefront, ListingService.GetImageUploadUrl, ListingService.ListMyListings | team-frontend-seller, platform-e2e |
| buyer sees featured items and theme | shop-page-featured-theme | team-frontend | domain-storefront-customisation | uses: ListingService.GetStorefront | team-frontend, platform-e2e |
| seller lists/stops own vouchers and flash sales (API) | promotion-seller-self-service | team-promotion | seller-centre-contract | uses: FlashSaleService.ListMyCampaigns, FlashSaleService.CancelMyCampaign, VoucherService.ListMyShopVouchers, VoucherService.DeactivateMyShopVoucher | team-promotion, team-gateway, platform-e2e |
| marketing hub in Seller Centre | seller-marketing-hub | team-frontend-seller | promotion-seller-self-service, domain-my-listings-filter | uses: FlashSaleService.ListMyCampaigns, FlashSaleService.CreateCampaign, FlashSaleService.CancelMyCampaign, VoucherService.ListMyShopVouchers, VoucherService.CreateVoucher, VoucherService.DeactivateMyShopVoucher | team-frontend-seller, team-frontend, platform-e2e |
| first-steps checklist | seller-getting-started | team-frontend-seller | seller-marketing-hub, seller-shop-customisation-ui | uses: ListingService.GetStorefront, ListingService.ListMyListings, VoucherService.ListMyShopVouchers, FlashSaleService.ListMyCampaigns | team-frontend-seller, platform-e2e |
| only own listings, real discount in bundles | domain-bundle-integrity | team-domain | – | uses: ListingService.CreateBundle | team-domain, platform-e2e |
| bundle bought at bundle price | order-bundle-checkout | team-order | seller-centre-contract, domain-bundle-integrity | uses: CartService.AddBundleToCart, ListingService.GetBundle | team-order, team-gateway, platform-e2e |
| "Thêm combo vào giỏ" on shop page | shop-bundle-purchase-ui | team-frontend | order-bundle-checkout, shop-page-featured-theme | uses: CartService.AddBundleToCart | team-frontend, platform-e2e |
| seller orders paged/searched server-side (API) | order-seller-list-paging | team-order | seller-centre-contract | uses: OrderService.ListSellerOrders | team-order, platform-e2e |
| my listings filtered server-side (API) | domain-my-listings-filter | team-domain | seller-centre-contract | uses: ListingService.ListMyListings | team-domain, platform-e2e |
| paged orders/listings and pickers in Seller Centre | seller-server-paging-ui | team-frontend-seller | order-seller-list-paging, domain-my-listings-filter | uses: OrderService.ListSellerOrders, ListingService.ListMyListings | team-frontend-seller, platform-e2e |
| bulk ship and batch print | seller-bulk-ship | team-frontend-seller | seller-server-paging-ui | uses: OrderService.UpdateOrderStatus, OrderService.GetOrder | team-frontend-seller, platform-e2e |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-frontend-seller/src/features/seller/nav.tsx` — seller-marketing-hub, seller-getting-started.
- `team-frontend-seller/src/app/seller/(dashboard)/page.tsx` — seller-getting-started, seller-server-paging-ui.
- `team-frontend-seller/src/features/seller/listingsPage.ts` — seller-server-paging-ui, seller-shop-customisation-ui, seller-marketing-hub (picker).
- `team-frontend-seller/src/app/seller/orders/(list)/page.tsx`, `src/lib/gateway/orders.ts` — seller-server-paging-ui, seller-bulk-ship.
- `team-frontend-seller/src/lib/gateway/promotion.ts`, `src/features/seller/actions.ts` — seller-marketing-hub, seller-shop-customisation-ui, seller-getting-started.
- `team-frontend/src/app/(shop)/shop/[id]/page.tsx` — shop-page-featured-theme, shop-bundle-purchase-ui.
- `team-gateway/internal/edge/` router wiring (promotion.go, cart.go) — promotion-seller-self-service, order-bundle-checkout.
- `team-domain/internal/rpc/listing.go` — domain-my-listings-filter, domain-bundle-integrity.
- `team-order/internal/store/order.go` — order-bundle-checkout, order-seller-list-paging.

## Out of initiative
- `/sell` keeps redirecting to `/seller/new` (team-frontend-seller/src/app/sell/page.tsx); unchanged.
- Hiding the combo price on `/shop/[id]` is a plain task only if buyable bundles are deferred (story 4).
- Plan-tied caps (featured listings, marketing tools) wait for the plan-entitlement initiative.

## NFR and data
- NFR: `ListSellerOrders` and `ListMyListings` paged calls p95 ≤ 150 ms locally at 5,000 orders /
  2,000 listings (needs indexes on `(seller_id, status, created_at)` and listing `(owner_id, status)`);
  page size default 20, max 100. `/seller/orders` server render p95 ≤ 1 s at 5,000 orders.
  `AddBundleToCart` p95 ≤ 300 ms for ≤ 10 listings; bundles hold 2..10 listings. Bulk ship ≤ 50
  orders, ≤ 4 calls in flight, whole batch ≤ 15 s locally.
- Personal data: banner image and tagline are seller-provided public shop content (purpose shop
  page; basis contract; retention while the shop exists). Packing slips print the buyer's shipping
  name/address/phone already shown on order detail — print-only, not stored or logged by the seller
  app. Search `query` strings are not logged. Seller and buyer ids are never logged raw (sloglint
  forbidden-keys).
- Risk routing expected: `proto` (contract slice), `payment` (bundle discount in checkout totals),
  `auth` (new seller-scoped RPCs and gateway routes), `pii` (batch packing slips).

## Conventions
- Seller RPCs mirror the owner-check pattern of `CreateCampaign` (team-promotion/internal/flashsale/flashsale.go:114-160,
  `listing.CheckOwner`) and principal-scoped reads like `ListMyVouchers`; gateway forwarders mirror
  team-gateway/internal/edge/order.go; admin edge policy (policy.go) is not touched.
- Storefront: extend `StorefrontService.Upsert` in team-domain/internal/listing/storefront.go; owner
  forcing stays (`sf.SellerID = ownerID`). The seller form must send the full current storefront
  (as `upsertStorefrontAction` does for the name, team-frontend-seller/src/features/seller/actions.ts:168-200).
- Seller UI follows the `fe-platform` skill (shared ui, tokens); URL-state lists use
  `listParams.ts`/`SellerPagination`; e2e page objects set `app = "seller"`.
- Seller-route FEATURES entries go to `team-frontend-seller/FEATURES.yaml`; `/shop`, `/cart`, `/vouchers`
  entries to `team-frontend/FEATURES.yaml`; API-level entries to the service repo.
- Go Docker-backed tests need `GO_SDK_REQUIRE_DOCKER=1`; gates run through `gate_digest.py`.
- Do not touch service ports or admin-app routes.

## Locked
2026-11-13 — Confirmed by Bùi Đăng Phúc (all defaults accepted). No KYC step; max 6 featured listings (plan cap later); themes default/dark/festive (header + accent); cancel only SCHEDULED flash sales; buyable bundles built with proportional discount, variant listings refused; bulk price/stock edit out.
