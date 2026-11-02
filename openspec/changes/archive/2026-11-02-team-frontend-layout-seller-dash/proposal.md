## Why

Plan F rebuilds page layouts on the fe-platform facades. The seller dashboard and the seller order pages still use the pre-P7 dense layout: a muted page background, tight 16 px gutters, a single column of stacked cards, 32 px navigation rows and text-sized secondary links. This change moves them, and the seller shell they share, to the airy, large-radius indigo language of `fe-platform/templates/app`.

## What Changes

- Seller shell (`src/app/seller/layout.tsx`): page background `bg-surface-page`, content in a `<main>` landmark with wider gutters and 32 px section rhythm.
- `SellerSidebar`, `SellerNavList`, `SellerNavDrawer`, `ShopCard`, `SellerLogoutButton`: every navigation row, the collapse toggle, the logout button, the mobile menu button and the public-shop link are 44 px touch targets with `rounded-xl`; the mobile top bar is sticky and names the area above the shop name; the shop card is a muted rounded block.
- `/seller`: KPI row in a labelled section with wider gaps; recent orders and quick actions sit side by side from `lg` (2/3 + 1/3) and stack below; quick actions are full-width outline links with icons; "Xem tất cả" is a button-shaped link. The loading skeleton follows the same grid.
- `/seller/orders`: the status tabs and filter bar sit in the card's top band, the table and pagination in its body.
- `/seller/orders/[id]`: two columns from `lg` (stepper and tabbed items on the left, status, order facts and recipient on the right); on small screens status, facts and recipient come first. The skeleton follows the same grid.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-seller/src/app/seller/layout.tsx`, `(dashboard)/{page,loading}.tsx`, `orders/(list)/{page,loading}.tsx`, `orders/[id]/loading.tsx`.
- `team-frontend-seller/src/features/seller/{SellerSidebar,SellerNavList,SellerNavDrawer,ShopCard,SellerLogoutButton,KpiRow,QuickActions,RecentOrders,OrderDetailView}.tsx`.
- Visible changes only. No server action, data fetching, route, test id, role, accessible name or heading changes.

## Non-goals

Other seller pages (`new`, `[id]/edit`, `ads`, `bundles`, `analytics`, `wallet`, `plans`, `shop`), shared seller pieces used by them (`SellerPageHeader`, `SellerFilterBar`, `SellerPagination`, `SellerSkeletons`), `src/components/ui`, `fe-platform`, tailwind config, dark theme.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend-seller`; no horizontal page overflow at 375, 768 and 1280 px; interactive controls in the shell at least 44 px high; e2e selectors (`kpi-row` direct-child cells, `packing-slip`, tab and pagination navigation names) keep working.
