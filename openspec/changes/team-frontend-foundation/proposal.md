## Why

The frontend is moving to three Next apps (buyer, seller, admin) built on the shared `fe-platform` packages. Nothing yet records that decision, and `team-frontend` cannot consume the packages: its Docker build copies only its own directory, so a `file:../fe-platform` link or the local Verdaccio registry would break `npm ci` in the image.

## What Changes

- ADR-0006 in `fe-platform/docs/ADR/`: replaces the earlier "no mapping into team-frontend" decision; records the three-app model, what fe-platform shares and what stays in apps, package delivery by vendored tarballs, per-app sessions with distinct cookie names, admin isolation, ports and merge order.
- `fe-platform` gets `npm run vendor` (packs `tokens`, `ui`, `api-client` into each app's `vendor/` under stable names) and `npm run vendor:check` (repacks to a temp directory and fails on any byte difference), wired into `npm run check`.
- `team-frontend` depends on the vendored tarballs, lists them in `transpilePackages`, and a test proves the packages import and resolve from inside the app. No component, token or page changes: the UI is untouched.
- The vendored tarballs are committed so `docker build` from `team-frontend/` needs no network beyond npm.
- Nothing for `team-frontend-seller` or `team-frontend-admin` yet; they get the same `vendor/` pattern when created.

## Capabilities

### New Capabilities
none (build and delivery plumbing plus a decision record)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `fe-platform/docs/ADR/0006-*.md`, `fe-platform/scripts/vendor.mjs`, `fe-platform/package.json`, `fe-platform/docs/agents/project.md`.
- `team-frontend/package.json`, `package-lock.json`, `next.config.mjs`, `vendor/*.tgz`, one smoke test.
- Docker build context of `team-frontend` is unchanged.

## Non-goals

Applying the tokens preset, replacing any component, the `session` package (A3), CI for team-frontend (A4), creating the seller and admin apps, publishing to a registry.

## Non-functional requirements

- Tarball packing is deterministic: the same sources give byte-identical files, so `vendor:check` has no false alarms.
- `team-frontend` still passes `npm run check` and builds from a clean copy of its own directory with no access to `../fe-platform`.
- No credential is committed; no registry is contacted for the vendored packages.
