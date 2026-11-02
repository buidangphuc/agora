## Why

P10 F rebuilds the main buyer pages on the fe-platform visual language (mobile-first, airy spacing, large radius, indigo, 44 px touch targets). The cart page still used the tight pre-P7 spacing, and its quantity stepper was 32 px high, below the touch target.

## What Changes

- `src/app/(shop)/cart/page.tsx`: wider vertical rhythm, larger page heading, the empty state sits in a padded Card with a large primary link.
- `src/features/cart/CartGroups.tsx`: each line stacks on mobile (checkbox, larger rounded thumbnail, title, price, then the stepper and line total aligned under the text) and becomes one row from `sm`; long shop names truncate instead of overflowing.
- `src/features/cart/CartQuantityControl.tsx`: the stepper gets `min-h-touch`, so its buttons are 44 px high.
- `src/features/cart/CartSelection.tsx`: the list column can shrink (`min-w-0`) and the grid gap grows at `lg`.
- `loading.tsx` and `error.tsx` follow the same footprint and spacing.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/cart/*` and `team-frontend/src/features/cart/{CartGroups,CartQuantityControl,CartSelection}.tsx` only.
- Visible changes only; no server action, data fetching, route, `data-testid`, role, accessible name or heading text changes.

## Non-goals

Shared `src/components/ui/*`, `OrderSummary`/`MobileSummaryBar` (owned by the order feature), the product-page purchase components in `features/cart` (`PurchasePanel`, `BuyBar`, `AddToCartButton`), layouts, tailwind config, fe-platform.

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend`; no horizontal overflow at 375, 768 and 1280 px; buttons, steppers and checkboxes at least 44 px high.
