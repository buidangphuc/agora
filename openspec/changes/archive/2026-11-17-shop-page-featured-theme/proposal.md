## Why

Initiative `seller-centre` story 2: a seller brands the shop (banner, tagline, featured listings, theme) and buyers
must see it. The buyer shop page `/shop/[id]` (`team-frontend`) already renders the banner and tagline in
`ShopHeaderCard` (hero) but ignores `Storefront.featured_listing_ids` and `Storefront.theme`
(listing.proto `Storefront`, fields 5 and 6), so what the seller saves in Seller Centre never shows up.

initiative: seller-centre

## What Changes

- `/shop/[id]` shows a "Sản phẩm nổi bật" block above the product grid: the storefront's featured listings, in the
  stored order, at most 6, only those that are published and belong to this shop. Missing, unpublished, deleted or
  foreign ids are skipped silently. The block is absent when nothing is left to show.
- Featured listings are not repeated in the "Sản phẩm của shop" grid below (featured first, the rest follow).
- The hero `ShopHeaderCard` applies the storefront theme: key `default`, `dark` or `festive` selects a header colour
  and an accent colour. An unknown or empty key renders as `default`. Nothing else changes with the theme.
- Reads only existing `ListingService.GetStorefront` (and the existing `GetListing` already used by the page for
  listing resolution). No proto, gateway or backend change.

## Capabilities

### New Capabilities
- `ui-shop-storefront`: featured listings block and theme on the buyer shop page.

### Modified Capabilities
none

## Impact

- `team-frontend/src/features/shop/ShopStorefrontView.tsx`, `ShopHeaderCard.tsx`, new
  `featured.ts` / `shopTheme.ts` helpers, `src/app/(shop)/shop/[id]/page.tsx` (hot file shared with
  `shop-bundle-purchase-ui`, which is built after this slice), `team-frontend/FEATURES.yaml`.
- `platform-e2e` feature + steps for the buyer shop page.

## Non-goals

- No change to the seller form (`seller-shop-customisation-ui`), to storefront validation
  (`domain-storefront-customisation`) or to the combo block (`shop-bundle-purchase-ui`).
- No theme effect beyond header colour and accent colour; no new theme keys.
- No featured-listing cap logic in the buyer app: the cap of 6 is enforced on write; the page also shows at most 6.

## Non-functional requirements

- Server-rendered with the rest of the page; no extra client JS; no new gateway round trip for ids already in the
  shop's listing result; at most 6 extra `GetListing` calls (in parallel) for featured ids missing from it.
- Page p95 stays within today's budget plus 100 ms locally with 6 featured ids.
- Degradation: if `GetStorefront` fails the page renders as today (no block, default theme); a failing featured
  lookup hides that one item, never the page.
- Theme colours meet WCAG AA contrast (4.5:1) for header text on header background, in all three themes.

## Data classification

- Banner URL, tagline, featured ids, theme: seller-provided public shop content. Purpose: shop page display. Basis:
  contract (seller terms). Retention: while the shop exists. No buyer personal data is read, stored or logged by
  this slice. Seller ids are not logged raw.

## Threat model

- Assets: integrity of what buyers see on a shop page.
- Abuse: a seller features another shop's or an unpublished/banned listing (stored before validation shipped, or via
  a direct API call); a seller stores a hostile theme key or tagline. Mitigation: the page re-checks each featured
  listing (published, `seller_id` equals the shop) and renders it from the listing service, not from storefront data;
  the theme key is mapped through a closed table (unknown = `default`), never used as a class name or style value
  directly; tagline and banner keep today's escaped React rendering.
- Authz: public read, no authentication; no write path in this slice.
