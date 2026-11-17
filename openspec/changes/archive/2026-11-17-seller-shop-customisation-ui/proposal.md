## Why

`/seller/shop` edits only the shop name, although the storefront already stores banner, tagline, featured
listings and theme (`Storefront` fields in `platform-core/packages/proto/platform/listing/v1/listing.proto`).
A seller cannot brand the shop page. This slice adds the branding form to the seller app. The server-side
validation is `domain-storefront-customisation`; the buyer rendering is `shop-page-featured-theme`.

initiative: seller-centre

## What changes

- `/seller/shop` gets a branding form next to the name field: banner upload, tagline (<= 120 characters),
  up to 6 featured listings chosen with a server-filtered search picker and ordered, and a theme choice
  (`default`, `dark`, `festive`). One Save sends the full current storefront.
- Banner upload reuses `getUploadUrlAction` (`ListingService.GetImageUploadUrl`) and a browser PUT to the
  presigned URL, like the listing form; the returned public URL becomes `banner_url`.
- The featured picker calls `ListingService.ListMyListings` with `statuses = [PUBLISHED]` and `query`
  (fields added by `seller-centre-contract`, filtered by `domain-my-listings-filter`), one page of 20 per
  keystroke-debounced search. It never walks the catalogue.
- `upsertStorefrontAction` takes the full set of branding fields; the seller id still comes from the session.
- Changes in `team-frontend-seller`: `src/features/seller/ShopProfileForm.tsx` (extended), new
  `ShopBannerField.tsx`, `FeaturedListingsPicker.tsx`, `shopBranding.ts` (client mirror of the rules),
  `actions.ts` (`upsertStorefrontAction`, new `searchMyListingsAction`), `src/lib/gateway/listings.ts`
  (`listMyListings` accepts `statuses`, `query`), `src/app/seller/shop/page.tsx`.
- No change to proto, team-domain, the buyer app, `BecomeSeller` or the onboarding screen.

## Capabilities

### New Capabilities
- `seller-shop-customisation-ui`: the seller shop-branding form.

### Modified Capabilities
none (the existing `shop-display-name` behaviour is kept: the name rules and the "Đã lưu" feedback do not change).

## Impact

- team-frontend-seller (code), platform-e2e (feature file). Depends on `domain-storefront-customisation` and
  `domain-my-listings-filter`. Hot files shared with other slices: `listingsPage.ts`, `actions.ts`,
  `src/lib/gateway/listings.ts`; build is serialised by initiative-build.

## NFR

- Save round trip (action + `UpsertStorefront`) p95 <= 800 ms locally excluding the image PUT.
- Featured picker search returns <= 20 rows per call, debounced 300 ms; p95 <= 400 ms locally at 2,000 listings.
- Banner file: image/jpeg, image/png or image/webp, <= 10 MB (same cap as the listing form); the check runs before any upload.
- Availability: a failed `GetStorefront` blocks saving (nothing is overwritten from a blank read); the page shows a retry alert.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| banner image / `banner_url` | seller-provided public shop content | shop page header | contract | while the shop exists |
| tagline | seller-provided public shop content | shop page header | contract | while the shop exists |
| featured listing ids, theme | public shop configuration | shop page layout and colour | contract | while the shop exists |

No buyer personal data is read or written. The seller id comes from the session, is never a client field and is not logged raw.

## Threat model

- Assets: the shop page content shown to every buyer; the seller's own storefront record.
- Abuse cases: (1) a seller overwrites another seller's storefront by posting a foreign `sellerId`; (2) featured
  ids of another seller's listings; (3) a banner URL pointing to an arbitrary host (tracking pixel, hotlinked
  content); (4) an over-long or control-character tagline; (5) saving one field blanks the others; (6) a stale
  or blank read followed by a save wipes the storefront.
- Authz rule: the action requires a session principal with `listing.write`; `sellerId` is taken from the
  session, never from the form; ownership of featured ids, the banner bucket and the theme set are enforced by
  team-domain (`INVALID_ARGUMENT`), the UI mirrors them only for early feedback and never replaces them.
