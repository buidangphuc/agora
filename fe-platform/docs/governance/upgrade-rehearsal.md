# Upgrade rehearsal: one breaking token change

Date: 2026-10-29. Goal: show that a consumer outside the workspace upgrades across a breaking token change by following `versioning.md`, `contributing.md` and `upgrade-guide.md`. The rename is rehearsal only: it was made in a throwaway git worktree; this repository's tokens still contain `border-subtle`.

## Setup

- Registry: Verdaccio on 127.0.0.1:4873 (`infra/registry/`), holding `tokens` 0.1.0 and 0.1.1 and `ui` 0.1.0 (0.1.1 of tokens was the real first release: it added type declarations after the consumer hit TS7016 on the 0.1.0 install).
- Consumer: `examples/consumer`, installed only from the registry, using the class `border-border-subtle` in `app/page.tsx` plus Button, Link, Table.

## Steps and results

1. Breaking change in the worktree: rename `border-subtle` to `border-soft` in `packages/tokens/src/tokens.mjs`, rebuild `dist` (css and types), regenerate the catalog. `npm run check` in the worktree: green, 126 tests.
2. Major changeset for `@fe-platform/tokens` with a `## Migration` section (class and CSS variable rename, grep command).
3. `npm run release:version`: tokens 0.1.1 to 1.0.0; ui bumped to 0.1.1 with its tokens range rewritten to `^1.0.0`; CHANGELOG carries the Migration text.
4. `npm run release:publish` to the local registry: published `tokens@1.0.0` and `ui@0.1.1` (no git tags created).
5. Consumer, upgrade guide step 2: `npm install @fe-platform/tokens@^1.0.0 @fe-platform/ui@^0.1.1` (0.5 s).
6. Consumer, step 3: `npm run check` fails at the class check in 0.4 s: `no CSS generated for class: border-border-subtle`, `1 missing of 60 classes`.
7. Evidence that the class check is what catches it: `next build` alone, run at this point, reports "Compiled successfully". The renamed token otherwise breaks silently (the border just disappears).
8. Migration step 4: `grep -rn "border-subtle" app` found the single use in `app/page.tsx`; replaced it with `border-border-soft`.
9. `npm run check` in the consumer: class check 0 missing of 59, typecheck, build all pass (about 8 s).

## Cleanup

`tokens@1.0.0` and `ui@0.1.1` were unpublished from the local registry; it lists `tokens` 0.1.0 and 0.1.1 and `ui` 0.1.0 again. The consumer was restored to the committed `package.json`, lockfile and page and passes `npm run check`. The worktree was deleted.

## Findings

- Without the consumer's class check a token rename is invisible to type checking and the build. The upgrade guide therefore requires the class check in every consumer.
- `changeset publish` creates git tags by default and npm refuses to publish without a token even to an anonymous registry; `scripts/release-publish.mjs` handles both for the loopback registry.
- Tokens shipped without type declarations in 0.1.0; fixed in 0.1.1 and guarded by a test that compares `dist/types` with a fresh build.
