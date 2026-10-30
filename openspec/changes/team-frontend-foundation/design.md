## Context

See proposal.md. `fe-platform` packages `tokens`, `ui` and `api-client` already publish TypeScript source (`files: ["src", ...]`); `tokens` also ships `dist/`. `npm pack` normalises tarball timestamps, so output is reproducible. `team-frontend` is `private`, builds from `./team-frontend` (compose context) with `npm ci --ignore-scripts` then `npm run build`; its generated proto stubs (`src/generated`) are gitignored and made by `npm run proto`. Cookie, ports and the split of apps are decided in plan P10 and recorded in the ADR.

## Decisions

1. **Vendored tarballs with stable names:** `vendor/fe-platform-tokens.tgz`, `fe-platform-ui.tgz`, `fe-platform-api-client.tgz`. Stable names keep `package.json` and `package-lock.json` free of version churn; the lockfile integrity hash changes when contents change, which is the intended signal. Alternative considered: versioned file names; rejected because every package bump would rename files and edit dependencies.
2. **`scripts/vendor.mjs`** in `fe-platform`: for each package runs `npm pack --pack-destination <tmp> --workspace <name>` after building `tokens` (so `dist/` is current), renames the output to the stable name and copies it into every directory listed in `vendor.targets` in `fe-platform/package.json` (initially `../team-frontend/vendor`). `--check` packs into a temp directory and compares bytes with the committed files, printing each mismatch.
3. **`npm run check`** gains `vendor:check` so a changed package fails the fe-platform gate until `npm run vendor` is run and the tarballs are committed. The team-frontend app is not checked by the fe-platform gate beyond the tarballs; its own `npm run check` covers the rest.
4. **Consumption:** `team-frontend/package.json` gets `@fe-platform/tokens`, `@fe-platform/ui` and `@fe-platform/api-client` as `file:./vendor/...tgz` dependencies; `next.config.mjs` adds `transpilePackages` for the three (they ship TypeScript); no Tailwind preset yet (B1) so nothing changes visually. The `ui` and `api-client` dependencies (Radix, connect-web, next, react ranges) resolve through npm as for any package.
5. **Proof of use without changing UI:** a vitest file `src/app/fe-platform-packages.test.ts` imports the three packages (api-client's `ApiError` and `toApiError`, tokens' `components`, ui's `Button` export) and asserts they resolve; `server-only` needs the existing alias in vitest.
6. **Clean-copy check:** copy `team-frontend/` alone to a temp directory, run `npm ci`, `npm run proto`, `npm run check` and `npm run build`; this proves no hidden dependency on `../fe-platform` and is the stand-in for the Docker build. If the Docker daemon is reachable, build the image from that directory too.
7. **ADR-0006** is written first and states the whole P10 direction so later changes cite it.

## Risks / Trade-offs

- [Committed binary tarballs bloat git history] -> they are small source archives; the long-term fix is a shared authenticated registry (ADR-0004), after which only the dependency lines change.
- [Lockfile integrity drift when contents change] -> expected; `npm run vendor` plus a lockfile refresh is part of the update recipe in the docs.
- [`npm pack` output could vary between npm versions] -> the check runs in the same repo toolchain; if it differs across machines the test reports it and the recipe is to repack with the repo's npm version (pinned in `engines`/docs).
- [`ui` pulls React 19 peer assumptions] -> team-frontend already uses React 19.3 and Next 15.5, same majors as fe-platform.
