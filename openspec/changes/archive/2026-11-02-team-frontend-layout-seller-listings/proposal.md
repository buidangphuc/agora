## Why

P10 F rebuilds the main seller pages on the fe-platform visual language (mobile-first, airy spacing, large radius, indigo, 44 px touch targets). The product list on `/seller` and the listing studio on `/seller/new` and `/seller/[id]/edit` still used the tight pre-P7 spacing: a single long column of form cards, a small textarea, a five-column variant row that squeezed at tablet width, and a product table padded inside the card.

## What Changes

- `src/features/seller/ProductList.tsx`: the card header carries a description; the filter bar sits in its own padded band; the table runs edge to edge in the card; empty, error and pagination states get their own padded bands.
- `src/features/seller/ProductTable.tsx`: larger rounded thumbnails, two-line semibold titles, the stock column hides below `sm` and the stock shows under the product code instead, wider action spacing.
- `src/features/listing/ListingForm.tsx`: two-column layout from `lg` (fields left, sticky status card right), airier sections with `CardDescription`, a 44 px textarea on the input tokens, a larger dashed image drop zone, price/stock and variant grids that wrap at 2 columns before 3/4, a full-width "Thêm phân loại" on mobile, and a taller action bar (still sticky on mobile).
- `src/app/seller/new/loading.tsx`, `src/app/seller/[id]/edit/loading.tsx`: the skeleton mirrors the two-column form.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-seller` only: the files listed above.
- Visible changes only; no server action, data fetching, route, `data-testid`, role, accessible name or heading text changes.

## Non-goals

Shared `src/components/ui/*`, `SellerPageHeader`, `SellerFilterBar`, `SellerPagination`, `SellerSkeletons`, the `/seller` dashboard page itself (KPIs, quick actions, recent orders), other seller pages, tailwind config, fe-platform.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend-seller`; no horizontal overflow at 375, 768 and 1280 px; buttons, links styled as buttons and form controls at least 44 px high.
