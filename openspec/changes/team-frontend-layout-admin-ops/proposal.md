## Why

Plan P10 section F rebuilds the main pages of the three Next apps in the fe-platform visual language. In team-frontend-admin, the admin shell wraps its eight navigation links onto several lines at 375 px, the operations cockpit (`/admin/cockpit`) is still a dark HUD built from raw palette classes (slate, emerald, cyan, amber), and the moderation queue (`/admin/listings`) is a bare unstyled table with no page heading.

## What Changes

- Admin shell: brand mark and title, a truncating user name and the logout button on one row; the `Quản trị` navigation becomes one horizontally scrolling row of 44 px ghost links (no wrapping at any width); main content gets wider gutters from `md`.
- Cockpit: light fe-platform surfaces only. A status `Badge`, a `text-2xl md:text-3xl` heading, quick links as `secondary`/`ghost` link buttons, four metric `Card`s, the services radar on the platform `Table` (sticky service column, port hidden below `md`, P99 hidden below `sm`, status as a toned `Badge`), recent orders and traces in flush `Card`s with footers. No raw palette class remains.
- Moderation queue: page heading and description on `/admin/listings`, the queue in a `Card` on the platform `Table` (`sm` action buttons, 44 px), an `Empty` state for "Hàng chờ trống", and "Trang sau" as a secondary link button. The error state sits in a padded `Card`.
- Presentation only: no change to server actions, data fetching, polling, `data-testid`, roles, accessible names, link targets or the texts used by tests and platform-e2e.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-admin/src/components/shell/AdminShell.tsx`, `src/features/admin/CockpitView.tsx`, `src/features/admin/ModerationQueueView.tsx`, `src/app/admin/listings/page.tsx`.
- `ModerationQueueView` is shared with the queue tab of `/admin/cases`, so that tab picks up the new table and empty state too.
- Visible changes: light cockpit instead of the dark HUD, emoji removed from cockpit headings and links, a new `h1` on the moderation page.

## Non-goals

`src/components/ui`, fe-platform, tailwind config, and the other admin pages (cases, users, audit, vouchers, campaigns, plans).
