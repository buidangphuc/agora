## Why

Plan P10 section D moves UI components that two or more of the buyer, seller and admin apps use into `@fe-platform/ui`. Tag, Badge, Empty and Result are used by the buyer app and by seller or admin screens, so each app should get them from the platform with token-backed classes instead of a team-frontend copy.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): new `Tag` (server), `ClosableTag` (client, 44 px close button), `Badge` (soft or solid, `sm`/`md`, pill), `Empty` and `Result` (optional `statusLabel`, English defaults). Tokens add the shared `tone` class map and the `tag`, `badge`, `empty` and `result` class maps; tones use semantic colours only (`border-current` on the coloured tones).
- team-frontend `Tag`, `TagClient`, `Badge`, `Empty` and `Result` become facades: `color` maps to `tone`, `closeLabel` keeps the "Xoá" default, Badge `mall` and `discount` map to solid danger and solid warning, `xs` maps to `sm`, Empty keeps the "Không có dữ liệu" default, Result passes the Vietnamese status labels. `tagView.tsx` is removed. `className` is gone from all four.
- Call sites that passed `className`: `whitespace-nowrap` and `shrink-0` move into the platform tag and badge base, `OrderStatusBadge` drops its unused `className`, Timeline and the dev catalogue wrap Empty and Result in a div.
- team-frontend `tailwind.config.ts` scans the platform tokens and ui sources so platform class maps are generated.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/packages/{tokens,ui}`, `fe-platform/scripts/catalog.test.mjs`, `fe-platform/.changeset/ui-status-display.md`, `team-frontend/src/components/ui`, about eight call sites and three tests, `team-frontend/tailwind.config.ts`.
- Visible changes: coloured tags and badges lose their 10 % tint and use a white fill with a coloured border and text; the discount badge is amber with white text instead of yellow with indigo; a closable tag has a 44 px hit area.

## Non-goals

Alert, Statistic and Timeline (they keep `tones.ts`); tint tokens; dark theme; Button, Card, Input, Checkbox, Table, Select, Modal, Toast, Alert, Link.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; role, accessible-name and `data-testid` selectors keep working.
