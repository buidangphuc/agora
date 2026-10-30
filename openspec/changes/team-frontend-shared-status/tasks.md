## 1. Platform

- [x] 1.1 Tag, ClosableTag, Badge, Empty, Result with tone/tag/badge/empty/result tokens, states, tests, typecheck guards, catalog test, changeset, catalog and vendor; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Rewrite Tag, TagClient, Badge, Empty and Result as facades, remove `tagView.tsx`; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Replace `className` on Tag, Badge, Empty and Result with base classes or wrapper divs; verify `tsc` has no errors
- [x] 3.2 Point tone-class tests at `components.tone`; scan platform sources in `tailwind.config.ts`; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-shared-status --strict`
