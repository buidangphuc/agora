# fe-platform project facts

## Feedback loop

Run from `fe-platform/`. One command, cheapest first; CI runs the same one.

```bash
npm run check
```

It runs, in order: `biome check .` (format, lint, a11y), `tsc --noEmit` per workspace, `scripts/check-boundaries.mjs`, `scripts/check-disables.mjs`, `vitest run`. Fix a red step, rerun; never silence it.

Production build: `npm run build -w templates/app`.

## Pinned versions

Biome 1.9.4, TypeScript ^5.6, Next ^15.5, React ^19.3, Vitest ^5.0. Node 22. Biome rule names were verified against 1.9.4 (see `docs/ADR/0002-biome-instead-of-eslint.md`).

## CI

`.github/workflows/fe-platform-ci.yml` at the agora root runs `npm ci`, `npm run check` and the template build on changes under `fe-platform/`.

## Layout

`packages/config` (tsconfig and Biome base), `templates/app` (Next App Router shell with `(public)` and `(internal)` route groups), `scripts/` (checks), `docs/ADR/`, `docs/agents/`.

## Not yet built

Tokens (P2), ui (P3), api-client and reference feature (P4), agent catalog (P5), governance and registry (P6). Tracker: none; work goes through OpenSpec changes in `openspec/changes/`.
