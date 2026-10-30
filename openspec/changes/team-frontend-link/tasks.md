## 1. Platform

- [x] 1.1 Link `variant`/`size`/`fullWidth` over `next/link`; `link.subtle` token; tests, states, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facade

- [x] 2.1 Add `src/components/ui/Link.tsx` facade with tests and a barrel export; verify the facade and barrel tests pass

## 3. Call sites

- [x] 3.1 Move styled text links and button-like links to the facade (wrapper `span` for layout), keep structural links on `next/link`; route both `LinkButton` components through the facade; delete the three link-button class helpers; verify `tsc` has no errors
- [x] 3.2 Keep href, accessible names and `data-testid`; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-link --strict`
