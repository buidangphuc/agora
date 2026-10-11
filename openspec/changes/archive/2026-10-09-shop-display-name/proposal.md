## Why

The buyer UI can only render "Shop #abc123" wherever a shop is named, because no contract carries a shop or
seller display name. It shows up in four places: the cart grouped by shop (`ViewCartItem` has only
`sellerId`), the product-detail shop header and `/shop/<id>` (`Storefront` has `slug`, `tagline`, `banner_url`
but no name), `/account/following` (`ListFollowedSellers` returns bare `seller_ids`), and the seller pages.
`team-identity` only knows a login `username`, which is a credential handle and must not be shown publicly.
The `ui-phase-*` changes need a real name to render.

## What Changes

- **Owner**: `team-domain` owns the shop display name. The seller's storefront row (`storefronts`, already
  owner-scoped, one row per seller) gains a `display_name`, stored in the existing `config` JSONB (no
  migration).
- **platform-core proto (additive only)**: `listing.v1.Storefront` gains `string display_name = 7`;
  `ListingService` gains `BatchGetStorefronts(seller_ids) -> repeated ShopSummary{seller_id, display_name,
  slug}` (new RPC + 3 new messages). Nothing is renamed, removed or renumbered.
- **team-domain**: `UpsertStorefront` accepts, trims and validates `display_name`; `GetStorefront` returns it;
  `BatchGetStorefronts` is implemented with one `WHERE seller_id = ANY($1)` query, max 100 ids.
- **team-gateway**: forwards `BatchGetStorefronts` like every other listing RPC (read path, same auth
  posture as `GetStorefront`). No composition and no business logic in the gateway.
- **team-frontend (gateway wrappers)**: `getStorefront` returns `displayName`; `getCart` and
  `listFollowedSellers` resolve names with **one** batch call per page render; a shared `shopLabel()` helper
  returns the display name or falls back to `Shop #<first 6 chars of sellerId>`. `ViewCartItem` gains
  `sellerDisplayName`.
- **platform-e2e**: new scenarios, plus `team-domain/FEATURES.yaml` and `team-frontend/FEATURES.yaml` entries.

## Capabilities

### New Capabilities
- `shop-display-name`: a seller-owned, publicly readable shop display name, with a batch lookup and a
  deterministic UI fallback.

### Modified Capabilities
<!-- none: openspec/specs has no storefront capability to modify; the requirements are ADDED. -->

## Impact

- **Affected repos**: `platform-core` (proto), `team-domain` (owner), `team-gateway` (forwarder),
  `team-frontend` (`src/lib/gateway/{listings,cart,engagement}.ts` + shared helper), `platform-e2e`.
  Not touched: `team-order`, `team-engagement`, `team-identity`, `team-search`.
- **Architecture rules**
  - Rule 1: the frontend calls only the gateway (two gateway reads joined in the server-side wrapper is UI
    shaping, not business logic).
  - Rule 2: the gateway only forwards.
  - Rule 3: no service copies the name into its own DB (no `seller_display_name` in order or engagement).
  - Rule 4: contract change lives only in `platform-core/packages/proto`; consumers regenerate, never
    hand-edit.
  - Rule 5 (events): **no event impact.** Storefront writes emit no Kafka event today and none is added;
    `listing.events` and `ListingChanged` are untouched. If `team-search` later wants to denormalise the
    name for seller facets, a `StorefrontChanged` event is a follow-up change (see design.md).
- **Compatibility**: purely additive; old clients ignore the new field and RPC. `buf breaking` must pass.
- **Data**: existing storefront rows have no name, so every current shop renders the fallback until the
  seller sets one. Sellers with no storefront also render the fallback.

## Non-goals

- A seller-facing edit form for the name (owned by `ui-phase-seller`; this change only makes the API
  accept it, and e2e sets it through the API).
- Copying or denormalising the name into `team-order`, `team-engagement`, `team-search` or `team-chat`, and
  any new Kafka event.
- Showing or using the identity `username` as a shop name; adding a profile to `team-identity`.
- Search facet labels (`FacetBucket` key = seller_id), chat thread headers, order history shop names.
  They can adopt `BatchGetStorefronts` later.
- Backfilling names for existing sellers, name uniqueness, moderation or profanity filtering, and
  localisation of names.
- Gateway-side response enrichment or caching.
