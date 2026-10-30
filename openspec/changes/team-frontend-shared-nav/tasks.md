## 1. Platform

- [x] 1.1 Add Tabs, Pagination, Breadcrumb, Stepper and Timeline to `@fe-platform/ui` with token class maps, states, tests, catalog entries and a changeset; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Revendor the tarballs, refresh the lock integrity and rewrite the five team-frontend components as facades; remove `TabsClient.tsx` and `tabStyles.ts`; verify the existing component tests pass unchanged

## 3. Call sites

- [x] 3.1 Replace `className` on the six call sites with wrapper divs and update the search pagination selector; verify `tsc` has no errors
- [x] 3.2 Scan the platform sources in team-frontend `tailwind.config.ts`; verify every token class appears in the built CSS

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-shared-nav --strict`
