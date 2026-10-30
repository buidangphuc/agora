## 1. Token source

- [x] 1.1 Create `fe-platform/packages/tokens` (package.json, tsconfig using `packages/config`) with `src/tokens.mjs`: primitives from `docs/tokens-starting-values.md`, semantic light and dark referencing primitives, component class maps; verify `npm run typecheck` passes with checkJs and every semantic value resolves to a primitive
- [x] 1.2 Define dark theme semantic values; verify each is a declared pair in task 3.1

## 2. Build outputs

- [x] 2.1 Add `scripts/build.mjs` emitting `dist/tokens.css` (light, `[data-theme="dark"]`, prefers-color-scheme fallback); verify output contains every semantic token in both themes
- [x] 2.2 Add `src/preset.mjs` replacing colour, spacing, radius, fontSize theme keys with token-backed values; verify an unknown utility such as `p-[13px]` is not generated
- [x] 2.3 Add vitest test that rebuilds into a temp dir and equals committed `dist/`; verify it fails after editing a token without rebuilding
- [x] 2.4 Consume preset and CSS in `templates/app` (tailwind config, global CSS import); verify `npm run build -w templates/app` passes

## 3. Gates

- [x] 3.1 Add `src/contrast.mjs` and `src/pairs.mjs` (declared text/bg and non-text pairs, both themes, normal/large); verify tests pin black/white 21:1 and `#ee4d2d` on white 3.66:1
- [x] 3.2 Add vitest suite checking every declared pair meets 4.5 (normal) / 3 (large, non-text) and that a deliberate low-contrast pair is reported failing; verify action-primary with text-inverse passes in both themes
- [x] 3.3 Add root `scripts/check-tokens.mjs` (raw colour literals, arbitrary Tailwind values, `tokens-allow: <reason>`) with tests; verify it flags a seeded `#fff` and `w-[13px]` in a fixture
- [x] 3.4 Wire token check into `npm run check` (after boundaries) and update `docs/agents/project.md` and `AGENTS.md`

## 4. Done-condition

- [x] 4.1 Deliberately set a failing pair, verify `npm run check` exits non-zero, revert, verify exit 0; deliberately add `bg-[#fff]` in the template, verify non-zero, revert
- [x] 4.2 Update `docs/tokens-starting-values.md` to point at `packages/tokens` as authority and remove the known-issue section; run pre-commit-ci checks (CI workflow unchanged, still `npm run check`)
- [x] 4.3 e2e track: none (no user-facing capability, no FEATURES.yaml entry)
