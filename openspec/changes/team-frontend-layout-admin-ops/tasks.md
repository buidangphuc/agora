## 1. Layout

- [x] 1.1 Restyle the admin shell header and make the eight-link navigation one scrolling row; verify the AdminShell and admin layout vitest files pass
- [x] 1.2 Rebuild the cockpit on Card, Badge, Table and Link with semantic tokens; verify the CockpitView vitest file passes
- [x] 1.3 Rebuild the moderation queue and the listings page header and error state; verify the ModerationQueueView and listings page vitest files pass and `tsc` has no errors

## 2. Verification

- [x] 2.1 `npm run check` and `npm run build` in `team-frontend-admin`
- [x] 2.2 Load the routes at 375, 768 and 1280 px: no horizontal overflow, controls at least 44 px high
- [x] 2.3 `openspec validate team-frontend-layout-admin-ops --strict`
