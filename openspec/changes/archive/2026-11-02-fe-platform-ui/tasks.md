## 1. Package and tokens

- [x] 1.1 Create `fe-platform/packages/ui` (package.json, tsconfig from `packages/config`, `cx` helper, entry file); add `transpilePackages` to the template; verify `npm run typecheck` and template build pass with an empty export
- [x] 1.2 Add `border-danger` and `surface-overlay` tokens and extend the token class maps (select, checkbox, dialog, toast, table); verify contrast gate and class-map test still pass in both themes

## 2. Native components

- [x] 2.1 Button (variants primary/secondary/ghost, sizes, `loading`, `disabled`, explicit `type`); verify states render and axe reports zero violations
- [x] 2.2 Input (label, `errorText`, `aria-invalid`, `aria-describedby`, disabled); verify the error state is announced via text, not colour only
- [x] 2.3 Table (semantic `table/thead/th scope`, caption, empty state slot); verify axe clean and header scopes present

## 3. Radix-wrapped components

- [x] 3.1 Checkbox with label and error state (`"use client"`); verify keyboard toggle and axe clean
- [x] 3.2 Select with label, placeholder, error state (`"use client"`); verify open/select by keyboard and axe clean
- [x] 3.3 Dialog with title, description, close control, focus return (`"use client"`); verify focus moves in and returns, Escape closes
- [x] 3.4 Toast provider and `useToast` with live-region semantics (`"use client"`); verify message is announced and dismissible

## 4. Gates

- [x] 4.1 State-matrix test: every component renders every declared state and passes axe (colour-contrast rule off, stated in the test); verify a deliberately inaccessible fixture fails
- [x] 4.2 Client-boundary test: files importing Radix or using hooks start with `"use client"`; verify a seeded violation fails
- [x] 4.3 Prop-surface test: no component accepts `className` or `style`; verify a seeded violation fails

## 5. Wiring and done-condition

- [x] 5.1 Render one of each component on the template `(internal)/dashboard` page; verify `npm run build -w templates/app` passes
- [x] 5.2 Update `docs/agents/project.md`, `AGENTS.md` and `docs/agents/frontend.md` (use `packages/ui` components, no hand-rolled controls); run `npm run check`
- [x] 5.3 Done-condition: seed an unlabeled button, verify `npm run check` exits non-zero, revert, verify exit 0
- [x] 5.4 e2e track: none (no user-facing capability, no FEATURES.yaml entry)
