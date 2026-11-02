## 1. P0 decisions and starting values

- [x] 1.1 Write `fe-platform/docs/ADR/` records for the eight settled decisions plus Biome-over-ESLint and Connect/proto-over-OpenAPI deviations; verify each ADR has chosen option, reason and reopen condition
- [x] 1.2 Record starting token values (palette, type scale, spacing step, radius) with source file references from `team-frontend`; verify every value lists its source
- [x] 1.3 Write guidelines v0 at `fe-platform/docs/agents/frontend.md` (one page: emphasis hierarchy, spacing/layout rules, empty/error tone, no-disable rule); verify it is under one page and referenced from `fe-platform/AGENTS.md`

## 2. P1 workspace and shell

- [x] 2.1 Create `fe-platform/` with npm workspaces (`packages/*`, `templates/*`); verify `npm install` succeeds
- [x] 2.2 Add `packages/config` (tsconfig strict base, `biome.json`); verify app and package consume it
- [x] 2.3 Add `templates/app`: empty Next.js App Router shell with `(public)` and `(internal)` route groups, strict TS; verify `npm run build -w templates/app` passes
- [x] 2.4 Add repo scripts (disable-comment counter, import-boundary check) and root `npm run check` = biome → tsc → scripts → vitest; verify it passes on the shell
- [x] 2.5 Pin installed Biome/TS/Next versions; check which a11y and import-restriction rules Biome provides, add minimal ESLint only for gaps; record in ADR and `docs/agents/project.md`
- [x] 2.6 Write `fe-platform/docs/agents/project.md` listing the check loop in order; verify it matches `npm run check`

## 3. CI and wiring

- [x] 3.1 Add a CI job running `npm run check` for `fe-platform` and an AGENTS.md repo-map line; verify the job config references the same command
- [x] 3.2 Done-condition: introduce a deliberate lint error, verify `npm run check` exits non-zero, revert, verify it passes again
- [x] 3.3 e2e track: none (no user-facing capability, no FEATURES.yaml entry); confirm `repo-doctor` reports no new drift
