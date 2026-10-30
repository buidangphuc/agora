## Why

Plan P10 section D moves team-frontend components that two or more of the buyer, seller and admin apps use into `@fe-platform/ui`. A usage scan showed `PriceTag` (with `formatPrice`), `Image`, `Avatar` and `Drawer` are used by the buyer app and by the seller or admin app, so each app would otherwise keep its own copy.

## What Changes

- `@fe-platform/ui` (minor): new `PriceTag`, `formatPrice`, `Image` (with the `Aspect` type), `Avatar`, `initials` and a Radix `Drawer`. Visible strings are props with English defaults (`fallbackLabel`, `anonymousLabel`, `label`, `closeLabel`). The catalog marks lower-case exports as `kind: "function"`.
- `@fe-platform/tokens` (minor): `priceTag`, `image`, `avatar` and `drawer` class maps; spacing steps 14 and 20; the preset gains `aspectRatio` (`4/3`, `3/4`, `2/1`).
- team-frontend `PriceTag`, `Image`, `Avatar`, `Drawer` become facades: `PriceTag`, `formatPrice` and `initials` are re-exports; `Image`, `Avatar` and `Drawer` pass the Vietnamese labels; `Drawer` keeps `isOpen`. `ImageClient.tsx` is deleted; `aspect.ts` reads the classes from tokens.
- team-frontend `tailwind.config.ts` scans the platform ui sources and token class maps so their classes reach the built CSS.
- `className` is gone from `PriceTag` and `Image`: `Image` takes `radius` (`xl`, `lg`, `none`); the two `PriceTag` call sites that passed `className` use a wrapper div.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/{tokens,ui}`, `fe-platform/scripts/catalog*.mjs`, `fe-platform/.changeset/ui-media-price-drawer.md`, `team-frontend/src/components/ui`, six call sites in `team-frontend/src/features`, two tests, `team-frontend/tailwind.config.ts`.
- Visible changes: the discount badge uses `surface-muted` instead of the `primary-50` tint; the Drawer overlay is the scrim token without blur; the Drawer close button is 44 px high; the ListingCard image follows the Card corner radius.

## Non-goals

Button, Card, Input, Checkbox, Table, Select, Modal/Dialog, Toast, Alert and Link (other branches own them); `Skeleton`; `formatSoldCount`; a tint token for the discount badge; dark theme.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green in `team-frontend`; roles, accessible names and `data-testid` used by e2e stay the same.
