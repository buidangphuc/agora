## Why

Q21 (PM review 2026-10-14): after `BecomeSeller` and the terms screen a new seller lands on `/seller` with no guidance on what to do next. Shop name, banner, first listing and first voucher or flash sale are spread over four pages. This slice adds a "Bắt đầu bán hàng" checklist card to the seller dashboard that shows which steps are done and links to the page for each.

initiative: seller-centre

## What changes

- `/seller` (`team-frontend-seller/src/app/seller/(dashboard)/page.tsx`) renders a "Bắt đầu bán hàng" card above the KPI row. The card is a separate async server component streamed inside `Suspense`, so the existing dashboard sections never wait for it.
- Four steps, each with done / not done / "chưa rõ" state and a link:
  1. "Đặt tên gian hàng" - done when `GetStorefront.storefront.display_name` is non-empty; link `/seller/shop`.
  2. "Thêm ảnh bìa gian hàng" - done when `storefront.banner_url` is non-empty; link `/seller/shop`.
  3. "Đăng sản phẩm đầu tiên" - done when `ListMyListings` (page size 1) returns at least one listing of any status; link `/seller/new`.
  4. "Tạo voucher hoặc flash sale đầu tiên" - done when `ListMyShopVouchers` or `ListMyCampaigns` (page size 1 each) returns at least one row of any state; link `/seller/marketing`.
- The card is hidden when all four steps are done. A step whose data source fails or times out (3 s) shows "chưa rõ" and counts as not done for hiding, never as an error page. Step 4 is "chưa rõ" only when neither source answered with a row and at least one failed.
- No KYC step, no dismiss button, no persisted state: done-state is derived from the data on every render.
- Pure helper `deriveGettingStarted` in `team-frontend-seller/src/features/seller/gettingStarted.ts` and component `GettingStartedCard.tsx` in the same folder.
- Gateway wrappers: reuses `getStorefront` and `listMyListings` (`src/lib/gateway/listings.ts`); uses the `listMyShopVouchers` and `listMyCampaigns` wrappers in `src/lib/gateway/promotion.ts` added by `seller-marketing-hub` (this slice depends on it; it adds none of its own).

## Capabilities

### New Capabilities
- `seller-getting-started`: the first-steps checklist card on the seller dashboard.

### Modified Capabilities
None.

## Impact

`team-frontend-seller` only: `src/app/seller/(dashboard)/page.tsx` and `loading.tsx` (hot files shared with `seller-server-paging-ui`), new `gettingStarted.ts`, `GettingStartedCard.tsx`, tests; `FEATURES.yaml` entry (written by the e2e-author); `platform-e2e` feature. Consumes `ListingService.GetStorefront`, `ListingService.ListMyListings`, `VoucherService.ListMyShopVouchers`, `FlashSaleService.ListMyCampaigns` unchanged from `seller-centre-contract`. Depends on `seller-marketing-hub` (page `/seller/marketing`, wrappers) and `seller-shop-customisation-ui` (banner upload on `/seller/shop`).

## Non-goals

KYC or identity step; plan or entitlement steps; dismissing or snoozing the card; per-step analytics events; any proto or backend change; changing `BecomeSeller` or the onboarding screen; paging or filtering of the dashboard lists (owned by `seller-server-paging-ui`).

## NFR

- The card adds no latency to the rest of `/seller`: it streams in a `Suspense` boundary; the four reads run in parallel, each capped at 3 s, so the card settles within 3.2 s worst case locally and p95 <= 600 ms when all sources are healthy.
- Each read asks for page size 1 (storefront is a single get); no list is walked.
- Availability: any failing source degrades only its own step to "chưa rõ"; the dashboard still renders HTTP 200. No `*_ENABLED` flag and no cached fallback.
- The card is a server component: no client JS beyond links.

## Data classification

- Storefront `display_name` and `banner_url`: public shop content (purpose shop page; basis contract; retention while the shop exists). Only emptiness is evaluated; values are not rendered by the card, not stored, not logged.
- Listing, voucher and campaign rows: only "at least one exists" is evaluated; row content is discarded. Not personal data.
- Seller id (from the session principal): used as the `GetStorefront` key; never logged raw (sloglint forbidden-keys) and never rendered.
- No new personal data is collected or stored by the seller app.

## Threat model

- Assets: the seller's shop state (what is configured), the seller session.
- Abuse cases: (1) a client-supplied seller id to read another shop's state - the card takes the seller id only from the session principal, no query parameter or form field; (2) a buyer or anonymous visitor reaching the card - the page guard (`redirectGuestToLogin`, `listing.write` scope check) runs before the card renders, and the three seller-scoped RPCs take the seller from the gateway principal and need `listing.write`; (3) a slow upstream used to hang the dashboard - each read times out at 3 s inside its own `Suspense` boundary; (4) leaking values in logs or markup - only booleans reach the component props.
- Authz rule: enforced upstream (`listing.write`; seller from the principal in `ListMyShopVouchers` and `ListMyCampaigns`; `GetStorefront` is public read). The UI never decides authorization.
