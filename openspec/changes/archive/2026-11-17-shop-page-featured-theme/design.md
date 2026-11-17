## Context

`ShopPage` loads shop listings (`searchListings` filtered by seller), the storefront (`loadStorefront`), bundles and
the rating summary, then passes them to `ShopStorefrontView`, which renders `ShopHeaderCard variant="hero"`, sort
tabs and `ListingGrid`. `ViewStorefront` already carries `featuredListingIds` and `theme`.

## Decisions

- Featured resolution is a pure helper `resolveFeatured(storefront, shopListings, fetched)`. Ids are deduplicated,
  truncated to 6, mapped in stored order. A listing is kept only when `status` is published (or unset, as
  `searchListings` already treats it) and `sellerId` equals the shop. Ids already present in the shop listing result
  are reused; the rest are fetched in parallel with the existing `getListing` (null on any failure) in the page
  server component. Why: the search result can be shorter than the shop catalogue, and a featured item must not
  vanish only because of that; no new RPC.
- Featured items are removed from the main grid list and the main grid count/tab labels keep counting all published
  listings, so numbers match the "Tất cả sản phẩm" tab. Sorting (`?sort=`) applies to the main grid only; the
  featured block keeps the seller's order.
- Theme is a closed table in `shopTheme.ts`: `default | dark | festive` to `{ headerBg, headerText, accent }` token
  classes; `data-theme` on the hero card carries the resolved key for tests. Unknown/empty falls to `default`.
  Header colour = the tagline strip and hero title area background; accent = stat values and the featured block
  heading underline. Uses tokens, no raw hex outside the table.
- The compact header (PDP) is not themed.
- `generateMetadata` is unchanged.

## Risks

- `shop/[id]/page.tsx` is also edited by `shop-bundle-purchase-ui`; this slice only adds a prop pass-through and
  one data load, keeping the bundles block untouched so that merge stays small.
