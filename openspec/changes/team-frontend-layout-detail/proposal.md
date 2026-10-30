## Why

P10 section F rebuilds the buyer pages of `team-frontend` in the `@fe-platform/ui` visual language. The listing detail page (PDP) still used the pre-platform anatomy: one dense card around gallery and buy box, small chips and links under the 44 px touch height, and tight sections. This change gives the PDP the platform layout: mobile-first, airy spacing, large radii, indigo accents, 44 px touch targets and no horizontal overflow.

## What Changes

- `src/app/(shop)/listing/[id]/page.tsx`: the header becomes a two-column grid from `md` (gallery left, sticky; buy box in a padded `Card` right) instead of one card; larger title; price panel with a large radius; actions split into a collection/share row and the alert toggles; guarantees as pills; the report control sits in its own row. Body sections and recommendations keep their order with wider gaps.
- `loading.tsx` matches the new grid; `error.tsx` and `not-found.tsx` get airy vertical spacing.
- PDP blocks restyled (markup and classes only): `ImageGallery` (rounded-3xl stage, larger thumbnail gaps), `PdpAnchorNav` (44 px pill links), `VariantSelector` (44 px chips), `ShippingInfo` (stacks on mobile), `SpecsSection`, `ShareButton`, `ReportListingButton`, `AlertToggle` (44 px controls), the Q&A blocks (`QASection`, `QuestionList`, `QAAskForm`, `QAAnswerForm`) and the review blocks (`ReviewSection`, `ReviewList`, `ReviewRatingFilter` as 44 px indigo pills, `AiReviewSummary`, `ReviewHideButton`).

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/listing/**`, `team-frontend/src/features/listing/{ImageGallery,PdpAnchorNav,VariantSelector,ShippingInfo,SpecsSection,ShareButton,ReportListingButton}.tsx`, `team-frontend/src/features/listing/qa/*`, `team-frontend/src/features/review/{ReviewSection,ReviewList,ReviewRatingFilter,AiReviewSummary,ReviewHideButton}.tsx`, `team-frontend/src/components/alerts/AlertToggle.tsx` (rendered only by the PDP).
- Visible changes only; no data fetching, server action, route, test id, role, accessible name or heading changes.

## Non-goals

Shared UI facades (`src/components/ui/*`), `fe-platform`, the tailwind config, shared layouts and the ConsumerShell; components shared with other pages (`ListingCard`, `FavoriteButton`, `AddToCollectionButton`, `ShopHeaderCard`, cart `PurchasePanel`/`BuyBar`, recommendations); seller and admin code; dark theme.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend`; no horizontal overflow at 375, 768 and 1280 px; interactive controls on the PDP at least 44 px high; platform-e2e selectors (`h1`, `pdp-price`, `pdp-rating`, `fieldset label`, `review-filter`, `alert-toggle`, the anchor nav name) keep working.
