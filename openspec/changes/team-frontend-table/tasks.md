## 1. Platform

- [x] 1.1 Generic Table with render cells, align, sticky, hideBelow, rowKey, rowTestId, captionHidden, loading and error states; tokens, tests, states, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facade

- [x] 2.1 Rewrite `Table.tsx` as a facade over the platform Table with its test; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Replace column `className` with `sticky`/`hideBelow` or a content min-width, drop table `className`; scan platform class maps in `tailwind.config.ts`; verify `tsc` has no errors and the build CSS contains `sm:table-cell` and `md:table-cell`

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 `openspec validate team-frontend-table --strict`
