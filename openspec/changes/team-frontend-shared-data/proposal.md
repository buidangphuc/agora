## Why

Plan P10 section D moves team-frontend UI components that two or more of the future buyer, seller and admin apps use into `@fe-platform/ui`. A usage scan shows that buyer pages and seller or admin pages all use the data display components Descriptions, Statistic, Skeleton, PageSkeleton and Progress. This change gives those apps one shared implementation built from tokens.

## What Changes

- `@fe-platform/ui` (minor) adds `Skeleton`, `PageSkeleton`, `Progress`, `Descriptions` and `Statistic`. They are server-safe, take no `className` or `style`, and take their classes from token class maps. `Descriptions` and `Statistic` take a `fallback` node that replaces the body or value, used for error and empty states. `Statistic` adds `tone="danger"`. `Skeleton` and `PageSkeleton` take a screen-reader `label`.
- `@fe-platform/tokens` (minor) adds the class maps `skeleton`, `pageSkeleton`, `progress`, `descriptions` and `statistic`. The Tailwind preset adds the `4/3`, `3/4` and `2/1` aspect ratios.
- In team-frontend, the five components in `src/components/ui` become facades. `Skeleton` and `PageSkeleton` pass the Vietnamese loading label, and `PageSkeleton` also keeps `data-testid="page-skeleton"`. `Progress` keeps its `label` default. `Descriptions` and `Statistic` map `error`/`onRetry` to the team `ErrorState`, and `Descriptions` maps empty items to the team `Empty`.
- Call sites that passed `className` to `Skeleton` or `Progress` wrap the component in a `div` that carries those classes. KpiRow's ineffective `border-danger` class becomes `tone`. The catalogue page's `w-full` on `Descriptions` is removed.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/{tokens,ui}`, `fe-platform/scripts/catalog.test.mjs`, `fe-platform/.changeset/ui-data-display.md`; `team-frontend/src/components/ui`, about ten call-site files, two team tests, `team-frontend/tailwind.config.ts` (scans the vendored token class maps), `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`.
- Visible changes: skeleton blocks use `surface-muted`. The large avatar skeleton is 48 px instead of 56 px. Progress bars are 8 px (sm) and 12 px (md). The success fill uses `text-success`. Descriptions text is 14 px. Overdue KPI values are red.

## Non-goals

Button, Card, Input, Checkbox, Table, Select, Modal/Dialog, Toast, Alert and Link, which other branches own. A platform error or empty component. Dark theme. `DescriptionItem.span`, which no call site rendered.

## Non-functional requirements

`npm run check` passes in `fe-platform`, and `npm run check` and `npm run build` pass in `team-frontend`. e2e selectors keep working: `data-variant`, `dt`/`dd`, `.animate-pulse`, `progressbar` and `page-skeleton`.
