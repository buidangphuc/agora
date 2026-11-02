## 1. Platform

- [x] 1.1 Add `fullWidth="mobile"` to Button and tokens, tests, state, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Rewrite `Button.tsx` and `Card.tsx` as facades; update their tests; verify the tests pass

## 3. Call sites

- [x] 3.1 Convert every `className` on Button, Card and Card parts; verify `tsc` has no errors
- [x] 3.2 Update tests that asserted old classes; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 Run the colour e2e scenarios against the local build
- [x] 4.3 `openspec validate team-frontend-button-card --strict`
