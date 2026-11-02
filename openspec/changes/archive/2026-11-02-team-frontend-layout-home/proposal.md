## Why

Plan P10 section F rebuilds the main buyer pages on the `@fe-platform/ui` visual language. The home page still used the old look: flat card-bar section headings, small hub tiles, an inline retry link and a hero CTA below the 44 px touch target. This change gives `/` the platform look (mobile-first, airy, large radius, indigo) without touching data or shared layout.

## What Changes

- `Hero`: larger radius and padding, bigger heading, `Mua ngay` as a medium button-link that is full width on mobile, `Xem kho voucher` at the 44 px touch height.
- `ServiceHubs`: larger radius, taller tiles on a tinted icon surface, larger labels from `sm`.
- New page-local `SectionHeading` (h2 + description + optional action) used by `FeedBlock`, `RecentlyViewedRow`; the old card-bar headings go.
- `FeedBlock`: retry is a secondary button-link, `Xem thêm gợi ý` is a full-width-on-mobile secondary button-link.
- `FlashSaleSection`: larger radius, neutral heading, `Xem tất cả` as a ghost button-link, edge-to-edge scroll strip on mobile.
- Page and route skeleton: `flex` column with wider gaps; skeletons keep the new footprints.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/(home)/{page,loading}.tsx`, `team-frontend/src/features/home/*`.
- Visible change only: headings, spacing, radius and control sizes on `/`.

## Non-goals

Shared UI facades, fe-platform, tailwind config, the shell header/footer, `CategoryBar`, `ListingCard`, `RecommendationsRow`, `LoyaltyWidget`, the AI assistant, data fetching and server actions, dark theme.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend`; no horizontal overflow at 375, 768 and 1280 px; every control on the page owned by this change is at least 44 px high; roles, accessible names, ids and `data-testid` used by vitest and platform-e2e are kept.
