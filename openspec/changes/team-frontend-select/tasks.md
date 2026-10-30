## 1. Platform

- [x] 1.1 Add `NativeSelect` with label, labelHidden, required mark, help, error, placeholder; tokens `select.native` and `select.icon`; tests, states, typecheck, catalog test, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facade

- [x] 2.1 Rewrite `Select.tsx` as a facade over `NativeSelect` with tests; move `FormItem.test.tsx` to a plain `<select>`; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Fold `FormItem` and sibling labels into Select, replace `aria-label` with `label`/`labelHidden`, drop `className`/`selectSize`/`invalid`, move listing category options to `options`; verify `tsc` has no errors
- [x] 3.2 Check platform-e2e `select_option`, `#role`, `#categoryId`, `return-reason` and combobox-name selectors against the new markup; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-select --strict`
