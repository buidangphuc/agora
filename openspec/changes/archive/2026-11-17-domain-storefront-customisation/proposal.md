## Why

`ListingService.UpsertStorefront` stores banner_url, tagline, featured_listing_ids and theme but
team-domain validates only slug and display_name (`team-domain/internal/listing/storefront.go`). The
seller UI and buyer shop page of initiative `seller-centre` (story 2) cannot ship safely on an
unvalidated write path: any URL, any text, any listing id (including another seller's) would be
stored and later rendered on a public page. The owning service must be authoritative.

initiative: seller-centre

## What changes

- `StorefrontService.Upsert` (team-domain) validates, in this order, and rejects the whole save with
  INVALID_ARGUMENT on the first failure (nothing is stored):
  - tagline: at most 120 runes after trimming, no control characters (trimmed value is stored).
  - banner_url: empty, or an object URL under the configured storage public base
    (`STORAGE_PUBLIC_BASE_URL`, the listing-images bucket that `GetImageUploadUrl` returns as
    `public_url`), with no query string or fragment. A value equal to the one already stored is
    accepted unchanged so legacy rows never block saving other fields.
  - featured_listing_ids: at most 6, no empty or duplicate ids, every id an existing listing owned by
    the caller (a foreign listing and an unknown id give the same answer). Order is kept.
  - theme: empty (means `default`) or one of `default`, `dark`, `festive`.
- A port to read listings by id is added to `StorefrontService` (reuse `store.ListingRepository.Get`;
  no new RPC, no proto field change). The proto comments on `Storefront` are updated by the
  `seller-centre-contract` slice; this slice changes no `.proto`.
- Featured listings that are later unpublished or deleted are NOT re-validated on read: `GetStorefront`
  returns the stored ids unchanged and the buyer slice skips unavailable ones.
- Existing `UpsertStorefront` behaviour is kept: owner forced from the principal, `listing.write`
  scope, slug rules, display_name rules, `ErrSlugTaken`.
- The 6-featured limit is a fixed constant now; the `featured_listings` plan entitlement later
  overrides it (non-goal here).

## NFR

- `UpsertStorefront` p95 <= 150 ms locally, including one batched listing lookup for <= 6 ids (one
  query, not one per id).
- No new availability dependency: the lookup uses team-domain's own database; if it fails the call
  fails with UNAVAILABLE/INTERNAL and nothing is stored (no in-memory fallback, no enable flag).
- Limits: tagline 120 runes, banner_url 2048 bytes, 6 featured ids, 3 theme keys.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| banner_url | seller-provided public shop content (object URL, no personal data expected) | shop page header | contract (seller agreement) | while the shop exists |
| tagline | seller-provided public shop content, free text; could contain personal data the seller chooses to publish | shop page header | contract | while the shop exists |
| featured_listing_ids | public references | shop page ordering | contract | while the shop exists |
| theme | public setting | shop page colours | contract | while the shop exists |

Tagline and banner values and the seller id are never written to logs (sloglint forbidden-keys);
rejection logs carry only the rule name.

## Threat model

- Assets: integrity of public shop pages; other sellers' listings; buyers rendering the page.
- Abuse cases: (1) featuring another seller's listing to borrow its visibility; (2) banner_url
  pointing at an external or `javascript:`/`data:` URL, tracking pixel or off-bucket host; (3) oversized
  or control-character tagline used for layout abuse or log injection; (4) unbounded featured list
  for write amplification; (5) writing another seller's storefront through `seller_id` in the body;
  (6) probing which listing ids exist.
- Authz rule: only a principal with `listing.write` writes; the row key is always the principal id
  (request `seller_id` ignored); a listing id that is foreign or missing returns the same
  INVALID_ARGUMENT with no distinguishing detail, so existence of foreign listings is not leaked.
- No new RPC, no new edge route, no admin path touched.
