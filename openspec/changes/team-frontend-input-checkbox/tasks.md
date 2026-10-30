## 1. Platform

- [x] 1.1 Input required mark; Checkbox indeterminate, labelHidden, id, data-testid; tokens, tests, states, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Rewrite `Input.tsx` and `Checkbox.tsx` as facades with tests; stub `ResizeObserver` in `vitest.setup.ts`; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Fold `FormItem` labels into Input, drop `className`/`inputSize`, move checkboxes to `onCheckedChange`/`labelHidden`/`indeterminate`; verify `tsc` has no errors
- [x] 3.2 Update tests that relied on native checkboxes or label text; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 Run the colour e2e scenarios against the local build
- [x] 4.3 `openspec validate team-frontend-input-checkbox --strict`
