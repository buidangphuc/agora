## Why

The combo shown on `/shop/[id]` is about to become buyable at its bundle price (initiative `seller-centre`, story 4).
Today `ListingService.CreateBundle` (`team-domain/internal/listing/bundle.go:31-45`) stores whatever it is given: a
seller can bundle another seller's listings, a single listing, a variant listing, or set a bundle price at or above the
sum of the parts, so the "discount" is fake or the bundle sells someone else's goods. `order-bundle-checkout` trusts the
bundle read by `GetBundle`, so the write side must be sound first.

initiative: seller-centre

## What Changes

- `BundleService.Create` validates before storing, all server-side in team-domain:
  - 2 to 10 listing ids, no duplicates;
  - every listing exists and is owned by the caller (a foreign or unknown listing is indistinguishable: one answer);
  - no listing has variants (variant listings are refused);
  - `bundle_price` is a positive integer strictly less than the sum of the current listing prices.
- `BundleService` gains a read dependency on the listing repository (same database, no cross-service call).
- Error mapping: a count/duplicate/variant/price violation is `INVALID_ARGUMENT`; a foreign or missing listing is
  `PERMISSION_DENIED`.
- `GetBundle` and `ListBundlesBySeller` stay unchanged and keep returning existing bad bundles; they are not deleted or
  migrated. Whether a stored bundle is currently buyable is decided by its consumers (`order-bundle-checkout`).
- No proto change: `CreateBundleRequest/Response` are consumed as they are; the new rules are documented in the
  contract slice comments. No migration.

## Capabilities

### New Capabilities
- `listing-bundle-integrity`: bundle creation is limited to the caller's own, non-variant listings and a real discount.

### Modified Capabilities
none

## Impact

- `team-domain/internal/listing/bundle.go`, `team-domain/internal/rpc/errors.go`, `team-domain/internal/rpc/bundle.go`,
  `team-domain/internal/app/app.go` (wiring), tests beside them, `team-domain/FEATURES.yaml`.
- `platform-e2e/tests/e2e/features/seller/bundle_integrity.feature` and its steps.
- Hot file `team-domain/internal/rpc/listing.go` is shared with `domain-my-listings-filter`; this slice only touches
  `bundle.go` and wiring, so it should not conflict.

## NFR

- `CreateBundle` p95 <= 150 ms locally (one batched listing read of at most 10 ids by primary key, no per-id loop of
  remote calls).
- Availability: a failed listing read fails the call (`UNAVAILABLE`); there is no fallback that skips validation and no
  `*_ENABLED` flag.
- Limits: 2..10 listings per bundle; title as today (required).

## Data classification

- Bundle title, listing ids and price are seller-provided public shop content: purpose shop page and combo sale, basis
  contract, retention while the seller's shop exists. No personal data is added or logged.
- Seller ids and listing ids are never logged raw (sloglint forbidden-keys).

## Threat model

- Assets: other sellers' listings and their trust; the integrity of the bundle price an order will charge.
- Abuse cases: bundling a competitor's listing to sell or discredit it; bundle price above the parts to mislead; a price
  of 0 or negative to give goods away; a bundle of one listing or 1000 listings; probing which listing ids exist
  through different error answers; a variant listing whose variant price/stock the bundle cannot honour.
- Authz rule: `listing.write` scope; the seller is the principal, never a request field; every listing must be owned by
  that principal, and foreign and unknown ids return the same PERMISSION_DENIED so ids cannot be enumerated.
