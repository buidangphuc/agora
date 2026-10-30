## 1. Platform

- [x] 1.1 Token class maps and preset aspect ratios; `Skeleton`, `PageSkeleton`, `Progress`, `Descriptions`, `Statistic` with JSDoc, states, tests, catalog test entries, changeset; `npm run build` in tokens, `npm run catalog`, `npm run vendor`; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Rewrite the five team components as facades (Vietnamese labels, `page-skeleton` test id, `ErrorState`/`Empty` fallbacks); refresh lock integrity and `npm ci`; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Wrap `Skeleton`/`Progress` call sites that passed `className`, map KpiRow to `tone`, drop `w-full` on catalogue `Descriptions`; verify `tsc` has no errors

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-shared-data --strict`
