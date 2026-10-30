## Why

Plan P10 section D moves team-frontend components that two or more of the buyer, seller and admin apps use into `@fe-platform/ui`. Tabs, Pagination, Breadcrumb, Stepper and Timeline are used by buyer and seller screens, so they become platform components and the team-frontend files become thin facades.

## What Changes

- `@fe-platform/ui` (minor): new `Tabs` (client tablist with arrow keys and panels, or server link list with `hrefFor`), `Pagination`, `Breadcrumb`, `Stepper` and `Timeline`. Labels are props with English defaults (`label`, `previousLabel`, `nextLabel`, `pageLabel`). No `className` or `style`.
- `@fe-platform/tokens` (minor): `tabs`, `pagination`, `breadcrumb`, `stepper` and `timeline` class maps; tab, page, breadcrumb and step links keep `min-h-touch`. The class-map test accepts side border widths such as `border-b-2`.
- team-frontend: `Tabs`, `Breadcrumb` and `Stepper` re-export the platform; `Pagination` passes the Vietnamese labels; `Timeline` keeps loading, error and empty states and renders the platform list. `TabsClient.tsx` and `tabStyles.ts` move to the platform.
- Call sites that passed `className` (cart breadcrumb, order pagination, search pagination, order stepper, dev catalogue) use a wrapper div.
- team-frontend `tailwind.config.ts` scans `@fe-platform/ui` sources and the token class maps so platform classes reach the app CSS.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/{tokens,ui}`, `fe-platform/scripts/catalog.test.mjs`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- `team-frontend/src/components/ui` (5 facades, 2 files removed), 6 call sites, 1 test selector, `tailwind.config.ts`, `vendor/*.tgz`, `package-lock.json`.
- Visible changes: tabs, page links, breadcrumb links and step links are at least 44 px high; page cells are 48 px wide; timeline and vertical stepper dots are 12 px; the warning dot uses the warning text colour; the horizontal stepper connector is a flex line instead of an absolute bar.

## Non-goals

Button, Card, Input, Checkbox, Table, Select, Dialog, Toast, Alert and Link; seller `CompactPagination`; dark theme.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; roles, accessible names, `aria-current` and `data-testid` used by tests and e2e stay the same.
