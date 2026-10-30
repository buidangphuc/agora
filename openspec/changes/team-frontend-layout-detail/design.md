## Decisions

- Reference layout: `fe-platform/templates/app/app/(public)/products/[id]/page.tsx` (two columns from `md`, image with a large radius, details in a `Card`). The PDP buy box is taller than the gallery, so the gallery column is the sticky one (`md:sticky md:top-32 md:self-start`); a sticky column taller than the viewport would never stick.
- Grid: one column on mobile, two equal columns at `md`, `lg:grid-cols-12` with 6/6 spans and wider gaps at `lg`. Both columns carry `min-w-0` so long titles and share links cannot widen the grid.
- Touch targets use the `min-h-touch` token (44 px from the platform primitives) on hand-rolled controls: anchor links, variant chips, rating filter pills, share, alert toggles and the report link. Platform `Button`/`Link` already carry it.
- Rating filter "on" state moves from `bg-text-primary` (near-black) to `bg-action-primary` (indigo), the platform accent.
- The rating count in the header links to `#reviews` (same text, new anchor), so the summary is one tap from the review section.
- Only semantic token classes are added; untouched legacy classes in `LiveFlashSaleStock` (flash-sale palette, kept raw by the palette change) are left alone.
- Existing comments in untouched data-loading code are kept; no new comments are added. JSX section comments in the rewritten header were dropped.

## Visual check

`next start -p 3112` on the production build, Playwright at 375, 768 and 1280 px on `/listing/abc` and `/listing/abc?variant=x&rating=5#reviews`. The gateway was not running, so the route rendered `error.tsx`; at every width `scrollWidth == innerWidth` and no visible control in `main` was under 44 px. The full PDP anatomy is covered by the vitest page tests only.

## Risks

- `FavoriteButton` (28 px, gallery overlay) and the `AddToCollectionButton` trigger (about 34 px) stay under 44 px: both are shared with `ListingCard` and the favorites page and belong to those pages' changes.
- The full PDP was not seen in a browser with live data; spacing of a real listing with many variants, a flash sale and long descriptions is unverified.
