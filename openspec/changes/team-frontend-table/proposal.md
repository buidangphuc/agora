## Why

C1b and C2 moved Button, Card, Input and Checkbox onto `@fe-platform/ui`. C3 does the same for Table, so the buyer, seller and admin screens share one data table with the platform caption rule, tokens and states instead of a local copy.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): `Table` is generic over its row type and gains `render` cells, column `align`, `sticky` (first column pinned below `md`) and `hideBelow` (`sm` or `md`), `rowKey`, `rowTestId`, `captionHidden`, a loading state (`loading`, `loadingRows`, `aria-busy`) and an error state (`error`, `onRetry`, `retryLabel`) with a retry Button. The table sits in a horizontal scroll container. Tokens add the matching `table` class maps; existing `rows`/`header` callers keep working.
- `team-frontend/src/components/ui/Table.tsx` becomes a facade: `dataSource`, `rowKey`, `dataIndex`, `title`, `rowProps` (`data-testid`), `emptyText`, `loading`, `error` and `onRetry` keep their names and map onto the platform props; `caption` is now required and always screen-reader only; `className` on the table and on columns is gone.
- Column `className` at call sites becomes `sticky: true` or `hideBelow`; minimum widths move onto the cell content (`min-w-48`, `min-w-56`, `min-w-64`).
- `team-frontend/tailwind.config.ts` scans the platform component class maps so classes only the platform uses (`sm:table-cell`, `md:table-cell`, checkbox fill) are generated.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src` (13 files with 17 Table call sites, the facade and its test), `team-frontend/tailwind.config.ts`, `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: tables lose the rounded outer frame and take the platform padding (16 px cells, 16 px body text); the empty state is centred text without the Empty icon; the error state is the platform alert with a secondary "Thử lại" button.

## Non-goals

Select, Link, Dialog, Toast, Alert swaps; sorting, pagination or selection inside Table; dark theme; server actions or routes.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; e2e selectors that use the table role and caption name, `columnheader`, `row` and `data-testid` keep working.
