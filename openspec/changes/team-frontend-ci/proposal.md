## Why

`team-frontend` has no CI of its own: `.github/workflows` only runs Stryker mutation testing on changed files (`asdlc-gates.yml`), so `npm run check`, the production build and the Docker image build are never run on a pull request. The apps are about to depend on vendored fe-platform tarballs (ADR-0006), where a stale tarball or a Dockerfile that misses `vendor/` fails only at image build time (it did while building A2).

## What Changes

- New workflow `.github/workflows/team-frontend-ci.yml`: on pull requests and pushes to main touching `team-frontend/**` it runs `npm ci`, `npm run proto`, `npm run check` and `npm run build`, then builds the Docker image from `team-frontend/` alone.
- `team-frontend/.dockerignore` (new) keeps `node_modules`, `.next` and `.git` out of the build context so the image installs its own dependencies and a symlinked or host-built `node_modules` cannot leak in.
- `fe-platform-ci.yml` also triggers on `team-frontend/vendor/**`, so editing a committed tarball by hand runs the platform's `vendor:check`.
- Nothing is added to `scripts/check-tokens.mjs`: the new fe-platform classes are not used in `team-frontend` yet, and token changes belong to plan item B1.
- The workflow is written and validated locally (YAML parse, every step run by hand); it has not run on GitHub.

## Capabilities

### New Capabilities
none (CI configuration)

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `.github/workflows/team-frontend-ci.yml` (new), `.github/workflows/fe-platform-ci.yml` (paths), `team-frontend/.dockerignore` (new), `fe-platform/docs/agents/project.md`.
- Pull requests that touch `team-frontend/` get two more checks; marking them required is a repository setting a human must apply.

## Non-goals

Required-check settings, Stryker changes, CI for the seller and admin apps (created later with the same shape), end-to-end tests in CI, pushing images to a registry.

## Non-functional requirements

- Same Node version as the image (22) and the same commands a developer runs locally; no secrets used.
- `permissions: contents: read` only; concurrent runs for the same ref are cancelled.
