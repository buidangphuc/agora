## 1. Code — team-frontend: shared foundations

- [x] 1.1 Add the internal `useDialog` hook (focus trap, Escape, return focus, scroll lock) with tests; verify the Vitest file passes
- [x] 1.2 Add `Skeleton`, `Empty`, `Spin` (used by the others) with tests; verify the tests pass and the token lint is clean

## 2. Code — team-frontend: existing primitives to the contract

- [x] 2.1 Button: `aria-busy`, `aria-disabled`, focus-visible ring, width-preserving loading; verify the loading-width and disabled scenario tests pass
- [x] 2.2 Input: label/`htmlFor`, `aria-describedby`, `aria-invalid`; verify the error-announced scenario test passes
- [x] 2.3 Modal on `useDialog` with `role="dialog"`, `aria-modal`, `aria-labelledby`; verify the focus-trap scenario test passes
- [x] 2.4 Tabs with tablist roles and arrow keys, plus the server-compatible link variant (`hrefFor`); verify the keyboard and link-tabs scenario tests pass
- [x] 2.5 Badge, Card (`loading`), PriceTag, Result, Descriptions, Statistic (`loading`), Stepper (`aria-current`) on Tier 2/3 tokens; verify their tests pass and the token lint is clean

## 3. Code — team-frontend: new components

- [ ] 3.1 Navigation: `Breadcrumb`, `Pagination` (server-compatible links); verify the pagination scenario test passes
- [ ] 3.2 Data Entry: `FormItem`, `Select`, `Checkbox`, `Radio`/`RadioGroup`, `QuantityPicker`, `Rate`; verify the bounds scenario test and per-component tests pass
- [ ] 3.3 Data Display: `Tag`, `Table` (empty/loading/error), `Timeline`, `Avatar`, `Image` (aspect, fallback, lazy); verify the empty-table and image-fallback scenario tests pass
- [ ] 3.4 Feedback: `Alert`, `Drawer` (on `useDialog`), `Progress`; verify their tests pass
- [ ] 3.5 Export everything from `src/components/ui/index.ts`; verify `npm run check` and `npx next build` pass

## 4. Code — team-frontend: catalogue

- [ ] 4.1 Add `/dev/ui` rendering every component in every state, 404 in production; verify a smoke test renders it and `NODE_ENV=production next build && next start` returns 404 for `/dev/ui`

## 5. E2E — platform-e2e

- [ ] 5.1 Add `team-frontend/FEATURES.yaml` entries for every scenario (`status: planned`); verify `make -C platform-e2e features-check`
- [ ] 5.2 Add `tests/e2e/features/frontend/ui_components.feature` driving `/dev/ui` (keyboard focus ring, modal focus trap, tabs arrows, quantity bounds); verify green and flip to `automated`
- [ ] 5.3 Run `openspec validate ui-core-components --strict`; verify it is valid
