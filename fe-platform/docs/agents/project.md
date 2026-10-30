# fe-platform project facts

## Feedback loop

Run from `fe-platform/`. One command, cheapest first; CI runs the same one.

```bash
npm run check
```

It runs, in order: `biome check .` (format, lint, a11y), `tsc --noEmit` per workspace, `scripts/check-boundaries.mjs`, `scripts/check-disables.mjs`, `scripts/check-tokens.mjs` (raw colours and Tailwind arbitrary values outside `packages/tokens`), `vitest run` (jsdom; includes the axe state matrix for `packages/ui`, the client-boundary scan, the contrast gate and a test that `packages/tokens/dist/tokens.css` equals a fresh build).

After editing `packages/tokens/src/tokens.mjs`, run `npm run build -w packages/tokens` and commit `dist/`. Fix a red step, rerun; never silence it.

Production build: `npm run build -w templates/app`.

## Pinned versions

Biome 1.9.4, TypeScript ^5.6, Next ^15.5, React ^19.3, Vitest ^5.0, Tailwind ^3.4, Radix primitives ^1 (dialog, select, checkbox, toast), axe-core ^4.

`vite` is a direct dev dependency on purpose: left as a vitest peer, npm drops rolldown's native binding from the lockfile and `npm ci` then breaks `vitest`.

`packages/ui` is TypeScript consumed from source: the app lists it in `transpilePackages` and Tailwind `content`; consumers that import `@fe-platform/tokens` need `allowJs` in their tsconfig. Run the dev server or build to see components; axe in jsdom does not judge contrast (the token gate does) or real-browser layout. Node 22. Biome rule names were verified against 1.9.4 (see `docs/ADR/0002-biome-instead-of-eslint.md`).

## CI

`.github/workflows/fe-platform-ci.yml` at the agora root runs `npm ci`, `npm run check` and the template build on changes under `fe-platform/`.

## Layout

`packages/config` (tsconfig and Biome base), `packages/tokens` (tokens, CSS variables, Tailwind preset, contrast pairs), `packages/ui` (Button, Input, Select, Checkbox, Dialog, Toast, Table), `templates/app` (Next App Router shell with `(public)` and `(internal)` route groups), `scripts/` (checks), `docs/ADR/`, `docs/agents/`.

## Not yet built

api-client and reference feature (P4), agent catalog (P5), governance and registry (P6). Tracker: none; work goes through OpenSpec changes in `openspec/changes/`.
